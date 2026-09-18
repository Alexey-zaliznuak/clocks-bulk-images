package vkads

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestParsePackagePatternIDs(t *testing.T) {
	got := ParsePackagePatternIDs([]byte(`{"settings":{"banner":{"patterns":[486,422,525]}}}`))
	if len(got) != 3 || got[0] != 486 || got[2] != 525 {
		t.Fatalf("got %v", got)
	}
}

func TestParsePackagePatternIDsFromValues(t *testing.T) {
	got := ParsePackagePatternIDs([]byte(`{"targetings":{"pads":{"values":[3417,5206]}},"settings":{"patterns":{"values":[486,422,525,527],"defaults":[486]}}}`))
	if len(got) != 4 || got[0] != 486 || got[3] != 527 {
		t.Fatalf("got %v", got)
	}
}

func TestParsePackagePatternIDsFromObjects(t *testing.T) {
	got := ParsePackagePatternIDs([]byte(`{"settings":{"banner":{"patterns":[{"id":486},{"id":422}]}}}`))
	if len(got) != 2 || got[0] != 486 || got[1] != 422 {
		t.Fatalf("got %v", got)
	}
}

func TestParsePackagePatternIDsFromIDMap(t *testing.T) {
	got := ParsePackagePatternIDs([]byte(`{"settings":{"patterns":{"values":{"486":{},"422":{},"525":{}}}}}`))
	want := map[int64]bool{486: true, 422: true, 525: true}
	if len(got) != 3 {
		t.Fatalf("got %v", got)
	}
	for _, id := range got {
		if !want[id] {
			t.Fatalf("unexpected %d in %v", id, got)
		}
	}
}

func TestHTTPErrorIncludesRequestAndResponse(t *testing.T) {
	err := &HTTPError{
		StatusCode: 400,
		Method:     "POST",
		Path:       "/api/v2/ad_groups.json",
		Request:    `{"name":"Аркадий"}`,
		Body:       `{"error":{"code":"validation_failed"}}`,
	}
	got := err.Error()
	if !strings.Contains(got, "request: {\"name\":\"Аркадий\"}") || !strings.Contains(got, "response: {\"error\"") {
		t.Fatalf("got %s", got)
	}
}

func TestRequestLogBodySkipsMultipart(t *testing.T) {
	if got := requestLogBody([]byte("abc"), "multipart/form-data"); got != "<multipart 3 bytes>" {
		t.Fatalf("got %s", got)
	}
}

const packageOptionsWithPadPatterns = `{"targetings":[{"name":"pads","default":[102641,1265106],
	"patterns":[
		{"pad":"102641","patterns":[{"id":400,"required":false},{"id":401,"required":false}]},
		{"pad":"1265106","patterns":[{"id":486,"required":false},{"id":525,"required":false}]}
	],
	"values":[102641,1265106]}],
	"settings":[{"name":"autobidding_mode","values":["max_goals"]}]}`

func TestParsePackagePadPatterns(t *testing.T) {
	got := ParsePackagePadPatterns([]byte(packageOptionsWithPadPatterns))
	if len(got) != 2 || len(got[102641]) != 2 || got[102641][0] != 400 {
		t.Fatalf("got %v", got)
	}
	if len(got[1265106]) != 2 || got[1265106][0] != 486 {
		t.Fatalf("got %v", got)
	}
}

func TestPackagePatternIDsForPadsNarrowsToSelectedPads(t *testing.T) {
	pkg := Package{ID: 3122, Options: []byte(packageOptionsWithPadPatterns)}
	got := PackagePatternIDsForPads(pkg, []int{1265106})
	if len(got) != 2 || got[0] != 486 || got[1] != 525 {
		t.Fatalf("got %v", got)
	}
}

func TestPackagePatternIDsForPadsFallsBackToAllPads(t *testing.T) {
	pkg := Package{ID: 3122, Options: []byte(packageOptionsWithPadPatterns)}
	got := PackagePatternIDsForPads(pkg, []int{999999})
	if len(got) != 4 {
		t.Fatalf("got %v", got)
	}
}

func TestPackageAllowedPatternIDsReadsPadPatterns(t *testing.T) {
	pkg := Package{ID: 3122, Options: []byte(packageOptionsWithPadPatterns)}
	got := PackageAllowedPatternIDs(pkg)
	if len(got) != 4 {
		t.Fatalf("got %v", got)
	}
}

func TestParsePadPatternIDs(t *testing.T) {
	got := parsePadPatternIDs([]byte(`"486,422,525"`))
	if len(got) != 3 || got[0] != 486 {
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
	if _, ok := body["patterns"]; ok {
		t.Fatal("banner has no patterns field, VK infers it from the roles")
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

func TestPickPackageBannerPatternSkipsCatchAll(t *testing.T) {
	patterns := []BannerPattern{
		{ID: 349, Name: "all_patters_type", Description: "видео", Format: []BannerSlot{
			{Field: "url", Role: "primary", Required: true},
			{Field: "content", Role: "video_vertical", Required: true},
		}},
		{ID: 486, Name: "community video", Format: []BannerSlot{
			{Field: "url", Role: "primary", Required: true},
			{Field: "content", Role: "video_vertical", Required: true},
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
