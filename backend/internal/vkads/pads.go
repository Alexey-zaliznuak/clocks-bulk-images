package vkads

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"sort"
	"strconv"
	"strings"
	"time"
)

// PadNode is one branch of the VK Ads placements tree shown in the cabinet.
type PadNode struct {
	ID       string    `json:"id"`
	Name     string    `json:"name"`
	Pads     []int     `json:"pads,omitempty"`
	Children []PadNode `json:"children,omitempty"`
}

type rawPadNode struct {
	ID          any             `json:"id"`
	Name        string          `json:"name"`
	Description string          `json:"description"`
	Children    []rawPadNode    `json:"children"`
	Items       []rawPadNode    `json:"items"`
	Tree        json.RawMessage `json:"tree"`
	Pads        []int           `json:"pads"`
	PadID       int             `json:"pad_id"`
	Params      struct {
		Pads []int `json:"pads"`
	} `json:"params"`
}

// ListPlacementTree returns the cabinet placement tree, or a grouped list of
// package pads if the tree resource is empty.
func (s *Service) ListPlacementTree(ctx context.Context) ([]PadNode, error) {
	s.mu.Lock()
	if s.pads != nil && s.now().Sub(s.padsAt) < 30*time.Minute {
		out := append([]PadNode(nil), s.pads...)
		s.mu.Unlock()
		return out, nil
	}
	s.mu.Unlock()

	data, err := s.Get(ctx, "/api/v2/pads_trees.json?limit=50")
	if err != nil {
		return nil, err
	}
	trees := ParsePadsTrees(data)
	if len(CollectPadIDs(trees)) == 0 {
		listed, listErr := s.ListPackagePads(ctx, 0)
		if listErr != nil && len(trees) == 0 {
			return nil, listErr
		}
		if grouped := GroupPackagePads(listed); len(grouped) > 0 {
			trees = grouped
		}
	}
	s.mu.Lock()
	s.pads = trees
	s.padsAt = s.now()
	s.mu.Unlock()
	return append([]PadNode(nil), trees...), nil
}

// PadsTreeByID fetches a single tree. The cached listing is capped at 50, and
// the tree of a package can sit past that page.
func (s *Service) PadsTreeByID(ctx context.Context, id int64) ([]PadNode, error) {
	data, err := s.Get(ctx, fmt.Sprintf("/api/v2/pads_trees.json?_id=%d&limit=1", id))
	if err != nil {
		return nil, err
	}
	return ParsePadsTrees(data), nil
}

// PackagePlacements returns the pads tree of the package with its leaves
// named. The tree resource labels its branches only, so the placements arrive
// as bare ids — unusable in the form and unrecognisable as "the VK feed".
func (s *Service) PackagePlacements(ctx context.Context, pkg Package) []PadNode {
	tree := s.PackageTree(ctx, pkg)
	listed, err := s.ListPackagePads(ctx, pkg.ID)
	if err != nil {
		log.Printf("vkads: каталог площадок недоступен: %v", err)
		return tree
	}
	if len(tree) == 0 {
		return GroupPackagePads(listed)
	}
	return LabelPadTree(tree, PadLabels(listed))
}

// PackageTree returns the pads tree that targetings.pads of this package must
// belong to. Falling back to a wider set instead is what VK answers with
// "pad(s) that's not permitted in this pad tree".
func (s *Service) PackageTree(ctx context.Context, pkg Package) []PadNode {
	if trees, err := s.ListPlacementTree(ctx); err == nil {
		if tree := padsTreeForPackage(pkg, trees); len(tree) > 0 {
			return tree
		}
	}
	if pkg.PadsTreeID <= 0 {
		return nil
	}
	tree, err := s.PadsTreeByID(ctx, pkg.PadsTreeID)
	if err != nil {
		log.Printf("vkads: дерево площадок %d недоступно: %v", pkg.PadsTreeID, err)
		return nil
	}
	return tree
}

// ParsePadsTrees turns pads_trees.json into a checkbox tree.
func ParsePadsTrees(data []byte) []PadNode {
	var env struct {
		Items []rawPadNode `json:"items"`
	}
	if err := json.Unmarshal(data, &env); err == nil && len(env.Items) > 0 {
		return compactRoots(convertRoots(env.Items))
	}
	var items []rawPadNode
	if err := json.Unmarshal(data, &items); err == nil && len(items) > 0 {
		return compactRoots(convertRoots(items))
	}
	var one rawPadNode
	if err := json.Unmarshal(data, &one); err == nil && (one.Name != "" || len(one.Children) > 0 || len(one.Tree) > 0) {
		return compactRoots(convertRoots([]rawPadNode{one}))
	}
	return nil
}

func convertRoots(items []rawPadNode) []PadNode {
	out := make([]PadNode, 0, len(items))
	for _, item := range items {
		node := convertNode(item, false)
		if node.Name == "" && len(item.Tree) > 0 {
			node.Name = firstNonEmpty(item.Description, "Площадки")
		}
		if nested := parseEmbeddedTree(item.Tree); len(nested) > 0 {
			node.Children = append(node.Children, nested...)
		}
		if node.Name != "" || len(node.Children) > 0 || len(node.Pads) > 0 {
			out = append(out, node)
		}
	}
	return out
}

func parseEmbeddedTree(raw json.RawMessage) []PadNode {
	if len(raw) == 0 {
		return nil
	}
	var list []rawPadNode
	if err := json.Unmarshal(raw, &list); err == nil && len(list) > 0 {
		return convertTreeNodes(list)
	}
	var one rawPadNode
	if err := json.Unmarshal(raw, &one); err != nil {
		return nil
	}
	node := convertNode(one, true)
	if (node.ID == "root" || node.Name == "") && len(node.Children) > 0 && len(node.Pads) == 0 {
		return node.Children
	}
	if node.Name != "" || len(node.Children) > 0 || len(node.Pads) > 0 {
		return []PadNode{node}
	}
	return nil
}

func convertTreeNodes(items []rawPadNode) []PadNode {
	out := make([]PadNode, 0, len(items))
	for _, item := range items {
		converted := convertNode(item, true)
		if converted.Name != "" || len(converted.Children) > 0 || len(converted.Pads) > 0 {
			out = append(out, converted)
		}
	}
	return out
}

func convertNode(raw rawPadNode, leafIDsArePads bool) PadNode {
	pads := append([]int(nil), raw.Params.Pads...)
	pads = append(pads, raw.Pads...)
	if raw.PadID > 0 {
		pads = append(pads, raw.PadID)
	}
	rawChildren := raw.Children
	if len(rawChildren) == 0 {
		rawChildren = raw.Items
	}
	children := make([]PadNode, 0, len(rawChildren))
	for _, child := range rawChildren {
		converted := convertNode(child, leafIDsArePads)
		if converted.Name != "" || len(converted.Children) > 0 || len(converted.Pads) > 0 {
			children = append(children, converted)
		}
	}
	// Official PadsTree: a leaf's id is the pad id. The PadsTree resource id is
	// the tree id and must not be sent as targetings.pads.
	if leafIDsArePads && len(children) == 0 && len(pads) == 0 {
		if id := numericID(raw.ID); id > 0 {
			pads = []int{id}
		}
	}
	return PadNode{
		ID:       stringifyID(raw.ID),
		Name:     firstNonEmpty(raw.Name, raw.Description),
		Pads:     uniqueInts(pads),
		Children: children,
	}
}

// ResolvePads keeps only ids the package sells. Package.pads_tree_id is the
// tree that targetings.pads must belong to; other trees and the mixed
// packages_pads list are rejected as "not permitted in this pad tree".
// An empty selection means the VK feed, never "wherever VK feels like": an
// omitted targetings.pads lets the cabinet spend the budget on every placement
// of the package.
func ResolvePads(selected []int, pkg Package, trees []PadNode) []int {
	tree := padsTreeForPackage(pkg, trees)
	if out := ResolvePadsInTree(selected, pkg, tree); len(out) > 0 {
		return out
	}
	if len(tree) > 0 {
		return nil
	}
	// The package names a tree the listing did not carry. The feed of the
	// cabinet at large, narrowed to what the package sells, is the safest
	// guess left.
	return intersectPadIDs(PickVKFeedPadIDs(trees), allowedPads(pkg, nil))
}

// ResolvePadsInTree picks the placements inside the tree of the package, which
// is the only tree targetings.pads may name.
func ResolvePadsInTree(selected []int, pkg Package, tree []PadNode) []int {
	allowed := allowedPads(pkg, tree)
	if len(allowed) == 0 {
		return nil
	}
	if out := intersectPadIDs(selected, allowed); len(out) > 0 {
		return out
	}
	return intersectPadIDs(PickVKFeedPadIDs(tree), allowed)
}

// allowedPads narrows the package tree by what the package itself sells. Both
// halves are needed: the tree alone still holds placements this package does
// not offer, and the options alone may name placements of another tree, which
// VK answers with "pad(s) that's not permitted in this pad tree".
func allowedPads(pkg Package, tree []PadNode) map[int]struct{} {
	fromTree := CollectPadIDs(tree)
	fromPackage := PackagePadIDs(pkg)
	if len(fromTree) == 0 {
		return intSet(fromPackage)
	}
	if len(fromPackage) == 0 {
		return intSet(fromTree)
	}
	if both := intersectPadIDs(fromTree, intSet(fromPackage)); len(both) > 0 {
		return intSet(both)
	}
	return intSet(fromTree)
}

// PackagePadIDs lists the placements this package sells. The per-pad pattern
// map is the precise answer — it spells out which banner patterns run on which
// placement. options.targetings[pads].values is deliberately ignored: it is
// everything the pads targeting accepts cabinet-wide, and reading it as an
// allow-list is what sent 80 placements of other trees to VK.
func PackagePadIDs(pkg Package) []int {
	if byPad := ParsePackagePadPatterns(pkg.Options); len(byPad) > 0 {
		ids := make([]int, 0, len(byPad))
		for pad := range byPad {
			ids = append(ids, pad)
		}
		sort.Ints(ids)
		return ids
	}
	_, defaults := ParsePackagePadOptions(pkg.Options)
	return defaults
}

// ParsePackagePadOptions splits options.targetings[pads] into every placement
// the package allows and the subset the cabinet preselects.
func ParsePackagePadOptions(raw json.RawMessage) (values, defaults []int) {
	if len(raw) == 0 {
		return nil, nil
	}
	var opts struct {
		Targetings []struct {
			Name    string `json:"name"`
			Default []int  `json:"default"`
			Values  []int  `json:"values"`
		} `json:"targetings"`
	}
	if err := json.Unmarshal(raw, &opts); err != nil {
		return nil, nil
	}
	for _, t := range opts.Targetings {
		if t.Name != "pads" {
			continue
		}
		return uniqueInts(t.Values), uniqueInts(t.Default)
	}
	return nil, nil
}

// PlacementOptions is the placement picker for one campaign: the package that
// will carry it, the placements that package allows and the ones VK
// preselects.
type PlacementOptions struct {
	Package Package
	Trees   []PadNode
	// Default is the preselection for a fresh campaign: the VK feed.
	Default []int
}

// PlacementTreeForSettings returns the placements these settings can actually
// use. The cabinet lists dozens of trees for every package it sells, but a
// group may only target the tree of its own package — ResolvePads drops
// everything else at upload time, so offering it in the form is misleading.
func (s *Service) PlacementTreeForSettings(ctx context.Context, settings Settings) (*PlacementOptions, error) {
	packages, err := s.ListPackages(ctx)
	if err != nil {
		return nil, err
	}
	pkg := PickCommunityMessagePackage(packages, settings.Normalize().TargetAction)
	if pkg == nil {
		return nil, fmt.Errorf("vkads: нет пакета для сообщества / отправки сообщения")
	}
	scoped := s.PackagePlacements(ctx, *pkg)
	// The form must offer exactly what ResolvePads will accept, otherwise a
	// selection made here is dropped, or worse, rejected by VK on upload.
	if narrowed := FilterPadTree(scoped, allowedPads(*pkg, scoped)); len(narrowed) > 0 {
		scoped = narrowed
	}
	// Default is what the upload would pick on its own — the VK feed — so the
	// form starts out showing the choice it is actually going to make.
	defaults := ResolvePadsInTree(nil, *pkg, scoped)
	return &PlacementOptions{
		Package: *pkg,
		Trees:   PrunePadTree(scoped),
		Default: defaults,
	}, nil
}

// PadLabels maps placement ids to the human names of packages_pads.json. The
// pads tree itself only names its branches, so without this the leaves read as
// bare numbers — and the feed cannot be recognised by name either.
func PadLabels(pads []Pad) map[int]string {
	out := make(map[int]string, len(pads))
	for _, pad := range pads {
		if pad.ID <= 0 {
			continue
		}
		if label := firstNonEmpty(pad.Description, pad.Name); label != "" {
			out[pad.ID] = label
		}
	}
	return out
}

// LabelPadTree names the unnamed leaves of a tree from the placement catalogue.
func LabelPadTree(nodes []PadNode, labels map[int]string) []PadNode {
	if len(labels) == 0 {
		return nodes
	}
	out := make([]PadNode, 0, len(nodes))
	for _, node := range nodes {
		node.Children = LabelPadTree(node.Children, labels)
		if node.Name == "" && len(node.Pads) == 1 {
			node.Name = labels[node.Pads[0]]
		}
		out = append(out, node)
	}
	return out
}

// FilterPadTree keeps only the placements of allowed, dropping the branches
// left with nothing to offer.
func FilterPadTree(nodes []PadNode, allowed map[int]struct{}) []PadNode {
	out := make([]PadNode, 0, len(nodes))
	for _, node := range nodes {
		node.Pads = intersectPadIDs(node.Pads, allowed)
		node.Children = FilterPadTree(node.Children, allowed)
		if len(node.Pads) == 0 && len(node.Children) == 0 {
			continue
		}
		out = append(out, node)
	}
	return out
}

// PrunePadTree makes a cabinet tree readable. The raw resource is full of
// branches that carry no placement at all, of unnamed wrappers and of siblings
// repeating the same label, and each of those reaches the form as an empty or
// duplicated checkbox.
func PrunePadTree(nodes []PadNode) []PadNode {
	out := make([]PadNode, 0, len(nodes))
	at := map[string]int{}
	add := func(node PadNode) {
		if i, ok := at[node.Name]; ok {
			out[i].Pads = uniqueInts(append(out[i].Pads, node.Pads...))
			out[i].Children = PrunePadTree(append(out[i].Children, node.Children...))
			return
		}
		at[node.Name] = len(out)
		out = append(out, node)
	}
	for _, node := range nodes {
		node.Children = PrunePadTree(node.Children)
		if len(node.Pads) == 0 && len(node.Children) == 0 {
			continue
		}
		if node.Name == "" {
			if len(node.Children) > 0 {
				for _, child := range node.Children {
					add(child)
				}
				continue
			}
			node.Name = fmt.Sprintf("Площадка %d", node.Pads[0])
		}
		add(node)
	}
	return out
}

func padsTreeForPackage(pkg Package, trees []PadNode) []PadNode {
	if pkg.PadsTreeID <= 0 {
		return nil
	}
	id := strconv.FormatInt(pkg.PadsTreeID, 10)
	for _, tree := range trees {
		if tree.ID == id {
			return []PadNode{tree}
		}
	}
	return nil
}

func PickVKFeedPadIDs(nodes []PadNode) []int {
	var vk, feed []int
	var walk func(PadNode, string)
	walk = func(n PadNode, parent string) {
		blob := parent + " " + n.Name + " " + n.ID
		if len(n.Pads) > 0 {
			if containsFold(blob, "лента", "feed") && containsFold(blob, "vk", "вк", "вконтакте", "vkontakte") {
				vk = append(vk, n.Pads...)
			} else if containsFold(blob, "лента", "feed") {
				feed = append(feed, n.Pads...)
			}
		}
		for _, child := range n.Children {
			walk(child, blob)
		}
	}
	for _, node := range nodes {
		walk(node, "")
	}
	if len(vk) > 0 {
		return uniqueInts(vk)
	}
	return uniqueInts(feed)
}

func padIDs(pads []Pad) []int {
	out := make([]int, 0, len(pads))
	for _, pad := range pads {
		if pad.ID > 0 {
			out = append(out, pad.ID)
		}
	}
	return out
}

func orPadIDs(selected, fallback []int) []int {
	if len(selected) > 0 {
		return selected
	}
	return fallback
}

func intSet(ids []int) map[int]struct{} {
	out := make(map[int]struct{}, len(ids))
	for _, id := range ids {
		if id > 0 {
			out[id] = struct{}{}
		}
	}
	return out
}

func intersectPadIDs(want []int, allowed map[int]struct{}) []int {
	out := make([]int, 0, len(want))
	seen := map[int]struct{}{}
	for _, id := range want {
		if _, ok := allowed[id]; !ok {
			continue
		}
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		out = append(out, id)
	}
	return out
}

func numericID(value any) int {
	switch v := value.(type) {
	case float64:
		return int(v)
	case int:
		return v
	case int64:
		return int(v)
	case json.Number:
		n, err := v.Int64()
		if err != nil || n <= 0 {
			return 0
		}
		return int(n)
	case string:
		n, err := strconv.Atoi(strings.TrimSpace(v))
		if err != nil || n <= 0 {
			return 0
		}
		return n
	default:
		return 0
	}
}

func compactRoots(nodes []PadNode) []PadNode {
	if len(nodes) == 1 && (nodes[0].ID == "root" || nodes[0].Name == "") && len(nodes[0].Children) > 0 {
		return nodes[0].Children
	}
	return nodes
}

// GroupPackagePads builds a coarse tree when pads_trees is unavailable.
func GroupPackagePads(pads []Pad) []PadNode {
	order := []string{"ВКонтакте", "Одноклассники", "Почта", "Дзен", "Юла", "Другие"}
	groups := map[string][]PadNode{}
	for _, pad := range pads {
		if pad.ID <= 0 {
			continue
		}
		family := padFamily(pad)
		label := firstNonEmpty(pad.Description, pad.Name, fmt.Sprintf("Площадка %d", pad.ID))
		groups[family] = append(groups[family], PadNode{
			ID:   strconv.Itoa(pad.ID),
			Name: label,
			Pads: []int{pad.ID},
		})
	}
	var out []PadNode
	for _, name := range order {
		children := groups[name]
		if len(children) == 0 {
			continue
		}
		out = append(out, PadNode{ID: name, Name: name, Children: children})
	}
	return out
}

func padFamily(pad Pad) string {
	blob := strings.ToLower(pad.Name + " " + pad.Description)
	switch {
	case containsFold(blob, "вконтакте", "vkontakte", "vk_"):
		return "ВКонтакте"
	case containsFold(blob, "однокласс", "odkl", "ok.ru"):
		return "Одноклассники"
	case containsFold(blob, "почт", "mail"):
		return "Почта"
	case containsFold(blob, "дзен", "dzen", "zen"):
		return "Дзен"
	case containsFold(blob, "юла", "youla"):
		return "Юла"
	default:
		return "Другие"
	}
}

// CollectPadIDs walks the tree and returns every pad id.
func CollectPadIDs(nodes []PadNode) []int {
	var out []int
	var walk func(PadNode)
	walk = func(n PadNode) {
		out = append(out, n.Pads...)
		for _, child := range n.Children {
			walk(child)
		}
	}
	for _, node := range nodes {
		walk(node)
	}
	return uniqueInts(out)
}

func stringifyID(value any) string {
	switch v := value.(type) {
	case nil:
		return ""
	case string:
		return v
	case float64:
		return strconv.FormatInt(int64(v), 10)
	case json.Number:
		return v.String()
	default:
		return fmt.Sprint(v)
	}
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func uniqueInts(in []int) []int {
	seen := make(map[int]struct{}, len(in))
	out := make([]int, 0, len(in))
	for _, n := range in {
		if n <= 0 {
			continue
		}
		if _, ok := seen[n]; ok {
			continue
		}
		seen[n] = struct{}{}
		out = append(out, n)
	}
	return out
}
