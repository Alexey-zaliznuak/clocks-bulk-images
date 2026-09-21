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

func TestMatchAudiencePrefersExactThenSubstring(t *testing.T) {
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
	got = MatchAudience("Гущин", onlyPrefixed)
	if got == nil || got.ID != 5 {
		t.Fatalf("Аудитория Гущинов must match Гущин, got %#v", got)
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
	if !strings.Contains(string(many.Objective), "community") || many.Objective.ForAPI() != "socialengagement" {
		t.Fatalf("array objective = %q", many.Objective)
	}
}

func TestPickCommunityPackage(t *testing.T) {
	packages := []Package{
		{ID: 1, Name: "Traffic", Objective: "traffic"},
		{ID: 2, Name: "Community clicks", Objective: "socialengagement"},
		{ID: 3, Name: "Community", Objective: "community", PricedGoal: &PriceGoal{Name: "Отправка сообщения"}},
	}
	got := PickCommunityPackage(packages, "send_message")
	if got == nil || got.ID != 3 {
		t.Fatalf("got %#v", got)
	}
}

// The cabinet names the campaign after the package's priced goal, so the join
// package must never stand in for a campaign that collects messages.
func TestPickCommunityPackageSkipsTheJoinGoal(t *testing.T) {
	join := Package{
		ID:         3122,
		Name:       "or_tt_crossdevice_community_vk_ocpm_socialengagement_pricedGoals_join",
		Objective:  "socialengagement",
		PricedGoal: &PriceGoal{Name: "Подписка на сообщество"},
	}
	message := Package{
		ID:         3127,
		Name:       "or_tt_crossdevice_community_vk_ocpm_socialengagement_pricedGoals_message",
		Objective:  "socialengagement",
		PricedGoal: &PriceGoal{Name: "Отправка сообщения"},
	}
	packages := []Package{join, message}
	if got := PickCommunityPackage(packages, "send_message"); got == nil || got.ID != 3127 {
		t.Fatalf("сообщение → %#v", got)
	}
	if got := PickCommunityPackage(packages, "join_community"); got == nil || got.ID != 3122 {
		t.Fatalf("вступление → %#v", got)
	}
	if got := PickCommunityPackage([]Package{join}, "send_message"); got != nil {
		t.Fatalf("подписочный пакет не должен подменять сообщения: %#v", got)
	}
}

func TestStopAfterPage(t *testing.T) {
	if !stopAfterPage(0, 50, 0, 0, 0, 1, 8) {
		t.Fatal("empty page must stop")
	}
	if !stopAfterPage(50, 50, 0, 50, 0, 2, 8) {
		t.Fatal("no new ids must stop")
	}
	if !stopAfterPage(12, 50, 12, 62, 0, 2, 8) {
		t.Fatal("short page must stop")
	}
	if !stopAfterPage(50, 50, 50, 200, 180, 4, 8) {
		t.Fatal("count reached must stop")
	}
	if !stopAfterPage(50, 50, 50, 400, 0, 8, 8) {
		t.Fatal("page cap must stop")
	}
	if stopAfterPage(50, 50, 50, 50, 0, 1, 8) {
		t.Fatal("first full page with new ids should continue")
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
