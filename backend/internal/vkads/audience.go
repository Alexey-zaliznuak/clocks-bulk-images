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

// MatchAudience returns a segment whose name equals value ignoring case and
// surrounding spaces. «Аудитория Гущинов» is not a match for «Гущин».
// If several names match, any of them is fine — we keep the newest.
func MatchAudience(value string, items []Segment) *Segment {
	want := strings.ToLower(strings.TrimSpace(value))
	if want == "" {
		return nil
	}
	var best *Segment
	for i := range items {
		if strings.ToLower(strings.TrimSpace(items[i].Name)) != want {
			continue
		}
		best = newerSegment(best, &items[i])
	}
	return best
}

func newerSegment(best, cand *Segment) *Segment {
	if best == nil {
		return cand
	}
	if cand.Created.After(best.Created) || (cand.Created.Equal(best.Created) && cand.ID > best.ID) {
		return cand
	}
	return best
}
