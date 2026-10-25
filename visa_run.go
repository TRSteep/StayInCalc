package main

import (
	"fmt"
	"strings"
)

// Возможность нового срока по правилам счётчика не равна разрешению пересечь границу.
// Условия повторного въезда берутся на дату расчёта, без продления прошлой поездки.
func visaRunBlock(b *strings.Builder, rule *Rule, r Result) {
	vr := rule.VisaRun
	perEntry := rule.MaxConsecutive > 0 || rule.MaxConsecutiveMonths > 0
	var total []string
	if rule.MaxInPeriod > 0 {
		total = append(total, fmt.Sprintf("%d дней за %d дней", rule.MaxInPeriod, rule.PeriodDays))
	}
	if rule.MaxPerYear > 0 {
		year := "календарный год"
		if rule.YearMode == "rolling" {
			year = "последние 365 дней"
		}
		total = append(total, fmt.Sprintf("%d дней за %s", rule.MaxPerYear, year))
	}
	switch {
	case rule.Limits.unlimited() && rule.Registration.Required:
		b.WriteString("**Визаран: для обнуления общего лимита дней не требуется.** Соблюдайте условия регистрации; новый въезд не заменяет её оформление. ")
	case vr.MaxPerYear > 0 && r.EntriesInYear >= vr.MaxPerYear:
		b.WriteString("**Визаран: новый въезд в этом году недоступен по лимиту въездов.** ")
	case vr.Allowed && perEntry:
		if len(total) > 0 {
			b.WriteString("**Визаран: возможен с ограничениями.** ")
		} else {
			b.WriteString("**Визаран: возможен по правилу срока за въезд.** ")
		}
		fmt.Fprintf(b, "Новый въезд начинает новый срок: %s. ", limitsText(Limits{MaxConsecutive: rule.MaxConsecutive, MaxConsecutiveMonths: rule.MaxConsecutiveMonths, ExcludeEntryDay: rule.ExcludeEntryDay}))
		if vr.MinDaysOutside > 0 {
			fmt.Fprintf(b, "Для обнуления счётчика нужно минимум **%d полных дней вне страны**. ", vr.MinDaysOutside)
		} else {
			b.WriteString("Для счётчика не требуется полный день вне страны. ")
		}
		if len(total) > 0 {
			fmt.Fprintf(b, "Лимит **%s** сохраняется; после его исчерпания короткий выезд дней не добавит. ", strings.Join(total, "; "))
		}
	case len(total) > 0:
		fmt.Fprintf(b, "**Визаран: не обнуляет общий лимит.** Сохраняется **%s**. Вернуться можно только при доступном остатке дней. ", strings.Join(total, "; "))
	case perEntry && vr.MinDaysOutside > 0:
		fmt.Fprintf(b, "**Визаран: только после перерыва.** Для нового срока нужно минимум **%d полных дней вне страны**. ", vr.MinDaysOutside)
	default:
		b.WriteString("**Визаран: возможность нового срока не подтверждена заданным правилом.** ")
	}
	if vr.MaxPerYear > 0 {
		fmt.Fprintf(b, "Не более **%d въездов в календарном году**. ", vr.MaxPerYear)
		if r.EntriesInYear >= vr.MaxPerYear {
			b.WriteString("**На дату расчёта годовой лимит въездов уже исчерпан.** ")
		}
	}
	if !r.Ref.IsZero() && ((rule.MaxInPeriod > 0 && r.Remaining.InPeriod <= 0) || (rule.MaxPerYear > 0 && r.Remaining.InYear <= 0)) {
		b.WriteString("**На дату расчёта общий лимит дней исчерпан.** Даты следующей поездки смотрите ниже. ")
	}
	if rule.VisaRequired {
		b.WriteString("Для повторного въезда нужна действующая виза с доступным въездом либо применимое основание освобождения. ")
	}
	b.WriteString("Окончательное решение о допуске принимает пограничная служба.\n\n")
	if vr.Note != "" {
		fmt.Fprintf(b, "_%s_\n\n", vr.Note)
	}
}
