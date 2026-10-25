package main

import (
	"encoding/json"
	"strings"
	"testing"

	"fyne.io/fyne/v2/test"
)

func TestRulesRejectInvalidLimits(t *testing.T) {
	cases := []Rule{
		{Limits: Limits{MaxConsecutive: -1}},
		{Vehicle: VehicleRule{Limits: Limits{MaxInPeriod: 90}}},
		{Registration: Registration{Extended: Limits{MaxInPeriod: 90}}},
		{Registration: Registration{Extended: Limits{MaxConsecutive: -1}}},
		{VisaRun: VisaRun{MaxPerYear: -1}},
	}
	for i, rule := range cases {
		rule.Citizenships = []string{"RU"}
		b, err := json.Marshal(CountriesFile{Countries: []Country{{ID: "TEST", Rules: []Rule{rule}}}})
		if err != nil {
			t.Fatal(err)
		}
		if _, err = ParseCountries(b); err == nil {
			t.Errorf("invalid rule %d accepted", i)
		}
	}
	// Продление может менять лишь лимит, наследуя длину периода.
	rule := Rule{Citizenships: []string{"RU"}, Limits: Limits{MaxInPeriod: 30, PeriodDays: 180}, Registration: Registration{Extended: Limits{MaxInPeriod: 90}}}
	b, _ := json.Marshal(CountriesFile{Countries: []Country{{ID: "TEST", Rules: []Rule{rule}}}})
	if _, err := ParseCountries(b); err != nil {
		t.Fatal(err)
	}
}

func TestInvalidTripsStopForecast(t *testing.T) {
	cases := map[string][]Trip{
		"reversed":        {{Entry: "2026-02-02", Exit: "2026-02-01"}},
		"invalid entry":   {{Entry: "31.02.2026"}},
		"invalid exit":    {{Entry: "2026-01-01", Exit: "2026-02-30"}},
		"missing entry":   {{Exit: "2026-01-01"}},
		"open overlap":    {{Entry: "2026-01-01"}, {Entry: "2026-02-01"}},
		"closed overlap":  {{Entry: "2026-01-01", Exit: "2026-02-05"}, {Entry: "2026-02-01"}},
		"duplicate entry": {{Entry: "2026-01-01", Exit: "2026-01-01"}, {Entry: "2026-01-01"}},
	}
	for name, trips := range cases {
		t.Run(name, func(t *testing.T) {
			r := Evaluate(trips, Limits{MaxConsecutive: 30}, 0, d("2026-02-02"))
			if r.Error == nil || !r.LatestExit.IsZero() {
				t.Fatalf("expected error without forecast: %+v", r)
			}
			var b strings.Builder
			stayBlock(&b, r, Limits{}, "вы", nil)
			if !strings.Contains(b.String(), "Ошибка данных") || strings.Contains(b.String(), "Выехать не позднее") {
				t.Fatal(b.String())
			}
		})
	}
}

func TestTripOrderAndSameDayBorder(t *testing.T) {
	trips := []Trip{{Entry: "2026-02-01"}, {}, {Entry: "2026-01-01", Exit: "2026-02-01"}}
	r := Evaluate(trips, Limits{MaxConsecutive: 30}, 0, d("2026-02-02"))
	if r.Error != nil || r.Used.Consecutive != 2 || r.CurrentEntry != d("2026-02-01") {
		t.Fatalf("%+v", r)
	}
	if trips[0].Entry != "2026-02-01" {
		t.Fatal("input mutated")
	}
}

func TestRegistrationHistoryUsesEachTrip(t *testing.T) {
	rule := &Rule{Limits: Limits{MaxConsecutive: 60}, VisaRun: VisaRun{Allowed: true}, Registration: Registration{ExtendsStay: true, Extended: Limits{MaxConsecutive: 90}}}
	trips := []Trip{{Entry: "2026-01-01", Exit: "2026-03-31", Registered: true}, {Entry: "2026-06-01"}}
	r := evaluate(trips, rule.Limits, 0, d("2026-06-02"), rule)
	if len(r.Violations) != 0 {
		t.Fatalf("extended historical trip flagged: %+v", r.Violations)
	}
	trips[0].Registered, trips[1].Registered = false, true
	r = evaluate(trips, rule.Limits.Merge(rule.Registration.Extended), 0, d("2026-06-02"), rule)
	if len(r.Violations) != 1 || r.Violations[0].Date != d("2026-03-02") {
		t.Fatalf("current extension hid historical violation: %+v", r.Violations)
	}
}

func TestHistoricalRegistrationDeadline(t *testing.T) {
	rule := &Rule{Registration: Registration{Required: true, WithinDays: 3}, VisaRun: VisaRun{Allowed: true}}
	trips := []Trip{{Entry: "2026-01-01", Exit: "2026-01-05"}, {Entry: "2026-02-01", Registered: true}}
	r := evaluate(trips, Limits{}, 0, d("2026-02-02"), rule)
	if len(r.Violations) != 1 || r.Violations[0].Date != d("2026-01-04") || r.Violations[0].Reason != "срок без регистрации" {
		t.Fatalf("%+v", r.Violations)
	}
}

func TestEntryLimitAffectsForecast(t *testing.T) {
	rule := &Rule{Limits: Limits{MaxConsecutive: 30}, VisaRun: VisaRun{Allowed: true, MaxPerYear: 2}}
	trips := []Trip{{Entry: "2026-01-01", Exit: "2026-01-02"}, {Entry: "2026-02-01", Exit: "2026-02-02"}}
	r := evaluate(trips, rule.Limits, 0, d("2026-03-01"), rule)
	if !r.EntryBlocked || r.EarliestEntry == nil || *r.EarliestEntry != d("2027-01-01") || r.StayFromEarly != d("2027-01-30") {
		t.Fatalf("%+v", r)
	}
	trips = append(trips, Trip{Entry: "2026-03-01"})
	r = evaluate(trips, rule.Limits, 0, d("2026-03-02"), rule)
	if len(r.Violations) != 1 || r.Violations[0].Reason != "лимит въездов в году" {
		t.Fatalf("%+v", r.Violations)
	}
	if r.EntryBlocked {
		t.Fatal("continued stay treated as a new entry")
	}
}

func TestUnlimitedDaysDoNotBypassEntryLimit(t *testing.T) {
	rule := &Rule{VisaRun: VisaRun{Allowed: true, MaxPerYear: 1}}
	r := evaluate([]Trip{{Entry: "2026-01-01", Exit: "2026-01-02"}}, Limits{}, 0, d("2026-12-31"), rule)
	var b strings.Builder
	stayBlock(&b, r, Limits{}, "вы", nil)
	if !r.EntryBlocked || r.EarliestEntry == nil || *r.EarliestEntry != d("2027-01-01") || !strings.Contains(b.String(), "Въезд на 31.12.2026 невозможен") {
		t.Fatal(b.String())
	}
}

func TestNextEntryHasOwnRegistrationDeadline(t *testing.T) {
	rule := &Rule{VisaRun: VisaRun{Allowed: true, MaxPerYear: 1}, Registration: Registration{Required: true, WithinDays: 3}}
	r := evaluate([]Trip{{Entry: "2026-01-01", Exit: "2026-01-02", Registered: true}}, Limits{}, 0, d("2026-12-31"), rule)
	var b strings.Builder
	stayBlock(&b, r, Limits{}, "вы", nil)
	if r.EarliestEntry == nil || r.StayFromEarly != d("2027-01-03") || !strings.Contains(b.String(), "до 03.01.2027 (3 дн.)") {
		t.Fatal(b.String())
	}
}

func TestNextEntryDoesNotInheritExtension(t *testing.T) {
	rule := &Rule{Limits: Limits{MaxConsecutive: 30}, VisaRun: VisaRun{Allowed: true, MaxPerYear: 1}, Registration: Registration{ExtendsStay: true, Extended: Limits{MaxConsecutive: 60}}}
	r := evaluate([]Trip{{Entry: "2026-01-01", Registered: true}}, rule.Limits.Merge(rule.Registration.Extended), 0, d("2026-03-02"), rule)
	if !r.Overstay || r.EarliestEntry == nil || *r.EarliestEntry != d("2027-01-01") || r.StayFromEarly != d("2027-01-30") {
		t.Fatalf("%+v", r)
	}
}

func TestForecastHorizonIsNotExitDeadline(t *testing.T) {
	lim := Limits{MaxConsecutive: 2000}
	r := Evaluate(nil, lim, 0, d("2026-01-01"))
	var b strings.Builder
	stayBlock(&b, r, lim, "вы", nil)
	if !r.ForecastLimited || !strings.Contains(b.String(), "предел срока не найден") || strings.Contains(b.String(), "можно находиться до") {
		t.Fatal(b.String())
	}
	r = Evaluate(nil, Limits{MaxConsecutive: horizonDays}, 0, d("2026-01-01"))
	if r.ForecastLimited || r.DaysLeft()+1 != horizonDays {
		t.Fatalf("boundary: %+v", r)
	}
}

func TestDuplicateProfileSelection(t *testing.T) {
	s := &Store{Dir: t.TempDir()}
	if err := s.Init(); err != nil {
		t.Fatal(err)
	}
	p1, err := s.NewProfile("Иван", "RU")
	if err != nil {
		t.Fatal(err)
	}
	p2, err := s.NewProfile("Иван", "BY")
	if err != nil {
		t.Fatal(err)
	}
	u := newUI(test.NewApp(), s)
	if len(u.profileSel.Options) != 2 || u.profileSel.Options[0] == u.profileSel.Options[1] {
		t.Fatal(u.profileSel.Options)
	}
	u.reloadProfiles(p2.file)
	if u.cur.file != p2.file || u.citizenship.Selected != citizenshipLabel("BY") {
		t.Fatal("second profile not selected")
	}
	u.reloadProfiles(p1.file)
	if u.cur.file != p1.file || u.citizenship.Selected != citizenshipLabel("RU") {
		t.Fatal("first profile not selected")
	}
}

func TestRestoresLastOpenedProfile(t *testing.T) {
	s := &Store{Dir: t.TempDir()}
	if err := s.Init(); err != nil {
		t.Fatal(err)
	}
	first, err := s.NewProfile("Иван", "RU")
	if err != nil {
		t.Fatal(err)
	}
	second, err := s.NewProfile("Иван", "BY")
	if err != nil {
		t.Fatal(err)
	}
	u := newUI(test.NewApp(), s)
	u.reloadProfiles(second.file)
	reopened := newUI(test.NewApp(), s)
	if reopened.cur == nil || reopened.cur.file != second.file || reopened.citizenship.Selected != citizenshipLabel("BY") {
		t.Fatal("last selected profile was not restored")
	}
	if err := s.DeleteProfile(second); err != nil {
		t.Fatal(err)
	}
	fallback := newUI(test.NewApp(), s)
	if fallback.cur == nil || fallback.cur.file != first.file {
		t.Fatal("missing profile did not fall back to an existing one")
	}
	last, err := s.LastProfile()
	if err != nil || last != first.file {
		t.Fatalf("fallback not saved: %q, %v", last, err)
	}
}

func TestDateFieldDoesNotCalculateWithStaleInput(t *testing.T) {
	s := &Store{Dir: t.TempDir()}
	if err := s.Init(); err != nil {
		t.Fatal(err)
	}
	if _, err := s.NewProfile("Тест", "RU"); err != nil {
		t.Fatal(err)
	}
	u := newUI(test.NewApp(), s)
	_, entry := u.dateField("2026-01-01", func(raw string) { u.cur.Trips[u.country.ID] = []Trip{{Entry: raw}}; u.recalc() })
	entry.SetText("31.02.2026")
	if !strings.Contains(u.report(), "Ошибка данных") {
		t.Fatal(u.report())
	}
	entry.SetText("01.02.2026")
	if strings.Contains(u.report(), "Ошибка данных") {
		t.Fatal(u.report())
	}
	u.refEntry.SetText("")
	if !strings.Contains(u.report(), "Ошибка даты расчёта") {
		t.Fatal(u.report())
	}
}
