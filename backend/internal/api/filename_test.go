package api

import (
	"testing"

	"named_clocks/backend/internal/store"
)

func TestVideoFileName(t *testing.T) {
	cases := []struct {
		name  string
		first string
		last  string
		want  string
	}{
		{"имя и фамилия", "Иван", "Иванов", "Иван_Иванов.mp4"},
		{"с отчеством", "Иван", "Иванов Петрович", "Иван_Иванов_Петрович.mp4"},
		{"только имя", "Иван", "", "Иван.mp4"},
		{"без имени", "", "", "video.mp4"},
		{"запрещённые символы", `Ив/ан*`, `Ива:нов?`, "Иван_Иванов.mp4"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := videoFileName(&store.Task{FirstName: c.first, LastName: c.last})
			if got != c.want {
				t.Fatalf("videoFileName() = %q, want %q", got, c.want)
			}
		})
	}
}
