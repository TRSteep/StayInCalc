package main

import (
	"fmt"
	"strings"
	"time"
)

type ReturnPlan struct {
	Days        int
	Entry       time.Time
	Exit        time.Time
	OutsideDays int
	Registered  bool
}

// planReturn ищет первый въезд, позволяющий провести target дней подряд.
// Сценарий заменяет последующие планы: человек выезжает в departure,
// затем остаётся вне страны до найденной даты. Оба дня пересечения учитываются.
func planReturn(trips []Trip, rule *Rule, departure, notBefore time.Time, target int, registered bool) *ReturnPlan {
	if target < 1 || validateTrips(trips) != nil {
		return nil
	}
	end := dayNum(departure)
	searchStart := max(end, dayNum(notBefore))
	first := end
	type interval struct{ from, to int }
	var history []interval
	for _, trip := range trips {
		entry, ok := parseISO(trip.Entry)
		if !ok || dayNum(entry) > end {
			continue
		}
		from, to := dayNum(entry), end
		if exit, ok := parseISO(trip.Exit); ok {
			to = min(to, dayNum(exit))
		}
		history = append(history, interval{from, to})
		first = min(first, from)
	}
	past := newPresence(first-1, searchStart+horizonDays+target)
	for _, trip := range history {
		past.markEntry(trip.from)
		for day := trip.from; day <= trip.to; day++ {
			past.set(day, true)
		}
	}
	for entry := searchStart; entry <= searchStart+horizonDays; entry++ {
		entryRule := rule.ForEntry(dayTime(entry).Format(dateISO))
		lim := entryRule.Limits
		if registered && entryRule.Registration.ExtendsStay {
			lim = lim.Merge(entryRule.Registration.Extended)
		}
		if validateLimits(lim) != nil {
			return nil
		}
		if !registered && entryRule.Registration.Required && entryRule.Registration.WithinDays > 0 && target > entryRule.Registration.WithinDays {
			continue
		}
		if rule.VisaRun.MaxPerYear > 0 {
			count := 1 // планируемый въезд
			for _, trip := range history {
				if dayTime(trip.from).Year() == dayTime(entry).Year() {
					count++
				}
			}
			if count > rule.VisaRun.MaxPerYear {
				continue
			}
		}
		probe := clonePresence(past)
		probe.markEntry(entry)
		allowed := true
		for day := entry; day < entry+target; day++ {
			probe.set(day, true)
			if probe.broken(day, lim, resetGap(rule)) != "" {
				allowed = false
				break
			}
		}
		if allowed {
			return &ReturnPlan{target, dayTime(entry), dayTime(entry + target - 1), max(0, entry-end-1), registered}
		}
	}
	return nil
}

// fullResetDate — дата, к которой все дни прошлых поездок покинут учитываемые
// окна, а ограниченные годовые счётчики въездов обнулятся. Это строже, чем дата,
// когда уже можно провести полный срок: дни могут выпадать и во время поездки.
// Будущие планы заменяются сценарием выезда в departure и отсутствия до въезда.
func fullResetDate(trips []Trip, rule *Rule, departure, notBefore time.Time) time.Time {
	date := notBefore
	lim := rule.ForEntry(notBefore.Format(dateISO)).Limits
	later := func(candidate time.Time) {
		if candidate.After(date) {
			date = candidate
		}
	}
	var last time.Time
	for _, trip := range trips {
		entry, ok := parseISO(trip.Entry)
		if !ok || entry.After(departure) {
			continue
		}
		exit := departure
		if actual, ok := parseISO(trip.Exit); ok && actual.Before(exit) {
			exit = actual
		}
		if exit.After(last) {
			last = exit
		}
		if lim.MaxInPeriod > 0 {
			later(exit.AddDate(0, 0, lim.PeriodDays))
		}
		if lim.MaxPerYear > 0 {
			if lim.YearMode == "rolling" {
				later(exit.AddDate(0, 0, 365))
			} else {
				later(time.Date(exit.Year()+1, 1, 1, 0, 0, 0, 0, time.UTC))
			}
		}
		if rule.VisaRun.MaxPerYear > 0 {
			later(time.Date(entry.Year()+1, 1, 1, 0, 0, 0, 0, time.UTC))
		}
	}
	if !last.IsZero() {
		if rule.VisaRun.MinDaysOutside > 0 {
			later(last.AddDate(0, 0, rule.VisaRun.MinDaysOutside+1))
		} else {
			later(last) // повторный въезд в день выезда допустим для счётчика
		}
	}
	return date
}

// Сравниваем доступный непрерывный срок с новой поездкой без истории.
// Полный срок может стать доступен ещё до выпадения всех прошлых дней.
func firstEntryStayAvailable(trips []Trip, rule *Rule, departure, ref time.Time) bool {
	entryRule := rule.ForEntry(ref.Format(dateISO))
	if entryRule.Limits.unlimited() {
		return !fullResetDate(trips, rule, departure, ref).After(ref)
	}
	fresh := evaluate(nil, entryRule.Limits, resetGap(rule), ref, rule)
	if fresh.Error != nil || fresh.ForecastLimited || fresh.EntryBlocked {
		return false
	}
	target := dayNum(fresh.LatestExit) - dayNum(ref) + 1
	registered := entryRule.Registration.Required && entryRule.Registration.WithinDays > 0
	plan := planReturn(trips, rule, departure, ref, target, registered)
	return plan != nil && plan.Entry.Equal(ref)
}

func returnPlans(b *strings.Builder, trips []Trip, rule *Rule, r Result, cp *Cap) {
	if r.Error != nil {
		return
	}
	// «Сегодня» — выбранная дата расчёта, в том числе при планировании.
	departure := r.Ref
	if !r.InCountry {
		departure = r.Ref.AddDate(0, 0, -1)
		found := false
		for _, trip := range trips {
			if exit, ok := parseISO(trip.Exit); ok && exit.Before(r.Ref) {
				if !found || exit.After(departure) {
					departure = exit
					found = true
				}
			}
		}
	}
	b.WriteString("### Возвращение\n\n")
	if r.InCountry {
		fmt.Fprintf(b, "Если выехать сегодня, **%s**, и не совершать других поездок:\n\n", fd(departure))
	} else {
		fmt.Fprintf(b, "Вы вне страны. Варианты въезда начиная с **%s**:\n\n", fd(r.Ref))
	}
	reset := fullResetDate(trips, rule, departure, r.Ref)
	fullStay := firstEntryStayAvailable(trips, rule, departure, r.Ref)
	fmt.Fprintf(b, "# Полное обнуление дней: %s\n\n", fd(reset))
	if !reset.After(r.Ref) {
		b.WriteString("По заданным счётчикам лимиты пребывания уже действуют **как при первом въезде**.\n\n")
	} else {
		b.WriteString("К этой дате все дни прошлых поездок выйдут из учитываемых периодов, а ограниченные годовые счётчики въездов обнулятся. Лимиты будут действовать **как при первом въезде**, если до этой даты не совершать новых поездок. Доступные дни могут появляться постепенно раньше этой даты.\n\n")
		if fullStay {
			b.WriteString("Уже на дату расчёта новый въезд позволяет провести полный непрерывный срок **как при первом въезде**: старые дни успевают выпадать во время поездки.\n\n")
		}
	}
	if rule.Registration.Required || rule.Registration.ExtendsStay {
		b.WriteString("Условия регистрации и отдельного продления сохраняются; прежнее продление не переносится на новый въезд.\n\n")
	}
	if rule.VisaRequired {
		b.WriteString("Дата восстановления счётчиков не заменяет действующую визу или иное основание въезда.\n\n")
	}
	if fullStay {
		return
	}
	for _, target := range []int{14, 30} {
		label := "14 дней"
		if target == 30 {
			label = "месяц (30 дней)"
		}
		plan := planReturn(trips, rule, departure, r.Ref, target, false)
		if rule.Registration.Required || rule.Registration.ExtendsStay {
			extended := planReturn(trips, rule, departure, r.Ref, target, true)
			if extended != nil && (plan == nil || extended.Entry.Before(plan.Entry)) {
				plan = extended
			}
		}
		if plan == nil {
			fmt.Fprintf(b, "- **На %s:** подходящий въезд в пределах трёх лет не найден.\n", label)
			continue
		}
		gap := fmt.Sprintf("**%d** полных дн. вне страны", plan.OutsideDays)
		if plan.OutsideDays == 0 {
			gap = "без полных дней вне страны"
		}
		fmt.Fprintf(b, "- **На %s** — %s. Въезд **%s** → пребывание до **%s**", label, gap, fd(plan.Entry), fd(plan.Exit))
		if plan.Registered {
			b.WriteString("; потребуется регистрация / продление")
		}
		b.WriteString(".\n")
	}
	b.WriteString("\n")
}
