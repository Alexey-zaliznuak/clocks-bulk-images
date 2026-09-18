package vkads

import (
	"context"
	"encoding/json"
	"fmt"
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
		node := convertNode(item)
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
		return convertRoots(list)
	}
	var one rawPadNode
	if err := json.Unmarshal(raw, &one); err != nil {
		return nil
	}
	node := convertNode(one)
	if (node.ID == "root" || node.Name == "") && len(node.Children) > 0 && len(node.Pads) == 0 {
		return node.Children
	}
	if node.Name != "" || len(node.Children) > 0 || len(node.Pads) > 0 {
		return []PadNode{node}
	}
	return nil
}

func convertNode(raw rawPadNode) PadNode {
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
		converted := convertNode(child)
		if converted.Name != "" || len(converted.Children) > 0 || len(converted.Pads) > 0 {
			children = append(children, converted)
		}
	}
	return PadNode{
		ID:       stringifyID(raw.ID),
		Name:     firstNonEmpty(raw.Name, raw.Description),
		Pads:     uniqueInts(pads),
		Children: children,
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
