package worker

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"log"
	"mime"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"named_clocks/backend/internal/openrouter"
	"named_clocks/backend/internal/store"
)

func (w *Worker) processCampaignItem(ctx context.Context, item *store.AdCampaignItem) {
	for !store.IsTerminal(item.Status) {
		if ctx.Err() != nil {
			return
		}
		var err error
		switch item.Status {
		case store.StatusQueued, store.StatusImageCreating:
			err = w.campaignCreateImage(ctx, item)
		case store.StatusImagePolling:
			err = w.campaignPollImage(ctx, item)
		case store.StatusImageReady:
			if item.ImageObject == "" {
				err = w.campaignStoreImage(ctx, item)
			} else {
				err = w.campaignCreateVideo(ctx, item)
			}
		case store.StatusVideoCreating:
			err = w.campaignCreateVideo(ctx, item)
		case store.StatusVideoPolling:
			err = w.campaignPollVideo(ctx, item)
		case store.StatusVideoDownloading:
			err = w.campaignDownloadVideo(ctx, item)
		case store.StatusAudioMixing:
			err = w.campaignMixAudio(ctx, item)
		default:
			err = fmt.Errorf("unknown campaign item status %q", item.Status)
		}
		if err != nil {
			if ctx.Err() == nil {
				w.handleCampaignError(ctx, item, err)
			}
			return
		}
		if err := w.store.SaveAdCampaignItem(ctx, item); err != nil {
			log.Printf("worker: save campaign item %s: %v", item.ID, err)
			return
		}
	}
	if err := w.store.ReleaseAdCampaignItem(ctx, item.ID); err != nil {
		log.Printf("worker: release campaign item %s: %v", item.ID, err)
	}
}

func (w *Worker) handleCampaignError(ctx context.Context, item *store.AdCampaignItem, cause error) {
	item.Attempts++
	temporary := isTransient(cause)
	imanatorOutage := temporary && isImanatorStage(item.Status)
	if !temporary || (!imanatorOutage && item.Attempts >= w.maxAttempts) {
		item.Status, item.Error = store.StatusFailed, cause.Error()
		_ = w.store.SaveAdCampaignItem(ctx, item)
		_ = w.store.ReleaseAdCampaignItem(ctx, item.ID)
		return
	}
	item.Error = cause.Error()
	delay := retryDelay(item.Status, item.Attempts)
	if err := w.store.RescheduleAdCampaignItem(ctx, item, delay); err != nil {
		log.Printf("worker: reschedule campaign item %s: %v", item.ID, err)
	}
}

func (w *Worker) campaignCreateImage(ctx context.Context, item *store.AdCampaignItem) error {
	if item.ImanatorOrderID != "" {
		item.Status = store.StatusImagePolling
		return nil
	}
	item.Status = store.StatusImageCreating
	if err := w.store.SaveAdCampaignItem(ctx, item); err != nil {
		return err
	}
	settings := make(map[string]string, len(item.ImageSettings)+1)
	for key, value := range item.ImageSettings {
		settings[key] = value
	}
	settings[item.NameSettingKey] = item.Value
	order, err := w.imanator.CreateOrder(ctx, item.TemplateID, settings)
	if err != nil {
		return fmt.Errorf("imanator create campaign image: %w", err)
	}
	item.ImanatorOrderID = order.ID
	if order.IsFailed() {
		return fmt.Errorf("imanator order failed immediately: status=%s", order.Status)
	}
	if order.IsReady() {
		item.ImageURL, item.Status = order.Result, store.StatusImageReady
	} else {
		item.Status = store.StatusImagePolling
	}
	return nil
}

func (w *Worker) campaignPollImage(ctx context.Context, item *store.AdCampaignItem) error {
	deadline := time.Now().Add(w.stageTimeout)
	for time.Now().Before(deadline) {
		if err := sleepCtx(ctx, w.pollInterval); err != nil {
			return err
		}
		_ = w.store.TouchAdCampaignItem(ctx, item.ID)
		order, err := w.imanator.GetOrder(ctx, item.ImanatorOrderID)
		if err != nil {
			wrapped := fmt.Errorf("imanator get campaign order: %w", err)
			if isTransient(err) {
				return transient(wrapped)
			}
			return wrapped
		}
		if order.IsFailed() {
			return fmt.Errorf("imanator order failed: status=%s", order.Status)
		}
		if order.IsReady() {
			item.ImageURL, item.Status = order.Result, store.StatusImageReady
			return nil
		}
	}
	return pollTimeout("imanator", item.ImanatorOrderID, w.stageTimeout)
}

func (w *Worker) campaignStoreImage(ctx context.Context, item *store.AdCampaignItem) error {
	requestURL, err := url.Parse(item.ImageURL)
	if err != nil || (requestURL.Scheme != "http" && requestURL.Scheme != "https") || requestURL.Host == "" {
		return fmt.Errorf("invalid Imanator image URL")
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, requestURL.String(), nil)
	if err != nil {
		return err
	}
	client := &http.Client{Timeout: 2 * time.Minute}
	resp, err := client.Do(req)
	if err != nil {
		return transient(fmt.Errorf("download Imanator image: %w", err))
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		err := fmt.Errorf("download Imanator image: status %d", resp.StatusCode)
		if resp.StatusCode >= 500 {
			return transient(err)
		}
		return err
	}
	const maxImageBytes = 25 << 20
	if resp.ContentLength > maxImageBytes {
		return fmt.Errorf("Imanator image exceeds %d bytes", maxImageBytes)
	}
	body := http.MaxBytesReader(nil, resp.Body, maxImageBytes)
	header := make([]byte, 512)
	n, readErr := io.ReadFull(body, header)
	if readErr != nil && readErr != io.ErrUnexpectedEOF {
		return fmt.Errorf("read Imanator image: %w", readErr)
	}
	header = header[:n]
	contentType := strings.ToLower(http.DetectContentType(header))
	if !strings.HasPrefix(contentType, "image/") {
		return fmt.Errorf("Imanator URL returned %s instead of an image", contentType)
	}
	ext := imageExtension(contentType, requestURL.Path)
	object := fmt.Sprintf("campaigns/%s/%s.image%s", item.CampaignID, item.ID, ext)
	if err := w.storage.Upload(ctx, object, io.MultiReader(bytes.NewReader(header), body), -1, contentType); err != nil {
		return transient(err)
	}
	item.ImageObject = object
	return nil
}

func imageExtension(contentType, sourcePath string) string {
	switch contentType {
	case "image/jpeg":
		return ".jpg"
	case "image/png":
		return ".png"
	case "image/webp":
		return ".webp"
	case "image/gif":
		return ".gif"
	}
	if !strings.HasPrefix(contentType, "image/") {
		switch strings.ToLower(filepath.Ext(sourcePath)) {
		case ".jpg", ".jpeg", ".png", ".webp", ".gif":
			return strings.ToLower(filepath.Ext(sourcePath))
		default:
			return ""
		}
	}
	if exts, _ := mime.ExtensionsByType(contentType); len(exts) > 0 {
		return exts[0]
	}
	switch strings.ToLower(filepath.Ext(sourcePath)) {
	case ".jpg", ".jpeg", ".png", ".webp", ".gif":
		return strings.ToLower(filepath.Ext(sourcePath))
	default:
		return ""
	}
}

func (w *Worker) campaignCreateVideo(ctx context.Context, item *store.AdCampaignItem) error {
	if item.OpenRouterJobID != "" {
		item.Status = store.StatusVideoPolling
		return nil
	}
	item.Status = store.StatusVideoCreating
	if err := w.store.SaveAdCampaignItem(ctx, item); err != nil {
		return err
	}
	job, err := w.openrouter.CreateVideo(ctx, openrouter.CreateVideoParams{
		Model: item.VideoModel, Prompt: item.VideoPrompt, ImageURL: item.ImageURL,
		Duration: item.VideoDuration, Resolution: item.VideoResolution,
		AspectRatio: item.VideoAspectRatio, GenerateAudio: item.GenerateAudio,
	})
	if err != nil {
		return fmt.Errorf("openrouter create campaign video: %w", err)
	}
	item.OpenRouterJobID = job.ID
	if job.IsFailed() {
		item.OpenRouterJobID = ""
		return fmt.Errorf("openrouter campaign job failed immediately: %s", job.Error)
	}
	if job.IsReady() {
		recordCampaignCost(item, job)
		item.Status = store.StatusVideoDownloading
	} else {
		item.Status = store.StatusVideoPolling
	}
	return nil
}

func (w *Worker) campaignPollVideo(ctx context.Context, item *store.AdCampaignItem) error {
	deadline := time.Now().Add(w.stageTimeout)
	failures := 0
	for time.Now().Before(deadline) {
		if err := sleepCtx(ctx, w.pollInterval); err != nil {
			return err
		}
		_ = w.store.TouchAdCampaignItem(ctx, item.ID)
		job, err := w.openrouter.GetVideo(ctx, item.OpenRouterJobID)
		if err != nil {
			failures++
			if failures >= maxConsecutivePollErrors {
				return transient(fmt.Errorf("openrouter get campaign video: %w", err))
			}
			continue
		}
		failures = 0
		if job.IsFailed() {
			item.OpenRouterJobID = ""
			return fmt.Errorf("openrouter campaign job failed: %s", job.Error)
		}
		if job.IsReady() {
			recordCampaignCost(item, job)
			item.Status = store.StatusVideoDownloading
			return nil
		}
	}
	return pollTimeout("openrouter", item.OpenRouterJobID, w.stageTimeout)
}

func recordCampaignCost(item *store.AdCampaignItem, job *openrouter.VideoJob) {
	if job != nil && job.Cost != nil {
		item.CostUSD = *job.Cost
	}
}

func (w *Worker) campaignDownloadVideo(ctx context.Context, item *store.AdCampaignItem) error {
	defer w.keepCampaignItemLeased(ctx, item.ID)()
	job, err := w.openrouter.GetVideo(ctx, item.OpenRouterJobID)
	if err != nil {
		return fmt.Errorf("openrouter re-get campaign video: %w", err)
	}
	if !job.IsReady() {
		item.Status = store.StatusVideoPolling
		return nil
	}
	recordCampaignCost(item, job)
	body, err := w.openrouter.DownloadVideo(ctx, item.OpenRouterJobID, 0)
	if err != nil {
		return transient(fmt.Errorf("download campaign video: %w", err))
	}
	defer body.Close()
	source := fmt.Sprintf("campaigns/%s/%s.raw.mp4", item.CampaignID, item.ID)
	if err := w.storage.Upload(ctx, source, body, -1, "video/mp4"); err != nil {
		return transient(err)
	}
	item.SourceVideoObject = source
	if item.GenerateAudio {
		item.VideoObject, item.Status = source, store.StatusDone
	} else {
		item.Status = store.StatusAudioMixing
	}
	return nil
}

func (w *Worker) campaignMixAudio(ctx context.Context, item *store.AdCampaignItem) error {
	if item.AudioObject == "" {
		return fmt.Errorf("no soundtrack selected for campaign item")
	}
	if w.ffmpeg == nil {
		return fmt.Errorf("ffmpeg is not available on this server")
	}
	defer w.keepCampaignItemLeased(ctx, item.ID)()
	dir, err := w.ffmpeg.NewTempDir("campaign-mix-")
	if err != nil {
		return transient(err)
	}
	defer os.RemoveAll(dir)
	videoPath, audioPath, outputPath := filepath.Join(dir, "source.mp4"), filepath.Join(dir, "audio.mp3"), filepath.Join(dir, "out.mp4")
	if err := w.fetchToFile(ctx, item.SourceVideoObject, videoPath); err != nil {
		return err
	}
	if err := w.fetchToFile(ctx, item.AudioObject, audioPath); err != nil {
		return err
	}
	if err := w.ffmpeg.StretchAndMux(ctx, videoPath, audioPath, outputPath); err != nil {
		return fmt.Errorf("fit campaign video to soundtrack: %w", err)
	}
	out, err := os.Open(outputPath)
	if err != nil {
		return err
	}
	defer out.Close()
	size, err := fileSize(out)
	if err != nil {
		return err
	}
	object := fmt.Sprintf("campaigns/%s/%s.mp4", item.CampaignID, item.ID)
	if err := w.storage.Upload(ctx, object, out, size, "video/mp4"); err != nil {
		return transient(err)
	}
	item.VideoObject, item.Status = object, store.StatusDone
	return nil
}

func (w *Worker) keepCampaignItemLeased(ctx context.Context, id string) func() {
	ctx, cancel := context.WithCancel(ctx)
	done := make(chan struct{})
	go func() {
		defer close(done)
		ticker := time.NewTicker(max(w.leaseTimeout/3, time.Second))
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				_ = w.store.TouchAdCampaignItem(ctx, id)
			}
		}
	}()
	return func() { cancel(); <-done }
}
