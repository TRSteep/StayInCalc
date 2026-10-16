package main

import (
	"testing"
	"time"
)

func d(s string) time.Time { t, _ := time.Parse(dateISO, s); return t }

func TestSchengen90of180(t *testing.T) {
	lim := Limits{MaxConsecutive: 90, MaxInPeriod: 90, PeriodDays: 180}
	// 60 дней в январе-феврале, сейчас въехал 1 мая
	trips := []Trip{
		{Entry: "2026-01-01", Exit: "2026-03-01"}, // 60 дней
		{Entry: "2026-05-01"},
	}
	r := Evaluate(trips, lim, 1, d("2026-05-01"))
	if !r.InCountry {
		t.Fatal("должен быть в стране")
	}
	if r.Used.InPeriod != 61 {
		t.Fatalf("использовано за период %d, ожидалось 61", r.Used.InPeriod)
	}
	// 30 дней осталось на 1 мая; январские дни начинают «сгорать» с 30 июня,
	// поэтому последний день — 30 мая (1 мая + 29).
	if got := r.LatestExit.Format(dateISO); got != "2026-05-30" {
		t.Fatalf("последний день %s, ожидалось 2026-05-30", got)
	}
}

func TestConsecutiveAndVisaRun(t *testing.T) {
	lim := Limits{MaxConsecutive: 30}
	trips := []Trip{
		{Entry: "2026-01-01", Exit: "2026-01-30"}, // 30 дней
		{Entry: "2026-02-01"},                     // визаран: 31 января вне страны
	}
	// Визаран разрешён: достаточно 1 дня вне страны
	r := Evaluate(trips, lim, 1, d("2026-02-01"))
	if r.Overstay {
		t.Fatal("с разрешённым визараном нарушения быть не должно")
	}
	if got := r.LatestExit.Format(dateISO); got != "2026-03-02" {
		t.Fatalf("последний день %s, ожидалось 2026-03-02", got)
	}
	// Визаран не обнуляет счётчик, пока не проведено 30 дней вне страны
	r = Evaluate(trips, lim, 30, d("2026-02-01"))
	if !r.Overstay {
		t.Fatal("ожидалось превышение")
	}
	if r.EarliestEntry == nil || r.EarliestEntry.Format(dateISO) != "2026-03-02" {
		t.Fatalf("ближайший въезд %v, ожидалось 2026-03-02", r.EarliestEntry)
	}
}

func TestCalendarYear(t *testing.T) {
	lim := Limits{MaxPerYear: 183, YearMode: "calendar"}
	trips := []Trip{{Entry: "2026-01-01", Exit: "2026-06-01"}} // 152 дня
	r := Evaluate(trips, lim, 1, d("2026-09-01"))
	if r.InCountry || r.Remaining.InYear != 31 {
		t.Fatalf("остаток в году %d, ожидалось 31", r.Remaining.InYear)
	}
	if got := r.LatestExit.Format(dateISO); got != "2026-10-01" {
		t.Fatalf("при въезде 1 сентября последний день %s, ожидалось 2026-10-01", got)
	}
}

func TestDefaultCountriesValid(t *testing.T) {
	if _, err := ParseCountries(defaultCountriesJSON); err != nil {
		t.Fatal(err)
	}
}

// Выезд и въезд в один день: при resetGap=0 срок «подряд» начинается с нового въезда.
func TestSameDayCrossingResets(t *testing.T) {
	lim := Limits{MaxConsecutive: 365}
	trips := []Trip{
		{Entry: "2026-03-01", Exit: "2026-03-12"},
		{Entry: "2026-06-01", Exit: "2026-07-29"},
		{Entry: "2026-07-29"},
	}
	r := Evaluate(trips, lim, 0, d("2026-08-02"))
	if r.Used.Consecutive != 5 {
		t.Fatalf("подряд %d, ожидалось 5 (с 29.07 по 02.08)", r.Used.Consecutive)
	}
	if got := r.LatestExit.Format(dateISO); got != "2027-07-28" {
		t.Fatalf("последний день %s, ожидалось 2027-07-28", got)
	}
	// Если визаран не обнуляет счётчик (нужен хотя бы 1 день вне страны) — считается с 01.06
	r = Evaluate(trips, lim, 1, d("2026-08-02"))
	if r.Used.Consecutive != 63 {
		t.Fatalf("подряд %d, ожидалось 63", r.Used.Consecutive)
	}
}
