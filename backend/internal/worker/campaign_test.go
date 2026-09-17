package worker

import "testing"

func TestImageExtension(t *testing.T) {
	tests := []struct {
		contentType string
		path        string
		want        string
	}{
		{"image/jpeg", "/result", ".jpg"},
		{"image/png", "/result.jpg", ".png"},
		{"application/octet-stream", "/result.webp", ".webp"},
		{"application/octet-stream", "/result.exe", ""},
	}
	for _, test := range tests {
		if got := imageExtension(test.contentType, test.path); got != test.want {
			t.Errorf("imageExtension(%q, %q) = %q, want %q", test.contentType, test.path, got, test.want)
		}
	}
}
