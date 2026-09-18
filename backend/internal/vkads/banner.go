package vkads

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"path/filepath"
	"strings"
	"unicode/utf8"
)

type BannerPattern struct {
	ID          int64         `json:"id"`
	Name        string        `json:"name"`
	Description string        `json:"description"`
	Format      []BannerSlot  `json:"format"`
}

type BannerSlot struct {
	Field    string `json:"field"`
	Role     string `json:"role"`
	Required bool   `json:"required"`
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
		if stopAfterPage(len(page), listPageSize, added, len(all), env.Count, pages+1, 8) {
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

func (s *Service) CreateBanner(ctx context.Context, body map[string]any) (int64, error) {
	data, err := s.Post(ctx, "/api/v2/banners.json", body)
	if err != nil {
		return 0, err
	}
	var out struct {
		ID int64 `json:"id"`
	}
	if err := json.Unmarshal(data, &out); err != nil || out.ID == 0 {
		return 0, fmt.Errorf("vkads banner: unexpected %s", truncate(data, 300))
	}
	return out.ID, nil
}

func BannerBody(name string, groupID, urlID, contentID int64, title, text, cta, videoRole string, pattern *BannerPattern) map[string]any {
	body := map[string]any{
		"name":        name,
		"status":      "active",
		"ad_group_id": groupID,
	}
	urls := map[string]any{}
	content := map[string]any{}
	blocks := map[string]any{}
	slots := communityBannerSlots(videoRole)
	if pattern != nil {
		slots = pattern.Format
	}
	for _, slot := range slots {
		switch slot.Field {
		case "url":
			urls[slot.Role] = map[string]any{"id": urlID}
		case "content":
			if strings.Contains(slot.Role, "video") {
				content[slot.Role] = map[string]any{"id": contentID}
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

func communityBannerSlots(videoRole string) []BannerSlot {
	if videoRole == "" {
		videoRole = "video_horizontal"
	}
	return []BannerSlot{
		{Field: "url", Role: "primary", Required: true},
		{Field: "content", Role: videoRole, Required: true},
		{Field: "textblock", Role: "title_40_vkads", Required: true},
		{Field: "textblock", Role: "text_2000", Required: true},
		{Field: "textblock", Role: "cta_community_vk", Required: true},
	}
}

func PickVideoBannerPattern(patterns []BannerPattern, videoRole string) *BannerPattern {
	var best *BannerPattern
	bestScore := -1
	for i := range patterns {
		p := &patterns[i]
		score := patternScore(*p, videoRole)
		if score < 0 {
			continue
		}
		if score > bestScore {
			bestScore = score
			best = p
		}
	}
	return best
}

func patternScore(p BannerPattern, videoRole string) int {
	var hasURL, hasVideo, hasTitle, hasText, hasCTA bool
	score := 0
	blob := strings.ToLower(p.Name + " " + p.Description)
	if containsFold(blob, "сообществ", "community", "vkads", "соц") {
		score += 3
	}
	for _, slot := range p.Format {
		switch slot.Field {
		case "url":
			hasURL = true
		case "content":
			if strings.Contains(slot.Role, "video") {
				hasVideo = true
				if slot.Role == videoRole {
					score += 4
				}
				score++
			} else if slot.Required {
				return -1
			}
		case "textblock":
			switch {
			case strings.HasPrefix(slot.Role, "title"):
				hasTitle = true
			case strings.HasPrefix(slot.Role, "text"):
				hasText = true
			case strings.HasPrefix(slot.Role, "cta"):
				hasCTA = true
				if slot.Role == "cta_community_vk" {
					score += 3
				}
			}
		}
	}
	if !hasURL || !hasVideo || !hasTitle || !hasText {
		return -1
	}
	if hasCTA {
		score++
	}
	return score
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
