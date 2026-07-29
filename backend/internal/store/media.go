package store

import (
	"context"
	"database/sql"
	"time"
)

// Media asset kinds.
const KindAudio = "audio"

// MediaAsset is an uploaded file in the media library. The bytes live in object
// storage under Object; only metadata is kept in Postgres.
type MediaAsset struct {
	ID              string    `json:"id"`
	Kind            string    `json:"kind"`
	Title           string    `json:"title"`
	Filename        string    `json:"filename"`
	Object          string    `json:"object"`
	ContentType     string    `json:"contentType"`
	SizeBytes       int64     `json:"sizeBytes"`
	DurationSeconds float64   `json:"durationSeconds"`
	CreatedAt       time.Time `json:"createdAt"`
	UpdatedAt       time.Time `json:"updatedAt"`

	// URL is a freshly generated presigned link (not stored in DB).
	URL string `json:"url,omitempty"`
}

const mediaColumns = `
	id, kind, title, filename, object, content_type, size_bytes,
	duration_seconds, created_at, updated_at`

func scanMediaAsset(row interface{ Scan(...any) error }) (*MediaAsset, error) {
	var m MediaAsset
	if err := row.Scan(
		&m.ID, &m.Kind, &m.Title, &m.Filename, &m.Object, &m.ContentType,
		&m.SizeBytes, &m.DurationSeconds, &m.CreatedAt, &m.UpdatedAt,
	); err != nil {
		return nil, err
	}
	return &m, nil
}

// CreateMediaAsset inserts a media asset row. When ID is already set (reserved
// before the upload so the object key could be derived from it) it is kept.
func (s *Store) CreateMediaAsset(ctx context.Context, m *MediaAsset) error {
	if m.ID != "" {
		return s.db.QueryRowContext(ctx, `
			INSERT INTO media_assets
				(id, kind, title, filename, object, content_type, size_bytes, duration_seconds)
			VALUES ($1,$2,$3,$4,$5,$6,$7,$8)
			RETURNING id, created_at, updated_at`,
			m.ID, m.Kind, m.Title, m.Filename, m.Object, m.ContentType, m.SizeBytes, m.DurationSeconds,
		).Scan(&m.ID, &m.CreatedAt, &m.UpdatedAt)
	}
	return s.db.QueryRowContext(ctx, `
		INSERT INTO media_assets
			(kind, title, filename, object, content_type, size_bytes, duration_seconds)
		VALUES ($1,$2,$3,$4,$5,$6,$7)
		RETURNING id, created_at, updated_at`,
		m.Kind, m.Title, m.Filename, m.Object, m.ContentType, m.SizeBytes, m.DurationSeconds,
	).Scan(&m.ID, &m.CreatedAt, &m.UpdatedAt)
}

// ReserveMediaAssetID allocates an id up front so the object key can be derived
// from it before the bytes are uploaded.
func (s *Store) ReserveMediaAssetID(ctx context.Context) (string, error) {
	var id string
	err := s.db.QueryRowContext(ctx, `SELECT gen_random_uuid()`).Scan(&id)
	return id, err
}

// ListMediaAssets returns assets of the given kind, newest first.
func (s *Store) ListMediaAssets(ctx context.Context, kind string, limit int) ([]*MediaAsset, error) {
	if limit <= 0 || limit > 500 {
		limit = 200
	}
	var rows *sql.Rows
	var err error
	if kind != "" {
		rows, err = s.db.QueryContext(ctx,
			`SELECT `+mediaColumns+` FROM media_assets WHERE kind=$1 ORDER BY created_at DESC LIMIT $2`,
			kind, limit)
	} else {
		rows, err = s.db.QueryContext(ctx,
			`SELECT `+mediaColumns+` FROM media_assets ORDER BY created_at DESC LIMIT $1`, limit)
	}
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []*MediaAsset
	for rows.Next() {
		m, err := scanMediaAsset(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, m)
	}
	return out, rows.Err()
}

// GetMediaAsset returns a single asset, or nil if it does not exist.
func (s *Store) GetMediaAsset(ctx context.Context, id string) (*MediaAsset, error) {
	row := s.db.QueryRowContext(ctx, `SELECT `+mediaColumns+` FROM media_assets WHERE id=$1`, id)
	m, err := scanMediaAsset(row)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return m, nil
}

// MediaAssetInUse reports whether an asset is still needed by a task that has
// not reached a terminal state, so deleting it would break that task.
func (s *Store) MediaAssetInUse(ctx context.Context, id string) (bool, error) {
	var inUse bool
	err := s.db.QueryRowContext(ctx, `
		SELECT EXISTS(
			SELECT 1 FROM tasks
			WHERE audio_asset_id = $1 AND status NOT IN ('done','failed')
		)`, id).Scan(&inUse)
	return inUse, err
}

// DeleteMediaAsset removes the metadata row. Tasks referencing it keep their
// audio_object snapshot (audio_asset_id is set to NULL by the FK).
func (s *Store) DeleteMediaAsset(ctx context.Context, id string) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM media_assets WHERE id=$1`, id)
	return err
}
