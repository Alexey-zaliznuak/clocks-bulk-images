package vkads

import (
	"strings"
	"time"
)

// Segment is a VK Ads remarketing audience.
type Segment struct {
	ID      int64     `json:"id"`
	Name    string    `json:"name"`
	Created time.Time `json:"-"`
}

// MatchAudience returns the newest segment whose name equals value ignoring
// letter case only. Extra spaces or punctuation in the audience name are a miss.
func MatchAudience(value string, items []Segment) *Segment {
	want := strings.ToLower(value)
	if want == "" {
		return nil
	}
	var best *Segment
	for i := range items {
		if strings.ToLower(items[i].Name) != want {
			continue
		}
		if best == nil || items[i].Created.After(best.Created) ||
			(items[i].Created.Equal(best.Created) && items[i].ID > best.ID) {
			best = &items[i]
		}
	}
	return best
}
