package vkads

import "testing"

func testCatalog() *Catalog {
	return &Catalog{
		Package:   Package{ID: 1, Objective: "community"},
		RussiaID:  188,
		Pads:      []int{3417},
		URLID:     9,
		DateStart: "2026-09-18",
	}
}

func testSettings(optimization bool, day float64) Settings {
	return Settings{
		CommunityID:  1,
		Optimization: optimization,
		BudgetDay:    &day,
		Sex:          "male",
		AgeFrom:      24,
		AgeTo:        65,
	}
}

func TestPlanBodyBudgetStringWhenOptimized(t *testing.T) {
	body := PlanBody("кампания", testSettings(true, 999), testCatalog())
	got, ok := body["budget_limit_day"].(string)
	if !ok || got != "999" {
		t.Fatalf("plan budget_limit_day = %#v", body["budget_limit_day"])
	}
	if _, ok := body["ad_groups"]; ok {
		t.Fatal("plan must not send empty ad_groups")
	}
	if _, ok := body["enable_utm"]; ok {
		t.Fatal("ad_plan does not accept enable_utm")
	}
	group := NestedGroupBody("Гущин", 77, testSettings(true, 999), testCatalog())
	if _, ok := group["ad_plan_id"]; ok {
		t.Fatal("nested group must not have ad_plan_id")
	}
	AttachCampaigns(body, []map[string]any{group})
	list, _ := body["campaigns"].([]any)
	if len(list) != 1 {
		t.Fatalf("campaigns = %#v", body["campaigns"])
	}
}

func TestGroupBodyOmitsBudgetWhenOptimized(t *testing.T) {
	body := GroupBody("Иван", 10, 77, testSettings(true, 999), testCatalog())
	if _, ok := body["budget_limit_day"]; ok {
		t.Fatalf("group should not carry campaign budget, got %#v", body["budget_limit_day"])
	}
	if body["name"] != "Иван" {
		t.Fatalf("group name = %#v", body["name"])
	}
}

func TestGroupBodyBudgetWhenNotOptimized(t *testing.T) {
	settings := testSettings(false, 999)
	plan := PlanBody("кампания", settings, testCatalog())
	if _, ok := plan["budget_limit_day"]; ok {
		t.Fatalf("plan should not carry group budget, got %#v", plan["budget_limit_day"])
	}
	group := GroupBody("Петров", 10, 77, settings, testCatalog())
	got, ok := group["budget_limit_day"].(string)
	if !ok || got != "999" {
		t.Fatalf("group budget_limit_day = %#v", group["budget_limit_day"])
	}
}

func TestParseCreatePlanReadsNestedCampaigns(t *testing.T) {
	planID, groups, err := parseCreatePlan([]byte(`{"id":340,"campaigns":[{"id":321},{"id":322}]}`))
	if err != nil || planID != 340 || len(groups) != 2 || groups[0] != 321 || groups[1] != 322 {
		t.Fatalf("plan=%d groups=%v err=%v", planID, groups, err)
	}
}
