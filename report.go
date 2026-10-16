package main

import (
	"fmt"
	"strings"
	"time"
)

func fd(t time.Time) string { return t.Format(dateUI) }

func yesNo(b bool) string {
	if b {
		return "да"
	}
	return "нет"
}

func limitsText(l Limits) string {
	var parts []string
	if l.MaxConsecutive > 0 {
		parts = append(parts, fmt.Sprintf("до %d дн. подряд", l.MaxConsecutive))
	}
	if l.MaxInPeriod > 0 {
		parts = append(parts, fmt.Sprintf("%d дн. из %d", l.MaxInPeriod, l.PeriodDays))
	}
	if l.MaxPerYear > 0 {
		y := "в календарном году"
		if l.YearMode == "rolling" {
			y = "за 365 дней"
		}
		parts = append(parts, fmt.Sprintf("%d дн. %s", l.MaxPerYear, y))
	}
	if len(parts) == 0 {
		return "без ограничений по сроку"
	}
	return strings.Join(parts, "; ")
}

func usageLine(b *strings.Builder, title string, used, max, rem int) {
	if max == 0 {
		fmt.Fprintf(b, "- %s: %d (не ограничено)\n", title, used)
		return
	}
	if rem < 0 {
		fmt.Fprintf(b, "- %s: **%d из %d — превышено на %d**\n", title, used, max, -rem)
		return
	}
	fmt.Fprintf(b, "- %s: %d из %d, осталось **%d**\n", title, used, max, rem)
}

// Cap — предел, не зависящий от счётчиков дней (например, срок без регистрации).
type Cap struct {
	Date   time.Time
	Reason string
	Alt    string // что будет, если предел снять
	Status string // дополнительная строка в блоке статуса
}

// stayBlock — общий вывод для человека и машины.
func stayBlock(b *strings.Builder, r Result, lim Limits, who string, cp *Cap) {
	ref := fd(r.Ref)
	if r.InCountry {
		n := dayNum(r.Ref) - dayNum(r.CurrentEntry) + 1
		fmt.Fprintf(b, "- Статус: %s в стране с %s (%d-й день поездки)\n", who, fd(r.CurrentEntry), n)
	} else {
		fmt.Fprintf(b, "- Статус: на %s %s вне страны\n", ref, who)
	}
	if r.InCountry && lim.MaxConsecutive > 0 {
		usageLine(b, "Дней подряд", r.Used.Consecutive, lim.MaxConsecutive, r.Remaining.Consecutive)
	}
	if lim.PeriodDays > 0 {
		usageLine(b, fmt.Sprintf("За последние %d дней", lim.PeriodDays), r.Used.InPeriod, lim.MaxInPeriod, r.Remaining.InPeriod)
	}
	yt := fmt.Sprintf("В %d году", r.Ref.Year())
	if lim.YearMode == "rolling" {
		yt = "За последние 365 дней"
	}
	if lim.MaxPerYear > 0 || lim.YearMode != "rolling" {
		usageLine(b, yt, r.Used.InYear, lim.MaxPerYear, r.Remaining.InYear)
	}
	if cp != nil && cp.Status != "" {
		b.WriteString(cp.Status)
	}
	b.WriteString("\n")

	if cp != nil && !r.Overstay && cp.Date.Before(r.LatestExit) {
		left := dayNum(cp.Date) - dayNum(r.Ref)
		switch {
		case left < 0:
			fmt.Fprintf(b, "**⚠ Истёк %s (%s).** Нужна регистрация или выезд.\n\n", cp.Reason, fd(cp.Date))
		case r.InCountry:
			fmt.Fprintf(b, "**Выехать не позднее: %s** — ещё %d дн. после %s (сработает: %s).\n\n",
				fd(cp.Date), left, ref, cp.Reason)
		default:
			fmt.Fprintf(b, "**При въезде %s можно находиться до %s** включительно (%d дн., сработает: %s).\n\n",
				ref, fd(cp.Date), left+1, cp.Reason)
		}
		fmt.Fprintf(b, "**%s — до %s** (ещё %d дн. после %s, сработает: %s).\n\n",
			cp.Alt, fd(r.LatestExit), r.DaysLeft(), ref, r.LatestReason)
		periodNote(b, r, lim)
		violations(b, r)
		return
	}

	switch {
	case r.Overstay:
		fmt.Fprintf(b, "**⚠ ПРЕВЫШЕНИЕ СРОКА** (%s). Последний законный день был %s.\n\n", r.LatestReason, fd(r.LatestExit))
	case r.Unlimited:
		b.WriteString("**Ограничений по сроку нет.**\n\n")
	case r.InCountry:
		fmt.Fprintf(b, "**Выехать не позднее: %s** — ещё %d дн. после %s (сработает: %s).\n\n",
			fd(r.LatestExit), r.DaysLeft(), ref, r.LatestReason)
		if r.PlannedExit != nil && r.PlannedExit.After(r.LatestExit) {
			fmt.Fprintf(b, "⚠ Запланированный выезд %s позже допустимого!\n\n", fd(*r.PlannedExit))
		}
	case r.EarliestEntry == nil:
		fmt.Fprintf(b, "**При въезде %s можно находиться до %s** включительно (%d дн., сработает: %s).\n\n",
			ref, fd(r.LatestExit), r.DaysLeft()+1, r.LatestReason)
	}
	if r.EarliestEntry != nil {
		fmt.Fprintf(b, "**Ближайший возможный въезд: %s**, тогда можно находиться до %s (%d дн.).\n\n",
			fd(*r.EarliestEntry), fd(r.StayFromEarly), dayNum(r.StayFromEarly)-dayNum(*r.EarliestEntry)+1)
	} else if !r.InCountry && !r.Unlimited && r.LatestExit.Before(r.Ref) {
		fmt.Fprintf(b, "Въезд на %s сейчас невозможен.\n\n", ref)
	}
	periodNote(b, r, lim)
	violations(b, r)
}

// periodNote поясняет, почему реальный остаток больше остатка «на сегодня»:
// пока вы в стране, дни старых поездок выпадают из скользящего окна.
func periodNote(b *strings.Builder, r Result, lim Limits) {
	if r.DroppedFromPeriod == 0 || r.Remaining.InPeriod < 0 || r.Overstay || r.DaysLeft() <= r.Remaining.InPeriod {
		return
	}
	fmt.Fprintf(b, "_Почему больше %d дн.: остаток %d из %d — это на %s. Пока вы в стране, из окна %d дней "+
		"выпадут %d дн. более ранних поездок, поэтому по правилу %d/%d можно оставаться до %s._\n\n",
		r.Remaining.InPeriod, r.Remaining.InPeriod, lim.MaxInPeriod, fd(r.Ref), lim.PeriodDays,
		r.DroppedFromPeriod, lim.MaxInPeriod, lim.PeriodDays, fd(r.LatestExit))
}

func violations(b *strings.Builder, r Result) {
	if len(r.Violations) > 0 {
		b.WriteString("Нарушения в истории/планах:\n\n")
		for _, v := range r.Violations {
			fmt.Fprintf(b, "- с %s — %s\n", fd(v.Date), v.Reason)
		}
		b.WriteString("\n")
	}
}

func (u *UI) report() string {
	var b strings.Builder
	if u.cur == nil {
		return "## Нет профиля\n\nНажмите «Новый», чтобы создать профиль. Для каждого члена семьи — отдельный профиль."
	}
	if u.country == nil {
		return "Выберите страну."
	}
	rule, ok := u.country.RuleFor(u.cur.Citizenship)
	fmt.Fprintf(&b, "## %s — %s", u.country.Name, u.cur.Name)
	if u.cur.Citizenship != "" {
		fmt.Fprintf(&b, " (%s)", u.cur.Citizenship)
	}
	b.WriteString("\n\n")
	if !ok {
		fmt.Fprintf(&b, "Для гражданства «%s» в countries.json нет правил. Добавьте правило с этим кодом или \"*\".\n", u.cur.Citizenship)
		return b.String()
	}

	// --- Правила ---
	b.WriteString("### Правила\n\n")
	if rule.VisaRequired {
		b.WriteString("- **Виза: требуется**")
	} else {
		b.WriteString("- Виза: не требуется")
	}
	if rule.VisaNote != "" {
		b.WriteString(" — " + rule.VisaNote)
	}
	b.WriteString("\n")
	fmt.Fprintf(&b, "- Срок: %s\n", limitsText(rule.Limits))

	vr := rule.VisaRun
	switch {
	case vr.Allowed && vr.MinDaysOutside == 0:
		b.WriteString("- Визаран: работает (можно въехать в день выезда)")
	case vr.Allowed:
		fmt.Fprintf(&b, "- Визаран: работает (вне страны минимум %d дн.)", vr.MinDaysOutside)
	case vr.MinDaysOutside > 0:
		fmt.Fprintf(&b, "- Визаран: не обнуляет срок, пока не проведено %d дн. вне страны", vr.MinDaysOutside)
	default:
		b.WriteString("- Визаран: не обнуляет срок")
	}
	if vr.MaxPerYear > 0 {
		fmt.Fprintf(&b, "; не более %d въездов в год", vr.MaxPerYear)
	}
	if vr.Note != "" {
		b.WriteString(" — " + vr.Note)
	}
	b.WriteString("\n")

	reg := rule.Registration
	if reg.Required {
		fmt.Fprintf(&b, "- Регистрация: **обязательна**")
		if reg.WithinDays > 0 {
			fmt.Fprintf(&b, " в течение %d дн. после въезда (без неё — не более %d дн. с даты въезда)", reg.WithinDays, reg.WithinDays)
		}
	} else {
		b.WriteString("- Регистрация: не требуется")
	}
	if reg.ExtendsStay {
		fmt.Fprintf(&b, "; продлевает срок → %s", limitsText(rule.Limits.Merge(reg.Extended)))
	} else {
		b.WriteString("; срок не увеличивает")
	}
	if reg.Note != "" {
		b.WriteString(" — " + reg.Note)
	}
	b.WriteString("\n")
	if rule.Notes != "" {
		fmt.Fprintf(&b, "- Примечание: %s\n", rule.Notes)
	}
	if rule.Updated != "" || rule.Source != "" {
		fmt.Fprintf(&b, "- Источник: %s %s\n", rule.Source, rule.Updated)
	}

	// --- Человек ---
	trips := u.cur.Trips[u.country.ID]
	lim := rule.Limits
	var curTrip *Trip
	refD := dayNum(u.refDate)
	for i := range trips {
		en, ok := parseISO(trips[i].Entry)
		if !ok || dayNum(en) > refD {
			continue
		}
		ex, hasEx := parseISO(trips[i].Exit)
		if !hasEx || dayNum(ex) >= refD {
			curTrip = &trips[i]
		}
	}
	registered := curTrip != nil && curTrip.Registered
	regLim := lim
	if reg.ExtendsStay {
		regLim = lim.Merge(reg.Extended)
	}
	if registered {
		lim = regLim
	}

	// Без регистрации можно находиться не более within_days с даты въезда;
	// дальше — по лимитам «с регистрацией».
	var regCap *Cap
	if reg.Required && reg.WithinDays > 0 && !registered {
		start := u.refDate // планируемый въезд
		if curTrip != nil {
			start, _ = parseISO(curTrip.Entry)
		}
		lim = regLim
		regCap = &Cap{
			Date:   start.AddDate(0, 0, reg.WithinDays-1),
			Reason: fmt.Sprintf("срок без регистрации — %d дн. с въезда %s", reg.WithinDays, fd(start)),
			Alt:    "С регистрацией",
		}
		if curTrip != nil {
			n := dayNum(u.refDate) - dayNum(start) + 1
			regCap.Status = fmt.Sprintf("- Без регистрации: %d-й день из %d, осталось **%d**\n",
				n, reg.WithinDays, max(0, reg.WithinDays-n))
		}
	}

	fmt.Fprintf(&b, "\n### Пребывание на %s\n\n", fd(u.refDate))
	if rule.VisaRequired {
		b.WriteString("_Нужна виза: срок ниже рассчитан по правилам страны; он не может превышать срок, указанный в визе._\n\n")
	}
	r := Evaluate(trips, lim, resetGap(rule), u.refDate)
	stayBlock(&b, r, lim, "вы", regCap)

	if curTrip != nil && reg.Required {
		switch {
		case registered:
			b.WriteString("- Регистрация: ✓ отмечена\n")
		case regCap == nil:
			b.WriteString("- **Регистрация не отмечена — требуется!**\n")
		case regCap.Date.Before(u.refDate):
			fmt.Fprintf(&b, "- **⚠ Регистрация просрочена** (нужно было до %s)\n", fd(regCap.Date))
		default:
			fmt.Fprintf(&b, "- **Зарегистрироваться до %s** — затем отметьте «Регистрация» у текущей поездки\n", fd(regCap.Date))
		}
	}
	if vr.MaxPerYear > 0 {
		mark := ""
		if r.EntriesInYear > vr.MaxPerYear {
			mark = " **⚠ превышено**"
		}
		fmt.Fprintf(&b, "- Въездов в %d году: %d из %d%s\n", u.refDate.Year(), r.EntriesInYear, vr.MaxPerYear, mark)
	}

	// --- Машина ---
	b.WriteString("\n### Автомобиль\n\n")
	vl := rule.Vehicle.Limits
	fmt.Fprintf(&b, "- Временный ввоз: %s", limitsText(vl))
	if rule.Vehicle.Note != "" {
		b.WriteString(" — " + rule.Vehicle.Note)
	}
	b.WriteString("\n")
	carTrips := u.cur.CarTrips[u.country.ID]
	hasCar := false
	for _, t := range carTrips {
		if t.Entry != "" {
			hasCar = true
		}
	}
	if !hasCar {
		b.WriteString("\nДаты въезда машины не указаны (вкладка «Автомобиль»).\n")
	} else {
		cr := Evaluate(carTrips, vl, 0, u.refDate) // срок машины идёт с каждого пересечения границы
		stayBlock(&b, cr, vl, "машина", nil)
	}
	return b.String()
}
