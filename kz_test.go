package main

import (
	"strings"
	"testing"
	"time"

	"fyne.io/fyne/v2/test"
)

// Для РФ действует 90/180. Уведомление не сокращает срок до 30 дней,
// а условия допуска автомобиля нельзя определить только по датам поездок.
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
		"**76** из 90, осталось **14**",    // на дату расчёта
		"# Выехать не позднее: 04.09.2026", // 90/180 с учётом выпадающих дней
		"выпадут 12 дн.",                   // пояснение про окно 180
		"Срок автомобиля требует проверки по документам ввоза",
	} {
		if !strings.Contains(rep, want) {
			t.Errorf("в отчёте нет %q\n%s", want, rep)
		}
	}
	for _, wrong := range []string{"С регистрацией — до", "срок без регистрации — 30", "Дней подряд: 5 из 365"} {
		if strings.Contains(rep, wrong) {
			t.Errorf("устаревшее правило %q\n%s", wrong, rep)
		}
	}
}
