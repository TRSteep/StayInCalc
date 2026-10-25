package main

import (
	"sort"
	"strings"
)

var citizenshipNames = map[string]string{
	"RU": "Россия", "BY": "Беларусь", "KZ": "Казахстан", "UA": "Украина",
	"AM": "Армения", "KG": "Кыргызстан", "GE": "Грузия", "AL": "Албания",
	"TR": "Турция", "TH": "Таиланд", "VN": "Вьетнам", "AE": "ОАЭ", "RS": "Сербия",
	"UZ": "Узбекистан", "TJ": "Таджикистан", "AZ": "Азербайджан", "MD": "Молдова",
	"CN": "Китай",
}

func citizenshipLabel(code string) string {
	code = strings.ToUpper(strings.TrimSpace(code))
	if name, ok := citizenshipNames[code]; ok {
		return name + " (" + code + ")"
	}
	return code
}

func citizenshipCode(label string) string {
	if before, after, ok := strings.Cut(label, " ("); ok && before != "" {
		return strings.TrimSuffix(after, ")")
	}
	return strings.ToUpper(strings.TrimSpace(label))
}

func (u *UI) citizenshipOptions() []string {
	codes := map[string]bool{"RU": true}
	for code := range citizenshipNames {
		codes[code] = true
	}
	for _, c := range u.countries.Countries {
		for _, r := range c.Rules {
			for _, code := range r.Citizenships {
				code = strings.ToUpper(strings.TrimSpace(code))
				if code != "" && code != "*" {
					codes[code] = true
				}
			}
		}
	}
	for _, p := range u.profiles {
		code := strings.ToUpper(strings.TrimSpace(p.Citizenship))
		if code != "" {
			codes[code] = true
		}
	}
	var labels []string
	for code := range codes {
		labels = append(labels, citizenshipLabel(code))
	}
	sort.Slice(labels, func(i, j int) bool {
		if labels[i] == citizenshipLabel("RU") {
			return labels[j] != labels[i]
		}
		if labels[j] == citizenshipLabel("RU") {
			return false
		}
		return labels[i] < labels[j]
	})
	return labels
}
