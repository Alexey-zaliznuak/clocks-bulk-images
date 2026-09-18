package vkads

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"mime/multipart"
	"net/url"
	"path/filepath"
	"strconv"
	"strings"
	"unicode/utf8"
)

type BannerPattern struct {
	ID          int64        `json:"id"`
	Name        string       `json:"name"`
	Description string       `json:"description"`
	Format      BannerFormat `json:"format"`
}

type BannerFormat []BannerSlot

type BannerSlot struct {
	Field    string `json:"field"`
	Role     string `json:"role"`
	Required bool   `json:"required"`
}

func (f *BannerFormat) UnmarshalJSON(data []byte) error {
	data = bytes.TrimSpace(data)
	if len(data) == 0 || string(data) == "null" {
		return nil
	}
	if data[0] == '[' {
		var slots []BannerSlot
		if err := json.Unmarshal(data, &slots); err != nil {
			return err
		}
		*f = slots
		return nil
	}
	var obj map[string]json.RawMessage
	if err := json.Unmarshal(data, &obj); err != nil {
		return err
	}
	var slots []BannerSlot
	for key, raw := range obj {
		var list []BannerSlot
		if json.Unmarshal(raw, &list) == nil && len(list) > 0 {
			for i := range list {
				if list[i].Field == "" {
					list[i].Field = key
				}
				if list[i].Role == "" {
					list[i].Role = key
				}
			}
			slots = append(slots, list...)
			continue
		}
		var one BannerSlot
		if json.Unmarshal(raw, &one) == nil && (one.Role != "" || one.Field != "") {
			if one.Field == "" {
				one.Field = key
			}
			if one.Role == "" {
				one.Role = key
			}
			slots = append(slots, one)
		}
	}
	*f = slots
	return nil
}

func isPatternKey(key string) bool {
	switch strings.ToLower(strings.ReplaceAll(key, "-", "_")) {
	case "patterns", "banner_patterns", "pattern_ids", "pattern":
		return true
	default:
		return false
	}
}

func collectPatternIDs(v any) []int64 {
	switch t := v.(type) {
	case []any:
		var ids []int64
		for _, item := range t {
			ids = append(ids, collectPatternIDs(item)...)
		}
		return ids
	case float64:
		if t > 0 {
			return []int64{int64(t)}
		}
	case json.Number:
		if n, err := t.Int64(); err == nil && n > 0 {
			return []int64{n}
		}
	case string:
		if n, err := strconv.ParseInt(strings.TrimSpace(t), 10, 64); err == nil && n > 0 {
			return []int64{n}
		}
	case map[string]any:
		if id := positiveInt64(t["id"]); id > 0 {
			return []int64{id}
		}
		var ids []int64
		for _, key := range []string{"values", "defaults", "ids", "items", "patterns", "banner_patterns"} {
			if child, ok := t[key]; ok {
				ids = append(ids, collectPatternIDs(child)...)
			}
		}
		if keys := numericMapKeys(t); len(keys) > 0 && len(keys) >= (len(t)+1)/2 {
			ids = append(ids, keys...)
		}
		return ids
	}
	return nil
}

func positiveInt64(v any) int64 {
	switch n := v.(type) {
	case float64:
		if n > 0 {
			return int64(n)
		}
	case json.Number:
		if id, err := n.Int64(); err == nil && id > 0 {
			return id
		}
	case int64:
		if n > 0 {
			return n
		}
	case int:
		if n > 0 {
			return int64(n)
		}
	case string:
		if id, err := strconv.ParseInt(strings.TrimSpace(n), 10, 64); err == nil && id > 0 {
			return id
		}
	}
	return 0
}

func ParsePackagePatternIDs(raw json.RawMessage) []int64 {
	if len(bytes.TrimSpace(raw)) == 0 {
		return nil
	}
	var root any
	if err := json.Unmarshal(raw, &root); err != nil {
		return nil
	}
	var ids []int64
	var walk func(any, string)
	walk = func(v any, key string) {
		if isPatternKey(key) {
			ids = append(ids, collectPatternIDs(v)...)
			return
		}
		switch t := v.(type) {
		case map[string]any:
			for k, child := range t {
				walk(child, k)
			}
		case []any:
			for _, child := range t {
				walk(child, key)
			}
		}
	}
	walk(root, "")
	if len(ids) == 0 {
		if arr, ok := root.([]any); ok {
			ids = collectPatternIDs(arr)
		}
	}
	return uniqueInt64s(ids)
}

func PackageAllowedPatternIDs(pkg Package) []int64 {
	ids := append([]int64{}, pkg.PatternIDs...)
	ids = append(ids, ParsePackagePatternIDs(pkg.Options)...)
	ids = append(ids, ParsePackagePatternIDs(pkg.Format)...)
	ids = append(ids, numericMapKeysRaw(pkg.Format)...)
	return uniqueInt64s(ids)
}

// ParsePackagePadPatterns reads the per-pad allow-list the cabinet keeps in
// options.targetings[pads].patterns: [{"pad":"1265106","patterns":[{"id":486}]}].
func ParsePackagePadPatterns(raw json.RawMessage) map[int][]int64 {
	if len(bytes.TrimSpace(raw)) == 0 {
		return nil
	}
	var root any
	if err := json.Unmarshal(raw, &root); err != nil {
		return nil
	}
	out := map[int][]int64{}
	var walk func(any)
	walk = func(v any) {
		switch t := v.(type) {
		case map[string]any:
			pad := positiveInt64(t["pad"])
			if pad > 0 {
				if ids := collectPatternIDs(t["patterns"]); len(ids) > 0 {
					out[int(pad)] = uniqueInt64s(append(out[int(pad)], ids...))
					return
				}
			}
			for _, child := range t {
				walk(child)
			}
		case []any:
			for _, child := range t {
				walk(child)
			}
		}
	}
	walk(root)
	if len(out) == 0 {
		return nil
	}
	return out
}

// PackagePatternIDsForPads narrows the package allow-list to the pads the
// campaign actually targets; VK rejects a banner whose pattern is not offered
// on those placements.
func PackagePatternIDsForPads(pkg Package, pads []int) []int64 {
	byPad := ParsePackagePadPatterns(pkg.Options)
	if len(byPad) == 0 {
		return PackageAllowedPatternIDs(pkg)
	}
	var ids []int64
	for _, pad := range pads {
		ids = append(ids, byPad[pad]...)
	}
	if len(ids) == 0 {
		for _, list := range byPad {
			ids = append(ids, list...)
		}
	}
	return uniqueInt64s(ids)
}

func numericMapKeys(m map[string]any) []int64 {
	var ids []int64
	for k := range m {
		if id := positiveInt64(k); id > 0 {
			ids = append(ids, id)
		}
	}
	return ids
}

func numericMapKeysRaw(raw json.RawMessage) []int64 {
	if len(bytes.TrimSpace(raw)) == 0 {
		return nil
	}
	var obj map[string]json.RawMessage
	if json.Unmarshal(raw, &obj) != nil || len(obj) == 0 {
		return nil
	}
	var ids []int64
	for k := range obj {
		if id := positiveInt64(k); id > 0 {
			ids = append(ids, id)
		}
	}
	if len(ids) == 0 || len(ids) < (len(obj)+1)/2 {
		return nil
	}
	return ids
}

func parsePadPatternIDs(raw json.RawMessage) []int64 {
	ids := ParsePackagePatternIDs(raw)
	if len(ids) > 0 {
		return ids
	}
	var s string
	if json.Unmarshal(raw, &s) != nil || strings.TrimSpace(s) == "" {
		return nil
	}
	for _, part := range strings.FieldsFunc(s, func(r rune) bool {
		return r == ',' || r == ';' || r == '|' || r == ' '
	}) {
		if id := positiveInt64(part); id > 0 {
			ids = append(ids, id)
		}
	}
	return uniqueInt64s(ids)
}

func uniqueInt64s(in []int64) []int64 {
	seen := map[int64]struct{}{}
	out := make([]int64, 0, len(in))
	for _, id := range in {
		if id <= 0 {
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

func (s *Service) ListBannerPatterns(ctx context.Context) ([]BannerPattern, error) {
	s.mu.Lock()
	if s.patterns != nil && s.now().Sub(s.patternsAt) < catalogTTL {
		out := append([]BannerPattern(nil), s.patterns...)
		s.mu.Unlock()
		return out, nil
	}
	s.mu.Unlock()

	var all []BannerPattern
	seen := map[int64]struct{}{}
	for pages, offset := 0, 0; ; pages++ {
		env, err := s.getList(ctx, "/api/v2/banner_patterns.json", offset, listPageSize)
		if err != nil {
			if len(all) > 0 {
				break
			}
			return nil, err
		}
		var page []BannerPattern
		if len(env.Items) > 0 {
			if err := json.Unmarshal(env.Items, &page); err != nil {
				return nil, fmt.Errorf("vkads banner_patterns: %w", err)
			}
		}
		added := 0
		for _, item := range page {
			if item.ID == 0 {
				continue
			}
			if _, ok := seen[item.ID]; ok {
				continue
			}
			seen[item.ID] = struct{}{}
			all = append(all, item)
			added++
		}
		if stopAfterPage(len(page), listPageSize, added, len(all), env.Count, pages+1, 20) {
			break
		}
		offset += len(page)
	}
	s.mu.Lock()
	s.patterns = all
	s.patternsAt = s.now()
	s.mu.Unlock()
	return append([]BannerPattern(nil), all...), nil
}

func (s *Service) ListPackagePatterns(ctx context.Context, pkg Package, pads []int) ([]BannerPattern, error) {
	ids := PackagePatternIDsForPads(pkg, pads)
	var fetchedPath string
	var fetchedBody []byte
	if len(ids) == 0 && pkg.ID != 0 {
		one, path, data, err := s.fetchPackage(ctx, pkg.ID)
		fetchedPath, fetchedBody = path, data
		if err == nil {
			pkg = one
			ids = PackagePatternIDsForPads(pkg, pads)
		}
	}
	if len(ids) == 0 {
		ids = s.patternIDsFromPads(ctx)
	}
	if len(ids) > 0 {
		out := withoutCatchAll(s.fetchPatternsByIDs(ctx, ids))
		if len(out) == 0 {
			all, err := s.ListBannerPatterns(ctx)
			if err != nil {
				return nil, err
			}
			want := map[int64]struct{}{}
			for _, id := range ids {
				want[id] = struct{}{}
			}
			for _, p := range all {
				if _, ok := want[p.ID]; ok {
					out = append(out, p)
				}
			}
			out = withoutCatchAll(out)
		}
		if len(out) > 0 {
			log.Printf("vkads package %d: %d разрешённых паттернов %v", pkg.ID, len(out), ids)
			return out, nil
		}
	}
	if len(fetchedBody) == 0 && pkg.ID != 0 {
		one, path, data, err := s.fetchPackage(ctx, pkg.ID)
		fetchedPath, fetchedBody = path, data
		if err == nil && one.ID != 0 {
			pkg = one
		}
	}
	if fetchedPath == "" {
		fetchedPath = packageFetchPath(pkg.ID)
	}
	logVKExchange("GET", fetchedPath, "<empty>", 200, string(orBytes(fetchedBody, pkg.Options, pkg.Format)), fmt.Errorf("пакет %d без паттернов", pkg.ID))
	log.Printf("vkads package %d dump pads=%v pads_tree_id=%d parsed_ids=%v\noptions: %s",
		pkg.ID, pads, pkg.PadsTreeID, ids, orJSON(pkg.Options))
	return nil, fmt.Errorf("vkads: пакет %d не задаёт паттерны объявлений", pkg.ID)
}

func orBytes(parts ...[]byte) []byte {
	for _, p := range parts {
		if len(bytes.TrimSpace(p)) > 0 {
			return p
		}
	}
	return nil
}

func orJSON(raw json.RawMessage) string {
	if len(bytes.TrimSpace(raw)) == 0 {
		return "<empty>"
	}
	return string(raw)
}

func packageFetchPath(id int64) string {
	return fmt.Sprintf("/api/v2/packages.json?fields=id,name,options,format,banner_format_id,pads_tree_id&_id=%d&limit=1", id)
}

func (s *Service) fetchPackage(ctx context.Context, id int64) (Package, string, []byte, error) {
	path := packageFetchPath(id)
	data, err := s.Get(ctx, path)
	if err != nil {
		return Package{}, path, nil, err
	}
	pkg := decodePackage(data)
	if pkg.ID == 0 {
		return Package{}, path, data, fmt.Errorf("vkads: пакет %d не найден", id)
	}
	return pkg, path, data, nil
}

func decodePackage(data []byte) Package {
	var one Package
	if json.Unmarshal(data, &one) == nil && one.ID != 0 {
		return one
	}
	var env struct {
		Items []Package `json:"items"`
	}
	if json.Unmarshal(data, &env) == nil && len(env.Items) > 0 {
		return env.Items[0]
	}
	return Package{}
}

func (s *Service) patternIDsFromPads(ctx context.Context) []int64 {
	pads, err := s.ListPackagePads(ctx, 0)
	if err != nil {
		return nil
	}
	var ids []int64
	for _, pad := range pads {
		ids = append(ids, parsePadPatternIDs(pad.Patterns)...)
	}
	return uniqueInt64s(ids)
}

func withoutCatchAll(in []BannerPattern) []BannerPattern {
	out := make([]BannerPattern, 0, len(in))
	for _, p := range in {
		if !isCatchAllPattern(p) {
			out = append(out, p)
		}
	}
	return out
}

func (s *Service) fetchPatternsByIDs(ctx context.Context, ids []int64) []BannerPattern {
	if len(ids) == 0 {
		return nil
	}
	parts := make([]string, 0, len(ids))
	for _, id := range ids {
		parts = append(parts, strconv.FormatInt(id, 10))
	}
	return s.fetchPatternsByQuery(ctx, url.Values{
		"_id__in": {strings.Join(parts, ",")},
		"fields":  {"id,name,description,format"},
	})
}

func (s *Service) fetchPatternsByQuery(ctx context.Context, extra url.Values) []BannerPattern {
	env, err := s.getListQuery(ctx, "/api/v2/banner_patterns.json", 0, listPageSize, extra)
	if err != nil || len(env.Items) == 0 {
		return nil
	}
	var page []BannerPattern
	if err := json.Unmarshal(env.Items, &page); err != nil {
		return nil
	}
	return page
}

func (s *Service) UploadVideo(ctx context.Context, filename string, r io.Reader, width, height int) (int64, error) {
	if width <= 0 {
		width = 1080
	}
	if height <= 0 {
		height = 1920
	}
	var buf bytes.Buffer
	w := multipart.NewWriter(&buf)
	part, err := w.CreateFormFile("file", filepath.Base(filename))
	if err != nil {
		return 0, err
	}
	if _, err := io.Copy(part, r); err != nil {
		return 0, err
	}
	meta, _ := json.Marshal(map[string]int{"width": width, "height": height})
	if err := w.WriteField("data", string(meta)); err != nil {
		return 0, err
	}
	if err := w.Close(); err != nil {
		return 0, err
	}
	data, err := s.PostMultipart(ctx, "/api/v2/content/video.json", w.FormDataContentType(), buf.Bytes())
	if err != nil {
		return 0, err
	}
	var out struct {
		ID int64 `json:"id"`
	}
	if err := json.Unmarshal(data, &out); err != nil || out.ID == 0 {
		return 0, fmt.Errorf("vkads content/video: unexpected %s", truncate(data, 300))
	}
	return out.ID, nil
}

func (s *Service) UploadStatic(ctx context.Context, filename string, r io.Reader, width, height int) (int64, error) {
	if width <= 0 {
		width = 1080
	}
	if height <= 0 {
		height = 1080
	}
	var buf bytes.Buffer
	w := multipart.NewWriter(&buf)
	part, err := w.CreateFormFile("file", filepath.Base(filename))
	if err != nil {
		return 0, err
	}
	if _, err := io.Copy(part, r); err != nil {
		return 0, err
	}
	meta, _ := json.Marshal(map[string]int{"width": width, "height": height})
	if err := w.WriteField("data", string(meta)); err != nil {
		return 0, err
	}
	if err := w.Close(); err != nil {
		return 0, err
	}
	data, err := s.PostMultipart(ctx, "/api/v2/content/static.json", w.FormDataContentType(), buf.Bytes())
	if err != nil {
		return 0, err
	}
	var out struct {
		ID int64 `json:"id"`
	}
	if err := json.Unmarshal(data, &out); err != nil || out.ID == 0 {
		return 0, fmt.Errorf("vkads content/static: unexpected %s", truncate(data, 300))
	}
	return out.ID, nil
}

func AttachBanner(group, banner map[string]any) map[string]any {
	delete(banner, "ad_group_id")
	group["banners"] = []any{banner}
	return group
}

func BannerBody(name string, groupID, urlID, videoID, imageID int64, title, text, cta string, pattern *BannerPattern) map[string]any {
	body := map[string]any{
		"name":   name,
		"status": "active",
	}
	if groupID > 0 {
		body["ad_group_id"] = groupID
	}
	urls := map[string]any{}
	content := map[string]any{}
	blocks := map[string]any{}
	var slots []BannerSlot
	// The pattern is not a banner field: VK infers it from the content,
	// textblock and url roles, then checks it against the package allow-list.
	if pattern != nil {
		slots = pattern.Format
	}
	for _, slot := range slots {
		switch slot.Field {
		case "url":
			urls[slot.Role] = map[string]any{"id": urlID}
		case "content":
			if strings.Contains(slot.Role, "video") {
				if videoID > 0 {
					content[slot.Role] = map[string]any{"id": videoID}
				}
			} else if imageID > 0 {
				content[slot.Role] = map[string]any{"id": imageID}
			}
		case "textblock":
			role := slot.Role
			switch {
			case strings.HasPrefix(role, "title"):
				blocks[role] = map[string]any{"text": clipRunes(title, roleMaxLen(role))}
			case strings.HasPrefix(role, "text"):
				blocks[role] = map[string]any{"text": clipRunes(text, roleMaxLen(role))}
			case strings.HasPrefix(role, "cta"):
				blocks[role] = map[string]any{"text": cta}
			case strings.HasPrefix(role, "about"):
				blocks[role] = map[string]any{"text": clipRunes(title, roleMaxLen(role))}
			}
		}
	}
	if len(urls) > 0 {
		body["urls"] = urls
	}
	if len(content) > 0 {
		body["content"] = content
	}
	if len(blocks) > 0 {
		body["textblocks"] = blocks
	}
	return body
}

func PickPackageBannerPattern(patterns []BannerPattern, videoRole string, haveImage bool) *BannerPattern {
	var best *BannerPattern
	bestScore := -1
	for i := range patterns {
		if isCatchAllPattern(patterns[i]) {
			continue
		}
		p := &patterns[i]
		score := packagePatternScore(*p, videoRole, haveImage)
		if score < 0 {
			continue
		}
		if score > bestScore {
			bestScore = score
			best = p
		}
	}
	if best != nil {
		return best
	}
	for i := range patterns {
		if isCatchAllPattern(patterns[i]) {
			continue
		}
		return &patterns[i]
	}
	return nil
}

func isCatchAllPattern(p BannerPattern) bool {
	name := strings.ToLower(p.Name)
	return strings.Contains(name, "all_pattern") || strings.Contains(name, "all_patters")
}

func packagePatternScore(p BannerPattern, videoRole string, haveImage bool) int {
	var hasURL, hasVideo bool
	score := 0
	blob := p.Name + " " + p.Description
	if containsFold(blob, "видео", "video") {
		score += 2
	}
	for _, slot := range p.Format {
		switch slot.Field {
		case "url":
			hasURL = true
		case "content":
			if strings.Contains(slot.Role, "video") {
				hasVideo = true
				if slot.Role == videoRole || strings.Contains(slot.Role, aspectHint(videoRole)) {
					score += 4
				}
				score += 2
			} else if slot.Required && !haveImage {
				return -1
			}
		}
	}
	if !hasURL {
		return -1
	}
	if hasVideo {
		score += 3
	}
	return score
}

func aspectHint(videoRole string) string {
	switch videoRole {
	case "video_vertical":
		return "vertical"
	case "video_horizontal":
		return "horizontal"
	default:
		return "square"
	}
}

func PatternNeedsImage(p *BannerPattern) bool {
	if p == nil {
		return false
	}
	for _, slot := range p.Format {
		if slot.Field == "content" && !strings.Contains(slot.Role, "video") {
			return true
		}
	}
	return false
}

func VideoRole(width, height int) string {
	if width <= 0 || height <= 0 {
		return "video_vertical"
	}
	ratio := float64(width) / float64(height)
	switch {
	case ratio > 1.3:
		return "video_horizontal"
	case ratio < 0.75:
		return "video_vertical"
	default:
		return "video_square"
	}
}

func CommunityCTA(targetAction string) string {
	if containsFold(targetAction, "join", "вступ") {
		return "signUp"
	}
	return "contactUs"
}

func clipRunes(s string, n int) string {
	if n <= 0 || utf8.RuneCountInString(s) <= n {
		return s
	}
	return string([]rune(s)[:n])
}

func roleMaxLen(role string) int {
	n := 0
	for _, r := range role {
		if r >= '0' && r <= '9' {
			n = n*10 + int(r-'0')
		} else if n > 0 {
			break
		}
	}
	if n == 0 {
		return 2000
	}
	return n
}
