package vkads

import "testing"

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
