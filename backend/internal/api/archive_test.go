package api

import (
	"archive/zip"
	"bytes"
	"context"
	"io"
	"strings"
	"testing"

	"named_clocks/backend/internal/store"
)

func TestUniqueVideoFileNames(t *testing.T) {
	tasks := []*store.Task{
		{FirstName: "Иван", LastName: "Иванов"},
		{FirstName: "Иван", LastName: "Иванов"},
		{FirstName: "Пётр", LastName: "Петров"},
		{FirstName: "Иван", LastName: "Иванов"},
	}
	got := uniqueVideoFileNames(tasks)
	want := []string{
		"Иван_Иванов.mp4",
		"Иван_Иванов_2.mp4",
		"Пётр_Петров.mp4",
		"Иван_Иванов_3.mp4",
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("uniqueVideoFileNames()[%d] = %q, want %q", i, got[i], want[i])
		}
	}
}

func TestArchiveFileName(t *testing.T) {
	cases := []struct {
		title string
		want  string
	}{
		{"Пачка 12 марта", "Пачка_12_марта.zip"},
		{"", "videos.zip"},
		{`bad/name:here`, "badnamehere.zip"},
	}
	for _, c := range cases {
		got := archiveFileName(&store.Batch{Title: c.title})
		if got != c.want {
			t.Fatalf("archiveFileName(%q) = %q, want %q", c.title, got, c.want)
		}
	}
}

type memDownloader map[string][]byte

func (m memDownloader) Download(_ context.Context, objectName string) (io.ReadCloser, error) {
	data, ok := m[objectName]
	if !ok {
		return nil, io.EOF
	}
	return io.NopCloser(bytes.NewReader(data)), nil
}

func TestWriteBatchZipUTF8Names(t *testing.T) {
	tasks := []*store.Task{
		{FirstName: "Иван", LastName: "Иванов", VideoObject: "a/1.mp4"},
		{FirstName: "Мария", LastName: "Петрова", VideoObject: "a/2.mp4"},
	}
	dl := memDownloader{
		"a/1.mp4": []byte("video-one"),
		"a/2.mp4": []byte("video-two"),
	}

	var buf bytes.Buffer
	if err := writeBatchZip(context.Background(), dl, &buf, tasks); err != nil {
		t.Fatalf("writeBatchZip: %v", err)
	}

	zr, err := zip.NewReader(bytes.NewReader(buf.Bytes()), int64(buf.Len()))
	if err != nil {
		t.Fatalf("zip.NewReader: %v", err)
	}
	if len(zr.File) != 2 {
		t.Fatalf("entries = %d, want 2", len(zr.File))
	}

	wantNames := map[string]string{
		"Иван_Иванов.mp4":   "video-one",
		"Мария_Петрова.mp4": "video-two",
	}
	for _, f := range zr.File {
		if f.NonUTF8 {
			t.Fatalf("entry %q has NonUTF8 set; Windows needs the UTF-8 flag", f.Name)
		}
		body, err := f.Open()
		if err != nil {
			t.Fatalf("open %q: %v", f.Name, err)
		}
		data, err := io.ReadAll(body)
		body.Close()
		if err != nil {
			t.Fatalf("read %q: %v", f.Name, err)
		}
		want, ok := wantNames[f.Name]
		if !ok {
			t.Fatalf("unexpected entry %q", f.Name)
		}
		if string(data) != want {
			t.Fatalf("entry %q = %q, want %q", f.Name, data, want)
		}
		delete(wantNames, f.Name)
	}
	if len(wantNames) != 0 {
		t.Fatalf("missing entries: %v", wantNames)
	}

	// Sanity: central directory should mention a Cyrillic name as UTF-8 bytes.
	if !strings.Contains(buf.String(), "Иван_Иванов.mp4") {
		t.Fatal("zip bytes do not contain UTF-8 Cyrillic filename")
	}
}
