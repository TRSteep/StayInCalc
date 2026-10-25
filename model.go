package main

import (
	_ "embed"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"
)

// ---------- Правила по странам (data/countries.json) ----------

//go:embed countries_default.json
var defaultCountriesJSON []byte

type CountriesFile struct {
	Version   string    `json:"version"`
	Comment   string    `json:"comment,omitempty"`
	Countries []Country `json:"countries"`
}

// Country — страна или зона с общим счётчиком дней (например, Шенгенская зона).
type Country struct {
	ID    string `json:"id"`
	Name  string `json:"name"`
	Rules []Rule `json:"rules"`
}

// Rule — правила для группы гражданств. Применяется первое правило,
// в списке citizenships которого есть гражданство профиля или "*".
type Rule struct {
	Citizenships []string `json:"citizenships"`
	VisaRequired bool     `json:"visa_required"`
	VisaNote     string   `json:"visa_note,omitempty"`

	Limits

	VisaRun      VisaRun      `json:"visa_run"`
	Registration Registration `json:"registration"`
	Vehicle      VehicleRule  `json:"vehicle"`

	Source  string       `json:"source,omitempty"`
	Updated string       `json:"updated,omitempty"`
	Notes   string       `json:"notes,omitempty"`
	History []RulePeriod `json:"history,omitempty"`
}

// RulePeriod сохраняет условия, предоставленные при въезде до смены режима.
type RulePeriod struct {
	Before       string       `json:"before"`
	Limits       Limits       `json:"limits"`
	Registration Registration `json:"registration"`
}

func (r *Rule) ForEntry(entry string) *Rule {
	copy := *r
	for _, period := range r.History {
		if entry < period.Before {
			copy.Limits, copy.Registration = period.Limits, period.Registration
			break
		}
	}
	return &copy
}

// Limits — числовые ограничения. 0 означает «не ограничено».
type Limits struct {
	MaxConsecutiveMonths int    `json:"max_consecutive_months,omitempty"`
	MaxConsecutive       int    `json:"max_consecutive"` // макс. дней подряд за один въезд
	MaxInPeriod          int    `json:"max_in_period"`   // макс. дней в скользящем периоде...
	PeriodDays           int    `json:"period_days"`     // ...длиной period_days (например, 90 из 180)
	MaxPerYear           int    `json:"max_per_year"`    // макс. дней в году
	YearMode             string `json:"year_mode"`       // "calendar" — календарный год, "rolling" — последние 365 дней
}

type VisaRun struct {
	Allowed        bool   `json:"allowed"`          // обнуляет ли выезд-въезд счётчик «подряд»
	MinDaysOutside int    `json:"min_days_outside"` // сколько дней нужно провести вне страны, чтобы счётчик обнулился
	MaxPerYear     int    `json:"max_per_year"`     // макс. число въездов в календарный год (0 — без ограничений)
	Note           string `json:"note,omitempty"`
}

type Registration struct {
	Manual      bool   `json:"manual,omitempty"`
	Required    bool   `json:"required"`
	WithinDays  int    `json:"within_days"`  // в течение скольких дней после въезда
	ExtendsStay bool   `json:"extends_stay"` // увеличивает ли регистрация срок пребывания
	Extended    Limits `json:"extended"`     // лимиты при наличии регистрации (0 — берётся базовый лимит)
	Note        string `json:"note,omitempty"`
}

type VehicleRule struct {
	Limits
	Note   string `json:"note,omitempty"`
	Manual bool   `json:"manual,omitempty"`
	Source string `json:"source,omitempty"`
}

func (c *Country) RuleFor(citizenship string) (*Rule, bool) {
	cz := strings.ToUpper(strings.TrimSpace(citizenship))
	for i := range c.Rules {
		for _, x := range c.Rules[i].Citizenships {
			x = strings.ToUpper(strings.TrimSpace(x))
			if x == cz && cz != "" {
				return &c.Rules[i], true
			}
		}
	}
	for i := range c.Rules {
		for _, x := range c.Rules[i].Citizenships {
			if strings.TrimSpace(x) == "*" {
				return &c.Rules[i], true
			}
		}
	}
	return nil, false
}

// Merge накладывает ненулевые значения o поверх l.
func (l Limits) Merge(o Limits) Limits {
	if o.MaxConsecutiveMonths > 0 {
		l.MaxConsecutiveMonths = o.MaxConsecutiveMonths
	}
	if o.MaxConsecutive > 0 {
		l.MaxConsecutive = o.MaxConsecutive
	}
	if o.MaxInPeriod > 0 {
		l.MaxInPeriod = o.MaxInPeriod
	}
	if o.PeriodDays > 0 {
		l.PeriodDays = o.PeriodDays
	}
	if o.MaxPerYear > 0 {
		l.MaxPerYear = o.MaxPerYear
	}
	if o.YearMode != "" {
		l.YearMode = o.YearMode
	}
	return l
}

func ParseCountries(data []byte) (*CountriesFile, error) {
	var cf CountriesFile
	if err := json.Unmarshal(data, &cf); err != nil {
		return nil, fmt.Errorf("ошибка JSON: %w", err)
	}
	seen := map[string]bool{}
	for i, c := range cf.Countries {
		if c.ID == "" {
			return nil, fmt.Errorf("страна №%d: пустой id", i+1)
		}
		if seen[c.ID] {
			return nil, fmt.Errorf("повторяющийся id страны: %s", c.ID)
		}
		seen[c.ID] = true
		if len(c.Rules) == 0 {
			return nil, fmt.Errorf("страна %s: нет ни одного правила", c.ID)
		}
		for j, r := range c.Rules {
			previous := ""
			for _, period := range r.History {
				if _, ok := parseISO(period.Before); !ok || (previous != "" && period.Before <= previous) {
					return nil, fmt.Errorf("страна %s: history.before должны быть корректными датами по возрастанию", c.ID)
				}
				previous = period.Before
				if err := validateLimits(period.Limits); err != nil {
					return nil, fmt.Errorf("страна %s, история: %w", c.ID, err)
				}
				if err := validateLimits(period.Limits.Merge(period.Registration.Extended)); err != nil {
					return nil, err
				}
			}
			if len(r.Citizenships) == 0 {
				return nil, fmt.Errorf("страна %s, правило №%d: пустой список citizenships", c.ID, j+1)
			}
			if r.VisaRun.MinDaysOutside < 0 || r.VisaRun.MaxPerYear < 0 || r.Registration.WithinDays < 0 {
				return nil, fmt.Errorf("страна %s, правило №%d: сроки и лимиты не могут быть отрицательными", c.ID, j+1)
			}
			for _, lim := range []Limits{r.Limits, r.Vehicle.Limits, r.Limits.Merge(r.Registration.Extended)} {
				if err := validateLimits(lim); err != nil {
					return nil, fmt.Errorf("страна %s, правило №%d: %w", c.ID, j+1, err)
				}
			}
			ext := r.Registration.Extended
			if ext.MaxConsecutiveMonths < 0 || ext.MaxConsecutive < 0 || ext.MaxInPeriod < 0 || ext.PeriodDays < 0 || ext.MaxPerYear < 0 {
				return nil, fmt.Errorf("страна %s: лимиты продления не могут быть отрицательными", c.ID)
			}
			for _, ym := range []string{r.YearMode, r.Vehicle.YearMode, r.Registration.Extended.YearMode} {
				if ym != "" && ym != "calendar" && ym != "rolling" {
					return nil, fmt.Errorf("страна %s: year_mode должен быть \"calendar\" или \"rolling\"", c.ID)
				}
			}
		}
	}
	return &cf, nil
}

func validateLimits(l Limits) error {
	if l.YearMode != "" && l.YearMode != "calendar" && l.YearMode != "rolling" {
		return errors.New("некорректный year_mode")
	}
	if l.MaxConsecutiveMonths < 0 || l.MaxConsecutive < 0 || l.MaxInPeriod < 0 || l.PeriodDays < 0 || l.MaxPerYear < 0 {
		return errors.New("лимиты не могут быть отрицательными")
	}
	if (l.MaxInPeriod > 0) != (l.PeriodDays > 0) {
		return errors.New("max_in_period и period_days задаются вместе")
	}
	return nil
}

// ---------- Профили (data/profiles/*.json) ----------

// Trip — одно пребывание: даты в формате ГГГГ-ММ-ДД, пустой Exit — ещё в стране.
type Trip struct {
	Entry      string `json:"entry"`
	Exit       string `json:"exit"`
	Registered bool   `json:"registered,omitempty"`
	Note       string `json:"note,omitempty"`
}

type Profile struct {
	Name                       string `json:"name"`
	Citizenship                string `json:"citizenship"`                            // код ISO, например RU
	VehicleRegistrationCountry string `json:"vehicle_registration_country,omitempty"` // страна номеров, независимо от гражданства
	// Поездки и въезды машины, сгруппированные по id страны из countries.json.
	Trips       map[string][]Trip `json:"trips"`
	CarTrips    map[string][]Trip `json:"car_trips"`
	LastCountry string            `json:"last_country,omitempty"`

	file string
}

const dateISO = "2006-01-02"
const dateUI = "02.01.2006"

type Store struct {
	Dir string
}

func (s *Store) countriesPath() string { return filepath.Join(s.Dir, "countries.json") }
func (s *Store) profilesDir() string   { return filepath.Join(s.Dir, "profiles") }

type appState struct {
	LastProfile string `json:"last_profile"`
}

func (s *Store) LastProfile() (string, error) {
	b, err := os.ReadFile(filepath.Join(s.Dir, "settings.json"))
	if errors.Is(err, os.ErrNotExist) {
		return "", nil
	}
	if err != nil {
		return "", err
	}
	var state appState
	if err := json.Unmarshal(b, &state); err != nil {
		return "", err
	}
	if state.LastProfile == "" {
		return "", nil
	}
	if filepath.Base(state.LastProfile) != state.LastProfile {
		return "", errors.New("некорректное имя последнего профиля")
	}
	return filepath.Join(s.profilesDir(), state.LastProfile), nil
}

func (s *Store) SaveLastProfile(profileFile string) error {
	name := ""
	if profileFile != "" {
		name = filepath.Base(profileFile)
	}
	b, err := json.MarshalIndent(appState{LastProfile: name}, "", "  ")
	if err != nil {
		return err
	}
	return writeAtomic(filepath.Join(s.Dir, "settings.json"), b)
}

func (s *Store) Init() error {
	if err := os.MkdirAll(s.profilesDir(), 0o755); err != nil {
		return err
	}
	if _, err := os.Stat(s.countriesPath()); errors.Is(err, os.ErrNotExist) {
		return os.WriteFile(s.countriesPath(), defaultCountriesJSON, 0o644)
	}
	return nil
}

func (s *Store) LoadCountriesRaw() ([]byte, error) { return os.ReadFile(s.countriesPath()) }

func (s *Store) LoadCountries() (*CountriesFile, error) {
	b, err := s.LoadCountriesRaw()
	if err != nil {
		return nil, err
	}
	return ParseCountries(b)
}

func (s *Store) SaveCountriesRaw(b []byte) error {
	if _, err := ParseCountries(b); err != nil {
		return err
	}
	return writeAtomic(s.countriesPath(), b)
}

func (s *Store) ListProfiles() ([]*Profile, error) {
	entries, err := os.ReadDir(s.profilesDir())
	if err != nil {
		return nil, err
	}
	var out []*Profile
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".json") {
			continue
		}
		p, err := s.loadProfile(filepath.Join(s.profilesDir(), e.Name()))
		if err != nil {
			continue
		}
		out = append(out, p)
	}
	sort.Slice(out, func(i, j int) bool { return strings.ToLower(out[i].Name) < strings.ToLower(out[j].Name) })
	return out, nil
}

func (s *Store) loadProfile(path string) (*Profile, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var p Profile
	if err := json.Unmarshal(b, &p); err != nil {
		return nil, err
	}
	p.file = path
	p.normalize()
	return &p, nil
}

func (p *Profile) normalize() {
	p.VehicleRegistrationCountry = strings.ToUpper(strings.TrimSpace(p.VehicleRegistrationCountry))
	if p.Trips == nil {
		p.Trips = map[string][]Trip{}
	}
	if p.CarTrips == nil {
		p.CarTrips = map[string][]Trip{}
	}
}

var unsafeChars = regexp.MustCompile(`[^\p{L}\p{N}_-]+`)

func (s *Store) NewProfile(name, citizenship string) (*Profile, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return nil, errors.New("имя профиля не задано")
	}
	base := strings.Trim(unsafeChars.ReplaceAllString(name, "_"), "_")
	if base == "" {
		base = "profile"
	}
	path := filepath.Join(s.profilesDir(), base+".json")
	for i := 2; ; i++ {
		if _, err := os.Stat(path); errors.Is(err, os.ErrNotExist) {
			break
		}
		path = filepath.Join(s.profilesDir(), fmt.Sprintf("%s_%d.json", base, i))
	}
	p := &Profile{Name: name, Citizenship: strings.ToUpper(strings.TrimSpace(citizenship)), file: path}
	p.normalize()
	return p, s.SaveProfile(p)
}

func (s *Store) SaveProfile(p *Profile) error {
	clean := *p
	clean.Trips = compactTrips(p.Trips)
	clean.CarTrips = compactTrips(p.CarTrips)
	b, err := json.MarshalIndent(&clean, "", "  ")
	if err != nil {
		return err
	}
	return writeAtomic(p.file, b)
}

func (s *Store) DeleteProfile(p *Profile) error { return os.Remove(p.file) }

// compactTrips убирает пустые строки и сортирует поездки по дате въезда.
func compactTrips(m map[string][]Trip) map[string][]Trip {
	out := map[string][]Trip{}
	for k, list := range m {
		var l []Trip
		for _, t := range list {
			if t.Entry == "" && t.Exit == "" && t.Note == "" {
				continue
			}
			l = append(l, t)
		}
		sort.SliceStable(l, func(i, j int) bool { return l[i].Entry < l[j].Entry })
		if len(l) > 0 {
			out[k] = l
		}
	}
	return out
}

func writeAtomic(path string, b []byte) error {
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, b, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

// ---------- Даты ----------

func parseISO(s string) (time.Time, bool) {
	if s == "" {
		return time.Time{}, false
	}
	t, err := time.Parse(dateISO, s)
	return t, err == nil
}

// parseUserDate принимает ДД.ММ.ГГГГ (а также Д.М.ГГГГ и ГГГГ-ММ-ДД).
func parseUserDate(s string) (time.Time, bool) {
	s = strings.TrimSpace(s)
	for _, f := range []string{dateUI, "2.1.2006", dateISO} {
		if t, err := time.Parse(f, s); err == nil {
			return t, true
		}
	}
	return time.Time{}, false
}

func isoToUI(s string) string {
	if t, ok := parseISO(s); ok {
		return t.Format(dateUI)
	}
	return s
}

func today() time.Time {
	n := time.Now()
	return time.Date(n.Year(), n.Month(), n.Day(), 0, 0, 0, 0, time.UTC)
}
