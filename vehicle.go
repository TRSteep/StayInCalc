package main

import "sort"

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
