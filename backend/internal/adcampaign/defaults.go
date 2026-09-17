package adcampaign

import (
	_ "embed"
	"strings"
	"unicode"
	"unicode/utf8"
)

//go:embed defaults/names.txt
var defaultNames string

//go:embed defaults/surnames.txt
var defaultSurnames string

const DefaultSurnameText = `{{name}}- фамилия, за которой стоит мужской характер.

Часы с твоей фамилией на циферблате - это не просто стиль, а символ внутренней силы.

Получи скидку 30% + браслет в подарок!

— На циферблате можно сделать не только фамилию, но также любой рисунок или пожелания
— предлагаем кожаные или металлические ремешки на выбор
— доставка действует по всей территории РФ
— Скидка 30% на первый заказ и браслет в подарок
— Скидка 50% на вторые часы
— Гарантия 12 месяцев

Жми кнопку "Узнать цену" и мы рассчитаем стоимость, доставку и покажем, как будут выглядеть твои уникальные часы!`

const DefaultNameText = `{{name}} - имя, за которым стоит мужской характер.

Часы с твоим именем на циферблате - это не просто стиль, а символ внутренней силы.

Получи скидку 30% + браслет в подарок!

— На циферблате можно сделать не только имя, но также любой рисунок или пожелания
— предлагаем кожаные или металлические ремешки на выбор
— доставка действует по всей территории РФ
— Скидка 30% на первый заказ и браслет в подарок
— Скидка 50% на вторые часы
— Гарантия 12 месяцев

Жми кнопку "Узнать цену" и мы рассчитаем стоимость, доставку и покажем, как будут выглядеть твои уникальные часы!`

type Diagnostics struct {
	DuplicateCount        int      `json:"duplicateCount"`
	InvalidCapitalization []string `json:"invalidCapitalization"`
}

func Defaults() (names, surnames []string, nameDiag, surnameDiag Diagnostics) {
	names, nameDiag = Normalize(strings.Split(defaultNames, "\n"))
	surnames, surnameDiag = Normalize(strings.Split(defaultSurnames, "\n"))
	return
}

// Normalize trims values, drops empty and exact duplicate values while
// preserving order. Capitalization problems are diagnostics, not rejection.
func Normalize(values []string) ([]string, Diagnostics) {
	seen := make(map[string]struct{}, len(values))
	out := make([]string, 0, len(values))
	var d Diagnostics
	for _, raw := range values {
		value := strings.TrimSpace(strings.TrimPrefix(raw, "\ufeff"))
		if value == "" {
			continue
		}
		if _, ok := seen[value]; ok {
			d.DuplicateCount++
			continue
		}
		seen[value] = struct{}{}
		out = append(out, value)
		r, _ := utf8.DecodeRuneInString(value)
		if !unicode.IsUpper(r) {
			d.InvalidCapitalization = append(d.InvalidCapitalization, value)
		}
	}
	return out, d
}

func HasNamePlaceholder(template string) bool {
	return strings.Contains(template, "{{name}}")
}
