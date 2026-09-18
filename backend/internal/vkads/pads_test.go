package vkads

import "testing"

func TestParsePadsTreesCabinetShape(t *testing.T) {
	raw := []byte(`{
		"items": [{
			"id": 27,
			"name": "Социальные сети и сервисы",
			"tree": {
				"id": "root",
				"name": "",
				"children": [
					{
						"id": "Вконтакте_1",
						"name": "ВКонтакте",
						"children": [
							{"id": "Лента_vk", "name": "Лента", "params": {"pads": [3417, 3420]}},
							{"id": "Клипы_vk", "name": "Клипы", "params": {"pads": [9991]}}
						]
					},
					{"id": "Одноклассники_1", "name": "Одноклассники", "pads": [100]}
				]
			}
		}]
	}`)
	got := ParsePadsTrees(raw)
	if len(got) != 1 || got[0].Name != "Социальные сети и сервисы" {
		t.Fatalf("root = %#v", got)
	}
	if len(got[0].Children) != 2 || got[0].Children[0].ID != "Вконтакте_1" {
		t.Fatalf("children = %#v", got[0].Children)
	}
	vk := got[0].Children[0]
	if len(vk.Children) != 2 || vk.Children[0].Name != "Лента" {
		t.Fatalf("vk children = %#v", vk.Children)
	}
	ids := CollectPadIDs(got)
	if len(ids) != 4 || ids[0] != 3417 || ids[3] != 100 {
		t.Fatalf("ids = %v", ids)
	}
}

func TestGroupPackagePads(t *testing.T) {
	got := GroupPackagePads([]Pad{
		{ID: 1, Name: "vk_feed", Description: "Лента ВКонтакте"},
		{ID: 2, Name: "odkl_feed", Description: "Лента Одноклассники"},
		{ID: 3, Name: "other_pad"},
	})
	if len(got) != 3 || got[0].Name != "ВКонтакте" || got[1].Name != "Одноклассники" {
		t.Fatalf("groups = %#v", got)
	}
	if len(got[0].Children) != 1 || got[0].Children[0].Pads[0] != 1 {
		t.Fatalf("vk = %#v", got[0])
	}
}
