package api

import (
	"strings"

	"named_clocks/backend/internal/store"
)

// videoFileName builds the name the finished clip is saved under:
// "<дата>_<имя>_<фамилия>_<отчество>.mp4". Missing parts are simply skipped —
// the last name field holds everything after the first token, so a patronymic
// ends up as its own segment.
func videoFileName(t *store.Task) string {
	parts := []string{t.CreatedAt.Format("2006-01-02")}
	parts = append(parts, strings.Fields(t.FirstName)...)
	parts = append(parts, strings.Fields(t.LastName)...)

	name := sanitizeFileName(strings.Join(parts, "_"))
	if name == "" {
		name = "video"
	}
	return name + ".mp4"
}

// sanitizeFileName drops the characters Windows, macOS and browsers refuse in a
// file name and trims the result to a length no file system objects to.
func sanitizeFileName(s string) string {
	s = strings.Map(func(r rune) rune {
		if r < 0x20 || r == 0x7f || strings.ContainsRune(`\/:*?"<>|`, r) {
			return -1
		}
		if r == ' ' {
			return '_'
		}
		return r
	}, s)

	runes := []rune(strings.Trim(s, "_-. "))
	if len(runes) > 120 {
		runes = runes[:120]
	}
	return strings.Trim(string(runes), "_-. ")
}
