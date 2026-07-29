package worker

import (
	"context"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"

	"named_clocks/backend/internal/storage"
	"named_clocks/backend/internal/store"
)

// Stage 4: fit the silent clip onto the selected soundtrack. The clip is slowed
// down (or sped up) so its length matches the mp3 exactly, then both are muxed
// into the deliverable video.
func (w *Worker) stageMixAudio(ctx context.Context, t *store.Task) error {
	if t.SourceVideoObject == "" {
		// Nothing downloaded yet — go back and fetch the clip.
		t.Status = store.StatusVideoDownloading
		return nil
	}
	if t.AudioObject == "" {
		return fmt.Errorf("no soundtrack selected for this task")
	}
	if w.ffmpeg == nil {
		return fmt.Errorf("ffmpeg is not available on this server")
	}

	// Interpolating a slow-down runs for minutes with nothing else touching the
	// task, so the lease has to be held explicitly.
	defer w.keepLeased(ctx, t.ID)()

	dir, err := w.ffmpeg.NewTempDir("mix-")
	if err != nil {
		return transient(fmt.Errorf("create temp dir: %w", err))
	}
	defer os.RemoveAll(dir)

	videoPath := filepath.Join(dir, "source.mp4")
	audioPath := filepath.Join(dir, "audio.mp3")
	outPath := filepath.Join(dir, "out.mp4")

	if err := w.fetchToFile(ctx, t.SourceVideoObject, videoPath); err != nil {
		return err
	}
	if err := w.fetchToFile(ctx, t.AudioObject, audioPath); err != nil {
		return err
	}

	if err := w.ffmpeg.StretchAndMux(ctx, videoPath, audioPath, outPath); err != nil {
		return fmt.Errorf("fit video to soundtrack: %w", err)
	}

	out, err := os.Open(outPath)
	if err != nil {
		return fmt.Errorf("open rendered video: %w", err)
	}
	defer out.Close()
	size, err := fileSize(out)
	if err != nil {
		return err
	}

	objectName := fmt.Sprintf("%s/%s.mp4", t.BatchID, t.ID)
	if err := w.storage.Upload(ctx, objectName, out, size, "video/mp4"); err != nil {
		return transient(err)
	}
	log.Printf("worker: task %s mixed with soundtrack into %s (%d bytes)", t.ID, objectName, size)

	t.VideoObject = objectName
	t.Status = store.StatusDone
	return nil
}

// fetchToFile streams an object from storage into a local file.
func (w *Worker) fetchToFile(ctx context.Context, objectName, path string) error {
	body, err := w.storage.Download(ctx, objectName)
	if err != nil {
		// A deleted object never comes back, so do not burn retries on it.
		if storage.IsNotFound(err) {
			return err
		}
		return transient(err)
	}
	defer body.Close()

	f, err := os.Create(path)
	if err != nil {
		return fmt.Errorf("create %s: %w", filepath.Base(path), err)
	}
	defer f.Close()
	if _, err := io.Copy(f, body); err != nil {
		return transient(fmt.Errorf("fetch %s: %w", objectName, err))
	}
	return f.Sync()
}

func fileSize(f *os.File) (int64, error) {
	st, err := f.Stat()
	if err != nil {
		return 0, fmt.Errorf("stat rendered video: %w", err)
	}
	return st.Size(), nil
}
