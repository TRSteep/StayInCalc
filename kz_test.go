package main

import (
	"strings"
	"testing"
	"time"

	"fyne.io/fyne/v2/test"
)

// Казахстан: без регистрации — 30 дней с въезда, с регистрацией — 90/180;
// машина — до 365 дней с последнего пересечения границы (въезд в день выезда обнуляет срок).
func TestKazakhstanReport(t *testing.T) {
	s := &Store{Dir: t.TempDir()}
	if err := s.Init(); err != nil {
		t.Fatal(err)
	}
	p, _ := s.NewProfile("Тест", "RU")
	p.LastCountry = "KZ"
	p.Trips["KZ"] = []Trip{
		{Entry: "2026-02-25", Exit: "2026-03-08"}, // 12 дней, выпадут из окна 180
		{Entry: "2026-06-07", Exit: "2026-07-08"},
		{Entry: "2026-07-08", Exit: "2026-08-05"},
		{Entry: "2026-08-05"},
	}
	p.CarTrips["KZ"] = []Trip{
		{Entry: "2026-02-25", Exit: "2026-03-08"},
		{Entry: "2026-06-07", Exit: "2026-08-05"},
		{Entry: "2026-08-05"},
	}
	s.SaveProfile(p)

	u := newUI(test.NewApp(), s)
	u.refDate = time.Date(2026, 8, 9, 0, 0, 0, 0, time.UTC)
	rep := u.report()
	for _, want := range []string{
		"**76** из 90, осталось **14**",           // на дату расчёта
		"# Выехать не позднее: 03.09.2026",        // дата в одной строке с подписью
		"С регистрацией — до 04.09.2026",          // 90/180 с учётом выпадающих дней
		"выпадут 12 дн.",                          // пояснение про окно 180
		"Дней подряд: 5 из 365, осталось **360**", // машина: с 05.08
	} {
		if !strings.Contains(rep, want) {
			t.Errorf("в отчёте нет %q\n%s", want, rep)
		}
	}
}
