package zaleycash

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestAccessToken(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v2/token" || r.Method != http.MethodPost {
			t.Fatalf("unexpected %s %s", r.Method, r.URL.Path)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer secret-key" {
			t.Fatalf("Authorization = %q", got)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"code":    200,
			"message": "OK",
			"response": map[string]any{
				"access_token": "zaley-tok",
				"expires_at":   1_700_000_000,
			},
		})
	}))
	defer srv.Close()

	tok, err := New(srv.URL, "secret-key", time.Second).AccessToken(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if tok.AccessToken != "zaley-tok" {
		t.Fatalf("token = %q", tok.AccessToken)
	}
	if !tok.ExpiresAt.Equal(time.Unix(1_700_000_000, 0)) {
		t.Fatalf("expires = %v", tok.ExpiresAt)
	}
}

func TestVKAdvertTokenSendsAccountName(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v2/vk_advert/token" {
			t.Fatalf("path = %s", r.URL.Path)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer zaley-tok" {
			t.Fatalf("Authorization = %q", got)
		}
		raw, _ := io.ReadAll(r.Body)
		var body map[string]string
		if err := json.Unmarshal(raw, &body); err != nil {
			t.Fatal(err)
		}
		if body["account_id"] != "Юлия тесты" {
			t.Fatalf("body = %s", raw)
		}
		if _, ok := body["id"]; ok {
			t.Fatalf("must not send numeric id: %s", raw)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"code":    200,
			"message": "OK",
			"response": map[string]any{
				"access_token": "vk-tok",
				"expires_at":   1_800_000_000,
			},
		})
	}))
	defer srv.Close()

	tok, err := New(srv.URL, "secret-key", time.Second).VKAdvertToken(
		context.Background(), "zaley-tok", "  Юлия тесты  ",
	)
	if err != nil {
		t.Fatal(err)
	}
	if tok.AccessToken != "vk-tok" {
		t.Fatalf("token = %q", tok.AccessToken)
	}
}

func TestVKAdvertTokenAcceptsCamelCase(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"code":    200,
			"message": "OK",
			"response": map[string]any{
				"accessToken": "vk-camel",
				"expiresAt":   1_800_000_000,
			},
		})
	}))
	defer srv.Close()

	tok, err := New(srv.URL, "secret-key", time.Second).VKAdvertToken(context.Background(), "zaley-tok", "Юлия тесты")
	if err != nil {
		t.Fatal(err)
	}
	if tok.AccessToken != "vk-camel" {
		t.Fatalf("token = %q", tok.AccessToken)
	}
	if !tok.ExpiresAt.Equal(time.Unix(1_800_000_000, 0)) {
		t.Fatalf("expires = %v", tok.ExpiresAt)
	}
}

func TestEnvelopeError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"code":    86,
			"message": "Several advertising accounts with that name",
		})
	}))
	defer srv.Close()

	_, err := New(srv.URL, "secret-key", time.Second).VKAdvertToken(context.Background(), "tok", "dup")
	if err == nil {
		t.Fatal("expected error")
	}
	httpErr, ok := err.(*HTTPError)
	if !ok || httpErr.Code != 86 {
		t.Fatalf("err = %v", err)
	}
	if !strings.Contains(err.Error(), "Several advertising accounts") {
		t.Fatalf("err = %v", err)
	}
}

func TestEmptyAccountName(t *testing.T) {
	_, err := New("http://example", "s", time.Second).VKAdvertToken(context.Background(), "tok", "  ")
	if err == nil {
		t.Fatal("expected error")
	}
}
