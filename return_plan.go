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
	lim := rule.Limits
	if registered && rule.Registration.ExtendsStay {
		lim = lim.Merge(rule.Registration.Extended)
	}
	if validateLimits(lim) != nil {
		return nil
	}
	if !registered && rule.Registration.Required && rule.Registration.WithinDays > 0 && target > rule.Registration.WithinDays {
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

func returnPlans(b *strings.Builder, trips []Trip, rule *Rule, r Result, cp *Cap) {
	if r.Error != nil || (!r.EntryBlocked && !r.Overstay && (r.Unlimited || r.ForecastLimited) && cp == nil) {
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
	b.WriteString("### Возвращение на 14 дней или месяц\n\n")
	if r.InCountry {
		fmt.Fprintf(b, "Если выехать сегодня, **%s**, и не совершать других поездок:\n\n", fd(departure))
	} else {
		fmt.Fprintf(b, "Вы вне страны. Варианты въезда начиная с **%s**:\n\n", fd(r.Ref))
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
		fmt.Fprintf(b, "**На %s** — %s.\n\nВъезд **%s** → пребывание до **%s**", label, gap, fd(plan.Entry), fd(plan.Exit))
		if plan.Registered {
			b.WriteString("; потребуется регистрация / продление")
		}
		b.WriteString(".\n\n")
	}
	b.WriteString("\n")
}
