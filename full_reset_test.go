package main

import (
	"strings"
	"testing"
)

func TestFullResetDates(t *testing.T) {
	for _, tc := range []struct {
		name                 string
		rule                 Rule
		trips                []Trip
		departure, ref, want string
	}{
		{"rolling180", Rule{Limits: Limits{MaxInPeriod: 90, PeriodDays: 180}}, []Trip{{Entry: "2026-01-01", Exit: "2026-03-31"}}, "2026-03-31", "2026-04-01", "2026-09-27"},
		{"rolling365Leap", Rule{Limits: Limits{MaxPerYear: 180, YearMode: "rolling"}}, []Trip{{Entry: "2024-02-29", Exit: "2024-02-29"}}, "2024-02-29", "2024-03-01", "2025-02-28"},
		{"calendarYear", Rule{Limits: Limits{MaxPerYear: 180}}, []Trip{{Entry: "2026-06-01", Exit: "2026-06-02"}}, "2026-06-02", "2026-07-01", "2027-01-01"},
		{"entryCount", Rule{VisaRun: VisaRun{MaxPerYear: 2}}, []Trip{{Entry: "2026-01-01", Exit: "2026-01-02"}}, "2026-01-02", "2026-01-03", "2027-01-01"},
		{"minimumGap", Rule{Limits: Limits{MaxConsecutive: 30}, VisaRun: VisaRun{Allowed: true, MinDaysOutside: 7}}, []Trip{{Entry: "2026-01-01"}}, "2026-01-15", "2026-01-15", "2026-01-23"},
		{"noTrip", Rule{Limits: Limits{MaxInPeriod: 90, PeriodDays: 180}}, nil, "2026-01-15", "2026-01-15", "2026-01-15"},
		{"replaceFuturePlans", Rule{Limits: Limits{MaxInPeriod: 90, PeriodDays: 180}}, []Trip{{Entry: "2026-01-01", Exit: "2026-02-01"}, {Entry: "2027-01-01"}}, "2026-01-15", "2026-01-15", "2026-07-14"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := fullResetDate(tc.trips, &tc.rule, d(tc.departure), d(tc.ref))
			if got != d(tc.want) {
				t.Fatalf("got %s want %s", fd(got), tc.want)
			}
		})
	}
}

func TestFullStaySuppressesShortOptionsBeforeAllDaysExpire(t *testing.T) {
	rule := &Rule{Limits: Limits{MaxInPeriod: 90, PeriodDays: 180}}
	trips := []Trip{{Entry: "2026-01-01", Exit: "2026-03-31"}}
	if !firstEntryStayAvailable(trips, rule, d("2026-03-31"), d("2026-06-30")) {
		t.Fatal("full 90-day stay already possible as old days expire")
	}
	if firstEntryStayAvailable(trips, rule, d("2026-03-31"), d("2026-06-29")) {
		t.Fatal("full stay is still blocked")
	}
	for _, tc := range []struct {
		ref   string
		short bool
	}{{"2026-04-01", true}, {"2026-06-30", false}, {"2026-09-28", false}} {
		r := evaluate(trips, rule.Limits, 0, d(tc.ref), rule)
		var b strings.Builder
		returnPlans(&b, trips, rule, r, nil)
		if strings.Contains(b.String(), "месяц (30 дней)") != tc.short {
			t.Fatal(b.String())
		}
		if !strings.Contains(b.String(), "# Полное обнуление дней:") {
			t.Fatal(b.String())
		}
	}
}
