package worker

import (
	"context"
	"fmt"
	"log"
	"time"

	"named_clocks/backend/internal/imanator"
	"named_clocks/backend/internal/openrouter"
	"named_clocks/backend/internal/storage"
	"named_clocks/backend/internal/store"
)

// Worker drives tasks through the 3-stage pipeline:
//   1. create image order in Imanator + poll until ready
//   2. create video job in OpenRouter (image-to-video) + poll until ready
//   3. download the video and store it in MinIO
type Worker struct {
	store        *store.Store
	imanator     *imanator.Client
	openrouter   *openrouter.Client
	storage      *storage.Storage
	concurrency  int
	pollInterval time.Duration
	stageTimeout time.Duration
}

func New(
	st *store.Store,
	im *imanator.Client,
	or *openrouter.Client,
	strg *storage.Storage,
	concurrency int,
	pollInterval, stageTimeout time.Duration,
) *Worker {
	if concurrency < 1 {
		concurrency = 1
	}
	return &Worker{
		store:        st,
		imanator:     im,
		openrouter:   or,
		storage:      strg,
		concurrency:  concurrency,
		pollInterval: pollInterval,
		stageTimeout: stageTimeout,
	}
}

// Run starts the worker pool and blocks until the context is cancelled.
func (w *Worker) Run(ctx context.Context) {
	log.Printf("worker: starting pool with concurrency=%d", w.concurrency)
	for i := 0; i < w.concurrency; i++ {
		go w.loop(ctx, i)
	}
	<-ctx.Done()
}

func (w *Worker) loop(ctx context.Context, id int) {
	idle := 2 * time.Second
	for {
		select {
		case <-ctx.Done():
			return
		default:
		}

		t, err := w.store.ClaimNext(ctx, w.stageTimeout)
		if err != nil {
			log.Printf("worker[%d]: claim error: %v", id, err)
			sleepCtx(ctx, idle)
			continue
		}
		if t == nil {
			sleepCtx(ctx, idle)
			continue
		}

		log.Printf("worker[%d]: picked task %s (%s %s) status=%s", id, t.ID, t.FirstName, t.LastName, t.Status)
		w.process(ctx, t)
	}
}

// process advances a single task as far as possible, resuming from its current status.
func (w *Worker) process(ctx context.Context, t *store.Task) {
	for !store.IsTerminal(t.Status) {
		select {
		case <-ctx.Done():
			return
		default:
		}

		var err error
		switch t.Status {
		case store.StatusQueued, store.StatusImageCreating:
			err = w.stageCreateImage(ctx, t)
		case store.StatusImagePolling:
			err = w.stagePollImage(ctx, t)
		case store.StatusImageReady, store.StatusVideoCreating:
			err = w.stageCreateVideo(ctx, t)
		case store.StatusVideoPolling:
			err = w.stagePollVideo(ctx, t)
		case store.StatusVideoDownloading:
			err = w.stageDownload(ctx, t)
		default:
			err = fmt.Errorf("unknown status %q", t.Status)
		}

		if err != nil {
			w.fail(ctx, t, err)
			return
		}
		if err := w.store.Save(ctx, t); err != nil {
			log.Printf("worker: save task %s: %v", t.ID, err)
			return
		}
	}

	if err := w.store.Release(ctx, t.ID); err != nil {
		log.Printf("worker: release task %s: %v", t.ID, err)
	}
	log.Printf("worker: task %s finished with status=%s", t.ID, t.Status)
}

// Stage 1a: create the Imanator image order.
func (w *Worker) stageCreateImage(ctx context.Context, t *store.Task) error {
	t.Status = store.StatusImageCreating
	order, err := w.imanator.CreateOrder(ctx, t.TemplateID, t.ImageSettings)
	if err != nil {
		return fmt.Errorf("imanator create order: %w", err)
	}
	t.ImanatorOrderID = order.ID
	if order.IsFailed() {
		return fmt.Errorf("imanator order failed immediately: status=%s", order.Status)
	}
	if order.IsReady() {
		t.ImageURL = order.Result
		t.Status = store.StatusImageReady
		return nil
	}
	t.Status = store.StatusImagePolling
	return nil
}

// Stage 1b: poll the Imanator order until the image is ready.
func (w *Worker) stagePollImage(ctx context.Context, t *store.Task) error {
	deadline := time.Now().Add(w.stageTimeout)
	for {
		if time.Now().After(deadline) {
			return fmt.Errorf("imanator poll timeout for order %s", t.ImanatorOrderID)
		}
		if err := sleepCtx(ctx, w.pollInterval); err != nil {
			return err
		}
		_ = w.store.Touch(ctx, t.ID)

		order, err := w.imanator.GetOrder(ctx, t.ImanatorOrderID)
		if err != nil {
			return fmt.Errorf("imanator get order: %w", err)
		}
		if order.IsFailed() {
			return fmt.Errorf("imanator order failed: status=%s", order.Status)
		}
		if order.IsReady() {
			t.ImageURL = order.Result
			t.Status = store.StatusImageReady
			return nil
		}
	}
}

// Stage 2a: create the OpenRouter video job (image-to-video).
func (w *Worker) stageCreateVideo(ctx context.Context, t *store.Task) error {
	t.Status = store.StatusVideoCreating
	job, err := w.openrouter.CreateVideo(ctx, openrouter.CreateVideoParams{
		Model:       t.VideoModel,
		Prompt:      t.VideoPrompt,
		ImageURL:    t.ImageURL,
		Duration:    t.VideoDuration,
		Resolution:  t.VideoResolution,
		AspectRatio: t.VideoAspectRatio,
	})
	if err != nil {
		return fmt.Errorf("openrouter create video: %w", err)
	}
	t.OpenRouterJobID = job.ID
	if job.IsFailed() {
		return fmt.Errorf("openrouter job failed immediately: %s", job.Error)
	}
	if job.IsReady() {
		t.Status = store.StatusVideoDownloading
		return nil
	}
	t.Status = store.StatusVideoPolling
	return nil
}

// Stage 2b: poll the OpenRouter job until the video is ready.
func (w *Worker) stagePollVideo(ctx context.Context, t *store.Task) error {
	deadline := time.Now().Add(w.stageTimeout)
	for {
		if time.Now().After(deadline) {
			return fmt.Errorf("openrouter poll timeout for job %s", t.OpenRouterJobID)
		}
		if err := sleepCtx(ctx, w.pollInterval); err != nil {
			return err
		}
		_ = w.store.Touch(ctx, t.ID)

		job, err := w.openrouter.GetVideo(ctx, t.OpenRouterJobID)
		if err != nil {
			return fmt.Errorf("openrouter get video: %w", err)
		}
		if job.IsFailed() {
			return fmt.Errorf("openrouter job failed: status=%s %s", job.Status, job.Error)
		}
		if job.IsReady() {
			t.Status = store.StatusVideoDownloading
			return nil
		}
	}
}

// Stage 3: download the finished video and upload it to MinIO.
func (w *Worker) stageDownload(ctx context.Context, t *store.Task) error {
	// Fetch the (short-lived) source URL fresh so it works even after a restart.
	job, err := w.openrouter.GetVideo(ctx, t.OpenRouterJobID)
	if err != nil {
		return fmt.Errorf("openrouter re-get video: %w", err)
	}
	if !job.IsReady() {
		t.Status = store.StatusVideoPolling
		return nil
	}

	body, err := w.openrouter.DownloadVideo(ctx, t.OpenRouterJobID, 0)
	if err != nil {
		return fmt.Errorf("download video: %w", err)
	}
	defer body.Close()

	objectName := fmt.Sprintf("%s/%s.mp4", t.BatchID, t.ID)
	if err := w.storage.Upload(ctx, objectName, body, -1, "video/mp4"); err != nil {
		return err
	}
	t.VideoObject = objectName
	t.Status = store.StatusDone
	return nil
}

func (w *Worker) fail(ctx context.Context, t *store.Task, cause error) {
	log.Printf("worker: task %s failed: %v", t.ID, cause)
	t.Status = store.StatusFailed
	t.Error = cause.Error()
	if err := w.store.Save(ctx, t); err != nil {
		log.Printf("worker: save failed task %s: %v", t.ID, err)
	}
	_ = w.store.Release(ctx, t.ID)
}

// sleepCtx sleeps for d unless the context is cancelled first.
func sleepCtx(ctx context.Context, d time.Duration) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-time.After(d):
		return nil
	}
}
