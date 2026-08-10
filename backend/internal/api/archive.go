package api

import (
	"archive/zip"
	"context"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"

	"named_clocks/backend/internal/store"
)

// handleBatchArchive packs every finished video of a batch into a zip, uploads
// the archive to MinIO and returns a short-lived download URL. The zip uses
// UTF-8 entry names (language-encoding flag), which Windows Explorer unpacks
// correctly including Cyrillic filenames.
func (s *Server) handleBatchArchive(w http.ResponseWriter, r *http.Request) {
	id := strings.TrimSpace(chi.URLParam(r, "id"))
	if id == "" {
		writeError(w, http.StatusBadRequest, "batch id is required")
		return
	}

	batch, err := s.store.GetBatch(r.Context(), id)
	if err != nil {
		log.Printf("api: archive get batch %s: %v", id, err)
		writeError(w, http.StatusInternalServerError, "could not load batch")
		return
	}
	if batch == nil {
		writeError(w, http.StatusNotFound, "batch not found")
		return
	}

	tasks, err := s.store.ListTasks(r.Context(), id, 0)
	if err != nil {
		log.Printf("api: archive list tasks %s: %v", id, err)
		writeError(w, http.StatusInternalServerError, "could not list tasks")
		return
	}

	ready := make([]*store.Task, 0, len(tasks))
	for _, t := range tasks {
		if t.Status == store.StatusDone && t.VideoObject != "" {
			ready = append(ready, t)
		}
	}
	if len(ready) == 0 {
		writeError(w, http.StatusConflict, "нет готовых видео для архива")
		return
	}

	tmp, err := os.CreateTemp("", "batch-archive-*.zip")
	if err != nil {
		log.Printf("api: archive temp file: %v", err)
		writeError(w, http.StatusInternalServerError, "could not build archive")
		return
	}
	tmpName := tmp.Name()
	defer func() {
		tmp.Close()
		os.Remove(tmpName)
	}()

	if err := writeBatchZip(r.Context(), s.storage, tmp, ready); err != nil {
		log.Printf("api: archive build %s: %v", id, err)
		writeError(w, http.StatusBadGateway, "не удалось собрать архив")
		return
	}
	if err := tmp.Sync(); err != nil {
		log.Printf("api: archive sync %s: %v", id, err)
		writeError(w, http.StatusInternalServerError, "could not build archive")
		return
	}
	stat, err := tmp.Stat()
	if err != nil {
		log.Printf("api: archive stat %s: %v", id, err)
		writeError(w, http.StatusInternalServerError, "could not build archive")
		return
	}
	if _, err := tmp.Seek(0, io.SeekStart); err != nil {
		log.Printf("api: archive seek %s: %v", id, err)
		writeError(w, http.StatusInternalServerError, "could not build archive")
		return
	}

	objectName := id + "/archive.zip"
	if err := s.storage.Upload(r.Context(), objectName, tmp, stat.Size(), "application/zip"); err != nil {
		log.Printf("api: archive upload %s: %v", id, err)
		writeError(w, http.StatusBadGateway, "не удалось сохранить архив")
		return
	}

	filename := archiveFileName(batch)
	dl, err := s.storage.PresignedDownloadURL(r.Context(), objectName, filename, 24*time.Hour)
	if err != nil {
		log.Printf("api: archive presign %s: %v", id, err)
		writeError(w, http.StatusInternalServerError, "could not create download link")
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"downloadUrl": dl,
		"filename":    filename,
		"count":       len(ready),
	})
}

// videoDownloader is the subset of storage used while packing a zip.
type videoDownloader interface {
	Download(ctx context.Context, objectName string) (io.ReadCloser, error)
}

func writeBatchZip(ctx context.Context, dl videoDownloader, w io.Writer, tasks []*store.Task) error {
	zw := zip.NewWriter(w)
	names := uniqueVideoFileNames(tasks)

	for i, t := range tasks {
		if err := ctx.Err(); err != nil {
			_ = zw.Close()
			return err
		}
		body, err := dl.Download(ctx, t.VideoObject)
		if err != nil {
			_ = zw.Close()
			return fmt.Errorf("download %s: %w", t.VideoObject, err)
		}

		hdr := &zip.FileHeader{
			Name:     names[i],
			Method:   zip.Deflate,
			Modified: time.Now(),
		}
		// NonUTF8=false (default) sets the UTF-8 language-encoding flag so
		// Windows Explorer shows Cyrillic names correctly after unpacking.
		entry, err := zw.CreateHeader(hdr)
		if err != nil {
			body.Close()
			_ = zw.Close()
			return fmt.Errorf("zip entry %s: %w", names[i], err)
		}
		if _, err := io.Copy(entry, body); err != nil {
			body.Close()
			_ = zw.Close()
			return fmt.Errorf("zip copy %s: %w", names[i], err)
		}
		body.Close()
	}

	if err := zw.Close(); err != nil {
		return fmt.Errorf("zip close: %w", err)
	}
	return nil
}

// uniqueVideoFileNames returns a zip entry name per task, appending _2, _3, …
// when several tasks share the same display name.
func uniqueVideoFileNames(tasks []*store.Task) []string {
	used := make(map[string]struct{}, len(tasks))
	out := make([]string, len(tasks))
	for i, t := range tasks {
		base := videoFileName(t)
		name := base
		if _, taken := used[name]; taken {
			stem := strings.TrimSuffix(base, ".mp4")
			for n := 2; ; n++ {
				candidate := fmt.Sprintf("%s_%d.mp4", stem, n)
				if _, taken = used[candidate]; !taken {
					name = candidate
					break
				}
			}
		}
		used[name] = struct{}{}
		out[i] = name
	}
	return out
}

// archiveFileName builds the "save as" name for the zip itself.
func archiveFileName(b *store.Batch) string {
	name := sanitizeFileName(b.Title)
	if name == "" {
		name = "videos"
	}
	return name + ".zip"
}
