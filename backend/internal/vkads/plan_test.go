package vkads

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

// zaleyStub mints whatever token the service asks for, so a test can focus on
// what ads.vk.com receives.
func zaleyStub(t *testing.T, now time.Time) *httptest.Server {
	t.Helper()
	exp := now.Add(time.Hour).Unix()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"code": 200, "message": "OK",
			"response": map[string]any{"access_token": "tok", "expires_at": exp},
		})
	}))
}

func TestCreateAdPlanWithGroupsRetriesTheOtherKey(t *testing.T) {
	start := time.Unix(1_700_000_000, 0)
	zaley := zaleyStub(t, start)
	defer zaley.Close()

	var seen []string
	ads := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, _ := io.ReadAll(r.Body)
		var body map[string]any
		_ = json.Unmarshal(raw, &body)
		if _, ok := body["campaigns"]; ok {
			seen = append(seen, "campaigns")
			w.WriteHeader(http.StatusBadRequest)
			_, _ = w.Write([]byte(`{"error":{"fields":{"ad_groups":{"code":"required","message":"Empty value"}}}}`))
			return
		}
		seen = append(seen, "ad_groups")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"id":        31656159,
			"ad_groups": []any{map[string]any{"id": 155313486, "banners": []any{map[string]any{"id": 238306345}}}},
		})
	}))
	defer ads.Close()

	clock := &clocks{}
	clock.set(start)
	svc := newService(t, zaley.URL, ads.URL, clock, 2*time.Minute)

	plan := map[string]any{"name": "Тест"}
	id, created, err := svc.CreateAdPlanWithGroups(context.Background(), plan, []map[string]any{{"name": "Аркадий"}})
	if err != nil {
		t.Fatal(err)
	}
	if id != 31656159 || len(created) != 1 || created[0].ID != 155313486 {
		t.Fatalf("id=%d created=%#v", id, created)
	}
	if len(seen) != 2 || seen[0] != "campaigns" || seen[1] != "ad_groups" {
		t.Fatalf("keys tried = %v", seen)
	}
	if _, ok := plan["campaigns"]; ok {
		t.Fatal("the plan handed in must stay untouched")
	}
}

func TestCreateAdPlanWithGroupsKeepsUnrelatedError(t *testing.T) {
	start := time.Unix(1_700_000_000, 0)
	zaley := zaleyStub(t, start)
	defer zaley.Close()

	var calls int
	ads := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"error":{"fields":{"banners":{"code":"bad_items"}}}}`))
	}))
	defer ads.Close()

	clock := &clocks{}
	clock.set(start)
	svc := newService(t, zaley.URL, ads.URL, clock, 2*time.Minute)

	if _, _, err := svc.CreateAdPlanWithGroups(context.Background(), map[string]any{}, nil); err == nil {
		t.Fatal("expected the banner error to surface")
	}
	if calls != 1 {
		t.Fatalf("a rejected banner must not be retried, calls=%d", calls)
	}
}
