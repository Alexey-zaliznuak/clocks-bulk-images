package vkads

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"sync/atomic"
	"testing"
	"time"

	"named_clocks/backend/internal/zaleycash"
)

type clocks struct {
	now atomic.Int64
}

func (c *clocks) set(t time.Time) { c.now.Store(t.UnixNano()) }
func (c *clocks) Now() time.Time  { return time.Unix(0, c.now.Load()) }

func newService(t *testing.T, zaleyURL, adsURL string, clock *clocks, skew time.Duration) *Service {
	t.Helper()
	s := New(Config{
		Zaley:       zaleycash.New(zaleyURL, "secret", time.Second),
		AccountName: "Юлия тесты",
		AdsBaseURL:  adsURL,
		RefreshSkew: skew,
		HTTPTimeout: time.Second,
		Now:         clock.Now,
	})
	if s == nil {
		t.Fatal("New returned nil")
	}
	return s
}

func TestNewRequiresAccount(t *testing.T) {
	if New(Config{Zaley: zaleycash.New("http://x", "s", time.Second)}) != nil {
		t.Fatal("expected nil without account name")
	}
	if New(Config{AccountName: "Юлия тесты"}) != nil {
		t.Fatal("expected nil without Zaley client")
	}
}

func TestAccessTokenCachesUntilSkew(t *testing.T) {
	var zaleyCalls, vkCalls atomic.Int32
	start := time.Unix(1_700_000_000, 0)
	zaleyExp := start.Add(time.Hour).Unix()
	vkExp := start.Add(2 * time.Hour).Unix()

	zaley := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v2/token":
			zaleyCalls.Add(1)
			_ = json.NewEncoder(w).Encode(map[string]any{
				"code": 200, "message": "OK",
				"response": map[string]any{"access_token": "zaley-1", "expires_at": zaleyExp},
			})
		case "/api/v2/vk_advert/token":
			vkCalls.Add(1)
			raw, _ := io.ReadAll(r.Body)
			var body map[string]string
			_ = json.Unmarshal(raw, &body)
			if body["account_id"] != "Юлия тесты" {
				t.Errorf("account_id = %s", raw)
			}
			_ = json.NewEncoder(w).Encode(map[string]any{
				"code": 200, "message": "OK",
				"response": map[string]any{"access_token": "vk-1", "expires_at": vkExp},
			})
		default:
			t.Errorf("unexpected path %s", r.URL.Path)
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer zaley.Close()

	clock := &clocks{}
	clock.set(start)
	svc := newService(t, zaley.URL, "http://unused", clock, 2*time.Minute)

	tok, err := svc.AccessToken(context.Background())
	if err != nil || tok != "vk-1" {
		t.Fatalf("first token = %q, %v", tok, err)
	}
	tok, err = svc.AccessToken(context.Background())
	if err != nil || tok != "vk-1" {
		t.Fatalf("cached token = %q, %v", tok, err)
	}
	if zaleyCalls.Load() != 1 || vkCalls.Load() != 1 {
		t.Fatalf("calls zaley=%d vk=%d, want 1/1", zaleyCalls.Load(), vkCalls.Load())
	}

	clock.set(time.Unix(vkExp, 0).Add(-time.Minute))
	if _, err := svc.AccessToken(context.Background()); err != nil {
		t.Fatal(err)
	}
	if vkCalls.Load() != 2 {
		t.Fatalf("expected refresh near expiry, vk calls=%d", vkCalls.Load())
	}
}

func TestGetRetriesAfter401(t *testing.T) {
	start := time.Unix(1_700_000_000, 0)
	var vkSeq atomic.Int32
	zaley := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		exp := start.Add(time.Hour).Unix()
		switch r.URL.Path {
		case "/api/v2/token":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"code": 200, "message": "OK",
				"response": map[string]any{"access_token": "zaley", "expires_at": exp},
			})
		case "/api/v2/vk_advert/token":
			n := vkSeq.Add(1)
			_ = json.NewEncoder(w).Encode(map[string]any{
				"code": 200, "message": "OK",
				"response": map[string]any{
					"access_token": "vk-" + strconv.Itoa(int(n)),
					"expires_at":   exp,
				},
			})
		}
	}))
	defer zaley.Close()

	var seen []string
	ads := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v2/user.json" {
			t.Fatalf("path = %s", r.URL.Path)
		}
		tok := r.Header.Get("Authorization")
		seen = append(seen, tok)
		if tok == "Bearer vk-1" {
			w.WriteHeader(http.StatusUnauthorized)
			_, _ = w.Write([]byte(`{"error":"expired"}`))
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"id": 42, "username": "yulia-tests", "types": []string{"advert"},
		})
	}))
	defer ads.Close()

	clock := &clocks{}
	clock.set(start)
	svc := newService(t, zaley.URL, ads.URL, clock, 2*time.Minute)

	u, err := svc.CurrentUser(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if u.Username != "yulia-tests" {
		t.Fatalf("user = %+v", u)
	}
	if len(seen) != 2 || seen[0] != "Bearer vk-1" || seen[1] != "Bearer vk-2" {
		t.Fatalf("auth headers = %#v", seen)
	}
}
