package worker

import (
	"context"
	"fmt"
	"log"
	"time"

	"named_clocks/backend/internal/imanator"
	"named_clocks/backend/internal/media"
	"named_clocks/backend/internal/openrouter"
	"named_clocks/backend/internal/storage"
	"named_clocks/backend/internal/store"
)

// Worker drives tasks through the pipeline:
//  1. create image order in Imanator + poll until ready
//  2. create video job in OpenRouter (image-to-video, silent) + poll until ready
//  3. download the clip and store it in MinIO
//  4. stretch the clip onto the selected soundtrack (skipped when the model was
//     asked to generate audio itself)
//
// Every stage is resumable: progress is persisted before and after each external
// call, so a crashed process, a network outage or a provider hiccup continues
// from the same point instead of losing the task.
type Worker struct {
	store      *store.Store
	imanator   *imanator.Client
	openrouter *openrouter.Client
	storage    *storage.Storage
	ffmpeg     *media.FFmpeg

	concurrency  int
	pollInterval time.Duration
	stageTimeout time.Duration
	leaseTimeout time.Duration
	maxAttempts  int
}

// Options bundles the worker's tunables.
type Options struct {
	Concurrency  int
	PollInterval time.Duration
	StageTimeout time.Duration
	LeaseTimeout time.Duration
	MaxAttempts  int
}

func New(
	st *store.Store,
	im *imanator.Client,
	or *openrouter.Client,
	strg *storage.Storage,
	ff *media.FFmpeg,
	o Options,
) *Worker {
	if o.Concurrency < 1 {
		o.Concurrency = 1
	}
	if o.PollInterval <= 0 {
		o.PollInterval = 2 * time.Second
	}
	if o.StageTimeout <= 0 {
		o.StageTimeout = 30 * time.Minute
	}
	if o.LeaseTimeout <= 0 {
		o.LeaseTimeout = 90 * time.Second
	}
	if o.MaxAttempts < 1 {
		o.MaxAttempts = 1
	}
	return &Worker{
		store:        st,
		imanator:     im,
		openrouter:   or,
		storage:      strg,
		ffmpeg:       ff,
		concurrency:  o.Concurrency,
		pollInterval: o.PollInterval,
		stageTimeout: o.StageTimeout,
		leaseTimeout: o.LeaseTimeout,
		maxAttempts:  o.MaxAttempts,
	}
}

// Run starts the worker pool and blocks until the context is cancelled.
func (w *Worker) Run(ctx context.Context) {
	log.Printf("worker: starting pool with concurrency=%d lease=%s stage_timeout=%s max_attempts=%d",
		w.concurrency, w.leaseTimeout, w.stageTimeout, w.maxAttempts)
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

		t, err := w.store.ClaimNext(ctx, w.leaseTimeout)
		if err != nil {
			log.Printf("worker[%d]: claim error: %v", id, err)
			sleepCtx(ctx, idle)
			continue
		}
		if t == nil {
			sleepCtx(ctx, idle)
			continue
		}

		log.Printf("worker[%d]: picked task %s (%s %s) status=%s attempts=%d",
			id, t.ID, t.FirstName, t.LastName, t.Status, t.Attempts)
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
		case store.StatusAudioMixing:
			err = w.stageMixAudio(ctx, t)
		default:
			err = fmt.Errorf("unknown status %q", t.Status)
		}

		if err != nil {
			// On shutdown leave the task exactly as it is: the lease expires and
			// it is picked up again from the same stage on the next start.
			if ctx.Err() != nil {
				log.Printf("worker: task %s interrupted at status=%s", t.ID, t.Status)
				return
			}
			w.handleStageError(ctx, t, err)
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

// handleStageError either schedules another attempt or gives up. Imanator
// outages are retried indefinitely once a minute: a provider-wide interruption
// must not turn a whole batch into failed tasks. Other temporary problems still
// use the shared attempt budget.
func (w *Worker) handleStageError(ctx context.Context, t *store.Task, cause error) {
	t.Attempts++
	temporary := isTransient(cause)
	imanatorOutage := temporary && isImanatorStage(t.Status)
	if !temporary || (!imanatorOutage && t.Attempts >= w.maxAttempts) {
		w.fail(ctx, t, cause)
		return
	}
	delay := retryDelay(t.Status, t.Attempts)
	t.Error = cause.Error()
	if imanatorOutage {
		log.Printf("worker: task %s Imanator unavailable at status=%s (attempt %d), retrying in %s: %v",
			t.ID, t.Status, t.Attempts, delay, cause)
	} else {
		log.Printf("worker: task %s transient failure at status=%s (attempt %d/%d), retrying in %s: %v",
			t.ID, t.Status, t.Attempts, w.maxAttempts, delay.Round(time.Second), cause)
	}
	if err := w.store.Reschedule(ctx, t, delay); err != nil {
		log.Printf("worker: reschedule task %s: %v", t.ID, err)
	}
}

// Stage 1a: create the Imanator image order.
func (w *Worker) stageCreateImage(ctx context.Context, t *store.Task) error {
	// A previous attempt already placed the order — never order twice.
	if t.ImanatorOrderID != "" {
		t.Status = store.StatusImagePolling
		return nil
	}
	t.Status = store.StatusImageCreating
	// Persist the intent before the call so a crash mid-request is visible.
	if err := w.store.Save(ctx, t); err != nil {
		return err
	}

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
			return pollTimeout("imanator", t.ImanatorOrderID, w.stageTimeout)
		}
		if err := sleepCtx(ctx, w.pollInterval); err != nil {
			return err
		}
		_ = w.store.Touch(ctx, t.ID)

		order, err := w.imanator.GetOrder(ctx, t.ImanatorOrderID)
		if err != nil {
			wrapped := fmt.Errorf("imanator get order: %w", err)
			if !isTransient(err) {
				return wrapped
			}
			// Reschedule immediately so the next request is a minute away. The
			// generic poll loop retries rapidly, which would hammer Imanator
			// during a provider-wide outage.
			return transient(wrapped)
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
	// A job id means a previous attempt already submitted (and we were billed
	// for) this generation — resume polling instead of paying twice.
	if t.OpenRouterJobID != "" {
		t.Status = store.StatusVideoPolling
		return nil
	}
	t.Status = store.StatusVideoCreating
	if err := w.store.Save(ctx, t); err != nil {
		return err
	}

	job, err := w.openrouter.CreateVideo(ctx, openrouter.CreateVideoParams{
		Model:         t.VideoModel,
		Prompt:        t.VideoPrompt,
		ImageURL:      t.ImageURL,
		Duration:      t.VideoDuration,
		Resolution:    t.VideoResolution,
		AspectRatio:   t.VideoAspectRatio,
		GenerateAudio: t.GenerateAudio,
	})
	if err != nil {
		return fmt.Errorf("openrouter create video: %w", err)
	}
	t.OpenRouterJobID = job.ID
	if job.IsFailed() {
		return w.deadVideoJob(t, fmt.Errorf("openrouter job failed immediately: %s", job.Error))
	}
	if job.IsReady() {
		recordCost(t, job)
		t.Status = store.StatusVideoDownloading
		return nil
	}
	t.Status = store.StatusVideoPolling
	return nil
}

// deadVideoJob drops the OpenRouter job id when the provider says the job will
// never produce a video, so a later manual retry starts a fresh generation
// instead of polling a corpse.
func (w *Worker) deadVideoJob(t *store.Task, cause error) error {
	t.OpenRouterJobID = ""
	return cause
}

// recordCost stores the OpenRouter USD cost on the task when it becomes known.
func recordCost(t *store.Task, job *openrouter.VideoJob) {
	if job != nil && job.Cost != nil {
		t.CostUSD = *job.Cost
	}
}

// Stage 2b: poll the OpenRouter job until the video is ready.
func (w *Worker) stagePollVideo(ctx context.Context, t *store.Task) error {
	deadline := time.Now().Add(w.stageTimeout)
	failures := 0
	for {
		if time.Now().After(deadline) {
			return pollTimeout("openrouter", t.OpenRouterJobID, w.stageTimeout)
		}
		if err := sleepCtx(ctx, w.pollInterval); err != nil {
			return err
		}
		_ = w.store.Touch(ctx, t.ID)

		job, err := w.openrouter.GetVideo(ctx, t.OpenRouterJobID)
		if err != nil {
			wrapped := fmt.Errorf("openrouter get video: %w", err)
			if !isTransient(err) {
				return wrapped
			}
			failures++
			if failures >= maxConsecutivePollErrors {
				return transient(wrapped)
			}
			log.Printf("worker: task %s: openrouter poll error %d/%d: %v",
				t.ID, failures, maxConsecutivePollErrors, err)
			continue
		}
		failures = 0

		if job.IsFailed() {
			return w.deadVideoJob(t, fmt.Errorf("openrouter job failed: status=%s %s", job.Status, job.Error))
		}
		if job.IsReady() {
			recordCost(t, job)
			t.Status = store.StatusVideoDownloading
			return nil
		}
	}
}

// Stage 3: download the finished clip and upload it to MinIO. Without model
// audio the clip is stored as the raw source for the mixing stage; otherwise it
// is already the deliverable.
func (w *Worker) stageDownload(ctx context.Context, t *store.Task) error {
	// Moving a whole clip through the process can outlast the lease on a slow
	// link, so keep renewing it while the bytes flow.
	defer w.keepLeased(ctx, t.ID)()

	// Fetch the (short-lived) source URL fresh so it works even after a restart.
	job, err := w.openrouter.GetVideo(ctx, t.OpenRouterJobID)
	if err != nil {
		return fmt.Errorf("openrouter re-get video: %w", err)
	}
	if job.IsFailed() {
		return w.deadVideoJob(t, fmt.Errorf("openrouter job failed: status=%s %s", job.Status, job.Error))
	}
	if !job.IsReady() {
		t.Status = store.StatusVideoPolling
		return nil
	}
	recordCost(t, job)

	body, err := w.openrouter.DownloadVideo(ctx, t.OpenRouterJobID, 0)
	if err != nil {
		return fmt.Errorf("download video: %w", err)
	}
	defer body.Close()

	if t.GenerateAudio {
		objectName := fmt.Sprintf("%s/%s.mp4", t.BatchID, t.ID)
		if err := w.storage.Upload(ctx, objectName, body, -1, "video/mp4"); err != nil {
			return err
		}
		t.VideoObject = objectName
		t.Status = store.StatusDone
		return nil
	}

	sourceName := fmt.Sprintf("%s/%s.raw.mp4", t.BatchID, t.ID)
	if err := w.storage.Upload(ctx, sourceName, body, -1, "video/mp4"); err != nil {
		return err
	}
	t.SourceVideoObject = sourceName
	t.Status = store.StatusAudioMixing
	return nil
}

func (w *Worker) fail(ctx context.Context, t *store.Task, cause error) {
	log.Printf("worker: task %s failed after %d attempt(s): %v", t.ID, t.Attempts, cause)
	t.Status = store.StatusFailed
	t.Error = cause.Error()
	if err := w.store.Save(ctx, t); err != nil {
		log.Printf("worker: save failed task %s: %v", t.ID, err)
	}
	_ = w.store.Release(ctx, t.ID)
}

// keepLeased renews the task lease in the background and returns the function
// that stops it. Long local work (an interpolated render takes minutes) would
// otherwise let the lease expire, and another worker would pick the same task up
// and redo it in parallel.
func (w *Worker) keepLeased(ctx context.Context, id string) func() {
	interval := w.leaseTimeout / 3
	if interval < time.Second {
		interval = time.Second
	}
	ctx, cancel := context.WithCancel(ctx)
	done := make(chan struct{})
	go func() {
		defer close(done)
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				if err := w.store.Touch(ctx, id); err != nil && ctx.Err() == nil {
					log.Printf("worker: renew lease for task %s: %v", id, err)
				}
			}
		}
	}()
	return func() {
		cancel()
		<-done
	}
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
