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

func TestParsePadsTreesOfficialLeafIDs(t *testing.T) {
	raw := []byte(`{
		"items": [{
			"id": 5,
			"tree": {"children": [{"id": 3417}, {"children": [{"id": 3420}]}]}
		}]
	}`)
	got := ParsePadsTrees(raw)
	ids := CollectPadIDs(got)
	if len(ids) != 2 || ids[0] != 3417 || ids[1] != 3420 {
		t.Fatalf("official leaf ids = %v trees=%#v", ids, got)
	}
	if len(got) != 1 || got[0].ID != "5" {
		t.Fatalf("tree id = %#v", got)
	}
}

func TestResolvePadsUsesPackageTreeOnly(t *testing.T) {
	trees := []PadNode{
		{
			ID:   "27",
			Name: "Соцсети",
			Children: []PadNode{
				{ID: "vk", Name: "ВКонтакте", Children: []PadNode{
					{Name: "Лента", Pads: []int{3417, 3420}},
					{Name: "Клипы", Pads: []int{9991}},
				}},
			},
		},
		{ID: "9", Name: "Другое дерево", Children: []PadNode{
			{Name: "Лента", Pads: []int{100}},
		}},
	}
	pkg := Package{ID: 3, PadsTreeID: 27}

	got := ResolvePads(nil, pkg, trees)
	if len(got) != 2 || got[0] != 3417 || got[1] != 3420 {
		t.Fatalf("default feed = %v", got)
	}

	got = ResolvePads([]int{100, 9991, 3417}, pkg, trees)
	if len(got) != 2 || got[0] != 9991 || got[1] != 3417 {
		t.Fatalf("filtered selection = %v", got)
	}

	got = ResolvePads([]int{100}, pkg, trees)
	if len(got) != 2 || got[0] != 3417 {
		t.Fatalf("foreign pads fall back to feed = %v", got)
	}

	if got := ResolvePads([]int{3417}, Package{PadsTreeID: 8}, trees); len(got) != 0 {
		t.Fatalf("unknown tree must not leak pads: %v", got)
	}
}

func TestParsePackageDefaultPads(t *testing.T) {
	got := ParsePackageDefaultPads([]byte(`{"targetings":[{"name":"geo"},{"name":"pads","default":[102641,1265106],"values":[102641,1265106,111756]}]}`))
	if len(got) != 2 || got[0] != 102641 || got[1] != 1265106 {
		t.Fatalf("got %v", got)
	}
}

func TestParsePackageDefaultPadsFallsBackToValues(t *testing.T) {
	got := ParsePackageDefaultPads([]byte(`{"targetings":[{"name":"pads","values":[111756]}]}`))
	if len(got) != 1 || got[0] != 111756 {
		t.Fatalf("got %v", got)
	}
}

func TestPrunePadTreeDropsEmptyAndMergesDuplicates(t *testing.T) {
	got := PrunePadTree([]PadNode{
		{ID: "1", Name: "Площадки", Children: []PadNode{
			{ID: "2", Name: "Desktop", Pads: []int{10}},
			{ID: "3", Name: "Mobile"},
			{ID: "4", Name: "Desktop", Pads: []int{11}},
			{ID: "5", Name: "", Children: []PadNode{{ID: "6", Name: "Лента", Pads: []int{12}}}},
		}},
		{ID: "7", Name: "Пусто"},
	})
	if len(got) != 1 || got[0].Name != "Площадки" {
		t.Fatalf("roots = %#v", got)
	}
	children := got[0].Children
	if len(children) != 2 {
		t.Fatalf("children = %#v", children)
	}
	if children[0].Name != "Desktop" || len(children[0].Pads) != 2 {
		t.Fatalf("merged desktop = %#v", children[0])
	}
	if children[1].Name != "Лента" {
		t.Fatalf("unnamed wrapper must be replaced by its child: %#v", children[1])
	}
}

func TestPrunePadTreeNamesBarePads(t *testing.T) {
	got := PrunePadTree([]PadNode{{ID: "9", Pads: []int{1265106}}})
	if len(got) != 1 || got[0].Name != "Площадка 1265106" {
		t.Fatalf("got %#v", got)
	}
}

func TestFilterPadTreeKeepsAllowedOnly(t *testing.T) {
	got := FilterPadTree([]PadNode{
		{Name: "VK", Children: []PadNode{
			{Name: "Лента", Pads: []int{1, 2}},
			{Name: "Клипы", Pads: []int{3}},
		}},
		{Name: "OK", Pads: []int{4}},
	}, intSet([]int{1, 3}))
	if len(got) != 1 || len(got[0].Children) != 2 {
		t.Fatalf("got %#v", got)
	}
	if len(got[0].Children[0].Pads) != 1 || got[0].Children[0].Pads[0] != 1 {
		t.Fatalf("feed = %#v", got[0].Children[0])
	}
}

func TestParsePackagePadOptionsSplitsValuesAndDefaults(t *testing.T) {
	values, defaults := ParsePackagePadOptions([]byte(`{"targetings":[{"name":"pads","default":[1],"values":[1,2,3]}]}`))
	if len(values) != 3 || len(defaults) != 1 || defaults[0] != 1 {
		t.Fatalf("values=%v defaults=%v", values, defaults)
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

