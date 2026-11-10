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

// returnStayEnd проверяет максимально допустимый срок для найденного въезда,
// а не только минимально запрошенные 14 дней. История обрывается днём выезда.
func returnStayEnd(trips []Trip, rule *Rule, departure time.Time, plan *ReturnPlan) (time.Time, bool) {
	entryRule := rule.ForEntry(plan.Entry.Format(dateISO))
	lim := entryRule.Limits
	if plan.Registered && entryRule.Registration.ExtendsStay {
		lim = lim.Merge(entryRule.Registration.Extended)
	}
	reg := entryRule.Registration
	if lim.unlimited() && (plan.Registered || !reg.Required || reg.WithinDays == 0) {
		return time.Time{}, true
	}
	first := dayNum(plan.Entry)
	for _, trip := range trips {
		if entry, ok := parseISO(trip.Entry); ok && !entry.After(departure) {
			first = min(first, dayNum(entry))
		}
	}
	probe := newPresence(first-1, dayNum(plan.Entry)+horizonDays)
	for _, trip := range trips {
		entry, ok := parseISO(trip.Entry)
		if !ok || entry.After(departure) {
			continue
		}
		end := departure
		if exit, ok := parseISO(trip.Exit); ok && exit.Before(end) {
			end = exit
		}
		probe.markEntry(dayNum(entry))
		for day := dayNum(entry); day <= dayNum(end); day++ {
			probe.set(day, true)
		}
	}
	start := dayNum(plan.Entry)
	probe.markEntry(start)
	for day := start; day <= start+horizonDays; day++ {
		if !plan.Registered && reg.Required && reg.WithinDays > 0 && day-start >= reg.WithinDays {
			return dayTime(day - 1), false
		}
		probe.set(day, true)
		if probe.broken(day, lim, resetGap(entryRule)) != "" {
			return dayTime(day - 1), false
		}
	}
	return dayTime(start + horizonDays), true
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
	b.WriteString("\n---\n\n## Возвращение\n\n")
	if r.InCountry {
		fmt.Fprintf(b, "Если выехать сегодня, **%s**, и не совершать других поездок:\n\n", fd(departure))
	} else {
		fmt.Fprintf(b, "Вы вне страны. Расчёт следующей поездки начиная с **%s**:\n\n", fd(r.Ref))
	}
	plan := planReturn(trips, rule, departure, r.Ref, 14, false)
	if rule.Registration.Required || rule.Registration.ExtendsStay {
		extended := planReturn(trips, rule, departure, r.Ref, 14, true)
		if extended != nil && (plan == nil || extended.Entry.Before(plan.Entry)) {
			plan = extended
		}
	}
	if plan == nil {
		b.WriteString("**Ближайший возможный въезд на 2 недели:** подходящая дата в пределах трёх лет не найдена.\n\n")
	} else {
		fmt.Fprintf(b, "# Ближайший возможный въезд на 2 недели: %s\n\n", fd(plan.Entry))
		end, limited := returnStayEnd(trips, rule, departure, plan)
		switch {
		case end.IsZero():
			b.WriteString("Точная дата выезда не определяется заданными числовыми лимитами; соблюдайте срок регистрации и разрешительных документов.\n\n")
		case limited:
			fmt.Fprintf(b, "Можно провести не менее двух недель. В пределах расчёта до **%s** предел срока не найден.\n\n", fd(end))
		default:
			fmt.Fprintf(b, "# Пребывание возможно до: %s\n\n", fd(end))
			fmt.Fprintf(b, "Включительно **%d дн.** Это максимально допустимый непрерывный срок по заданным правилам, а не только две недели.\n\n", dayNum(end)-dayNum(plan.Entry)+1)
		}
		fmt.Fprintf(b, "Полных дней вне страны перед этим въездом: **%d**.\n\n", plan.OutsideDays)
		if plan.Registered {
			b.WriteString("Для этого варианта потребуется регистрация / продление.\n\n")
		}
	}
	reset := fullResetDate(trips, rule, departure, r.Ref)
	fmt.Fprintf(b, "# Полное обнуление дней: %s\n\n", fd(reset))
	if !reset.After(r.Ref) {
		b.WriteString("Старые дни уже не ограничивают новый въезд по заданным счётчикам.\n\n")
	} else {
		b.WriteString("Все дни прежних поездок выйдут из учитываемых периодов к этой дате, **если до неё оставаться вне страны**. При въезде раньше начнут учитываться дни новой поездки — нулевого счётчика в эту дату уже не будет.\n\n")
	}
	if rule.Registration.Required || rule.Registration.ExtendsStay {
		b.WriteString("Условия регистрации сохраняются; прежнее продление не переносится на новый въезд.\n\n")
	}
	if rule.VisaRequired {
		b.WriteString("Для въезда нужна действующая виза или применимое основание освобождения.\n\n")
	}
}
