package vkads

import (
	"context"
	"encoding/json"
	"log"
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

// ctaWording is how the cabinet spells the identifiers of the banner field
// registry. It names whatever the registry returns and stands in for it when
// VK is unreachable.
var ctaWording = []CTAOption{
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
	data, err := s.Get(ctx, "/api/v2/banner_fields.json?limit=250")
	if err != nil {
		log.Printf("vkads: реестр полей недоступен (%v), берём известные кнопки", err)
		return ctaFallback()
	}
	ids := ctaIDsInRegistry(data, role)
	if len(ids) == 0 {
		log.Printf("vkads: в реестре полей нет значений для %s, берём известные кнопки", role)
		return ctaFallback()
	}
	known := ctaWordingByID()
	out := make([]CTAOption, 0, len(ids))
	for _, id := range ids {
		label := known[id]
		if label == "" {
			label = id
		}
		out = append(out, CTAOption{ID: id, Label: label})
	}
	return out
}

func ctaFallback() []CTAOption {
	return append([]CTAOption(nil), ctaWording...)
}

func ctaWordingByID() map[string]string {
	out := make(map[string]string, len(ctaWording))
	for _, o := range ctaWording {
		out[o.ID] = o.Label
	}
	return out
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
		log.Printf("vkads: кнопка %q не входит в список ВК, берём кнопку по целевому действию", want)
	}
	return defaultCTA(targetAction, options)
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

// ctaIDsInRegistry digs the allowed values of one role out of banner_fields.
// The registry nests fields differently per VK release, so the walk looks for
// any object that names the role and collects the identifiers around it rather
// than relying on one shape.
func ctaIDsInRegistry(data []byte, role string) []string {
	var doc any
	if err := json.Unmarshal(data, &doc); err != nil {
		return nil
	}
	var ids []string
	seen := map[string]struct{}{}
	var walk func(node any)
	walk = func(node any) {
		switch v := node.(type) {
		case map[string]any:
			if objectNames(v, role) {
				for _, id := range ctaValues(v) {
					if _, ok := seen[id]; ok {
						continue
					}
					seen[id] = struct{}{}
					ids = append(ids, id)
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
	return ids
}

func objectNames(obj map[string]any, role string) bool {
	for _, key := range []string{"role", "name", "field", "id"} {
		if s, ok := obj[key].(string); ok && strings.EqualFold(s, role) {
			return true
		}
	}
	return false
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
