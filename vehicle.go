package main

import (
	"sort"
	"time"
)

const vehicleCountryUnknown = "Не указана"

var extraVehicleCountries = map[string]string{
	"DE": "Германия", "FR": "Франция", "IT": "Италия", "ES": "Испания",
	"PL": "Польша", "LT": "Литва", "LV": "Латвия", "EE": "Эстония", "FI": "Финляндия",
	"GB": "Великобритания", "US": "США", "CH": "Швейцария", "NL": "Нидерланды",
	"BG": "Болгария", "RO": "Румыния", "ME": "Черногория", "MK": "Северная Македония",
}

func vehicleCountryLabel(code string) string {
	if code == "" {
		return vehicleCountryUnknown
	}
	if name, ok := extraVehicleCountries[code]; ok {
		return name + " (" + code + ")"
	}
	return citizenshipLabel(code)
}

func (u *UI) vehicleCountryOptions() []string {
	codes := map[string]bool{}
	for code := range citizenshipNames {
		codes[code] = true
	}
	for code := range extraVehicleCountries {
		codes[code] = true
	}
	for _, c := range u.countries.Countries {
		if len(c.ID) == 2 {
			codes[c.ID] = true
		}
	}
	for _, p := range u.profiles {
		if p.VehicleRegistrationCountry != "" {
			codes[p.VehicleRegistrationCountry] = true
		}
	}
	labels := []string{}
	for code, include := range codes {
		if include && code != "RU" {
			labels = append(labels, vehicleCountryLabel(code))
		}
	}
	sort.Strings(labels)
	return append([]string{vehicleCountryUnknown, vehicleCountryLabel("RU")}, labels...)
}

func (v VehicleRule) forRegistration(code string) (VehicleRule, int) {
	policy, ok := v.ByRegistration[code]
	if !ok {
		return v, 0
	}
	v.Limits, v.Manual, v.Note, v.Source = policy.Limits, false, policy.Note, policy.Source
	return v, policy.MinDaysOutsideAfterLimit
}

func vehicleStayDeadline(entry time.Time, lim Limits) time.Time {
	var deadline time.Time
	if lim.MaxConsecutiveMonths > 0 {
		deadline = entry.AddDate(0, lim.MaxConsecutiveMonths, 0).AddDate(0, 0, -1)
	}
	if lim.MaxConsecutive > 0 {
		d := entry.AddDate(0, 0, lim.MaxConsecutive-1)
		if deadline.IsZero() || d.Before(deadline) {
			deadline = d
		}
	}
	return deadline
}

// vehicleReentry учитывает непрерывный вывоз после использования всего срока.
// Нарушающий перерыв въезд не начинает новый законный год. Для открытой поездки
// дата возвращения предполагает выезд на дату расчёта.
func vehicleReentry(trips []Trip, lim Limits, minOutside int, ref time.Time) (invalidEntry, earliest time.Time, history []Violation) {
	if minOutside <= 0 || validateTrips(trips) != nil {
		return
	}
	ordered := append([]Trip(nil), trips...)
	sort.SliceStable(ordered, func(i, j int) bool { return ordered[i].Entry < ordered[j].Entry })
	var gate time.Time
	for _, trip := range ordered {
		entry, ok := parseISO(trip.Entry)
		if !ok || entry.After(ref) {
			continue
		}
		exit, hasExit := parseISO(trip.Exit)
		active := !hasExit || !exit.Before(ref)
		if !gate.IsZero() && entry.Before(gate) {
			history = append(history, Violation{Date: entry, Reason: "повторный въезд автомобиля до окончания обязательного вывоза"})
			if active {
				return entry, ref.AddDate(0, 0, minOutside+1), history
			}
			gate = exit.AddDate(0, 0, minOutside+1)
			continue
		}
		gate = time.Time{}
		deadline := vehicleStayDeadline(entry, lim)
		if deadline.IsZero() {
			continue
		}
		if active {
			if !ref.Before(deadline) {
				return time.Time{}, ref.AddDate(0, 0, minOutside+1), history
			}
			return
		}
		if !exit.Before(deadline) {
			gate = exit.AddDate(0, 0, minOutside+1)
		}
	}
	if gate.After(ref) {
		earliest = gate
	}
	return
}
