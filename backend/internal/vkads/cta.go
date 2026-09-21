package vkads

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"sort"
	"strings"
	"unicode"
)

// CTARoleCommunity is the textblock that holds the button of a community
// banner. The role also decides which captions VK accepts.
const CTARoleCommunity = "cta_community_vk"

// CTAOption is one button caption. The API takes an identifier such as
// "contactUs" and renders its own wording in the ad, so the form has to offer
// those identifiers instead of free text — typing "Узнать цену" into the
// textblock only makes VK fall back to its default button.
type CTAOption struct {
	ID    string `json:"id"`
	Label string `json:"label"`
}

// ctaWording names the identifiers the registry returns beyond the documented
// list — community banners sell most of them. They are deliberately kept out
// of the fallback list below: which buttons a cabinet offers depends on the
// goal, and sending an identifier VK does not sell fails the upload.
var ctaWording = map[string]string{
	"getPrice":    "Узнать цену",
	"price":       "Узнать цену",
	"subscribe":   "Подписаться",
	"message":     "Написать сообщение",
	"write":       "Написать",
	"getoffer":    "Получить предложение",
	"getOffer":    "Получить предложение",
	"askQuestion": "Задать вопрос",
	"startChat":   "Начать чат",
	"install":     "Установить",
}

// ctaKnown is the button list of the API documentation. It backs the form when
// the registry cannot be read, so an unreachable dictionary does not leave the
// form without a button to pick.
var ctaKnown = []CTAOption{
	{ID: "contactUs", Label: "Связаться"},
	{ID: "signUp", Label: "Вступить"},
	{ID: "learnMore", Label: "Узнать больше"},
	{ID: "learn", Label: "Узнать"},
	{ID: "get", Label: "Получить"},
	{ID: "order", Label: "Заказать"},
	{ID: "buy", Label: "Купить"},
	{ID: "buy_ticket", Label: "Купить билет"},
	{ID: "choose", Label: "Выбрать"},
	{ID: "begin", Label: "Начать"},
	{ID: "open", Label: "Открыть"},
	{ID: "apply", Label: "Подать заявку"},
	{ID: "enroll", Label: "Записаться"},
	{ID: "register", Label: "Зарегистрироваться"},
	{ID: "book", Label: "Забронировать"},
	{ID: "call", Label: "Позвонить"},
	{ID: "try", Label: "Попробовать"},
	{ID: "visitSite", Label: "Перейти на сайт"},
	{ID: "to_shop", Label: "В магазин"},
	{ID: "download", Label: "Скачать"},
	{ID: "watch", Label: "Смотреть"},
	{ID: "listen", Label: "Слушать"},
	{ID: "playGame", Label: "Играть"},
	{ID: "see_menu", Label: "Посмотреть меню"},
	{ID: "create", Label: "Создать"},
}

// CTAOptions returns the captions VK accepts for a role, falling back to the
// known wording when the registry cannot be read: an unreachable dictionary
// must not leave the form without a button to pick.
func (s *Service) CTAOptions(ctx context.Context, role string) []CTAOption {
	catalog, err := s.ctaRegistry(ctx)
	if err != nil {
		log.Printf("vkads: реестр полей недоступен (%v), берём известные кнопки", err)
		return ctaFallback()
	}
	ids := catalog[strings.ToLower(role)]
	if len(ids) == 0 {
		return ctaFallback()
	}
	out := make([]CTAOption, 0, len(ids))
	for _, id := range ids {
		out = append(out, CTAOption{ID: id, Label: ctaLabel(id)})
	}
	return out
}

// ctaRegistry walks banner_fields and caches the buttons it names. The
// registry lists every banner field VK has, well past the 50 rows one page
// allows, so it is paged through and kept for an hour like the other
// dictionaries.
func (s *Service) ctaRegistry(ctx context.Context) (map[string][]string, error) {
	// The form asks for the buttons from several places at once, and the walk
	// costs a page request per 50 fields, so callers queue up behind the first
	// one instead of each walking VK on their own.
	s.ctaMu.Lock()
	defer s.ctaMu.Unlock()

	s.mu.Lock()
	if s.ctaCatalog != nil && s.now().Sub(s.ctaAt) < catalogTTL {
		cached := s.ctaCatalog
		s.mu.Unlock()
		return cached, nil
	}
	s.mu.Unlock()

	catalog := map[string][]string{}
	var lastPage []byte
	fields := 0
	for pages, offset := 0, 0; pages < 40; pages++ {
		env, err := s.getList(ctx, "/api/v2/banner_fields.json", offset, listPageSize)
		if err != nil {
			if fields > 0 {
				break
			}
			return nil, err
		}
		var page []any
		if len(env.Items) > 0 {
			_ = json.Unmarshal(env.Items, &page)
		}
		lastPage = env.Items
		mergeCTACatalog(catalog, ctaCatalogInRegistry(env.Items))
		fields += len(page)
		if len(page) < listPageSize || (env.Count > 0 && fields >= env.Count) {
			break
		}
		offset += len(page)
	}
	logCTACatalog(catalog, fields, lastPage)
	s.mu.Lock()
	s.ctaCatalog = catalog
	s.ctaAt = s.now()
	s.mu.Unlock()
	return catalog, nil
}

func mergeCTACatalog(into, from map[string][]string) {
	for role, ids := range from {
		seen := map[string]struct{}{}
		for _, id := range into[role] {
			seen[id] = struct{}{}
		}
		for _, id := range ids {
			if _, ok := seen[id]; ok {
				continue
			}
			seen[id] = struct{}{}
			into[role] = append(into[role], id)
		}
	}
}

// logCTACatalog prints every button VK sells, not just the role at hand: the
// list differs per cabinet and per goal, and the log is the only place to see
// what the cabinet actually offers. A registry we read but found no buttons in
// is dumped raw so its shape can be worked out.
func logCTACatalog(catalog map[string][]string, fields int, lastPage []byte) {
	if len(catalog) == 0 {
		log.Printf("vkads: в реестре полей (%d шт.) не нашли кнопок, последняя страница: %s",
			fields, truncate(lastPage, 8000))
		return
	}
	roles := make([]string, 0, len(catalog))
	for role := range catalog {
		roles = append(roles, role)
	}
	sort.Strings(roles)
	for _, role := range roles {
		ids := catalog[role]
		labelled := make([]string, 0, len(ids))
		for _, id := range ids {
			if label := ctaLabel(id); label != id {
				labelled = append(labelled, fmt.Sprintf("%s (%s)", id, label))
				continue
			}
			labelled = append(labelled, id)
		}
		log.Printf("vkads: кнопки %s — %d: %s", role, len(ids), strings.Join(labelled, ", "))
	}
}

func ctaFallback() []CTAOption {
	return append([]CTAOption(nil), ctaKnown...)
}

func ctaLabel(id string) string {
	if label := ctaWording[id]; label != "" {
		return label
	}
	for _, o := range ctaKnown {
		if o.ID == id {
			return o.Label
		}
	}
	return id
}

// ResolveCTA turns the saved caption into an identifier VK understands. The
// form used to accept free text, so a campaign may still carry wording like
// "Узнать цену" that no button matches — in that case the target action picks
// the button, the same way it did before the field existed.
func ResolveCTA(saved, targetAction string, options []CTAOption) string {
	if len(options) == 0 {
		options = ctaFallback()
	}
	want := strings.TrimSpace(saved)
	if want != "" {
		for _, o := range options {
			if strings.EqualFold(o.ID, want) || strings.EqualFold(o.Label, want) {
				return o.ID
			}
		}
		if id := ctaByMeaning(want, options); id != "" {
			return id
		}
		log.Printf("vkads: кнопка %q не входит в список ВК, берём кнопку по целевому действию", want)
	}
	return defaultCTA(targetAction, options)
}

// ctaMeanings pairs the wording our forms use with the words VK puts inside
// its identifiers. The cabinet renames buttons between releases, so a caption
// is matched by meaning before it is given up on.
var ctaMeanings = []struct {
	caption []string
	button  []string
}{
	{caption: []string{"цен", "стоимост", "price"}, button: []string{"price"}},
	{caption: []string{"написа", "сообщен", "связ", "contact", "message"}, button: []string{"contactus", "message", "write"}},
	{caption: []string{"вступ", "подпис", "join", "subscribe"}, button: []string{"signup", "join", "subscribe"}},
	{caption: []string{"подробн", "больше", "more"}, button: []string{"learnmore", "learn", "more"}},
}

func ctaByMeaning(saved string, options []CTAOption) string {
	want := strings.ToLower(saved)
	for _, meaning := range ctaMeanings {
		if !containsAny(want, meaning.caption) {
			continue
		}
		for _, o := range options {
			if containsAny(strings.ToLower(o.ID), meaning.button) {
				log.Printf("vkads: кнопку %q ВК называет %q", saved, o.ID)
				return o.ID
			}
		}
	}
	return ""
}

func containsAny(s string, needles []string) bool {
	for _, n := range needles {
		if strings.Contains(s, n) {
			return true
		}
	}
	return false
}

// defaultCTA is the button that matches the goal of the campaign, narrowed to
// what the role actually offers.
func defaultCTA(targetAction string, options []CTAOption) string {
	preferred := []string{"contactUs", "signUp"}
	if isJoinAction(targetAction) {
		preferred = []string{"signUp", "contactUs"}
	}
	for _, id := range preferred {
		for _, o := range options {
			if o.ID == id {
				return o.ID
			}
		}
	}
	if len(options) > 0 {
		return options[0].ID
	}
	return "contactUs"
}

// ctaCatalogInRegistry digs every button role out of banner_fields, keyed by
// the lowercased role. The registry nests fields differently per VK release,
// so the walk looks for any object naming a cta role and collects the
// identifiers around it rather than relying on one shape.
func ctaCatalogInRegistry(data []byte) map[string][]string {
	var doc any
	if err := json.Unmarshal(data, &doc); err != nil {
		return nil
	}
	catalog := map[string][]string{}
	seen := map[string]map[string]struct{}{}
	var walk func(node any)
	walk = func(node any) {
		switch v := node.(type) {
		case map[string]any:
			if role := ctaRoleOf(v); role != "" {
				for _, id := range ctaValues(v) {
					if seen[role] == nil {
						seen[role] = map[string]struct{}{}
					}
					if _, ok := seen[role][id]; ok {
						continue
					}
					seen[role][id] = struct{}{}
					catalog[role] = append(catalog[role], id)
				}
			}
			for _, child := range v {
				walk(child)
			}
		case []any:
			for _, child := range v {
				walk(child)
			}
		}
	}
	walk(doc)
	for role, ids := range catalog {
		if len(ids) == 0 {
			delete(catalog, role)
		}
	}
	return catalog
}

// ctaRoleOf returns the button role an entry describes, if any. VK spells the
// roles "cta_community_vk", "cta_sites_full" and so on, so the prefix is what
// tells a button field from the rest of the registry.
func ctaRoleOf(obj map[string]any) string {
	for _, key := range []string{"role", "name", "field", "id"} {
		s, ok := obj[key].(string)
		if !ok {
			continue
		}
		if lower := strings.ToLower(s); strings.HasPrefix(lower, "cta") {
			return lower
		}
	}
	return ""
}

// ctaValues collects the identifiers of a registry entry, accepting both plain
// lists and the {value, name} pairs VK uses where it also ships wording.
func ctaValues(obj map[string]any) []string {
	var out []string
	var collect func(node any)
	collect = func(node any) {
		switch v := node.(type) {
		case string:
			if isCTAIdentifier(v) {
				out = append(out, v)
			}
		case map[string]any:
			for _, key := range []string{"value", "id", "cta"} {
				if s, ok := v[key].(string); ok && isCTAIdentifier(s) {
					out = append(out, s)
					return
				}
			}
		case []any:
			for _, child := range v {
				collect(child)
			}
		}
	}
	// The allowed values sit right under the entry in some releases and behind
	// a "limits" wrapper in others, so the search descends into the entry.
	var scan func(node any)
	scan = func(node any) {
		fields, ok := node.(map[string]any)
		if !ok {
			return
		}
		for key, value := range fields {
			if isValuesKey(key) {
				collect(value)
				continue
			}
			scan(value)
		}
	}
	scan(obj)
	return out
}

func isValuesKey(key string) bool {
	lower := strings.ToLower(key)
	return strings.Contains(lower, "value") || strings.Contains(lower, "choice") ||
		strings.Contains(lower, "enum") || strings.Contains(lower, "variant")
}

// isCTAIdentifier keeps the walk from picking up prose: identifiers are short
// latin words, everything else in the registry is a description or a limit.
func isCTAIdentifier(s string) bool {
	if s == "" || len(s) > 30 {
		return false
	}
	for _, r := range s {
		if r > unicode.MaxASCII {
			return false
		}
		if !unicode.IsLetter(r) && !unicode.IsDigit(r) && r != '_' {
			return false
		}
	}
	return true
}
