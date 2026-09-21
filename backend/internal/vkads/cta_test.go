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

func TestCTAIDsInRegistryReadsBothShapes(t *testing.T) {
	plain := []byte(`{"items":[{"name":"cta_community_vk","limits":{"values":["signUp","contactUs"]}},
		{"name":"title_40_vkads","limits":{"max_length":40}}]}`)
	got := ctaIDsInRegistry(plain, "cta_community_vk")
	if len(got) != 2 || got[0] != "signUp" || got[1] != "contactUs" {
		t.Fatalf("список значений = %v", got)
	}

	pairs := []byte(`{"items":[{"role":"cta_community_vk","values":[{"value":"signUp","name":"Вступить"}]}]}`)
	if got := ctaIDsInRegistry(pairs, "cta_community_vk"); len(got) != 1 || got[0] != "signUp" {
		t.Fatalf("пары значение-надпись = %v", got)
	}

	if got := ctaIDsInRegistry(plain, "cta_sites_full"); len(got) != 0 {
		t.Fatalf("чужая роль = %v", got)
	}
}
