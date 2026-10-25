package main

import (
	"strings"
	"testing"
)

func TestReturnPlansRollingWindow(t *testing.T) {
	rule := &Rule{Limits: Limits{MaxConsecutive: 90, MaxInPeriod: 90, PeriodDays: 180}, VisaRun: VisaRun{Allowed: true}}
	trips := []Trip{{Entry: "2026-01-01", Exit: "2026-03-31"}}
	for _, days := range []int{14, 30} {
		p := planReturn(trips, rule, d("2026-03-31"), d("2026-03-31"), days, false)
		if p == nil || p.Entry != d("2026-06-30") || p.OutsideDays != 90 || dayNum(p.Exit)-dayNum(p.Entry)+1 != days {
			t.Fatalf("%d days: %+v", days, p)
		}
	}
}

func TestReturnPlansBorderAndMinimumGap(t *testing.T) {
	rule := &Rule{Limits: Limits{MaxConsecutive: 30}, VisaRun: VisaRun{Allowed: true}}
	trips := []Trip{{Entry: "2026-01-01"}}
	p := planReturn(trips, rule, d("2026-01-30"), d("2026-01-20"), 14, false)
	if p == nil || p.Entry != d("2026-01-30") || p.OutsideDays != 0 {
		t.Fatalf("same day: %+v", p)
	}
	rule.VisaRun.MinDaysOutside = 7
	p = planReturn(trips, rule, d("2026-01-30"), d("2026-01-20"), 14, false)
	if p == nil || p.Entry != d("2026-02-07") || p.OutsideDays != 7 {
		t.Fatalf("minimum gap: %+v", p)
	}
}

func TestReturnPlansEntryLimitAndRegistration(t *testing.T) {
	rule := &Rule{VisaRun: VisaRun{Allowed: true, MaxPerYear: 1}, Registration: Registration{Required: true, WithinDays: 3}}
	trips := []Trip{{Entry: "2026-01-01", Exit: "2026-01-02", Registered: true}}
	if p := planReturn(trips, rule, d("2026-01-02"), d("2026-01-03"), 14, false); p != nil {
		t.Fatalf("needs registration: %+v", p)
	}
	p := planReturn(trips, rule, d("2026-01-02"), d("2026-01-03"), 14, true)
	if p == nil || p.Entry != d("2027-01-01") || !p.Registered {
		t.Fatalf("year reset: %+v", p)
	}
}

func TestReturnPlansRequireWholeRequestedStay(t *testing.T) {
	rule := &Rule{Limits: Limits{MaxConsecutive: 14}, VisaRun: VisaRun{Allowed: true}}
	trips := []Trip{{Entry: "2026-01-01", Exit: "2026-01-14"}}
	if p := planReturn(trips, rule, d("2026-01-14"), d("2026-01-14"), 30, false); p != nil {
		t.Fatalf("30-day stay impossible: %+v", p)
	}
	if p := planReturn(trips, rule, d("2026-01-14"), d("2026-01-14"), 14, false); p == nil {
		t.Fatal("14-day stay possible")
	}
}

func TestReturnAfterLongAbsence(t *testing.T) {
	rule := &Rule{Limits: Limits{MaxConsecutive: 30}, VisaRun: VisaRun{Allowed: true}}
	p := planReturn([]Trip{{Entry: "2020-01-01", Exit: "2020-01-02"}}, rule, d("2020-01-02"), d("2026-01-01"), 30, false)
	if p == nil || p.Entry != d("2026-01-01") {
		t.Fatalf("long absence: %+v", p)
	}
}

func TestReturnReportShowsEarlierConditionalExtension(t *testing.T) {
	rule := &Rule{Limits: Limits{MaxInPeriod: 30, PeriodDays: 180}, VisaRun: VisaRun{Allowed: true}, Registration: Registration{ExtendsStay: true, Extended: Limits{MaxInPeriod: 60}}}
	trips := []Trip{{Entry: "2026-01-01"}}
	r := evaluate(trips, rule.Limits, 0, d("2026-01-15"), rule)
	var b strings.Builder
	returnPlans(&b, trips, rule, r, nil)
	if !strings.Contains(b.String(), "Въезд **15.01.2026**") || !strings.Contains(b.String(), "потребуется регистрация / продление") {
		t.Fatal(b.String())
	}
}

func TestReportCardOrderAndOptionalCar(t *testing.T) {
	u := &UI{refDate: d("2026-01-15"), country: &Country{Name: "Тест", ID: "TEST", Rules: []Rule{{Citizenships: []string{"RU"}, Limits: Limits{MaxConsecutive: 30}, VisaRun: VisaRun{Allowed: true}}}}, cur: &Profile{Name: "Иван", Citizenship: "RU", Trips: map[string][]Trip{"TEST": {{Entry: "2026-01-01"}}}, CarTrips: map[string][]Trip{}}}
	report := u.report()
	if strings.Contains(report, "## Автомобиль") {
		t.Fatal("empty car card")
	}
	if !strings.Contains(report, "# Выехать не позднее: 30.01.2026") || !strings.Contains(report, "как при первом въезде") || strings.Contains(report, "месяц (30 дней)") {
		t.Fatal(report)
	}
	u.cur.CarTrips["TEST"] = []Trip{{Entry: "2026-01-01"}}
	report = u.report()
	last := -1
	for _, heading := range []string{"# Тест — Иван (RU)", "## Общая информация", "# Выехать не позднее:", "## Пребывание на", "## Автомобиль"} {
		i := strings.Index(report, heading)
		if i <= last {
			t.Fatalf("section order %s: %s", heading, report)
		}
		last = i
	}
}

func TestReturnScenarioStartsOnReferenceDate(t *testing.T) {
	rule := &Rule{Limits: Limits{MaxConsecutive: 30}, VisaRun: VisaRun{Allowed: true, MinDaysOutside: 7}}
	trips := []Trip{{Entry: "2026-01-01", Exit: "2026-01-25"}}
	r := evaluate(trips, rule.Limits, 7, d("2026-01-15"), rule)
	var b strings.Builder
	returnPlans(&b, trips, rule, r, nil)
	if !strings.Contains(b.String(), "Если выехать сегодня, **15.01.2026**") || !strings.Contains(b.String(), "Въезд **23.01.2026**") {
		t.Fatal(b.String())
	}
	if strings.Contains(b.String(), "Въезд **07.02.2026**") {
		t.Fatal("used deadline instead of reference date")
	}
}
