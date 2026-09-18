package vkads

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
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
		switch t := v.(type) {
		case map[string]any:
			for k, child := range t {
				walk(child, k)
			}
		case []any:
			if key == "patterns" || key == "banner_patterns" || key == "pattern_ids" {
				for _, item := range t {
					switch n := item.(type) {
					case float64:
						if n > 0 {
							ids = append(ids, int64(n))
						}
					case json.Number:
						if v, err := n.Int64(); err == nil && v > 0 {
							ids = append(ids, v)
						}
					}
				}
				return
			}
			for _, child := range t {
				walk(child, key)
			}
		}
	}
	walk(root, "")
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

func (s *Service) ListPackagePatterns(ctx context.Context, pkg Package) ([]BannerPattern, error) {
	ids := pkg.PatternIDs
	if len(ids) == 0 {
		ids = ParsePackagePatternIDs(pkg.Options)
	}
	if len(ids) == 0 && pkg.ID != 0 {
		data, err := s.Get(ctx, fmt.Sprintf("/api/v2/packages/%d.json?fields=id,options", pkg.ID))
		if err == nil {
			var one Package
			if json.Unmarshal(data, &one) == nil {
				ids = ParsePackagePatternIDs(one.Options)
			}
		}
	}
	all, err := s.ListBannerPatterns(ctx)
	if err != nil {
		return nil, err
	}
	if len(ids) == 0 {
		return all, nil
	}
	want := map[int64]struct{}{}
	for _, id := range ids {
		want[id] = struct{}{}
	}
	var out []BannerPattern
	for _, p := range all {
		if _, ok := want[p.ID]; ok {
			out = append(out, p)
		}
	}
	if len(out) == 0 {
		out = s.fetchPatternsByIDs(ctx, ids)
	}
	return out, nil
}

func (s *Service) fetchPatternsByIDs(ctx context.Context, ids []int64) []BannerPattern {
	if len(ids) == 0 {
		return nil
	}
	parts := make([]string, 0, len(ids))
	for _, id := range ids {
		parts = append(parts, strconv.FormatInt(id, 10))
	}
	env, err := s.getListQuery(ctx, "/api/v2/banner_patterns.json", 0, listPageSize, url.Values{
		"_id__in": {strings.Join(parts, ",")},
	})
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
	if len(patterns) > 0 {
		return &patterns[0]
	}
	return nil
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
