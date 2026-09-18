package vkads

// ShowHours is 06:00–21:00 as VK Ads hour slots (6 through 20).
func ShowHours() []int {
	hours := make([]int, 0, 15)
	for hour := 6; hour < 21; hour++ {
		hours = append(hours, hour)
	}
	return hours
}

func AgeList(from, to int, includeUnknown bool) []int {
	if from < 0 {
		from = 0
	}
	if to < from {
		to = from
	}
	n := to - from + 1
	if includeUnknown {
		n++
	}
	out := make([]int, 0, n)
	if includeUnknown {
		out = append(out, 0)
	}
	for age := from; age <= to; age++ {
		if age == 0 {
			continue
		}
		out = append(out, age)
	}
	return out
}

func sexList(sex string) []string {
	switch sex {
	case "female":
		return []string{"female"}
	case "all", "":
		return []string{"male", "female"}
	default:
		return []string{"male"}
	}
}

func fulltimeSchedule() map[string]any {
	hours := ShowHours()
	return map[string]any{
		"mon":   hours,
		"tue":   hours,
		"wed":   hours,
		"thu":   hours,
		"fri":   hours,
		"sat":   hours,
		"sun":   hours,
		"flags": []string{},
	}
}

func groupTargetings(settings Settings, audienceID, russiaRegion int64, pads []int) map[string]any {
	target := map[string]any{
		"age":      map[string]any{"age_list": AgeList(settings.AgeFrom, settings.AgeTo, settings.AgeUnknown)},
		"fulltime": fulltimeSchedule(),
		"geo":      map[string]any{"regions": []int64{russiaRegion}},
	}
	if sexes := sexList(settings.Sex); len(sexes) > 0 {
		target["sex"] = sexes
	}
	if len(pads) > 0 {
		target["pads"] = pads
	}
	if audienceID > 0 {
		target["segments"] = []int64{audienceID}
	}
	return target
}

// autobiddingMode maps the form to AdPlan/AdGroup. Both objects document only
// max_goals: no bid, daily or total budget required. That is the cabinet's
// «Минимальная цена», not second_price from old AdGroups examples.
func autobiddingMode(strategy string) string {
	switch strategy {
	case "second_price_mean":
		return "second_price_mean"
	default:
		return "max_goals"
	}
}
