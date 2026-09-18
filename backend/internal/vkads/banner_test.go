package vkads

import (
	"encoding/json"
	"testing"
)

func TestParsePackagePatternIDs(t *testing.T) {
	got := ParsePackagePatternIDs([]byte(`{"settings":{"banner":{"patterns":[486,422,525]}}}`))
	if len(got) != 3 || got[0] != 486 || got[2] != 525 {
		t.Fatalf("got %v", got)
	}
}

func TestBannerBodyUsesPackagePattern(t *testing.T) {
	pattern := &BannerPattern{ID: 486, Format: []BannerSlot{
		{Field: "url", Role: "primary", Required: true},
		{Field: "content", Role: "video_vertical", Required: true},
		{Field: "content", Role: "image_600x600", Required: true},
		{Field: "textblock", Role: "title_40_vkads", Required: true},
		{Field: "textblock", Role: "text_2000", Required: true},
		{Field: "textblock", Role: "cta_community_vk", Required: true},
	}}
	body := BannerBody("Аркадий", 0, 9, 55, 66, "RuTime | именные наручные часы", "Аркадий - имя", "contactUs", pattern)
	content, _ := body["content"].(map[string]any)
	if _, ok := content["video_vertical"].(map[string]any); !ok {
		t.Fatalf("video = %#v", content)
	}
	if _, ok := content["image_600x600"].(map[string]any); !ok {
		t.Fatalf("image = %#v", content)
	}
	if _, ok := body["ad_group_id"]; ok {
		t.Fatal("nested banner must omit ad_group_id")
	}
}

func TestPickPackageBannerPattern(t *testing.T) {
	patterns := []BannerPattern{
		{ID: 1, Name: "image only", Format: []BannerSlot{
			{Field: "url", Role: "primary", Required: true},
			{Field: "content", Role: "image_600x600", Required: true},
		}},
		{ID: 486, Name: "community video", Description: "видео", Format: []BannerSlot{
			{Field: "url", Role: "primary", Required: true},
			{Field: "content", Role: "video_vertical", Required: true},
			{Field: "textblock", Role: "title_40_vkads", Required: true},
		}},
	}
	got := PickPackageBannerPattern(patterns, "video_vertical", false)
	if got == nil || got.ID != 486 {
		t.Fatalf("got %#v", got)
	}
}

func TestBannerFormatObject(t *testing.T) {
	var p BannerPattern
	if err := json.Unmarshal([]byte(`{"id":486,"name":"x","format":{"url":[{"role":"primary","required":true}],"content":[{"role":"video_vertical","required":true}]}}`), &p); err != nil {
		t.Fatal(err)
	}
	if p.ID != 486 || len(p.Format) != 2 {
		t.Fatalf("%#v", p)
	}
}
