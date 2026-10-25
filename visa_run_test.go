package main

import (
	"strings"
	"testing"
)

func TestVisaRunExplainsResetAndAggregateLimits(t *testing.T) {
	rules := russianRules(t)
	for _, tc := range []struct{ id, want, extra string }{
		{"KZ", "не обнуляет общий лимит", "90 дней за 180 дней"},
		{"TR", "возможен с ограничениями", "90 дней за 180 дней"},
		{"GE", "возможен по правилу срока за въезд", "12 календарных мес."},
		{"AL", "не обнуляет общий лимит", "виза с доступным въездом"},
		{"AM", "не обнуляет общий лимит", "180 дней за последние 365 дней"},
	} {
		t.Run(tc.id, func(t *testing.T) {
			var b strings.Builder
			visaRunBlock(&b, rules[tc.id], Result{})
			if !strings.Contains(b.String(), tc.want) || !strings.Contains(b.String(), tc.extra) {
				t.Fatal(b.String())
			}
		})
	}
}

func TestVisaRunExplainsGapAndExhaustedEntries(t *testing.T) {
	rule := &Rule{Limits: Limits{MaxConsecutive: 30}, VisaRun: VisaRun{Allowed: true, MinDaysOutside: 7, MaxPerYear: 2}}
	var b strings.Builder
	visaRunBlock(&b, rule, Result{EntriesInYear: 2})
	if !strings.Contains(b.String(), "новый въезд в этом году недоступен") || !strings.Contains(b.String(), "годовой лимит въездов уже исчерпан") {
		t.Fatal(b.String())
	}
	b.Reset()
	visaRunBlock(&b, rule, Result{EntriesInYear: 1})
	if !strings.Contains(b.String(), "7 полных дней") {
		t.Fatal(b.String())
	}
}

func TestVisaRunDoesNotInheritOldThaiExtension(t *testing.T) {
	cf, err := ParseCountries(defaultCountriesJSON)
	if err != nil {
		t.Fatal(err)
	}
	for i := range cf.Countries {
		if cf.Countries[i].ID != "TH" {
			continue
		}
		u := &UI{country: &cf.Countries[i], refDate: d("2026-10-25"), cur: &Profile{Name: "Тест", Citizenship: "RU", Trips: map[string][]Trip{"TH": {{Entry: "2026-09-14", Registered: true}}}}}
		report := u.report()
		if !strings.Contains(report, "Новый въезд начинает новый срок: до 30 дн. подряд") {
			t.Fatal(report)
		}
		return
	}
	t.Fatal("TH missing")
}
