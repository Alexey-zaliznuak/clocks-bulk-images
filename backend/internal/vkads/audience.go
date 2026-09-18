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

// MatchAudience finds a segment by exact name first, then «Аудитория {name}».
// Letter case and surrounding spaces are ignored. If several match, the newest wins.
func MatchAudience(value string, items []Segment) *Segment {
	want := strings.ToLower(strings.TrimSpace(value))
	if want == "" {
		return nil
	}
	if found := matchAudienceName(want, items); found != nil {
		return found
	}
	return matchAudienceName("аудитория "+want, items)
}

func matchAudienceName(want string, items []Segment) *Segment {
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
