package vkads

import "testing"

func TestBannerBodyCommunityFallback(t *testing.T) {
	body := BannerBody("Аркадий", 10, 9, 55, "RuTime | именные наручные часы", "Аркадий - имя", "contactUs", "video_vertical", nil)
	if body["name"] != "Аркадий" {
		t.Fatalf("header = %#v", body)
	}
	if body["ad_group_id"] != int64(10) {
		t.Fatalf("ad_group_id = %#v", body["ad_group_id"])
	}
	content, _ := body["content"].(map[string]any)
	if _, ok := content["video_vertical"].(map[string]any); !ok {
		t.Fatalf("content = %#v", content)
	}
	blocks, _ := body["textblocks"].(map[string]any)
	title, _ := blocks["title_40_vkads"].(map[string]any)
	if title["text"] != "RuTime | именные наручные часы" {
		t.Fatalf("title = %#v", title)
	}
	cta, _ := blocks["cta_community_vk"].(map[string]any)
	if cta["text"] != "contactUs" {
		t.Fatalf("cta = %#v", cta)
	}
}

func TestPickVideoBannerPattern(t *testing.T) {
	patterns := []BannerPattern{
		{ID: 1, Name: "image only", Format: []BannerSlot{
			{Field: "url", Role: "primary", Required: true},
			{Field: "content", Role: "image_600x600", Required: true},
		}},
		{ID: 2, Name: "community video", Description: "сообщество", Format: []BannerSlot{
			{Field: "url", Role: "primary", Required: true},
			{Field: "content", Role: "video_vertical", Required: true},
			{Field: "textblock", Role: "title_40_vkads", Required: true},
			{Field: "textblock", Role: "text_2000", Required: true},
			{Field: "textblock", Role: "cta_community_vk", Required: true},
		}},
	}
	got := PickVideoBannerPattern(patterns, "video_vertical")
	if got == nil || got.ID != 2 {
		t.Fatalf("got %#v", got)
	}
	body := BannerBody("Хохлов", 1, 2, 3, "Заголовок", "Текст", "contactUs", "video_vertical", got)
	content, _ := body["content"].(map[string]any)
	if _, ok := content["image_600x600"]; ok {
		t.Fatal("must not send required image we do not have")
	}
	if _, ok := content["video_vertical"]; !ok {
		t.Fatalf("content = %#v", content)
	}
}

func TestVideoRoleAndCTA(t *testing.T) {
	if VideoRole(1080, 1920) != "video_vertical" {
		t.Fatal(VideoRole(1080, 1920))
	}
	if VideoRole(1920, 1080) != "video_horizontal" {
		t.Fatal(VideoRole(1920, 1080))
	}
	if CommunityCTA("send_message") != "contactUs" {
		t.Fatal(CommunityCTA("send_message"))
	}
	if roleMaxLen("title_40_vkads") != 40 || roleMaxLen("text_2000") != 2000 {
		t.Fatal("role max")
	}
	if clipRunes("абвгде", 3) != "абв" {
		t.Fatal(clipRunes("абвгде", 3))
	}
}
