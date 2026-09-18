package vkads

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

func (s *Service) Post(ctx context.Context, path string, body any) ([]byte, error) {
	raw, err := json.Marshal(body)
	if err != nil {
		return nil, err
	}
	return s.do(ctx, http.MethodPost, path, raw)
}

const (
	listPageSize     = 50
	maxPackagePages  = 8
	maxPadPages      = 20
	maxSegmentPages  = 200
)

// stopAfterPage ends a VK list walk: short/empty page, no new ids, count
// reached, or a hard page cap. packages.json ignores huge offsets and keeps
// returning items — without this we walk past 60k and hit 429.
func stopAfterPage(pageLen, pageSize, added, total, count, pages, maxPages int) bool {
	if pageLen == 0 || added == 0 {
		return true
	}
	if pageLen < pageSize {
		return true
	}
	if count > 0 && total >= count {
		return true
	}
	return maxPages > 0 && pages >= maxPages
}

type listEnvelope struct {
	Count  int             `json:"count"`
	Items  json.RawMessage `json:"items"`
	Offset int             `json:"offset"`
	Limit  int             `json:"limit"`
}

func (s *Service) getList(ctx context.Context, path string, offset, limit int) (listEnvelope, error) {
	return s.getListQuery(ctx, path, offset, limit, nil)
}

func (s *Service) getListQuery(ctx context.Context, path string, offset, limit int, extra url.Values) (listEnvelope, error) {
	q := url.Values{}
	for key, values := range extra {
		for _, value := range values {
			q.Add(key, value)
		}
	}
	if limit <= 0 || limit > listPageSize {
		limit = listPageSize
	}
	q.Set("limit", strconv.Itoa(limit))
	q.Set("offset", strconv.Itoa(offset))
	data, err := s.Get(ctx, path+"?"+q.Encode())
	if err != nil {
		return listEnvelope{}, err
	}
	var env listEnvelope
	if err := json.Unmarshal(data, &env); err != nil {
		return listEnvelope{}, fmt.Errorf("vkads %s: decode list: %w", path, err)
	}
	return env, nil
}

type rawSegment struct {
	ID      int64  `json:"id"`
	Name    string `json:"name"`
	Created string `json:"created"`
}

func parseCreated(value string) time.Time {
	for _, layout := range []string{
		time.RFC3339,
		"2006-01-02 15:04:05",
		"2006-01-02T15:04:05",
		"2006-01-02",
	} {
		if t, err := time.ParseInLocation(layout, value, time.Local); err == nil {
			return t
		}
	}
	return time.Time{}
}

func (s *Service) ListSegments(ctx context.Context) ([]Segment, error) {
	s.mu.Lock()
	if s.segments != nil && s.now().Sub(s.segmentsAt) < 5*time.Minute {
		out := append([]Segment(nil), s.segments...)
		s.mu.Unlock()
		return out, nil
	}
	s.mu.Unlock()

	var all []Segment
	seen := map[int64]struct{}{}
	for pages, offset := 0, 0; ; pages++ {
		env, err := s.getList(ctx, "/api/v2/remarketing/segments.json", offset, listPageSize)
		if err != nil {
			return nil, err
		}
		var raw []rawSegment
		if len(env.Items) > 0 {
			if err := json.Unmarshal(env.Items, &raw); err != nil {
				return nil, fmt.Errorf("vkads segments: %w", err)
			}
		}
		added := 0
		for _, item := range raw {
			if item.ID == 0 {
				continue
			}
			if _, ok := seen[item.ID]; ok {
				continue
			}
			seen[item.ID] = struct{}{}
			all = append(all, Segment{ID: item.ID, Name: item.Name, Created: parseCreated(item.Created)})
			added++
		}
		if stopAfterPage(len(raw), listPageSize, added, len(all), env.Count, pages+1, maxSegmentPages) {
			break
		}
		offset += len(raw)
	}
	s.mu.Lock()
	s.segments = all
	s.segmentsAt = s.now()
	s.mu.Unlock()
	return all, nil
}

// FindAudience returns an exact-name segment, else any name containing the value.
func (s *Service) FindAudience(ctx context.Context, name string) (*Segment, error) {
	items, err := s.ListSegments(ctx)
	if err != nil {
		return nil, err
	}
	if found := MatchAudience(name, items); found != nil {
		return found, nil
	}
	return nil, &AudienceNotFoundError{Name: name}
}

// AudienceNotFoundError means no segment matched the name. It is permanent.
type AudienceNotFoundError struct{ Name string }

func (e *AudienceNotFoundError) Error() string {
	return fmt.Sprintf("аудитория %q не найдена", e.Name)
}

type Package struct {
	ID          int64      `json:"id"`
	Name        string     `json:"name"`
	Objective   textList   `json:"objective"`
	PricedGoal  *PriceGoal `json:"priced_goal"`
	Description string     `json:"description"`
}

// textList accepts either "community" or ["community","socialengagement"].
type textList string

func (t *textList) UnmarshalJSON(data []byte) error {
	data = bytes.TrimSpace(data)
	if len(data) == 0 || string(data) == "null" {
		*t = ""
		return nil
	}
	if data[0] == '"' {
		var s string
		if err := json.Unmarshal(data, &s); err != nil {
			return err
		}
		*t = textList(s)
		return nil
	}
	var items []string
	if err := json.Unmarshal(data, &items); err != nil {
		return err
	}
	*t = textList(strings.Join(items, " "))
	return nil
}

func (t textList) ForAPI() string {
	fields := strings.Fields(string(t))
	for _, v := range fields {
		if strings.EqualFold(v, "community") {
			return v
		}
	}
	if len(fields) > 0 {
		return fields[0]
	}
	return string(t)
}

type PriceGoal struct {
	Name     string `json:"name"`
	SourceID int64  `json:"source_id"`
}

type Region struct {
	ID   int64  `json:"id"`
	Name string `json:"name"`
}

type Pad struct {
	ID          int    `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description"`
}

func (s *Service) ListPackages(ctx context.Context) ([]Package, error) {
	s.mu.Lock()
	if s.packages != nil && s.now().Sub(s.packagesAt) < catalogTTL {
		out := append([]Package(nil), s.packages...)
		s.mu.Unlock()
		return out, nil
	}
	s.mu.Unlock()

	var all []Package
	seen := map[int64]struct{}{}
	for pages, offset := 0, 0; ; pages++ {
		env, err := s.getList(ctx, "/api/v2/packages.json", offset, listPageSize)
		if err != nil {
			if len(all) > 0 {
				break
			}
			return nil, err
		}
		var page []Package
		if len(env.Items) > 0 {
			if err := json.Unmarshal(env.Items, &page); err != nil {
				return nil, fmt.Errorf("vkads packages: %w", err)
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
		if stopAfterPage(len(page), listPageSize, added, len(all), env.Count, pages+1, maxPackagePages) {
			break
		}
		offset += len(page)
	}
	s.mu.Lock()
	s.packages = all
	s.packagesAt = s.now()
	s.mu.Unlock()
	return all, nil
}

func (s *Service) ListRegions(ctx context.Context) ([]Region, error) {
	s.mu.Lock()
	if s.regions != nil && s.now().Sub(s.regionsAt) < catalogTTL {
		out := append([]Region(nil), s.regions...)
		s.mu.Unlock()
		return out, nil
	}
	s.mu.Unlock()

	var all []Region
	for _, q := range []string{"Россия", "Russia"} {
		extra := url.Values{}
		extra.Set("_q", q)
		env, err := s.getListQuery(ctx, "/api/v2/regions.json", 0, listPageSize, extra)
		if err != nil {
			continue
		}
		var page []Region
		if len(env.Items) > 0 {
			if err := json.Unmarshal(env.Items, &page); err != nil {
				return nil, fmt.Errorf("vkads regions: %w", err)
			}
		}
		if russiaRegionID(page) != 0 {
			all = page
			break
		}
	}
	if len(all) == 0 {
		all = []Region{{ID: 188, Name: "Россия"}}
	}
	s.mu.Lock()
	s.regions = all
	s.regionsAt = s.now()
	s.mu.Unlock()
	return all, nil
}

func (s *Service) ListPackagePads(ctx context.Context, packageID int64) ([]Pad, error) {
	_ = packageID
	s.mu.Lock()
	if s.packagePads != nil && s.now().Sub(s.packagePadsAt) < catalogTTL {
		out := append([]Pad(nil), s.packagePads...)
		s.mu.Unlock()
		return out, nil
	}
	s.mu.Unlock()

	var all []Pad
	seen := map[int]struct{}{}
	for pages, offset := 0, 0; ; pages++ {
		env, err := s.getList(ctx, "/api/v2/packages_pads.json", offset, listPageSize)
		if err != nil {
			if len(all) > 0 {
				break
			}
			return nil, err
		}
		var page []Pad
		if len(env.Items) > 0 {
			if err := json.Unmarshal(env.Items, &page); err != nil {
				return nil, err
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
		if stopAfterPage(len(page), listPageSize, added, len(all), env.Count, pages+1, maxPadPages) {
			break
		}
		offset += len(page)
	}
	s.mu.Lock()
	s.packagePads = all
	s.packagePadsAt = s.now()
	s.mu.Unlock()
	return all, nil
}

func (s *Service) CreateURL(ctx context.Context, rawURL string) (int64, error) {
	data, err := s.Post(ctx, "/api/v2/urls.json", map[string]any{"url": rawURL})
	if err != nil {
		return 0, err
	}
	var out struct {
		ID int64 `json:"id"`
	}
	if err := json.Unmarshal(data, &out); err != nil || out.ID == 0 {
		return 0, fmt.Errorf("vkads urls: unexpected %s", truncate(data, 300))
	}
	return out.ID, nil
}

func (s *Service) CreateAdPlan(ctx context.Context, body map[string]any) (int64, error) {
	data, err := s.Post(ctx, "/api/v2/ad_plans.json", body)
	if err != nil {
		return 0, err
	}
	var out struct {
		ID int64 `json:"id"`
	}
	if err := json.Unmarshal(data, &out); err != nil || out.ID == 0 {
		return 0, fmt.Errorf("vkads ad_plan: unexpected %s", truncate(data, 300))
	}
	return out.ID, nil
}

func (s *Service) CreateAdGroup(ctx context.Context, body map[string]any) (int64, error) {
	data, err := s.Post(ctx, "/api/v2/ad_groups.json", body)
	if err != nil {
		return 0, err
	}
	var out struct {
		ID int64 `json:"id"`
	}
	if err := json.Unmarshal(data, &out); err != nil || out.ID == 0 {
		return 0, fmt.Errorf("vkads ad_group: unexpected %s", truncate(data, 300))
	}
	return out.ID, nil
}

func containsFold(haystack string, needles ...string) bool {
	lower := strings.ToLower(haystack)
	for _, n := range needles {
		if strings.Contains(lower, strings.ToLower(n)) {
			return true
		}
	}
	return false
}

func PickCommunityMessagePackage(packages []Package, targetAction string) *Package {
	var fallback *Package
	for i := range packages {
		p := &packages[i]
		blob := string(p.Objective) + " " + p.Name + " " + p.Description
		community := containsFold(blob, "community", "socialengagement", "социаль", "сообществ", "групп")
		if !community {
			continue
		}
		if fallback == nil {
			fallback = p
		}
		if p.PricedGoal != nil && containsFold(p.PricedGoal.Name+" "+p.Description+" "+targetAction, "сообщен", "перепис", "conversation") {
			return p
		}
		if p.PricedGoal != nil && containsFold(p.PricedGoal.Name, "message") && containsFold(targetAction, "message") {
			return p
		}
	}
	return fallback
}

func russiaRegionID(regions []Region) int64 {
	for _, r := range regions {
		if containsFold(r.Name, "россия", "russia") && !containsFold(r.Name, "беларус", "казах") {
			return r.ID
		}
	}
	return 0
}

func PickRussiaRegion(regions []Region) int64 {
	if id := russiaRegionID(regions); id != 0 {
		return id
	}
	return 188
}

func PickVKFeedPads(pads []Pad) []int {
	var out []int
	for _, p := range pads {
		if containsFold(p.Name, "лента", "feed") && containsFold(p.Name, "vk", "вк", "вконтакте", "vkontakte") {
			out = append(out, p.ID)
		}
	}
	if len(out) == 0 {
		for _, p := range pads {
			if containsFold(p.Name, "лента", "feed") {
				out = append(out, p.ID)
			}
		}
	}
	return out
}
