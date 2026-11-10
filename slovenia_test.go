package main

import (
	"encoding/json"
	"strings"
	"testing"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/test"
	"fyne.io/fyne/v2/widget"
)

func TestSloveniaUsesExistingSchengenTripsAndKeepsCarSeparate(t *testing.T) {
	s := &Store{Dir: t.TempDir()}
	if err := s.Init(); err != nil {
		t.Fatal(err)
	}
	p, err := s.NewProfile("Турист", "RU")
	if err != nil {
		t.Fatal(err)
	}
	p.LastCountry = "SI"
	p.Trips["SCHENGEN"] = []Trip{{Entry: "2026-01-01", Exit: "2026-03-31"}}
	p.CarTrips["SI"] = []Trip{{Entry: "2026-01-10", Exit: "2026-01-20"}}
	p.CarTrips["SCHENGEN"] = []Trip{{Entry: "2025-01-01", Exit: "2025-01-05"}}
	if err := s.SaveProfile(p); err != nil {
		t.Fatal(err)
	}
	u := newUI(test.NewApp(), s)
	u.refEntry.SetText("2026.04.01")
	if u.country.ID != "SI" || u.tripKey(false) != "SCHENGEN" || u.tripKey(true) != "SI" {
		t.Fatal("wrong shared trip key")
	}
	report := u.report()
	for _, text := range []string{"Словения — Турист", "Виза: требуется", "**90** из 90", "Въезд на 2026.04.01 невозможен"} {
		if !strings.Contains(report, text) {
			t.Fatalf("missing %q: %s", text, report)
		}
	}
	// Edit the displayed date and verify it updates the canonical stored trip.
	var findEntry func(fyne.CanvasObject) *widget.Entry
	findEntry = func(object fyne.CanvasObject) *widget.Entry {
		if entry, ok := object.(*widget.Entry); ok && entry.Text == "2026.01.01" {
			return entry
		}
		if box, ok := object.(*fyne.Container); ok {
			for _, child := range box.Objects {
				if found := findEntry(child); found != nil {
					return found
				}
			}
		}
		return nil
	}
	entry := findEntry(u.tripRow(false, 0))
	if entry == nil {
		t.Fatal("saved Schengen entry not displayed in Slovenia")
	}
	entry.SetText("2026.01.02")
	stored, err := s.loadProfile(p.file)
	if err != nil {
		t.Fatal(err)
	}
	if stored.Trips["SCHENGEN"][0].Entry != "2026-01-02" || len(stored.Trips["SI"]) != 0 {
		t.Fatal("shared edit was copied to a separate country")
	}
	if stored.CarTrips["SI"][0].Entry != "2026-01-10" || stored.CarTrips["SCHENGEN"][0].Entry != "2025-01-01" {
		t.Fatal("personal edit changed car records")
	}
	u.countrySel.SetSelected("Шенгенская зона")
	if !strings.Contains(u.report(), "**89** из 90") {
		t.Fatal(u.report())
	}
	u.countrySel.SetSelected("Словения")
	u.copyTripsToCar()
	if u.cur.CarTrips["SI"][0].Entry != "2026-01-02" {
		t.Fatal("copy did not use shared personal trips")
	}
	if u.cur.CarTrips["SCHENGEN"][0].Entry != "2025-01-01" {
		t.Fatal("copy changed another country's car records")
	}
}

func TestStayGroupRejectsMissingSelfAndNestedGroups(t *testing.T) {
	for _, group := range []string{"MISSING", "SI", "SCHENGEN"} {
		cf, err := ParseCountries(defaultCountriesJSON)
		if err != nil {
			t.Fatal(err)
		}
		for i := range cf.Countries {
			if cf.Countries[i].ID == "SI" {
				cf.Countries[i].StayGroup = group
			}
			if group == "SCHENGEN" && cf.Countries[i].ID == "SCHENGEN" {
				cf.Countries[i].StayGroup = "SI"
			}
		}
		data, err := json.Marshal(cf)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := ParseCountries(data); err == nil {
			t.Fatalf("accepted invalid group %s", group)
		}
	}
}
