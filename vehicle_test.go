package main

import (
	"strings"
	"testing"
)

func kazakhstanVehicleUI(t *testing.T, ref string, trips []Trip) *UI {
	t.Helper()
	cf, err := ParseCountries(defaultCountriesJSON)
	if err != nil {
		t.Fatal(err)
	}
	for i := range cf.Countries {
		if cf.Countries[i].ID == "KZ" {
			return &UI{country: &cf.Countries[i], refDate: d(ref), cur: &Profile{Name: "Тест", Citizenship: "RU", VehicleRegistrationCountry: "RU", Trips: map[string][]Trip{}, CarTrips: map[string][]Trip{"KZ": trips}}}
		}
	}
	t.Fatal("KZ missing")
	return nil
}

func TestKazakhstanRussianVehicleUsesCalendarYear(t *testing.T) {
	u := kazakhstanVehicleUI(t, "2024-06-29", []Trip{{Entry: "2023-07-01"}})
	report := u.report()
	car := strings.Split(report, "## Автомобиль")[1]
	if !strings.Contains(car, "# Выехать не позднее: 30.06.2024") || strings.Contains(car, "Автоматический расчёт пока не выполняется") {
		t.Fatal(car)
	}
	if !strings.Contains(car, "30 календарных дней") {
		t.Fatal("missing mandatory absence condition")
	}
}

func TestVehicleRequiredAbsenceAfterFullYear(t *testing.T) {
	lim := Limits{MaxConsecutiveMonths: 12}
	trips := []Trip{{Entry: "2023-07-01", Exit: "2024-06-30"}}
	bad, earliest, _ := vehicleReentry(trips, lim, 30, d("2024-07-01"))
	if !bad.IsZero() || earliest != d("2024-07-31") {
		t.Fatalf("full 30 days outside: %v %v", bad, earliest)
	}
	trips = append(trips, Trip{Entry: "2024-07-01"})
	bad, earliest, history := vehicleReentry(trips, lim, 30, d("2024-07-05"))
	if bad != d("2024-07-01") || earliest != d("2024-08-05") || len(history) != 1 {
		t.Fatalf("short reentry: %v %v %v", bad, earliest, history)
	}
	u := kazakhstanVehicleUI(t, "2024-07-05", trips)
	car := strings.Split(u.report(), "## Автомобиль")[1]
	if !strings.Contains(car, "Нарушен обязательный перерыв") || strings.Contains(car, "# Выехать не позднее: 30.06.2025") {
		t.Fatal(car)
	}
	trips[1].Entry = "2024-07-31"
	bad, earliest, _ = vehicleReentry(trips, lim, 30, d("2024-08-01"))
	if !bad.IsZero() || !earliest.IsZero() {
		t.Fatal("valid 30-day absence rejected")
	}
}

func TestVehicleShortTripDoesNotRequireThirtyDays(t *testing.T) {
	bad, earliest, _ := vehicleReentry([]Trip{{Entry: "2024-01-01", Exit: "2024-01-10"}, {Entry: "2024-01-10"}}, Limits{MaxConsecutiveMonths: 12}, 30, d("2024-01-15"))
	if !bad.IsZero() || !earliest.IsZero() {
		t.Fatal("30-day absence imposed before full use")
	}
}

func TestVehicleOverstayReturnRespectsAbsence(t *testing.T) {
	u := kazakhstanVehicleUI(t, "2024-07-01", []Trip{{Entry: "2023-07-01"}})
	car := strings.Split(u.report(), "## Автомобиль")[1]
	if !strings.Contains(car, "ПРЕВЫШЕНИЕ СРОКА") || !strings.Contains(car, "Ближайший возможный въезд: 01.08.2024") {
		t.Fatal(car)
	}
}
