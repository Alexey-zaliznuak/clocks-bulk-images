package vkads

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestResolveCTAAcceptsIdentifierAndCaption(t *testing.T) {
	options := []CTAOption{{ID: "contactUs", Label: "Связаться"}, {ID: "signUp", Label: "Вступить"}}
	if got := ResolveCTA("signUp", "send_message", options); got != "signUp" {
		t.Fatalf("по идентификатору = %q", got)
	}
	if got := ResolveCTA("вступить", "send_message", options); got != "signUp" {
		t.Fatalf("по надписи = %q", got)
	}
}

// The ad text tells people to press "Узнать цену", so the caption has to find
// the price button whenever the cabinet sells one under its own name.
func TestResolveCTAFindsThePriceButtonByMeaning(t *testing.T) {
	options := []CTAOption{{ID: "contactUs", Label: "Связаться"}, {ID: "getPrice", Label: "Узнать цену"}}
	if got := ResolveCTA(DefaultBannerCTA, "send_message", options); got != "getPrice" {
		t.Fatalf("цена = %q", got)
	}
}

// Campaigns saved before the form offered a list still carry free text, and an
// unknown caption must not travel to VK as the button.
func TestResolveCTAFallsBackToTheAction(t *testing.T) {
	options := []CTAOption{{ID: "contactUs", Label: "Связаться"}, {ID: "signUp", Label: "Вступить"}}
	if got := ResolveCTA("Узнать цену", "send_message", options); got != "contactUs" {
		t.Fatalf("сообщение = %q", got)
	}
	if got := ResolveCTA("", "join_community", options); got != "signUp" {
		t.Fatalf("вступление = %q", got)
	}
}

// banner_fields caps a page at 50 rows and the buttons sit well past the first
// one, so the registry has to be paged through to the end.
func TestCTAOptionsPagesThroughTheRegistry(t *testing.T) {
	start := time.Unix(1_700_000_000, 0)
	zaley := zaleyStub(t, start)
	defer zaley.Close()

	var offsets []string
	ads := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		offsets = append(offsets, q.Get("offset"))
		if q.Get("limit") != "50" {
			w.WriteHeader(http.StatusBadRequest)
			_, _ = w.Write([]byte(`{"error":{"fields":{"limit":{"code":"max_value"}}}}`))
			return
		}
		items := make([]any, 0, 50)
		if q.Get("offset") == "0" {
			for i := 0; i < 50; i++ {
				items = append(items, map[string]any{"name": fmt.Sprintf("text_%d", i)})
			}
		} else {
			items = append(items, map[string]any{
				"name":   "cta_community_vk",
				"limits": map[string]any{"values": []string{"signUp", "contactUs"}},
			})
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"count": 51, "items": items})
	}))
	defer ads.Close()

	clock := &clocks{}
	clock.set(start)
	svc := newService(t, zaley.URL, ads.URL, clock, 2*time.Minute)

	got := svc.CTAOptions(context.Background(), CTARoleCommunity)
	if len(got) != 2 || got[0].ID != "signUp" || got[1].Label != "Связаться" {
		t.Fatalf("кнопки = %#v", got)
	}
	if strings.Join(offsets, ",") != "0,50" {
		t.Fatalf("страницы = %v", offsets)
	}

	// The second call must come from the cache rather than walk VK again.
	svc.CTAOptions(context.Background(), CTARoleCommunity)
	if strings.Join(offsets, ",") != "0,50" {
		t.Fatalf("реестр перечитан: %v", offsets)
	}
}

func TestCTACatalogInRegistryReadsBothShapes(t *testing.T) {
	plain := []byte(`{"items":[{"name":"cta_community_vk","limits":{"values":["signUp","contactUs"]}},
		{"name":"cta_sites_full","limits":{"values":["visitSite"]}},
		{"name":"title_40_vkads","limits":{"max_length":40}}]}`)
	catalog := ctaCatalogInRegistry(plain)
	got := catalog["cta_community_vk"]
	if len(got) != 2 || got[0] != "signUp" || got[1] != "contactUs" {
		t.Fatalf("список значений = %v", got)
	}
	if other := catalog["cta_sites_full"]; len(other) != 1 || other[0] != "visitSite" {
		t.Fatalf("вторая роль = %v", other)
	}
	if _, ok := catalog["title_40_vkads"]; ok {
		t.Fatalf("в каталог попала не кнопка: %v", catalog)
	}

	pairs := []byte(`{"items":[{"role":"cta_community_vk","values":[{"value":"signUp","name":"Вступить"}]}]}`)
	if got := ctaCatalogInRegistry(pairs)["cta_community_vk"]; len(got) != 1 || got[0] != "signUp" {
		t.Fatalf("пары значение-надпись = %v", got)
	}
}
