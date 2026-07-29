package api

import (
	"testing"
	"time"

	"named_clocks/backend/internal/store"
)

func TestVideoFileName(t *testing.T) {
	created := time.Date(2026, 7, 29, 12, 0, 0, 0, time.UTC)

	cases := []struct {
		name  string
		first string
		last  string
		want  string
	}{
		{"имя и фамилия", "Иван", "Иванов", "2026-07-29_Иван_Иванов.mp4"},
		{"с отчеством", "Иван", "Иванов Петрович", "2026-07-29_Иван_Иванов_Петрович.mp4"},
		{"только имя", "Иван", "", "2026-07-29_Иван.mp4"},
		{"без имени", "", "", "2026-07-29.mp4"},
		{"запрещённые символы", `Ив/ан*`, `Ива:нов?`, "2026-07-29_Иван_Иванов.mp4"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := videoFileName(&store.Task{FirstName: c.first, LastName: c.last, CreatedAt: created})
			if got != c.want {
				t.Fatalf("videoFileName() = %q, want %q", got, c.want)
			}
		})
	}
}
