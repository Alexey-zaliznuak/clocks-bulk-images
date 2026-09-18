package vkads

import (
	"context"
	"fmt"
	"strconv"
	"time"
)

// Catalog holds resolved VK Ads dictionary IDs for one upload.
type Catalog struct {
	Package   Package
	RussiaID  int64
	Pads      []int
	URLID     int64
	DateStart string
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
	pads := settings.Pads
	if len(pads) == 0 {
		listed, err := s.ListPackagePads(ctx, pkg.ID)
		if err != nil {
			return nil, err
		}
		pads = PickVKFeedPads(listed)
	}
	communityURL := fmt.Sprintf("https://vk.com/club%d", settings.CommunityID)
	urlID, err := s.CreateURL(ctx, communityURL)
	if err != nil {
		return nil, err
	}
	start := settings.DateStart
	if start == "" {
		start = createdAt.Format("2006-01-02")
	}
	return &Catalog{
		Package:   *pkg,
		RussiaID:  russia,
		Pads:      pads,
		URLID:     urlID,
		DateStart: start,
	}, nil
}

func PlanBody(name string, settings Settings, cat *Catalog) map[string]any {
	settings = settings.Normalize()
	body := map[string]any{
		"name":             name,
		"status":           "active",
		"date_start":       cat.DateStart,
		"enable_utm":       false,
		"utm":              settings.RefTags,
		"objective":        cat.Package.Objective.ForAPI(),
		"ad_object_type":   "url",
		"ad_object_id":     cat.URLID,
		"autobidding_mode": autobiddingMode(settings.BiddingStrategy, settings.Optimization),
		"ad_groups":        []any{},
	}
	if settings.Optimization {
		applyMoney(body, settings)
	}
	if cat.Package.PricedGoal != nil {
		body["priced_goal"] = cat.Package.PricedGoal
	}
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
		"enable_utm":       false,
		"utm":              settings.RefTags,
		"objective":        cat.Package.Objective.ForAPI(),
		"autobidding_mode": autobiddingMode(settings.BiddingStrategy, settings.Optimization),
		"targetings":       groupTargetings(settings, audienceID, cat.RussiaID, cat.Pads),
	}
	if !settings.Optimization {
		applyMoney(body, settings)
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
