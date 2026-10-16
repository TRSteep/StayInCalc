package main

import (
	"time"
)

// Дни считаются включительно: и день въезда, и день выезда — дни пребывания
// (так считают Шенген, РФ и большинство стран).

const horizonDays = 3 * 366 // насколько далеко вперёд ищем дату выезда

func dayNum(t time.Time) int { return int(t.Unix() / 86400) }
func dayTime(d int) time.Time {
	return time.Unix(int64(d)*86400, 0).UTC()
}

// presence — множество дней присутствия в виде массива со смещением.
type presence struct {
	off     int
	days    []bool
	entries []bool // дни въезда (пересечения границы)
}

func newPresence(from, to int) *presence {
	return &presence{off: from, days: make([]bool, to-from+1), entries: make([]bool, to-from+1)}
}

func (p *presence) markEntry(d int) {
	if i := d - p.off; i >= 0 && i < len(p.entries) {
		p.entries[i] = true
	}
}

func (p *presence) isEntry(d int) bool {
	i := d - p.off
	return i >= 0 && i < len(p.entries) && p.entries[i]
}

func (p *presence) set(d int, v bool) {
	if i := d - p.off; i >= 0 && i < len(p.days) {
		p.days[i] = v
	}
}

func (p *presence) has(d int) bool {
	i := d - p.off
	return i >= 0 && i < len(p.days) && p.days[i]
}

func (p *presence) count(from, to int) int {
	n := 0
	for d := from; d <= to; d++ {
		if p.has(d) {
			n++
		}
	}
	return n
}

// consecutive — длина текущего пребывания, заканчивающегося днём d.
// resetGap — сколько полных дней надо провести вне страны, чтобы счётчик обнулился:
// 0 — достаточно пересечь границу (выезд и въезд в один день начинают срок заново);
// отсутствие короче resetGap не прерывает пребывание, дни вне страны при этом не засчитываются.
func (p *presence) consecutive(d, resetGap int) (count, start int) {
	if !p.has(d) {
		return 0, d
	}
	gap := 0
	start = d
	for x := d; x >= p.off; x-- {
		if p.has(x) {
			count++
			start = x
			gap = 0
			if resetGap == 0 && p.isEntry(x) {
				break
			}
			continue
		}
		gap++
		if gap >= max(resetGap, 1) {
			break
		}
	}
	return count, start
}

func yearWindow(d int, mode string) (from, to int) {
	if mode == "rolling" {
		return d - 364, d
	}
	t := dayTime(d)
	return dayNum(time.Date(t.Year(), 1, 1, 0, 0, 0, 0, time.UTC)), d
}

type Usage struct {
	Consecutive int
	InPeriod    int
	InYear      int
}

func (p *presence) usage(d int, lim Limits, resetGap int) Usage {
	var u Usage
	u.Consecutive, _ = p.consecutive(d, resetGap)
	if lim.PeriodDays > 0 {
		u.InPeriod = p.count(d-lim.PeriodDays+1, d)
	}
	yf, yt := yearWindow(d, lim.YearMode)
	u.InYear = p.count(yf, yt)
	return u
}

// broken возвращает описание нарушенного ограничения на день d (пусто — всё в порядке).
func (p *presence) broken(d int, lim Limits, resetGap int) string {
	u := p.usage(d, lim, resetGap)
	switch {
	case lim.MaxConsecutive > 0 && u.Consecutive > lim.MaxConsecutive:
		return "максимум дней подряд"
	case lim.MaxInPeriod > 0 && u.InPeriod > lim.MaxInPeriod:
		return "лимит дней за период"
	case lim.MaxPerYear > 0 && u.InYear > lim.MaxPerYear:
		return "лимит дней в году"
	}
	return ""
}

func (l Limits) unlimited() bool {
	return l.MaxConsecutive == 0 && l.MaxInPeriod == 0 && l.MaxPerYear == 0
}

type Violation struct {
	Date   time.Time
	Reason string
}

type Result struct {
	Ref          time.Time
	InCountry    bool
	CurrentEntry time.Time
	PlannedExit  *time.Time // запланированная дата выезда текущей поездки, если указана

	Used      Usage // на дату расчёта
	Remaining Usage // остаток по каждому лимиту на дату расчёта (-1 — не ограничено)

	Unlimited     bool
	LatestExit    time.Time // последний допустимый день пребывания
	LatestReason  string    // какое ограничение сработает
	Overstay      bool      // лимит уже превышен на дату расчёта
	EarliestEntry *time.Time
	StayFromEarly time.Time // при въезде в EarliestEntry можно находиться до

	EntriesInYear int
	Violations    []Violation

	// Сколько дней старых поездок выпадет из скользящего периода до LatestExit —
	// поэтому фактический остаток может быть больше, чем остаток «на сегодня».
	DroppedFromPeriod int
}

// DaysLeft — сколько ещё дней после даты расчёта можно находиться в стране.
func (r Result) DaysLeft() int {
	return dayNum(r.LatestExit) - dayNum(r.Ref)
}

func remaining(max, used int) int {
	if max == 0 {
		return -1
	}
	return max - used
}

// Evaluate считает пребывание по списку поездок на дату ref.
func Evaluate(trips []Trip, lim Limits, resetGap int, ref time.Time) Result {
	refD := dayNum(ref)
	res := Result{Ref: ref, Unlimited: lim.unlimited()}

	type iv struct {
		from, to int
		open     bool
	}
	var ivs []iv
	minD, maxD := refD, refD
	for _, t := range trips {
		en, ok := parseISO(t.Entry)
		if !ok {
			continue
		}
		e := dayNum(en)
		x, open := e, true
		if ex, ok := parseISO(t.Exit); ok {
			x, open = dayNum(ex), false
			if x < e {
				continue
			}
		}
		if open {
			// открытая поездка: считаем, что человек в стране как минимум до даты расчёта
			if refD > x {
				x = refD
			}
		}
		ivs = append(ivs, iv{e, x, open})
		if e < minD {
			minD = e
		}
		if x > maxD {
			maxD = x
		}
		if e <= refD && (open || x >= refD) {
			res.InCountry = true
			res.CurrentEntry = en
			if !open {
				pe := dayTime(x)
				res.PlannedExit = &pe
			}
		}
	}

	lo := minD - 400
	hi := maxD + horizonDays + 400

	// Все известные дни (включая запланированные) — для поиска нарушений.
	all := newPresence(lo, hi)
	for _, v := range ivs {
		all.markEntry(v.from)
		for d := v.from; d <= v.to; d++ {
			all.set(d, true)
		}
	}
	for d := minD; d <= maxD; d++ {
		if all.has(d) {
			if why := all.broken(d, lim, resetGap); why != "" {
				if len(res.Violations) == 0 || res.Violations[len(res.Violations)-1].Reason != why ||
					!all.has(d-1) {
					res.Violations = append(res.Violations, Violation{dayTime(d), why})
				}
			}
		}
	}

	// Въезды в текущем календарном году (для лимита визаранов).
	ry := ref.Year()
	for _, v := range ivs {
		if dayTime(v.from).Year() == ry && v.from <= refD {
			res.EntriesInYear++
		}
	}

	// История до даты расчёта.
	past := newPresence(lo, hi)
	for _, v := range ivs {
		if v.from <= refD {
			past.markEntry(v.from)
		}
		for d := v.from; d <= v.to && d < refD; d++ {
			past.set(d, true)
		}
	}

	// Использовано на дату расчёта.
	probe := clonePresence(past)
	if res.InCountry {
		probe.set(refD, true)
		res.Used = probe.usage(refD, lim, resetGap)
	} else {
		res.Used = probe.usage(refD-1, lim, resetGap)
		res.Used.Consecutive = 0
		if lim.PeriodDays > 0 {
			res.Used.InPeriod = probe.count(refD-lim.PeriodDays+1, refD)
		}
		yf, yt := yearWindow(refD, lim.YearMode)
		res.Used.InYear = probe.count(yf, yt)
	}
	res.Remaining = Usage{
		Consecutive: remaining(lim.MaxConsecutive, res.Used.Consecutive),
		InPeriod:    remaining(lim.MaxInPeriod, res.Used.InPeriod),
		InYear:      remaining(lim.MaxPerYear, res.Used.InYear),
	}

	// Симуляция: остаёмся (или въезжаем) с даты start и смотрим, когда сработает лимит.
	simulate := func(start int) (last int, reason string, ok bool) {
		s := clonePresence(past)
		if !res.InCountry || start > refD {
			s.markEntry(start) // новый въезд
		}
		for d := start; d <= start+horizonDays; d++ {
			s.set(d, true)
			if why := s.broken(d, lim, resetGap); why != "" {
				return d - 1, why, d > start
			}
		}
		return start + horizonDays, "", true
	}

	last, why, ok := simulate(refD)
	res.LatestExit, res.LatestReason = dayTime(last), why
	if ok && lim.PeriodDays > 0 {
		res.DroppedFromPeriod = past.count(refD-lim.PeriodDays+1, min(last-lim.PeriodDays, refD-1))
	}
	if !ok {
		if res.InCountry {
			res.Overstay = true
			// ищем последний законный день в текущей поездке
			cur := clonePresence(past)
			cur.set(refD, true)
			for d := dayNum(res.CurrentEntry); d <= refD; d++ {
				if cur.broken(d, lim, resetGap) != "" {
					res.LatestExit = dayTime(d - 1)
					break
				}
			}
		}
		// ищем ближайшую дату, когда можно (снова) въехать
		for d := refD + 1; d <= refD+horizonDays; d++ {
			if l, _, ok := simulate(d); ok {
				e := dayTime(d)
				res.EarliestEntry = &e
				res.StayFromEarly = dayTime(l)
				break
			}
		}
	}
	return res
}

func clonePresence(p *presence) *presence {
	c := &presence{off: p.off, days: make([]bool, len(p.days)), entries: make([]bool, len(p.entries))}
	copy(c.days, p.days)
	copy(c.entries, p.entries)
	return c
}
