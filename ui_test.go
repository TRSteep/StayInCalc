package main

import (
	"image/png"
	"os"
	"testing"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/test"
)

func TestUISnapshot(t *testing.T) {
	out := os.Getenv("SNAPSHOT")
	if out == "" {
		t.Skip("SNAPSHOT не задан")
	}
	dir := t.TempDir()
	s := &Store{Dir: dir}
	if err := s.Init(); err != nil {
		t.Fatal(err)
	}
	p, _ := s.NewProfile("Иван", "RU")
	p.LastCountry = "TR"
	p.VehicleRegistrationCountry = "RU"
	p.Trips["TR"] = []Trip{{Entry: "2026-05-01", Exit: "2026-06-15"}, {Entry: "2026-09-20", Registered: false}}
	p.CarTrips["TR"] = []Trip{{Entry: "2026-05-01", Exit: "2026-06-15"}, {Entry: "2026-09-20"}}
	s.SaveProfile(p)
	s.NewProfile("Мария", "RU")

	a := test.NewApp()
	u := newUI(a, s)
	u.profileSel.SetSelected("Иван")
	refDate := os.Getenv("SNAPSHOT_DATE")
	if refDate == "" {
		refDate = "08.10.2026"
	}
	u.refEntry.SetText(refDate)
	u.win.Resize(fyne.NewSize(1250, 760))
	img := u.win.Canvas().Capture()
	f, _ := os.Create(out)
	defer f.Close()
	png.Encode(f, img)
	t.Log(u.report())
}
