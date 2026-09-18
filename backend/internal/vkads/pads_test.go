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

func TestParsePackagePadOptionsSkipsOtherTargetings(t *testing.T) {
	values, defaults := ParsePackagePadOptions([]byte(`{"targetings":[{"name":"geo","values":[1]},{"name":"pads","default":[102641,1265106],"values":[102641,1265106,111756]}]}`))
	if len(values) != 3 || len(defaults) != 2 || defaults[1] != 1265106 {
		t.Fatalf("values=%v defaults=%v", values, defaults)
	}
}

// cabinetVKTree mirrors the "ВКонтакте" branch of the live cabinet: the feed
// sits next to in-video, stories, mini apps and the sidebar.
func cabinetVKTree() []PadNode {
	return []PadNode{{ID: "27", Name: "Социальные сети и сервисы", Children: []PadNode{
		{ID: "Вконтакте_1", Name: "ВКонтакте", Children: []PadNode{
			{ID: "1265106_2", Name: "Лента", Pads: []int{1265106}},
			{ID: "1010345_3", Name: "В видео", Pads: []int{1010345}},
			{ID: "2243453_4", Name: "В историях", Pads: []int{2243453}},
			{ID: "2243456_5", Name: "В VK Mini Apps и играх с вознаграждением за просмотр (rewarded)", Pads: []int{2243456}},
			{ID: "1361696_6", Name: "В VK Mini Apps и играх перед загрузкой или при смене контента", Pads: []int{1361696}},
			{ID: "1985149_7", Name: "В VK Mini Apps и играх рядом с контентом", Pads: []int{1985149}},
			{ID: "1302973_8", Name: "Боковая колонка", Pads: []int{1302973}},
		}},
	}}}
}

func TestResolvePadsDefaultsToFeed(t *testing.T) {
	pkg := Package{ID: 3122, PadsTreeID: 27}
	got := ResolvePads(nil, pkg, cabinetVKTree())
	if len(got) != 1 || got[0] != 1265106 {
		t.Fatalf("empty selection must mean the VK feed, got %v", got)
	}
}

func TestResolvePadsKeepsExplicitSelection(t *testing.T) {
	pkg := Package{ID: 3122, PadsTreeID: 27}
	got := ResolvePads([]int{1010345, 999}, pkg, cabinetVKTree())
	if len(got) != 1 || got[0] != 1010345 {
		t.Fatalf("got %v", got)
	}
}

func TestResolvePadsFallsBackToCabinetFeedWhenTreeMissing(t *testing.T) {
	// The package tree is past the first page of pads_trees, so only the
	// per-pad pattern map tells us which placements the package sells.
	pkg := Package{
		ID:         3122,
		PadsTreeID: 999,
		Options:    []byte(`{"targetings":[{"name":"pads","patterns":[{"pad":"1265106","patterns":[{"id":486}]},{"pad":"1010345","patterns":[{"id":145}]}]}]}`),
	}
	got := ResolvePads(nil, pkg, cabinetVKTree())
	if len(got) != 1 || got[0] != 1265106 {
		t.Fatalf("got %v", got)
	}
}

func TestPackagePadIDsPrefersPatternMapOverValues(t *testing.T) {
	// values lists everything the pads targeting accepts cabinet-wide, so it
	// must lose to the per-pad map, which is specific to this package.
	pkg := Package{Options: []byte(`{"targetings":[{"name":"pads","values":[1265106,1010345,2263324,38277],"patterns":[{"pad":"1265106","patterns":[{"id":486}]},{"pad":"1010345","patterns":[{"id":145}]}]}]}`)}
	got := PackagePadIDs(pkg)
	if len(got) != 2 || got[0] != 1010345 || got[1] != 1265106 {
		t.Fatalf("got %v", got)
	}
}

func TestResolvePadsDropsPadsOutsideThePackage(t *testing.T) {
	// A selection made before the form was narrowed: most of these belong to
	// other packages and VK answers "not permitted in this pad tree".
	pkg := Package{
		ID:         3122,
		PadsTreeID: 27,
		Options:    []byte(`{"targetings":[{"name":"pads","patterns":[{"pad":"1265106","patterns":[{"id":486}]},{"pad":"1010345","patterns":[{"id":145}]}]}]}`),
	}
	got := ResolvePads([]int{2263324, 38277, 1265106, 2230567, 1302973}, pkg, cabinetVKTree())
	if len(got) != 1 || got[0] != 1265106 {
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

