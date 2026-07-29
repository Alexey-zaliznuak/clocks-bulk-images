package storage

import (
	"context"
	"io"
	"net/http"
	"os"
	"strings"
	"testing"
	"time"
)

// TestPresignedDownloadURLAgainstMinIO checks that the storage really echoes our
// Content-Disposition back, since the save-as name depends on it. Set
// MINIO_TEST_ENDPOINT (e.g. localhost:9100) to run it.
func TestPresignedDownloadURLAgainstMinIO(t *testing.T) {
	endpoint := os.Getenv("MINIO_TEST_ENDPOINT")
	if endpoint == "" {
		t.Skip("MINIO_TEST_ENDPOINT not set")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	s, err := New(ctx, Options{
		Endpoint:       endpoint,
		PublicEndpoint: endpoint,
		AccessKey:      envOr("MINIO_TEST_ACCESS_KEY", "minioadmin"),
		SecretKey:      envOr("MINIO_TEST_SECRET_KEY", "minioadmin"),
		Bucket:         "presign-test",
	})
	if err != nil {
		t.Fatalf("storage: %v", err)
	}

	const body = "not really a video"
	if err := s.Upload(ctx, "clip.mp4", strings.NewReader(body), int64(len(body)), "video/mp4"); err != nil {
		t.Fatalf("upload: %v", err)
	}

	url, err := s.PresignedDownloadURL(ctx, "clip.mp4", "2026-07-29_Иван_Иванов.mp4", time.Minute)
	if err != nil {
		t.Fatalf("presign: %v", err)
	}
	resp, err := http.Get(url)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	defer resp.Body.Close()
	got, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status %d: %s", resp.StatusCode, got)
	}
	if string(got) != body {
		t.Fatalf("body = %q, want %q", got, body)
	}
	disposition := resp.Header.Get("Content-Disposition")
	if !strings.Contains(disposition, `filename*=UTF-8''2026-07-29_%D0%98`) {
		t.Fatalf("Content-Disposition = %q", disposition)
	}
	if !strings.HasPrefix(disposition, "attachment;") {
		t.Fatalf("Content-Disposition = %q, want an attachment", disposition)
	}
}

func envOr(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}
