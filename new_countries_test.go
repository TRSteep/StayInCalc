package main

import (
	"strings"
	"testing"

	"fyne.io/fyne/v2/test"
)

func TestChinaCountsFromDayAfterEntry(t *testing.T) {
	rule := russianRules(t)["CN"]
	for _, tc := range []struct {
		ref  string
		used int
		over bool
	}{{"2026-10-01", 0, false}, {"2026-10-31", 30, false}, {"2026-11-01", 31, true}} {
		r := evaluate([]Trip{{Entry: "2026-10-01"}}, rule.Limits, resetGap(rule), d(tc.ref), rule)
		if r.Error != nil || r.Used.Consecutive != tc.used || r.Overstay != tc.over || r.LatestExit != d("2026-10-31") {
			t.Fatalf("%s: %+v", tc.ref, r)
		}
	}
	trips := []Trip{{Entry: "2026-10-01", Exit: "2026-10-31"}, {Entry: "2026-10-31"}}
	r := evaluate(trips, rule.Limits, resetGap(rule), d("2026-10-31"), rule)
	if r.LatestExit != d("2026-11-30") || r.Overstay {
		t.Fatalf("new entry did not restart 30 days: %+v", r)
	}
	if planReturn(nil, rule, d("2026-10-01"), d("2026-10-01"), 31, false) == nil || planReturn(nil, rule, d("2026-10-01"), d("2026-10-01"), 32, false) != nil {
		t.Fatal("return plan lost entry-day exclusion")
	}
}

func TestUzbekistanRegistrationReport(t *testing.T) {
	s := &Store{Dir: t.TempDir()}
	if err := s.Init(); err != nil {
		t.Fatal(err)
	}
	if _, err := s.NewProfile("Турист", "RU"); err != nil {
		t.Fatal(err)
	}
	u := newUI(test.NewApp(), s)
	u.refDate = d("2026-10-05")
	u.countrySel.SetSelected("Узбекистан")
	u.cur.Trips["UZ"] = []Trip{{Entry: "2026-10-01"}}
	report := u.report()
	if !strings.Contains(report, "Выехать не позднее: 15.10.2026") || strings.Contains(report, "Ограничений по сроку нет") {
		t.Fatal(report)
	}
	u.cur.Trips["UZ"][0].Registered = true
	report = u.report()
	if !strings.Contains(report, "до окончания действующей регистрации") || strings.Contains(report, "Ограничений по сроку нет") {
		t.Fatal(report)
	}
	rule := russianRules(t)["UZ"]
	if planReturn(nil, rule, d("2026-10-01"), d("2026-10-01"), 30, false) != nil || planReturn(nil, rule, d("2026-10-01"), d("2026-10-01"), 30, true) == nil {
		t.Fatal("long stay must require registration")
	}
}
