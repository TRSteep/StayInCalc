package main

import (
	"os"
	"strings"
	"testing"

	"fyne.io/fyne/v2/test"
)

func russianRules(t *testing.T) map[string]*Rule {
	t.Helper()
	cf, err := ParseCountries(defaultCountriesJSON)
	if err != nil {
		t.Fatal(err)
	}
	rules := map[string]*Rule{}
	for i := range cf.Countries {
		r, ok := cf.Countries[i].RuleFor("RU")
		if !ok {
			t.Fatalf("no RU rule for %s", cf.Countries[i].ID)
		}
		if r.Source == "" || r.Updated != "2026-10-25" {
			t.Fatalf("missing audit information for %s", cf.Countries[i].ID)
		}
		rules[cf.Countries[i].ID] = r
	}
	return rules
}

func TestRussianRulesAndAlbania(t *testing.T) {
	rules := russianRules(t)
	if len(rules) != 12 {
		t.Fatalf("countries: %d", len(rules))
	}
	if !rules["AL"].VisaRequired || rules["AL"].MaxInPeriod != 90 || rules["AL"].PeriodDays != 180 {
		t.Fatal("Albania requires a visa/valid exemption and 90/180")
	}
	if rules["KZ"].Registration.WithinDays != 0 || rules["KZ"].Registration.ExtendsStay {
		t.Fatal("arrival notification must not change 90/180")
	}
	if rules["RS"].Registration.WithinDays != 0 || rules["RS"].MaxConsecutive != 30 {
		t.Fatal("registration is not a one-day stay cap")
	}
	live, err := os.ReadFile("data/countries.json")
	if err != nil {
		t.Fatal(err)
	}
	if string(live) != string(defaultCountriesJSON) {
		t.Fatal("shipped and embedded rule sets differ")
	}
}

func TestThaiRulesByEntryDate(t *testing.T) {
	rule := russianRules(t)["TH"]
	for _, tc := range []struct {
		entry string
		days  int
	}{{"2024-04-01", 90}, {"2026-09-14", 60}, {"2026-09-15", 30}} {
		r := rule.ForEntry(tc.entry)
		if r.MaxConsecutive != tc.days {
			t.Fatalf("%s: %d", tc.entry, r.MaxConsecutive)
		}
	}
	// Старый въезд продолжает использовать предоставленные 60 дней после реформы.
	trips := []Trip{{Entry: "2026-09-14"}}
	r := evaluate(trips, rule.ForEntry(trips[0].Entry).Limits, 0, d("2026-10-25"), rule)
	if r.Overstay || r.LatestExit != d("2026-11-12") || len(r.Violations) != 0 {
		t.Fatalf("old entry: %+v", r)
	}
	r = evaluate([]Trip{{Entry: "2026-09-15"}}, rule.Limits, 0, d("2026-10-25"), rule)
	if !r.Overstay || r.LatestExit != d("2026-10-14") {
		t.Fatalf("new entry: %+v", r)
	}
	// Продление текущей старой поездки не переносится на новый въезд.
	plan := planReturn(trips, rule, d("2026-10-25"), d("2026-10-25"), 31, true)
	if plan != nil {
		t.Fatalf("new 30-day entry inherited old extension: %+v", plan)
	}
}

func TestGeorgiaCalendarYearAndSchengenWindow(t *testing.T) {
	rules := russianRules(t)
	r := Evaluate([]Trip{{Entry: "2023-07-01"}}, rules["GE"].Limits, 0, d("2024-06-29"))
	if r.LatestExit != d("2024-06-30") || r.Overstay {
		t.Fatalf("calendar year: %+v", r)
	}
	// 90 дней истрачены, после 90 полных дней вне страны дни начинают выпадать.
	r = Evaluate([]Trip{{Entry: "2026-01-01", Exit: "2026-03-31"}, {Entry: "2026-06-30"}}, rules["SCHENGEN"].Limits, 180, d("2026-06-30"))
	if r.Overstay || r.LatestExit != d("2026-09-27") {
		t.Fatalf("unnecessary consecutive cap: %+v", r)
	}
}

func TestCitizenshipSelectPreservesCodes(t *testing.T) {
	s := &Store{Dir: t.TempDir()}
	if err := s.Init(); err != nil {
		t.Fatal(err)
	}
	p, err := s.NewProfile("Тест", "ZZ")
	if err != nil {
		t.Fatal(err)
	}
	u := newUI(test.NewApp(), s)
	if u.citizenship.Selected != "ZZ" {
		t.Fatal("existing custom citizenship lost")
	}
	u.citizenship.SetSelected(citizenshipLabel("RU"))
	if u.cur.Citizenship != "RU" {
		t.Fatal("display label saved instead of ISO code")
	}
	loaded, err := s.loadProfile(p.file)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.Citizenship != "RU" {
		t.Fatal("selected citizenship was not saved")
	}
	u.countrySel.SetSelected("Албания")
	if !strings.Contains(u.report(), "Виза: требуется") {
		t.Fatal(u.report())
	}
}

func TestVehicleRegistrationIsIndependentAndSaved(t *testing.T) {
	s := &Store{Dir: t.TempDir()}
	if err := s.Init(); err != nil {
		t.Fatal(err)
	}
	p, err := s.NewProfile("Водитель", "RU")
	if err != nil {
		t.Fatal(err)
	}
	u := newUI(test.NewApp(), s)
	if u.vehicleCountry.Selected != vehicleCountryUnknown {
		t.Fatal("guessed vehicle country from citizenship")
	}
	u.vehicleCountry.SetSelected(vehicleCountryLabel("BY"))
	stored, err := s.loadProfile(p.file)
	if err != nil {
		t.Fatal(err)
	}
	if stored.VehicleRegistrationCountry != "BY" || stored.Citizenship != "RU" {
		t.Fatalf("independent values were not saved: %+v", stored)
	}
	u.reloadProfiles(p.file)
	if u.vehicleCountry.Selected != vehicleCountryLabel("BY") {
		t.Fatal("vehicle country was not restored")
	}
	u.countrySel.SetSelected("Турция")
	u.cur.CarTrips["TR"] = []Trip{{Entry: "2026-10-01"}}
	u.vehicleCountry.SetSelected(vehicleCountryLabel("TR"))
	report := u.report()
	if !strings.Contains(report, "Местная регистрация автомобиля") || strings.Contains(report, "Срок туристического ввоза зависит") {
		t.Fatal(report)
	}
	u.vehicleCountry.SetSelected(vehicleCountryUnknown)
	if u.cur.VehicleRegistrationCountry != "" {
		t.Fatal("could not clear vehicle registration")
	}
}
