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
	StatusQueued          = "queued"           // created, waiting for a worker
	StatusImageCreating   = "image_creating"   // sending order to Imanator
	StatusImagePolling    = "image_polling"    // waiting for Imanator to render
	StatusImageReady      = "image_ready"      // image URL obtained
	StatusVideoCreating   = "video_creating"   // sending job to OpenRouter
	StatusVideoPolling    = "video_polling"    // waiting for OpenRouter to render
	StatusVideoDownloading = "video_downloading" // downloading + uploading to S3
	StatusDone            = "done"             // finished, video in MinIO
	StatusFailed          = "failed"           // errored out
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

	Status string `json:"status"`
	Error  string `json:"error"`

	ImanatorOrderID string `json:"imanatorOrderId"`
	ImageURL        string `json:"imageUrl"`
	OpenRouterJobID string `json:"openrouterJobId"`
	VideoObject     string `json:"videoObject"`

	CreatedAt time.Time `json:"createdAt"`
	UpdatedAt time.Time `json:"updatedAt"`

	// VideoURL is a freshly generated presigned URL (not stored in DB).
	VideoURL string `json:"videoUrl,omitempty"`
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
			 video_model, video_prompt, video_duration, video_resolution, video_aspect_ratio, status)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11)`)
	if err != nil {
		return err
	}
	defer stmt.Close()

	for _, t := range tasks {
		settings, _ := json.Marshal(t.ImageSettings)
		if _, err := stmt.ExecContext(ctx,
			b.ID, t.FirstName, t.LastName, b.TemplateID, settings,
			t.VideoModel, t.VideoPrompt, t.VideoDuration, t.VideoResolution, t.VideoAspectRatio, StatusQueued,
		); err != nil {
			return fmt.Errorf("insert task: %w", err)
		}
	}
	return tx.Commit()
}

const taskColumns = `
	id, batch_id, first_name, last_name, template_id, image_settings,
	video_model, video_prompt, video_duration, video_resolution, video_aspect_ratio,
	status, error, imanator_order_id, image_url, openrouter_job_id, video_object,
	created_at, updated_at`

func scanTask(row interface{ Scan(...any) error }) (*Task, error) {
	var t Task
	var settingsRaw []byte
	if err := row.Scan(
		&t.ID, &t.BatchID, &t.FirstName, &t.LastName, &t.TemplateID, &settingsRaw,
		&t.VideoModel, &t.VideoPrompt, &t.VideoDuration, &t.VideoResolution, &t.VideoAspectRatio,
		&t.Status, &t.Error, &t.ImanatorOrderID, &t.ImageURL, &t.OpenRouterJobID, &t.VideoObject,
		&t.CreatedAt, &t.UpdatedAt,
	); err != nil {
		return nil, err
	}
	if len(settingsRaw) > 0 {
		_ = json.Unmarshal(settingsRaw, &t.ImageSettings)
	}
	if t.ImageSettings == nil {
		t.ImageSettings = map[string]string{}
	}
	return &t, nil
}

// ClaimNext atomically leases the next task that needs work. Returns nil if none.
func (s *Store) ClaimNext(ctx context.Context, leaseTimeout time.Duration) (*Task, error) {
	query := `
		UPDATE tasks SET locked_at = now(), updated_at = now()
		WHERE id = (
			SELECT id FROM tasks
			WHERE status NOT IN ('done','failed')
			  AND (locked_at IS NULL OR locked_at < now() - make_interval(secs => $1))
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
			status=$2, error=$3, imanator_order_id=$4, image_url=$5,
			openrouter_job_id=$6, video_object=$7, locked_at=now(), updated_at=now()
		WHERE id=$1`,
		t.ID, t.Status, t.Error, t.ImanatorOrderID, t.ImageURL, t.OpenRouterJobID, t.VideoObject,
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
		       COUNT(*) FILTER (WHERE t.status='failed') AS failed
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
			&b.Total, &b.Done, &b.Failed); err != nil {
			return nil, err
		}
		out = append(out, &b)
	}
	return out, rows.Err()
}
