package main

import (
	"reflect"
	"strings"
	"testing"

	"fyne.io/fyne/v2/test"
)

func TestCountryAndProfileListsSortByName(t *testing.T) {
	s := &Store{Dir: t.TempDir()}
	if err := s.Init(); err != nil {
		t.Fatal(err)
	}
	var last *Profile
	for _, name := range []string{"Яна", "борис", "Ёлка", "Анна", "Елена"} {
		p, err := s.NewProfile(name, "RU")
		if err != nil {
			t.Fatal(err)
		}
		if name == "Яна" {
			last = p
		}
	}
	if err := s.SaveLastProfile(last.file); err != nil {
		t.Fatal(err)
	}
	u := newUI(test.NewApp(), s)
	if want := []string{"Анна", "борис", "Елена", "Ёлка", "Яна"}; !reflect.DeepEqual(u.profileSel.Options, want) {
		t.Fatalf("profile order: %v", u.profileSel.Options)
	}
	if u.cur.file != last.file {
		t.Fatal("sorting changed restored profile")
	}
	u.countrySel.SetSelected("Кыргызстан")
	if u.country.ID != "KG" {
		t.Fatal("sorted selection points to another country")
	}
	// Changing rule order must not change the dropdown order or selected country.
	u.applyCountries(&CountriesFile{Countries: []Country{
		{ID: "ZZ", Name: "Япония"}, {ID: "KG", Name: "Кыргызстан"}, {ID: "AL", Name: "Албания"},
	}})
	if want := []string{"Албания", "Кыргызстан", "Япония"}; !reflect.DeepEqual(u.countrySel.Options, want) {
		t.Fatalf("country order: %v", u.countrySel.Options)
	}
	if u.country.ID != "KG" || u.countrySel.Selected != "Кыргызстан" {
		t.Fatal("selected country lost after rules reload")
	}
	u.profileSel.SetSelected("борис")
	if u.cur.Name != "борис" {
		t.Fatal("sorted profile selected wrong data")
	}
}

func TestKyrgyzstanTouristLimitAndRegistration(t *testing.T) {
	rule := russianRules(t)["KG"]
	if rule.VisaRequired || rule.Registration.WithinDays != 30 || rule.Registration.ExtendsStay || rule.VisaRun.Allowed {
		t.Fatalf("incorrect KG conditions: %+v", rule)
	}
	trips := []Trip{{Entry: "2026-01-01", Exit: "2026-03-31", Registered: true}, {Entry: "2026-04-01", Registered: true}}
	r := evaluate(trips, rule.Limits, resetGap(rule), d("2026-04-01"), rule)
	if !r.Overstay || r.Used.InPeriod != 91 {
		t.Fatalf("visa run reset 90/180: %+v", r)
	}
	r = evaluate([]Trip{{Entry: "2026-01-01"}}, rule.Limits, resetGap(rule), d("2026-01-31"), rule)
	if len(r.Violations) == 0 || !strings.Contains(r.Violations[0].Reason, "регистрации") {
		t.Fatalf("missing registration warning: %+v", r)
	}
	if planReturn(nil, rule, d("2026-01-01"), d("2026-01-01"), 31, false) != nil || planReturn(nil, rule, d("2026-01-01"), d("2026-01-01"), 31, true) == nil {
		t.Fatal("long trip must require registration")
	}
}
