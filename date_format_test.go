package main

import (
	"testing"

	"fyne.io/fyne/v2/test"
)

func TestYearFirstDateInputAndDisplay(t *testing.T) {
	for _, input := range []string{"2026.12.31", "2026.12.31 ", "31.12.2026", "2026-12-31"} {
		date, ok := parseUserDate(input)
		if !ok || date != d("2026-12-31") || fd(date) != "2026.12.31" {
			t.Fatalf("input %q: %v, %v", input, date, ok)
		}
	}
	for _, input := range []string{"2026.02.31", "2026.13.01", "2026.12.00"} {
		if _, ok := parseUserDate(input); ok {
			t.Fatalf("accepted invalid date %q", input)
		}
	}
	if isoToUI("2026-12-31") != "2026.12.31" {
		t.Fatal("saved ISO date not displayed in new format")
	}
}

func TestDateNotesKeepURLsAndNormalizeLegacyDates(t *testing.T) {
	text := "С 31.12.2026 до 2027-01-15; https://example.com/2026-12-31"
	want := "С 2026.12.31 до 2027.01.15; https://example.com/2026-12-31"
	if got := displayDates(text); got != want {
		t.Fatalf("%q", got)
	}
}

func TestDateFieldRetainsISOStorageAndNormalizesSubmittedInput(t *testing.T) {
	a := test.NewApp()
	u := &UI{app: a, win: a.NewWindow("Dates")}
	var stored string
	_, entry := u.dateField("2026-12-31", func(iso string) { stored = iso })
	if entry.Text != "2026.12.31" || entry.PlaceHolder != "ГГГГ.ММ.ДД" {
		t.Fatal(entry.Text, entry.PlaceHolder)
	}
	entry.SetText("2027.01.03")
	if stored != "2027-01-03" {
		t.Fatalf("new input storage: %q", stored)
	}
	entry.SetText("31.12.2026")
	entry.OnSubmitted(entry.Text)
	if stored != "2026-12-31" || entry.Text != "2026.12.31" {
		t.Fatalf("legacy input: %q, %q", stored, entry.Text)
	}
}
