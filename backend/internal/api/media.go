package api

import (
	"fmt"
	"io"
	"log"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"

	"named_clocks/backend/internal/store"
)

// audioPrefix is the object storage prefix for the media library.
const audioPrefix = "audio/"

// handleListAudio returns the mp3 library with fresh download links.
func (s *Server) handleListAudio(w http.ResponseWriter, r *http.Request) {
	assets, err := s.store.ListMediaAssets(r.Context(), store.KindAudio, 0)
	if err != nil {
		log.Printf("api: list audio: %v", err)
		writeError(w, http.StatusInternalServerError, "could not list audio")
		return
	}
	for _, a := range assets {
		url, err := s.storage.PresignedURL(r.Context(), a.Object, 24*time.Hour)
		if err != nil {
			log.Printf("api: presign %s: %v", a.Object, err)
			continue
		}
		a.URL = url
	}
	writeJSON(w, http.StatusOK, map[string]any{"assets": assets})
}

// handleUploadAudio accepts an mp3 upload, verifies it really contains audio,
// stores it in object storage and records it in the library.
func (s *Server) handleUploadAudio(w http.ResponseWriter, r *http.Request) {
	file, header, cleanup, err := s.receiveUpload(w, r, s.maxAudioBytes())
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	defer cleanup()

	ext := strings.ToLower(filepath.Ext(header.Filename))
	if ext != ".mp3" {
		writeError(w, http.StatusBadRequest, "поддерживаются только файлы .mp3")
		return
	}

	dir, err := s.ffmpeg.NewTempDir("upload-")
	if err != nil {
		log.Printf("api: temp dir: %v", err)
		writeError(w, http.StatusInternalServerError, "could not process upload")
		return
	}
	defer os.RemoveAll(dir)

	localPath := filepath.Join(dir, "audio.mp3")
	size, err := saveToFile(file, localPath)
	if err != nil {
		log.Printf("api: save upload: %v", err)
		writeError(w, http.StatusInternalServerError, "could not read upload")
		return
	}
	if size == 0 {
		writeError(w, http.StatusBadRequest, "файл пустой")
		return
	}

	info, err := s.ffmpeg.Probe(r.Context(), localPath)
	if err != nil {
		log.Printf("api: probe upload %s: %v", header.Filename, err)
		writeError(w, http.StatusBadRequest, "не удалось прочитать файл — это точно mp3?")
		return
	}
	if !info.HasAudio {
		writeError(w, http.StatusBadRequest, "в файле нет аудиодорожки")
		return
	}

	id, err := s.store.ReserveMediaAssetID(r.Context())
	if err != nil {
		log.Printf("api: reserve media id: %v", err)
		writeError(w, http.StatusInternalServerError, "could not save audio")
		return
	}
	object := audioPrefix + id + ".mp3"

	stored, err := os.Open(localPath)
	if err != nil {
		log.Printf("api: reopen upload: %v", err)
		writeError(w, http.StatusInternalServerError, "could not save audio")
		return
	}
	defer stored.Close()
	if err := s.storage.Upload(r.Context(), object, stored, size, "audio/mpeg"); err != nil {
		log.Printf("api: upload audio: %v", err)
		writeError(w, http.StatusBadGateway, "не удалось сохранить файл в хранилище")
		return
	}

	title := strings.TrimSpace(r.FormValue("title"))
	if title == "" {
		title = strings.TrimSuffix(header.Filename, ext)
	}
	asset := &store.MediaAsset{
		ID:              id,
		Kind:            store.KindAudio,
		Title:           title,
		Filename:        header.Filename,
		Object:          object,
		ContentType:     "audio/mpeg",
		SizeBytes:       size,
		DurationSeconds: info.Duration,
	}
	if err := s.store.CreateMediaAsset(r.Context(), asset); err != nil {
		log.Printf("api: create media asset: %v", err)
		// Do not leave an orphaned object behind.
		if rmErr := s.storage.Remove(r.Context(), object); rmErr != nil {
			log.Printf("api: rollback %s: %v", object, rmErr)
		}
		writeError(w, http.StatusInternalServerError, "could not save audio")
		return
	}
	if url, err := s.storage.PresignedURL(r.Context(), asset.Object, 24*time.Hour); err == nil {
		asset.URL = url
	}
	writeJSON(w, http.StatusCreated, map[string]any{"asset": asset})
}

// handleDeleteAudio removes an mp3 from the library unless a running task still
// needs it.
func (s *Server) handleDeleteAudio(w http.ResponseWriter, r *http.Request) {
	id := strings.TrimSpace(chi.URLParam(r, "id"))
	if id == "" {
		writeError(w, http.StatusBadRequest, "audio id is required")
		return
	}
	asset, err := s.store.GetMediaAsset(r.Context(), id)
	if err != nil {
		log.Printf("api: get media asset %s: %v", id, err)
		writeError(w, http.StatusInternalServerError, "could not delete audio")
		return
	}
	if asset == nil {
		writeError(w, http.StatusNotFound, "audio not found")
		return
	}
	inUse, err := s.store.MediaAssetInUse(r.Context(), id)
	if err != nil {
		log.Printf("api: media asset in use %s: %v", id, err)
		writeError(w, http.StatusInternalServerError, "could not delete audio")
		return
	}
	if inUse {
		writeError(w, http.StatusConflict, "файл используется незавершёнными задачами")
		return
	}
	if err := s.storage.Remove(r.Context(), asset.Object); err != nil {
		log.Printf("api: remove %s: %v", asset.Object, err)
	}
	if err := s.store.DeleteMediaAsset(r.Context(), id); err != nil {
		log.Printf("api: delete media asset %s: %v", id, err)
		writeError(w, http.StatusInternalServerError, "could not delete audio")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"deleted": id})
}

// handleExtractAudio converts an uploaded mp4 to mp3 and streams the result back
// as a download. Nothing is stored server-side.
func (s *Server) handleExtractAudio(w http.ResponseWriter, r *http.Request) {
	file, header, cleanup, err := s.receiveUpload(w, r, s.maxVideoBytes())
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	defer cleanup()

	dir, err := s.ffmpeg.NewTempDir("extract-")
	if err != nil {
		log.Printf("api: temp dir: %v", err)
		writeError(w, http.StatusInternalServerError, "could not process upload")
		return
	}
	defer os.RemoveAll(dir)

	// ffmpeg needs a seekable input for mp4, so the upload goes to disk first.
	inPath := filepath.Join(dir, "input"+strings.ToLower(filepath.Ext(header.Filename)))
	if _, err := saveToFile(file, inPath); err != nil {
		log.Printf("api: save upload: %v", err)
		writeError(w, http.StatusInternalServerError, "could not read upload")
		return
	}

	outPath := filepath.Join(dir, "audio.mp3")
	if err := s.ffmpeg.ExtractMP3(r.Context(), inPath, outPath); err != nil {
		log.Printf("api: extract audio from %s: %v", header.Filename, err)
		writeError(w, http.StatusBadRequest, "не удалось извлечь аудио: "+firstLine(err.Error()))
		return
	}

	out, err := os.Open(outPath)
	if err != nil {
		log.Printf("api: open extracted audio: %v", err)
		writeError(w, http.StatusInternalServerError, "could not read result")
		return
	}
	defer out.Close()
	st, err := out.Stat()
	if err != nil {
		log.Printf("api: stat extracted audio: %v", err)
		writeError(w, http.StatusInternalServerError, "could not read result")
		return
	}

	name := strings.TrimSuffix(header.Filename, filepath.Ext(header.Filename)) + ".mp3"
	w.Header().Set("Content-Type", "audio/mpeg")
	w.Header().Set("Content-Length", fmt.Sprintf("%d", st.Size()))
	w.Header().Set("Content-Disposition", contentDisposition(name))
	w.Header().Set("X-Filename", sanitizeASCII(name))
	if _, err := io.Copy(w, out); err != nil {
		log.Printf("api: stream extracted audio: %v", err)
	}
}

// ---------- upload helpers ----------

func (s *Server) maxAudioBytes() int64 { return s.maxAudioMB * 1024 * 1024 }
func (s *Server) maxVideoBytes() int64 { return s.maxVideoMB * 1024 * 1024 }

// receiveUpload reads the single `file` part of a multipart request, enforcing a
// hard size limit. The returned cleanup closes the part and drops temp files
// created by the multipart reader.
func (s *Server) receiveUpload(
	w http.ResponseWriter,
	r *http.Request,
	maxBytes int64,
) (multipart.File, *multipart.FileHeader, func(), error) {
	if s.ffmpeg == nil {
		return nil, nil, nil, fmt.Errorf("ffmpeg недоступен на сервере")
	}
	r.Body = http.MaxBytesReader(w, r.Body, maxBytes)
	// Buffer a few MB in memory, spill the rest to disk.
	if err := r.ParseMultipartForm(8 << 20); err != nil {
		return nil, nil, nil, fmt.Errorf("файл слишком большой или запрос повреждён (лимит %d МБ)", maxBytes/(1024*1024))
	}
	file, header, err := r.FormFile("file")
	if err != nil {
		if r.MultipartForm != nil {
			_ = r.MultipartForm.RemoveAll()
		}
		return nil, nil, nil, fmt.Errorf("файл не выбран")
	}
	cleanup := func() {
		file.Close()
		if r.MultipartForm != nil {
			_ = r.MultipartForm.RemoveAll()
		}
	}
	return file, header, cleanup, nil
}

// saveToFile copies an upload to disk and returns the number of bytes written.
func saveToFile(src io.Reader, path string) (int64, error) {
	dst, err := os.Create(path)
	if err != nil {
		return 0, err
	}
	defer dst.Close()
	n, err := io.Copy(dst, src)
	if err != nil {
		return 0, err
	}
	if err := dst.Sync(); err != nil {
		return 0, err
	}
	return n, nil
}

// contentDisposition builds a header that works for both ASCII and Cyrillic
// filenames (RFC 5987).
func contentDisposition(name string) string {
	return fmt.Sprintf(`attachment; filename="%s"; filename*=UTF-8''%s`,
		sanitizeASCII(name), urlEncode(name))
}

// sanitizeASCII strips characters that would break the plain filename parameter.
func sanitizeASCII(name string) string {
	var b strings.Builder
	for _, r := range name {
		if r < 32 || r > 126 || r == '"' || r == '\\' {
			b.WriteByte('_')
			continue
		}
		b.WriteRune(r)
	}
	out := b.String()
	if strings.Trim(out, "_.") == "" {
		return "audio.mp3"
	}
	return out
}

func urlEncode(s string) string {
	const hex = "0123456789ABCDEF"
	var b strings.Builder
	for i := 0; i < len(s); i++ {
		c := s[i]
		if (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9') ||
			c == '-' || c == '_' || c == '.' || c == '~' {
			b.WriteByte(c)
			continue
		}
		b.WriteByte('%')
		b.WriteByte(hex[c>>4])
		b.WriteByte(hex[c&0x0f])
	}
	return b.String()
}

func firstLine(s string) string {
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		return s[:i]
	}
	return s
}
