package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"time"
)

// Task status state machine.
const (
	StatusQueued           = "queued"            // created, waiting for a worker
	StatusImageCreating    = "image_creating"    // sending order to Imanator
	StatusImagePolling     = "image_polling"     // waiting for Imanator to render
	StatusImageReady       = "image_ready"       // image URL obtained
	StatusVideoCreating    = "video_creating"    // sending job to OpenRouter
	StatusVideoPolling     = "video_polling"     // waiting for OpenRouter to render
	StatusVideoDownloading = "video_downloading" // downloading + uploading to S3
	StatusAudioMixing      = "audio_mixing"      // stretching the clip onto the soundtrack
	StatusDone             = "done"              // finished, video in MinIO
	StatusFailed           = "failed"            // errored out
)

// IsTerminal reports whether a status will never change again on its own.
func IsTerminal(status string) bool {
	return status == StatusDone || status == StatusFailed
}

type Batch struct {
	ID         string    `json:"id"`
	Title      string    `json:"title"`
	TemplateID string    `json:"templateId"`
	VideoModel string    `json:"videoModel"`
	CreatedAt  time.Time `json:"createdAt"`

	// aggregate counts (filled by list queries)
	Total  int `json:"total"`
	Done   int `json:"done"`
	Failed int `json:"failed"`

	// aggregate cost across all tasks in the batch
	CostUSD float64 `json:"costUsd"`
	CostRUB float64 `json:"costRub"`
}

type Task struct {
	ID       string `json:"id"`
	BatchID  string `json:"batchId"`
	FirstName string `json:"firstName"`
	LastName  string `json:"lastName"`

	TemplateID       string            `json:"templateId"`
	ImageSettings    map[string]string `json:"imageSettings"`
	VideoModel       string            `json:"videoModel"`
	VideoPrompt      string            `json:"videoPrompt"`
	VideoDuration    *int              `json:"videoDuration,omitempty"`
	VideoResolution  string            `json:"videoResolution"`
	VideoAspectRatio string            `json:"videoAspectRatio"`

	// GenerateAudio asks the model itself for a soundtrack (expensive). When
	// false the clip comes back silent and AudioObject is muxed in instead.
	GenerateAudio bool   `json:"generateAudio"`
	AudioAssetID  string `json:"audioAssetId,omitempty"`
	AudioObject   string `json:"audioObject,omitempty"`

	Status string `json:"status"`
	Error  string `json:"error"`
	// Attempts counts transient failures already retried for this task.
	Attempts int `json:"attempts"`

	ImanatorOrderID string `json:"imanatorOrderId"`
	ImageURL        string `json:"imageUrl"`
	OpenRouterJobID string `json:"openrouterJobId"`
	// SourceVideoObject is the raw silent clip from OpenRouter, kept so mixing
	// can be retried without regenerating the video.
	SourceVideoObject string `json:"sourceVideoObject,omitempty"`
	VideoObject       string `json:"videoObject"`

	// CostUSD is the OpenRouter video generation cost in USD (0 until known).
	CostUSD float64 `json:"costUsd"`

	CreatedAt time.Time `json:"createdAt"`
	UpdatedAt time.Time `json:"updatedAt"`

	// VideoURL is a freshly generated presigned URL (not stored in DB).
	VideoURL string `json:"videoUrl,omitempty"`
	// DownloadURL is the same object presigned with a "save as" disposition.
	DownloadURL string `json:"downloadUrl,omitempty"`
	// CostRUB is derived from CostUSD and a live rate (not stored in DB).
	CostRUB float64 `json:"costRub"`
}

type Store struct {
	db *sql.DB
}

func New(db *sql.DB) *Store { return &Store{db: db} }

// CreateBatch inserts a batch together with all its tasks in one transaction.
func (s *Store) CreateBatch(ctx context.Context, b *Batch, tasks []*Task) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback() //nolint:errcheck

	err = tx.QueryRowContext(ctx,
		`INSERT INTO batches (title, template_id, video_model) VALUES ($1,$2,$3) RETURNING id, created_at`,
		b.Title, b.TemplateID, b.VideoModel,
	).Scan(&b.ID, &b.CreatedAt)
	if err != nil {
		return fmt.Errorf("insert batch: %w", err)
	}

	stmt, err := tx.PrepareContext(ctx, `
		INSERT INTO tasks
			(batch_id, first_name, last_name, template_id, image_settings,
			 video_model, video_prompt, video_duration, video_resolution, video_aspect_ratio,
			 generate_audio, audio_asset_id, audio_object, status)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14)`)
	if err != nil {
		return err
	}
	defer stmt.Close()

	for _, t := range tasks {
		settings, _ := json.Marshal(t.ImageSettings)
		if _, err := stmt.ExecContext(ctx,
			b.ID, t.FirstName, t.LastName, b.TemplateID, settings,
			t.VideoModel, t.VideoPrompt, t.VideoDuration, t.VideoResolution, t.VideoAspectRatio,
			t.GenerateAudio, nullUUID(t.AudioAssetID), t.AudioObject, StatusQueued,
		); err != nil {
			return fmt.Errorf("insert task: %w", err)
		}
	}
	return tx.Commit()
}

const taskColumns = `
	id, batch_id, first_name, last_name, template_id, image_settings,
	video_model, video_prompt, video_duration, video_resolution, video_aspect_ratio,
	generate_audio, audio_asset_id, audio_object,
	status, error, attempts, imanator_order_id, image_url, openrouter_job_id,
	source_video_object, video_object, cost_usd, created_at, updated_at`

func scanTask(row interface{ Scan(...any) error }) (*Task, error) {
	var t Task
	var settingsRaw []byte
	var audioAssetID sql.NullString
	if err := row.Scan(
		&t.ID, &t.BatchID, &t.FirstName, &t.LastName, &t.TemplateID, &settingsRaw,
		&t.VideoModel, &t.VideoPrompt, &t.VideoDuration, &t.VideoResolution, &t.VideoAspectRatio,
		&t.GenerateAudio, &audioAssetID, &t.AudioObject,
		&t.Status, &t.Error, &t.Attempts, &t.ImanatorOrderID, &t.ImageURL, &t.OpenRouterJobID,
		&t.SourceVideoObject, &t.VideoObject, &t.CostUSD, &t.CreatedAt, &t.UpdatedAt,
	); err != nil {
		return nil, err
	}
	t.AudioAssetID = audioAssetID.String
	if len(settingsRaw) > 0 {
		_ = json.Unmarshal(settingsRaw, &t.ImageSettings)
	}
	if t.ImageSettings == nil {
		t.ImageSettings = map[string]string{}
	}
	return &t, nil
}

// nullUUID turns an empty id into SQL NULL so it satisfies the UUID column.
func nullUUID(id string) any {
	if id == "" {
		return nil
	}
	return id
}

// ClaimNext atomically leases the next task that needs work. Returns nil if none.
// A task is available when its lease has expired (the previous worker died or
// never renewed it) and its retry backoff window has passed.
func (s *Store) ClaimNext(ctx context.Context, leaseTimeout time.Duration) (*Task, error) {
	query := `
		UPDATE tasks SET locked_at = now(), updated_at = now()
		WHERE id = (
			SELECT id FROM tasks
			WHERE status NOT IN ('done','failed')
			  AND (locked_at IS NULL OR locked_at < now() - make_interval(secs => $1))
			  AND (next_attempt_at IS NULL OR next_attempt_at <= now())
			ORDER BY created_at
			FOR UPDATE SKIP LOCKED
			LIMIT 1
		)
		RETURNING ` + taskColumns
	row := s.db.QueryRowContext(ctx, query, leaseTimeout.Seconds())
	t, err := scanTask(row)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	return t, err
}

// Save persists the mutable state of a task and renews its lease.
func (s *Store) Save(ctx context.Context, t *Task) error {
	_, err := s.db.ExecContext(ctx, `
		UPDATE tasks SET
			status=$2, error=$3, attempts=$4, imanator_order_id=$5, image_url=$6,
			openrouter_job_id=$7, source_video_object=$8, video_object=$9, cost_usd=$10,
			locked_at=now(), updated_at=now()
		WHERE id=$1`,
		t.ID, t.Status, t.Error, t.Attempts, t.ImanatorOrderID, t.ImageURL, t.OpenRouterJobID,
		t.SourceVideoObject, t.VideoObject, t.CostUSD,
	)
	return err
}

// Reschedule records a transient failure: the status is left untouched so the
// task resumes from the same stage, the lease is dropped and re-claiming is
// deferred until the backoff window has passed.
func (s *Store) Reschedule(ctx context.Context, t *Task, delay time.Duration) error {
	_, err := s.db.ExecContext(ctx, `
		UPDATE tasks SET
			error=$2, attempts=$3, imanator_order_id=$4, image_url=$5,
			openrouter_job_id=$6, source_video_object=$7, cost_usd=$8,
			next_attempt_at = now() + make_interval(secs => $9),
			locked_at=NULL, updated_at=now()
		WHERE id=$1`,
		t.ID, t.Error, t.Attempts, t.ImanatorOrderID, t.ImageURL, t.OpenRouterJobID,
		t.SourceVideoObject, t.CostUSD, delay.Seconds(),
	)
	return err
}

// Release clears the lease so a terminal task is not re-claimed.
func (s *Store) Release(ctx context.Context, id string) error {
	_, err := s.db.ExecContext(ctx, `UPDATE tasks SET locked_at=NULL, updated_at=now() WHERE id=$1`, id)
	return err
}

// Touch renews the worker lease during long polling stages.
func (s *Store) Touch(ctx context.Context, id string) error {
	_, err := s.db.ExecContext(ctx, `UPDATE tasks SET locked_at=now() WHERE id=$1`, id)
	return err
}

// retryResumeSQL picks the earliest stage a failed task can be resumed from
// without redoing work that has already been paid for: an already downloaded
// clip only needs re-mixing, a submitted OpenRouter job only needs polling.
const retryResumeSQL = `
	CASE
		WHEN source_video_object <> '' THEN '` + StatusAudioMixing + `'
		WHEN openrouter_job_id <> ''   THEN '` + StatusVideoPolling + `'
		WHEN image_url <> ''           THEN '` + StatusImageReady + `'
		ELSE '` + StatusQueued + `'
	END`

// RetryTask puts a single failed task back into the pipeline. Returns the status
// it was resumed at, or an empty string if the task is missing or not failed.
func (s *Store) RetryTask(ctx context.Context, id string) (string, error) {
	var status string
	err := s.db.QueryRowContext(ctx, `
		UPDATE tasks SET
			status = `+retryResumeSQL+`,
			error='', attempts=0, next_attempt_at=NULL, locked_at=NULL, updated_at=now()
		WHERE id=$1 AND status='failed'
		RETURNING status`, id).Scan(&status)
	if err == sql.ErrNoRows {
		return "", nil
	}
	return status, err
}

// RetryFailedInBatch re-queues every failed task of a batch and returns how many
// were affected.
func (s *Store) RetryFailedInBatch(ctx context.Context, batchID string) (int, error) {
	res, err := s.db.ExecContext(ctx, `
		UPDATE tasks SET
			status = `+retryResumeSQL+`,
			error='', attempts=0, next_attempt_at=NULL, locked_at=NULL, updated_at=now()
		WHERE batch_id=$1 AND status='failed'`, batchID)
	if err != nil {
		return 0, err
	}
	n, err := res.RowsAffected()
	return int(n), err
}

// ListTasks returns tasks, optionally filtered by batch, newest first.
func (s *Store) ListTasks(ctx context.Context, batchID string, limit int) ([]*Task, error) {
	if limit <= 0 || limit > 2000 {
		limit = 500
	}
	var rows *sql.Rows
	var err error
	if batchID != "" {
		rows, err = s.db.QueryContext(ctx,
			`SELECT `+taskColumns+` FROM tasks WHERE batch_id=$1 ORDER BY created_at DESC LIMIT $2`, batchID, limit)
	} else {
		rows, err = s.db.QueryContext(ctx,
			`SELECT `+taskColumns+` FROM tasks ORDER BY created_at DESC LIMIT $1`, limit)
	}
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []*Task
	for rows.Next() {
		t, err := scanTask(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, t)
	}
	return out, rows.Err()
}

// ListBatches returns batches with aggregate task counts, newest first.
func (s *Store) ListBatches(ctx context.Context, limit int) ([]*Batch, error) {
	if limit <= 0 || limit > 500 {
		limit = 100
	}
	rows, err := s.db.QueryContext(ctx, `
		SELECT b.id, b.title, b.template_id, b.video_model, b.created_at,
		       COUNT(t.id) AS total,
		       COUNT(*) FILTER (WHERE t.status='done')   AS done,
		       COUNT(*) FILTER (WHERE t.status='failed') AS failed,
		       COALESCE(SUM(t.cost_usd), 0) AS cost_usd
		FROM batches b
		LEFT JOIN tasks t ON t.batch_id = b.id
		GROUP BY b.id
		ORDER BY b.created_at DESC
		LIMIT $1`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []*Batch
	for rows.Next() {
		var b Batch
		if err := rows.Scan(&b.ID, &b.Title, &b.TemplateID, &b.VideoModel, &b.CreatedAt,
			&b.Total, &b.Done, &b.Failed, &b.CostUSD); err != nil {
			return nil, err
		}
		out = append(out, &b)
	}
	return out, rows.Err()
}

// GetBatch returns a single batch with aggregate counts and cost, or nil if
// no batch with the given id exists.
func (s *Store) GetBatch(ctx context.Context, id string) (*Batch, error) {
	row := s.db.QueryRowContext(ctx, `
		SELECT b.id, b.title, b.template_id, b.video_model, b.created_at,
		       COUNT(t.id) AS total,
		       COUNT(*) FILTER (WHERE t.status='done')   AS done,
		       COUNT(*) FILTER (WHERE t.status='failed') AS failed,
		       COALESCE(SUM(t.cost_usd), 0) AS cost_usd
		FROM batches b
		LEFT JOIN tasks t ON t.batch_id = b.id
		WHERE b.id = $1
		GROUP BY b.id`, id)
	var b Batch
	err := row.Scan(&b.ID, &b.Title, &b.TemplateID, &b.VideoModel, &b.CreatedAt,
		&b.Total, &b.Done, &b.Failed, &b.CostUSD)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &b, nil
}

// BatchExists reports whether a batch with the given id exists.
func (s *Store) BatchExists(ctx context.Context, id string) (bool, error) {
	var exists bool
	err := s.db.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM batches WHERE id=$1)`, id).Scan(&exists)
	return exists, err
}

// DeleteBatch removes a batch and (via ON DELETE CASCADE) all of its tasks.
func (s *Store) DeleteBatch(ctx context.Context, id string) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM batches WHERE id=$1`, id)
	return err
}
