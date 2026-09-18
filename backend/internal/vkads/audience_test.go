package vkads

import (
	"encoding/json"
	"strings"
	"testing"
	"time"
)

func TestMatchAudienceExactCaseInsensitive(t *testing.T) {
	older := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	newer := time.Date(2025, 6, 1, 0, 0, 0, 0, time.UTC)
	items := []Segment{
		{ID: 1, Name: "Иван", Created: older},
		{ID: 2, Name: "иван", Created: newer},
		{ID: 4, Name: "Иван_ауд", Created: newer.Add(2 * time.Hour)},
		{ID: 5, Name: "Мария", Created: newer},
	}
	got := MatchAudience("Иван", items)
	if got == nil || got.ID != 2 {
		t.Fatalf("got %#v, want newest exact Иван", got)
	}
	if MatchAudience(" Иван ", items) == nil || MatchAudience(" Иван ", items).ID != 2 {
		t.Fatal("spaces around the name should still be an exact match")
	}
	if MatchAudience("Пётр", items) != nil {
		t.Fatal("missing name must not match")
	}
}

func TestMatchAudiencePrefersExactThenAudiencePrefix(t *testing.T) {
	older := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	newer := time.Date(2025, 6, 1, 0, 0, 0, 0, time.UTC)
	both := []Segment{
		{ID: 1, Name: "Аудитория Аркадий", Created: newer},
		{ID: 2, Name: "аркадий", Created: older},
	}
	got := MatchAudience("Аркадий", both)
	if got == nil || got.ID != 2 {
		t.Fatalf("exact name must win, got %#v", got)
	}
	onlyPrefixed := []Segment{
		{ID: 3, Name: "аудитория аркадий", Created: older},
		{ID: 4, Name: "Аудитория Аркадий", Created: newer},
		{ID: 5, Name: "Аудитория Гущинов"},
	}
	got = MatchAudience("Аркадий", onlyPrefixed)
	if got == nil || got.ID != 4 {
		t.Fatalf("fallback Аудитория Аркадий = %#v", got)
	}
	if MatchAudience("Гущин", onlyPrefixed) != nil {
		t.Fatal("Аудитория Гущинов is not Аудитория Гущин")
	}
}

func TestShowHours(t *testing.T) {
	hours := ShowHours()
	if len(hours) != 15 || hours[0] != 6 || hours[len(hours)-1] != 20 {
		t.Fatalf("hours = %v", hours)
	}
}

func TestAgeList(t *testing.T) {
	got := AgeList(24, 26, false)
	if len(got) != 3 || got[0] != 24 || got[2] != 26 {
		t.Fatalf("age = %v", got)
	}
	got = AgeList(24, 25, true)
	if got[0] != 0 || len(got) != 3 {
		t.Fatalf("unknown age = %v", got)
	}
}

func TestTextListObjective(t *testing.T) {
	var one Package
	if err := json.Unmarshal([]byte(`{"id":1,"objective":"community"}`), &one); err != nil || one.Objective.ForAPI() != "community" {
		t.Fatalf("string objective: %+v %v", one, err)
	}
	var many Package
	if err := json.Unmarshal([]byte(`{"id":2,"objective":["socialengagement","community"]}`), &many); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(many.Objective), "community") || many.Objective.ForAPI() != "community" {
		t.Fatalf("array objective = %q", many.Objective)
	}
}

func TestPickCommunityMessagePackage(t *testing.T) {
	packages := []Package{
		{ID: 1, Name: "Traffic", Objective: "traffic"},
		{ID: 2, Name: "Community clicks", Objective: "socialengagement"},
		{ID: 3, Name: "Community", Objective: "community", PricedGoal: &PriceGoal{Name: "Отправка сообщения"}},
	}
	got := PickCommunityMessagePackage(packages, "send_message")
	if got == nil || got.ID != 3 {
		t.Fatalf("got %#v", got)
	}
}

func TestSettingsNormalizeEmpty(t *testing.T) {
	s := Settings{}.Normalize()
	if s.CommunityID != DefaultCommunityID || !s.Optimization || s.AgeFrom != 24 || s.BudgetDay == nil || *s.BudgetDay != 999 {
		t.Fatalf("defaults = %+v", s)
	}
	if s.BannerTitle != DefaultBannerTitle || s.BannerCTA != DefaultBannerCTA {
		t.Fatalf("banner defaults = %+v", s)
	}
}
