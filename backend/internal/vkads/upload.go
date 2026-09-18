package vkads

import (
	"context"
	"fmt"
	"log"
	"strconv"
	"strings"
	"time"
)

// Catalog holds resolved VK Ads dictionary IDs for one upload.
type Catalog struct {
	Package   Package
	RussiaID  int64
	Pads      []int
	URLID     int64
	DateStart string
	Patterns  []BannerPattern
}

func (s *Service) ResolveCatalog(ctx context.Context, settings Settings, createdAt time.Time) (*Catalog, error) {
	settings = settings.Normalize()
	packages, err := s.ListPackages(ctx)
	if err != nil {
		return nil, err
	}
	pkg := PickCommunityMessagePackage(packages, settings.TargetAction)
	if pkg == nil {
		return nil, fmt.Errorf("vkads: нет пакета для сообщества / отправки сообщения")
	}
	regions, err := s.ListRegions(ctx)
	if err != nil {
		return nil, err
	}
	russia := PickRussiaRegion(regions)
	tree := s.PackagePlacements(ctx, *pkg)
	pads := ResolvePadsInTree(settings.Pads, *pkg, tree)
	if len(pads) == 0 {
		listed, err := s.ListPackagePads(ctx, pkg.ID)
		if err != nil {
			return nil, err
		}
		pads = intersectPadIDs(orPadIDs(settings.Pads, PickVKFeedPads(listed)), intSet(padIDs(listed)))
	}
	if len(pads) == 0 {
		return nil, fmt.Errorf("vkads: не нашли ленту ВК среди площадок пакета %d — выберите места размещения вручную", pkg.ID)
	}
	log.Printf("vkads пакет %d: дерево %d (%d площадок), выбрано %v",
		pkg.ID, pkg.PadsTreeID, len(CollectPadIDs(tree)), pads)
	communityURL := fmt.Sprintf("https://vk.com/club%d", settings.CommunityID)
	if tags := strings.TrimSpace(settings.RefTags); tags != "" {
		communityURL += "?" + strings.TrimPrefix(tags, "?")
	}
	urlID, err := s.CreateURL(ctx, communityURL)
	if err != nil {
		return nil, err
	}
	start := settings.DateStart
	if start == "" {
		start = createdAt.Format("2006-01-02")
	}
	patterns, err := s.ListPackagePatterns(ctx, *pkg, pads)
	if err != nil {
		return nil, err
	}
	return &Catalog{
		Package:   *pkg,
		RussiaID:  russia,
		Pads:      pads,
		URLID:     urlID,
		DateStart: start,
		Patterns:  patterns,
	}, nil
}

func PlanBody(name string, settings Settings, cat *Catalog) map[string]any {
	settings = settings.Normalize()
	body := map[string]any{
		"name":             name,
		"status":           "active",
		"date_start": cat.DateStart,
		"objective":  cat.Package.Objective.ForAPI(),
	}
	if settings.Optimization {
		applyMoney(body, settings)
		body["autobidding_mode"] = autobiddingMode(settings.BiddingStrategy)
	}
	if cat.Package.PricedGoal != nil {
		body["priced_goal"] = cat.Package.PricedGoal
	}
	return body
}

// AttachGroups nests groups into an ad_plan create call under the given key,
// returning a copy so the same plan can be retried under the other spelling.
func AttachGroups(plan map[string]any, groups []map[string]any, key string) map[string]any {
	body := make(map[string]any, len(plan)+1)
	for k, v := range plan {
		body[k] = v
	}
	list := make([]any, 0, len(groups))
	for _, group := range groups {
		list = append(list, group)
	}
	body[key] = list
	return body
}

func NestedGroupBody(name string, audienceID int64, settings Settings, cat *Catalog) map[string]any {
	body := GroupBody(name, 0, audienceID, settings, cat)
	delete(body, "ad_plan_id")
	return body
}

func GroupBody(name string, planID, audienceID int64, settings Settings, cat *Catalog) map[string]any {
	settings = settings.Normalize()
	body := map[string]any{
		"name":             name,
		"status":           "active",
		"ad_plan_id":       planID,
		"package_id":       cat.Package.ID,
		"date_start":       cat.DateStart,
		"age_restrictions": settings.AgeRestrictions,
		"objective":        cat.Package.Objective.ForAPI(),
		"targetings":       groupTargetings(settings, audienceID, cat.RussiaID, cat.Pads),
	}
	if !settings.Optimization {
		applyMoney(body, settings)
		body["autobidding_mode"] = autobiddingMode(settings.BiddingStrategy)
	}
	if cat.Package.PricedGoal != nil {
		body["priced_goal"] = cat.Package.PricedGoal
	}
	return body
}

// applyMoney writes budget fields the way the VK Ads API examples do: decimal
// strings. With campaign optimization the budget lives on the ad_plan only;
// sending the same number on every group makes the cabinet replace it.
func applyMoney(body map[string]any, settings Settings) {
	if settings.BudgetDay != nil {
		body["budget_limit_day"] = formatMoney(*settings.BudgetDay)
	}
	if settings.BudgetTotal != nil {
		body["budget_limit"] = formatMoney(*settings.BudgetTotal)
	}
	if settings.MaxPrice != nil {
		body["max_price"] = formatMoney(*settings.MaxPrice)
	}
}

func formatMoney(v float64) string {
	return strconv.FormatFloat(v, 'f', -1, 64)
}

func FormatID(id int64) string {
	if id == 0 {
		return ""
	}
	return strconv.FormatInt(id, 10)
}
