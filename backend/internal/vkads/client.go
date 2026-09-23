package vkads

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log"
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
	listPageSize    = 50
	maxPackagePages = 8
	maxPadPages     = 20
	maxSegmentPages = 200
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
	ID          int64           `json:"id"`
	Name        string          `json:"name"`
	Objective   textList        `json:"objective"`
	PricedGoal  *PriceGoal      `json:"priced_goal"`
	Description string          `json:"description"`
	PadsTreeID  int64           `json:"pads_tree_id"`
	Options     json.RawMessage `json:"options"`
	PatternIDs  []int64         `json:"-"`
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
	prefer := []string{"socialengagement", "engagement", "community"}
	have := make(map[string]string, len(fields))
	for _, v := range fields {
		have[strings.ToLower(v)] = v
	}
	for _, name := range prefer {
		if v, ok := have[name]; ok {
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
	ID          int             `json:"id"`
	Name        string          `json:"name"`
	Description string          `json:"description"`
	Patterns    json.RawMessage `json:"patterns"`
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
		env, err := s.getListQuery(ctx, "/api/v2/packages.json", offset, listPageSize, url.Values{
			"fields": {"id,name,objective,description,pads_tree_id,options,priced_event_type,status"},
		})
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
			item.PatternIDs = PackageAllowedPatternIDs(item)
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
	registered, err := s.GetURL(ctx, out.ID)
	if err != nil {
		return 0, fmt.Errorf("vkads url %d: verify: %w", out.ID, err)
	}
	if !sameDestinationURL(rawURL, registered) {
		return 0, fmt.Errorf("vkads url %d: registered %q instead of %q", out.ID, registered, rawURL)
	}
	return out.ID, nil
}

// GetURL reads the canonical destination stored by VK for a URL object. A
// read-after-write check is important here: banners only receive the numeric
// id, so otherwise a reused/stale id can silently point at different REF tags.
func (s *Service) GetURL(ctx context.Context, id int64) (string, error) {
	data, err := s.Get(ctx, fmt.Sprintf("/api/v2/urls/%d.json", id))
	if err != nil {
		return "", err
	}
	var out struct {
		URL string `json:"url"`
	}
	if err := json.Unmarshal(data, &out); err != nil || strings.TrimSpace(out.URL) == "" {
		return "", fmt.Errorf("unexpected %s", truncate(data, 300))
	}
	return out.URL, nil
}

func sameDestinationURL(want, got string) bool {
	canonical := func(raw string) (string, bool) {
		u, err := url.Parse(strings.TrimSpace(raw))
		if err != nil || !u.IsAbs() {
			return "", false
		}
		u.Scheme = strings.ToLower(u.Scheme)
		u.Host = strings.ToLower(u.Host)
		// Query.Encode gives equivalent ordering and treats encoded and literal
		// VK macros ({{banner_id}} vs %7B%7Bbanner_id%7D%7D) alike.
		u.RawQuery = u.Query().Encode()
		u.Fragment = ""
		return u.String(), true
	}
	a, aOK := canonical(want)
	b, bOK := canonical(got)
	return aOK && bOK && a == b
}

type CreatedGroup struct {
	ID        int64
	BannerIDs []int64
}

func (s *Service) CreateAdPlan(ctx context.Context, body map[string]any) (int64, []CreatedGroup, error) {
	data, err := s.Post(ctx, "/api/v2/ad_plans.json", body)
	if err != nil {
		return 0, nil, err
	}
	return parseCreatePlan(data)
}

// planGroupKeys are the two names an ad_plan accepts its nested groups under.
// The resource documents ad_groups, but the live cabinet answers
// "campaigns: Empty value", so that spelling goes first.
var planGroupKeys = []string{"campaigns", "ad_groups"}

// CreateAdPlanWithGroups posts a plan together with its groups. A plan is never
// created empty: ad_plans.json rejects one without groups outright.
func (s *Service) CreateAdPlanWithGroups(ctx context.Context, plan map[string]any, groups []map[string]any) (int64, []CreatedGroup, error) {
	var err error
	for i, key := range planGroupKeys {
		var id int64
		var created []CreatedGroup
		id, created, err = s.CreateAdPlan(ctx, AttachGroups(plan, groups, key))
		if err == nil {
			return id, created, nil
		}
		// Only the cabinet disagreeing about the key is worth another call;
		// a rejected banner would fail the same way twice.
		if i == len(planGroupKeys)-1 || !mentionsOtherGroupKey(err, key) {
			return 0, nil, err
		}
		log.Printf("vkads ad_plan: %s rejected, retrying with %s", key, planGroupKeys[i+1])
	}
	return 0, nil, err
}

func mentionsOtherGroupKey(err error, used string) bool {
	msg := strings.ToLower(err.Error())
	for _, key := range planGroupKeys {
		if key != used && strings.Contains(msg, key) {
			return true
		}
	}
	return false
}

func parseCreatePlan(data []byte) (int64, []CreatedGroup, error) {
	var out struct {
		ID        int64           `json:"id"`
		Campaigns []createdNested `json:"campaigns"`
		AdGroups  []createdNested `json:"ad_groups"`
	}
	if err := json.Unmarshal(data, &out); err != nil || out.ID == 0 {
		return 0, nil, fmt.Errorf("vkads ad_plan: unexpected %s", truncate(data, 300))
	}
	var groups []CreatedGroup
	for _, item := range out.Campaigns {
		if g := item.asCreated(); g.ID != 0 {
			groups = append(groups, g)
		}
	}
	for _, item := range out.AdGroups {
		if g := item.asCreated(); g.ID != 0 {
			groups = append(groups, g)
		}
	}
	return out.ID, groups, nil
}

type createdNested struct {
	ID      int64 `json:"id"`
	Banners []struct {
		ID int64 `json:"id"`
	} `json:"banners"`
}

func (n createdNested) asCreated() CreatedGroup {
	g := CreatedGroup{ID: n.ID}
	for _, b := range n.Banners {
		if b.ID != 0 {
			g.BannerIDs = append(g.BannerIDs, b.ID)
		}
	}
	return g
}

func (s *Service) CreateAdGroup(ctx context.Context, body map[string]any) (CreatedGroup, error) {
	data, err := s.Post(ctx, "/api/v2/ad_groups.json", body)
	if err != nil {
		data, err = s.Post(ctx, "/api/v2/campaigns.json", body)
	}
	if err != nil {
		return CreatedGroup{}, err
	}
	return parseCreateGroup(data)
}

func parseCreateGroup(data []byte) (CreatedGroup, error) {
	var out createdNested
	if err := json.Unmarshal(data, &out); err != nil || out.ID == 0 {
		return CreatedGroup{}, fmt.Errorf("vkads campaign: unexpected %s", truncate(data, 300))
	}
	return out.asCreated(), nil
}

func (s *Service) DeleteAdGroup(ctx context.Context, id int64) error {
	if id <= 0 {
		return nil
	}
	body := map[string]any{"status": "deleted"}
	_, err := s.Post(ctx, fmt.Sprintf("/api/v2/ad_groups/%d.json", id), body)
	if err != nil {
		_, err = s.Post(ctx, fmt.Sprintf("/api/v2/campaigns/%d.json", id), body)
	}
	return err
}

func (s *Service) ListGroupBannerIDs(ctx context.Context, groupID int64) ([]int64, error) {
	data, err := s.Get(ctx, fmt.Sprintf("/api/v2/banners.json?_ad_group_id=%d&limit=20", groupID))
	if err != nil {
		return nil, err
	}
	var env struct {
		Items []struct {
			ID int64 `json:"id"`
		} `json:"items"`
	}
	if err := json.Unmarshal(data, &env); err != nil {
		return nil, err
	}
	var ids []int64
	for _, item := range env.Items {
		if item.ID != 0 {
			ids = append(ids, item.ID)
		}
	}
	return ids, nil
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

// PickCommunityPackage returns the community package whose priced goal matches
// the target action. VK sells the two goals as separate packages — one charges
// for joining the community, the other for writing to it — and the cabinet
// labels the campaign after whichever package created its groups. Picking the
// wrong one is invisible in our request and shows up as "Подписка на
// сообщество" next to a campaign that was meant to collect messages.
func PickCommunityPackage(packages []Package, targetAction string) *Package {
	wantJoin := isJoinAction(targetAction)
	var neutral *Package
	for i := range packages {
		p := &packages[i]
		if !isCommunityPackage(*p) {
			continue
		}
		join, message := packageGoal(*p)
		if join && message {
			continue
		}
		if (wantJoin && join) || (!wantJoin && message) {
			return p
		}
		if neutral == nil && !join && !message {
			neutral = p
		}
	}
	return neutral
}

func isJoinAction(targetAction string) bool {
	return containsFold(targetAction, "join", "вступ", "подпис", "subscribe")
}

func isCommunityPackage(p Package) bool {
	blob := string(p.Objective) + " " + p.Name + " " + p.Description
	return containsFold(blob, "community", "socialengagement", "социаль", "сообществ", "групп")
}

// packageGoal tells the community packages apart by the goal they charge for.
func packageGoal(p Package) (join, message bool) {
	blob := p.Name + " " + p.Description
	if p.PricedGoal != nil {
		blob += " " + p.PricedGoal.Name
	}
	join = containsFold(blob, "join", "вступ", "подпис", "subscribe")
	message = containsFold(blob, "message", "написать", "сообщен", "перепис", "conversation", "dialog")
	return join, message
}

// DescribePackage renders a package for the log: the id alone says nothing
// about which of the two community goals it sells.
func DescribePackage(p Package) string {
	goal := ""
	if p.PricedGoal != nil {
		goal = p.PricedGoal.Name
	}
	return fmt.Sprintf("%d %q цель %q", p.ID, p.Name, goal)
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
