package vkads

import "testing"

func TestGroupTargetings(t *testing.T) {
	day := 999.0
	settings := Settings{
		Sex: "male", AgeFrom: 24, AgeTo: 65, AgeUnknown: false,
		Optimization: true, BiddingStrategy: "min_price", BudgetDay: &day,
	}
	got := groupTargetings(settings, 77, 188, []int{1})
	hours, _ := got["fulltime"].(map[string]any)
	if hours == nil {
		t.Fatal("missing fulltime")
	}
	mon, _ := hours["mon"].([]int)
	if len(mon) != 15 || mon[0] != 6 || mon[14] != 20 {
		t.Fatalf("mon hours = %v", mon)
	}
	flags, _ := hours["flags"].([]string)
	if len(flags) != 0 {
		t.Fatalf("flags should be empty, got %v", flags)
	}
	geo, _ := got["geo"].(map[string]any)
	regions, _ := geo["regions"].([]int64)
	if len(regions) != 1 || regions[0] != 188 {
		t.Fatalf("regions = %v", regions)
	}
	segs, _ := got["segments"].([]int64)
	if len(segs) != 1 || segs[0] != 77 {
		t.Fatalf("segments = %v", segs)
	}
	if _, ok := got["remarketing"]; ok {
		t.Fatal("Targetings has segments, not remarketing")
	}
}

func TestAutobiddingMode(t *testing.T) {
	if got := autobiddingMode("min_price"); got != "max_goals" {
		t.Fatalf("min_price = %q", got)
	}
	if got := autobiddingMode("max_goals"); got != "max_goals" {
		t.Fatalf("max_goals = %q", got)
	}
	if got := autobiddingMode("second_price_mean"); got != "second_price_mean" {
		t.Fatalf("second_price_mean = %q", got)
	}
}
