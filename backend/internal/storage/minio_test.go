package storage

import "testing"

func TestContentDisposition(t *testing.T) {
	got := contentDisposition("2026-07-29_Иван.mp4")
	want := `attachment; filename="2026-07-29.mp4"; filename*=UTF-8''2026-07-29_%D0%98%D0%B2%D0%B0%D0%BD.mp4`
	if got != want {
		t.Fatalf("contentDisposition() = %q, want %q", got, want)
	}
	for _, c := range got {
		if c > 0x7e || c < 0x20 {
			t.Fatalf("non-ascii byte %q in header value %q", c, got)
		}
	}
}

func TestAsciiFallback(t *testing.T) {
	cases := map[string]string{
		"2026-07-29_Иван.mp4": "2026-07-29.mp4",
		"2026-07-29_Ivan.mp4": "2026-07-29_Ivan.mp4",
		"Иван.mp4":            "video.mp4",
	}
	for in, want := range cases {
		if got := asciiFallback(in); got != want {
			t.Fatalf("asciiFallback(%q) = %q, want %q", in, got, want)
		}
	}
}
