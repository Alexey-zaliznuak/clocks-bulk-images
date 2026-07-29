package db

import (
	"context"
	"os"
	"testing"
	"time"
)

// TestMigrationsApply checks the embedded SQL against a real Postgres. Set
// TEST_DATABASE_URL to run it; everything happens in a transaction that is
// rolled back, so the target database is left untouched.
//
//	TEST_DATABASE_URL=postgres://user:pass@localhost:5432/db?sslmode=disable go test ./internal/db
func TestMigrationsApply(t *testing.T) {
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("set TEST_DATABASE_URL to run migration tests")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	pool, err := Connect(ctx, dsn)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	defer pool.Close()

	tx, err := pool.BeginTx(ctx, nil)
	if err != nil {
		t.Fatalf("begin: %v", err)
	}
	defer tx.Rollback() //nolint:errcheck

	entries, err := migrationFiles.ReadDir("migrations")
	if err != nil {
		t.Fatalf("read migrations dir: %v", err)
	}
	if len(entries) == 0 {
		t.Fatal("no migrations found")
	}

	// Applying twice proves the migrations are idempotent, which matters because
	// Migrate re-runs every file on every start.
	for pass := 1; pass <= 2; pass++ {
		for _, e := range entries {
			content, err := migrationFiles.ReadFile("migrations/" + e.Name())
			if err != nil {
				t.Fatalf("read %s: %v", e.Name(), err)
			}
			if _, err := tx.ExecContext(ctx, string(content)); err != nil {
				t.Fatalf("pass %d: apply %s: %v", pass, e.Name(), err)
			}
		}
	}

	// The queries the store issues must match the resulting schema.
	for _, query := range []string{
		`SELECT id, batch_id, first_name, last_name, template_id, image_settings,
		        video_model, video_prompt, video_duration, video_resolution, video_aspect_ratio,
		        generate_audio, audio_asset_id, audio_object,
		        status, error, attempts, imanator_order_id, image_url, openrouter_job_id,
		        source_video_object, video_object, cost_usd, created_at, updated_at
		 FROM tasks WHERE false`,
		`SELECT id, kind, title, filename, object, content_type, size_bytes,
		        duration_seconds, created_at, updated_at
		 FROM media_assets WHERE false`,
		`SELECT id FROM tasks
		 WHERE status NOT IN ('done','failed')
		   AND (locked_at IS NULL OR locked_at < now() - make_interval(secs => 90))
		   AND (next_attempt_at IS NULL OR next_attempt_at <= now())`,
		`UPDATE tasks SET
		    status = CASE
		        WHEN source_video_object <> '' THEN 'audio_mixing'
		        WHEN openrouter_job_id <> ''   THEN 'video_polling'
		        WHEN image_url <> ''           THEN 'image_ready'
		        ELSE 'queued'
		    END,
		    error='', attempts=0, next_attempt_at=NULL, locked_at=NULL, updated_at=now()
		 WHERE batch_id='00000000-0000-0000-0000-000000000000' AND status='failed'`,
	} {
		if _, err := tx.ExecContext(ctx, query); err != nil {
			t.Fatalf("query does not match schema: %v\nquery: %s", err, query)
		}
	}
}
