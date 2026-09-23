package vkads

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

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
	if body["autobidding_mode"] != "max_goals" {
		t.Fatalf("plan autobidding_mode = %#v", body["autobidding_mode"])
	}
	if _, ok := body["ad_groups"]; ok {
		t.Fatal("plan must not send empty ad_groups")
	}
	if _, ok := body["enable_utm"]; ok {
		t.Fatal("ad_plan does not accept enable_utm")
	}
	if _, ok := body["ad_object_type"]; ok {
		t.Fatal("AdPlan has no ad_object_type")
	}
	if _, ok := body["ad_object_id"]; ok {
		t.Fatal("AdPlan has no ad_object_id")
	}
	group := NestedGroupBody("Гущин", 77, testSettings(true, 999), testCatalog())
	if _, ok := group["ad_plan_id"]; ok {
		t.Fatal("nested group must not have ad_plan_id")
	}
	if _, ok := group["autobidding_mode"]; ok {
		t.Fatal("optimized group must inherit autobidding_mode from the plan")
	}
	withGroups := AttachGroups(body, []map[string]any{group}, "campaigns")
	list, _ := withGroups["campaigns"].([]any)
	if len(list) != 1 {
		t.Fatalf("campaigns = %#v", withGroups["campaigns"])
	}
	if _, ok := body["campaigns"]; ok {
		t.Fatal("AttachGroups must not mutate the plan it is given")
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
	if _, ok := body["enable_utm"]; ok {
		t.Fatal("package does not allow enable_utm")
	}
	if _, ok := body["autobidding_mode"]; ok {
		t.Fatal("optimized group must inherit autobidding_mode from the plan")
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
	if group["autobidding_mode"] != "max_goals" {
		t.Fatalf("group autobidding_mode = %#v", group["autobidding_mode"])
	}
	if _, ok := plan["autobidding_mode"]; ok {
		t.Fatal("plan without optimization must not send autobidding_mode")
	}
}

func TestParseCreatePlanReadsNestedCampaigns(t *testing.T) {
	planID, groups, err := parseCreatePlan([]byte(`{"id":340,"campaigns":[{"id":321,"banners":[{"id":11}]},{"id":322}]}`))
	if err != nil || planID != 340 || len(groups) != 2 || groups[0].ID != 321 || groups[0].BannerIDs[0] != 11 || groups[1].ID != 322 {
		t.Fatalf("plan=%d groups=%v err=%v", planID, groups, err)
	}
}

func TestParseCreateGroupReadsBanners(t *testing.T) {
	got, err := parseCreateGroup([]byte(`{"id":9826424,"banners":[{"id":23826937}]}`))
	if err != nil || got.ID != 9826424 || len(got.BannerIDs) != 1 || got.BannerIDs[0] != 23826937 {
		t.Fatalf("got=%#v err=%v", got, err)
	}
}

func TestAttachBanner(t *testing.T) {
	group := NestedGroupBody("Аркадий", 77, testSettings(true, 999), testCatalog())
	banner := BannerBody("Аркадий", 0, 9, 55, nil, "title", "text", "contactUs", &BannerPattern{Format: []BannerSlot{
		{Field: "url", Role: "primary"},
		{Field: "content", Role: "video_vertical"},
	}})
	AttachBanner(group, banner)
	if _, ok := banner["ad_group_id"]; ok {
		t.Fatal("nested banner must not have ad_group_id")
	}
	list, _ := group["banners"].([]any)
	if len(list) != 1 {
		t.Fatalf("banners = %#v", group["banners"])
	}
}

func TestCommunityURLAppendsRefTags(t *testing.T) {
	got, err := CommunityURL(Settings{CommunityID: 42, RefTags: "?ref_source=manual&ref={{banner_id}}"})
	if err != nil {
		t.Fatal(err)
	}
	want := "https://vk.com/club42?ref_source=manual&ref={{banner_id}}"
	if got != want {
		t.Fatalf("CommunityURL() = %q, want %q", got, want)
	}
}

func TestCommunityURLKeepsCompleteManualURL(t *testing.T) {
	want := "https://vk.com/im?sel=-42&ref_source=manual&ref={{banner_id}}"
	got, err := CommunityURL(Settings{CommunityID: 42, RefTags: want})
	if err != nil {
		t.Fatal(err)
	}
	if got != want {
		t.Fatalf("CommunityURL() = %q, want %q", got, want)
	}
}

func TestSameDestinationURLAcceptsEncodedMacrosAndQueryOrder(t *testing.T) {
	want := "https://vk.com/club42?ref_source=manual&ref={{banner_id}}"
	got := "https://VK.COM/club42?ref=%7B%7Bbanner_id%7D%7D&ref_source=manual"
	if !sameDestinationURL(want, got) {
		t.Fatalf("expected equivalent URLs: %q and %q", want, got)
	}
}

func TestSameDestinationURLRejectsDefaultRef(t *testing.T) {
	want := "https://vk.com/club42?ref_source=manual&ref={{banner_id}}"
	got := "https://vk.com/club42?ref_source=vk_ads_yulya&ref={{banner_id}}"
	if sameDestinationURL(want, got) {
		t.Fatalf("expected different URLs: %q and %q", want, got)
	}
}

func TestCreateURLRejectsMismatchedRegisteredRef(t *testing.T) {
	start := time.Unix(1_700_000_000, 0)
	zaley := zaleyStub(t, start)
	defer zaley.Close()

	const manual = "https://vk.com/club42?ref_source=manual&ref={{banner_id}}"
	ads := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/api/v2/urls.json":
			var body struct {
				URL string `json:"url"`
			}
			_ = json.NewDecoder(r.Body).Decode(&body)
			if body.URL != manual {
				t.Fatalf("posted URL = %q", body.URL)
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"id": 9})
		case r.Method == http.MethodGet && r.URL.Path == "/api/v2/urls/9.json":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"id":  9,
				"url": "https://vk.com/club42?ref_source=vk_ads_yulya&ref={{banner_id}}",
			})
		default:
			http.NotFound(w, r)
		}
	}))
	defer ads.Close()

	clock := &clocks{}
	clock.set(start)
	svc := newService(t, zaley.URL, ads.URL, clock, 2*time.Minute)
	_, err := svc.CreateURL(context.Background(), manual)
	if err == nil || !strings.Contains(err.Error(), "registered") {
		t.Fatalf("CreateURL() error = %v", err)
	}
}
