package vkads

const (
	DefaultCommunityID     int64  = 231988974
	DefaultRefTags         string = "ref_source=vk_ads_yulya&ref={{banner_id}}"
	DefaultAgeRestrictions string = "0+"
	DefaultBidding         string = "min_price"
	DefaultTargetAction    string = "send_message"
	DefaultObjective       string = "community"
	DefaultBudgetDay              = 999.0
	DefaultBannerTitle     string = "RuTime | именные наручные часы"
	DefaultBannerCTA       string = "Узнать цену"
)

// Settings is the shared VK Ads form stored on the campaign and cloned into
// every ad group. Empty pads mean "resolve VK feed at upload time".
type Settings struct {
	CommunityID     int64    `json:"communityId"`
	Objective       string   `json:"objective"`
	TargetAction    string   `json:"targetAction"`
	Optimization    bool     `json:"optimization"`
	BiddingStrategy string   `json:"biddingStrategy"`
	BudgetDay       *float64 `json:"budgetDay,omitempty"`
	BudgetTotal     *float64 `json:"budgetTotal,omitempty"`
	MaxPrice        *float64 `json:"maxPrice,omitempty"`
	DateStart       string   `json:"dateStart,omitempty"`
	Sex             string   `json:"sex"`
	AgeFrom         int      `json:"ageFrom"`
	AgeTo           int      `json:"ageTo"`
	AgeUnknown      bool     `json:"ageUnknown"`
	AgeRestrictions string   `json:"ageRestrictions"`
	Pads            []int    `json:"pads,omitempty"`
	RefTags         string   `json:"refTags"`
	BannerTitle     string   `json:"bannerTitle"`
	BannerCTA       string   `json:"bannerCta"`
}

func DefaultSettings() Settings {
	day := DefaultBudgetDay
	return Settings{
		CommunityID:     DefaultCommunityID,
		Objective:       DefaultObjective,
		TargetAction:    DefaultTargetAction,
		Optimization:    true,
		BiddingStrategy: DefaultBidding,
		BudgetDay:       &day,
		Sex:             "male",
		AgeFrom:         24,
		AgeTo:           65,
		AgeUnknown:      false,
		AgeRestrictions: DefaultAgeRestrictions,
		RefTags:         DefaultRefTags,
		BannerTitle:     DefaultBannerTitle,
		BannerCTA:       DefaultBannerCTA,
	}
}

// Normalize fills blanks with defaults so a partial form still uploads.
func (s Settings) Normalize() Settings {
	if s.CommunityID == 0 && s.AgeFrom == 0 && s.BiddingStrategy == "" && s.Sex == "" && s.Objective == "" && s.TargetAction == "" {
		return DefaultSettings()
	}
	out := s
	if out.CommunityID <= 0 {
		out.CommunityID = DefaultCommunityID
	}
	if out.Objective == "" {
		out.Objective = DefaultObjective
	}
	if out.TargetAction == "" {
		out.TargetAction = DefaultTargetAction
	}
	if out.BiddingStrategy == "" {
		out.BiddingStrategy = DefaultBidding
	}
	if out.BudgetDay == nil {
		day := DefaultBudgetDay
		out.BudgetDay = &day
	}
	if out.Sex == "" {
		out.Sex = "male"
	}
	if out.AgeFrom <= 0 {
		out.AgeFrom = 24
	}
	if out.AgeTo <= 0 {
		out.AgeTo = 65
	}
	if out.AgeRestrictions == "" {
		out.AgeRestrictions = DefaultAgeRestrictions
	}
	if out.RefTags == "" {
		out.RefTags = DefaultRefTags
	}
	if out.BannerTitle == "" {
		out.BannerTitle = DefaultBannerTitle
	}
	if out.BannerCTA == "" {
		out.BannerCTA = DefaultBannerCTA
	}
	return out
}
