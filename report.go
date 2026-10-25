package main

import (
	"fmt"
	"net/url"
	"sort"
	"strings"
	"time"
)

func fd(t time.Time) string { return t.Format(dateUI) }

// Источник может содержать несколько URL через пробел или « ; ».
// Markdown-ссылки Fyne открывает системным браузером при нажатии.
func sourceLinks(source string) string {
	parts := strings.Fields(source)
	for i, part := range parts {
		u, err := url.Parse(part)
		if err == nil && (u.Scheme == "https" || u.Scheme == "http") && u.Hostname() != "" {
			parts[i] = fmt.Sprintf("[%s](<%s>)", u.Hostname(), u.String())
		}
	}
	return strings.Join(parts, " ")
}

func yesNo(b bool) string {
	if b {
		return "да"
	}
	return "нет"
}

func limitsText(l Limits) string {
	var parts []string
	if l.MaxConsecutiveMonths > 0 {
		parts = append(parts, fmt.Sprintf("до %d календарных мес. за въезд", l.MaxConsecutiveMonths))
	}
	if l.MaxConsecutive > 0 {
		parts = append(parts, fmt.Sprintf("до %d дн. подряд", l.MaxConsecutive))
		if l.ExcludeEntryDay {
			parts = append(parts, "со следующего дня после въезда")
		}
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
	if strings.HasPrefix(title, "За последние ") {
		fmt.Fprintf(b, "- %s: **%d** из %d, осталось **%d**\n", title, used, max, rem)
	} else {
		fmt.Fprintf(b, "- %s: %d из %d, осталось **%d**\n", title, used, max, rem)
	}
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
	stayDecision(b, r, who, cp)
	if r.Error != nil {
		return
	}
	stayUsage(b, r, lim, who, cp)
	periodNote(b, r, lim)
	violations(b, r)
}

func stayUsage(b *strings.Builder, r Result, lim Limits, who string, cp *Cap) {
	if r.Error != nil {
		return
	}
	ref := fd(r.Ref)
	if r.InCountry {
		n := dayNum(r.Ref) - dayNum(r.CurrentEntry) + 1
		fmt.Fprintf(b, "- Статус: %s в стране с %s (%d-й день поездки)\n", who, fd(r.CurrentEntry), n)
	} else {
		fmt.Fprintf(b, "- Статус: на %s %s вне страны\n", ref, who)
	}
	if r.InCountry && lim.MaxConsecutive > 0 {
		title := "Дней подряд"
		if lim.ExcludeEntryDay {
			title = "Дней срока (без дня въезда)"
		}
		usageLine(b, title, r.Used.Consecutive, lim.MaxConsecutive, r.Remaining.Consecutive)
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
	if cp != nil {
		b.WriteString(cp.Status)
	}
	b.WriteString("\n")
}

func stayDecision(b *strings.Builder, r Result, who string, cp *Cap) {
	if r.Error != nil {
		fmt.Fprintf(b, "**Ошибка данных (%s): %s.** Исправьте поездки для расчёта.\n\n", who, r.Error)
		return
	}
	ref := fd(r.Ref)
	if cp != nil && !r.EntryBlocked && (r.Unlimited || cp.Date.Before(r.LatestExit)) {
		left := dayNum(cp.Date) - dayNum(r.Ref)
		switch {
		case left < 0:
			fmt.Fprintf(b, "**⚠ Срок без регистрации истёк**\n\n# %s\n\n%s. Нужна регистрация или выезд.\n\n", fd(cp.Date), cp.Reason)
		case r.InCountry:
			fmt.Fprintf(b, "# Выехать не позднее: %s\n\nЕщё **%d дн.** после %s. Причина: %s.\n\n",
				fd(cp.Date), left, ref, cp.Reason)
		default:
			fmt.Fprintf(b, "# При въезде можно находиться до: %s\n\nВъезд: **%s**. Включительно: **%d дн.** Причина: %s.\n\n",
				fd(cp.Date), ref, left+1, cp.Reason)
		}
		if r.Unlimited {
			fmt.Fprintf(b, "**%s:** пребывание в пределах срока действующей регистрации. Дата её окончания в профиле не задана.\n\n", cp.Alt)
		} else if r.ForecastLimited {
			fmt.Fprintf(b, "**%s:** в пределах расчёта до %s предел срока не найден.\n\n", cp.Alt, fd(r.LatestExit))
		} else {
			if r.Overstay {
				fmt.Fprintf(b, "**Даже с регистрацией срок превышен.** Последний законный день: %s (%s).\n\n", fd(r.LatestExit), r.LatestReason)
			} else {
				fmt.Fprintf(b, "**%s — до %s** (ещё %d дн. после %s, сработает: %s).\n\n",
					cp.Alt, fd(r.LatestExit), r.DaysLeft(), ref, r.LatestReason)
			}
		}
		return
	}

	switch {
	case r.EntryBlocked:
		fmt.Fprintf(b, "**Въезд на %s невозможен:** %s.\n\n", ref, r.LatestReason)
	case r.Overstay:
		fmt.Fprintf(b, "**⚠ ПРЕВЫШЕНИЕ СРОКА**\n\nПоследний законный день\n\n# %s\n\nПричина: %s.\n\n", fd(r.LatestExit), r.LatestReason)
	case r.Unlimited:
		b.WriteString("**Ограничений по сроку нет.**\n\n")
	case r.ForecastLimited:
		fmt.Fprintf(b, "**В пределах расчёта до %s предел срока не найден.** Последний допустимый день пока не определён.\n\n", fd(r.LatestExit))
	case r.InCountry:
		fmt.Fprintf(b, "# Выехать не позднее: %s\n\nЕщё **%d дн.** после %s. Причина: %s.\n\n",
			fd(r.LatestExit), r.DaysLeft(), ref, r.LatestReason)
		if r.PlannedExit != nil && r.PlannedExit.After(r.LatestExit) {
			fmt.Fprintf(b, "⚠ Запланированный выезд %s позже допустимого!\n\n", fd(*r.PlannedExit))
		}
	case r.EarliestEntry == nil:
		fmt.Fprintf(b, "# При въезде можно находиться до: %s\n\nВъезд: **%s**. Включительно: **%d дн.** Причина: %s.\n\n",
			fd(r.LatestExit), ref, r.DaysLeft()+1, r.LatestReason)
	}
	if r.EarliestEntry != nil {
		if r.EarlyForecastLimited {
			fmt.Fprintf(b, "**Ближайший возможный въезд: %s.** В пределах расчёта до %s предел срока не найден.\n\n", fd(*r.EarliestEntry), fd(r.StayFromEarly))
		} else {
			fmt.Fprintf(b, "**Ближайший возможный въезд: %s**, тогда можно находиться до %s (%d дн.).\n\n",
				fd(*r.EarliestEntry), fd(r.StayFromEarly), dayNum(r.StayFromEarly)-dayNum(*r.EarliestEntry)+1)
		}
	} else if r.EntryBlocked {
		b.WriteString("Допустимая дата въезда в пределах расчёта не найдена.\n\n")
	}
}

// periodNote поясняет, почему реальный остаток больше остатка «на сегодня»:
// пока вы в стране, дни старых поездок выпадают из скользящего окна.
func periodNote(b *strings.Builder, r Result, lim Limits) {
	if r.ForecastLimited || r.DroppedFromPeriod == 0 || r.Remaining.InPeriod < 0 || r.Overstay || r.DaysLeft() <= r.Remaining.InPeriod {
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
	if u.refEntry != nil {
		if _, ok := parseUserDate(u.refEntry.Text); !ok {
			return "**Ошибка даты расчёта.** Введите дату в формате ДД.ММ.ГГГГ."
		}
	}
	if u.cur == nil {
		return "## Нет профиля\n\nНажмите «Новый», чтобы создать профиль. Для каждого члена семьи — отдельный профиль."
	}
	if u.country == nil {
		return "Выберите страну."
	}
	rule, ok := u.country.RuleFor(u.cur.Citizenship)
	context := fmt.Sprintf("%s — %s", u.country.Name, u.cur.Name)
	if u.cur.Citizenship != "" {
		context += " (" + u.cur.Citizenship + ")"
	}
	fmt.Fprintf(&b, "# %s\n\n---\n\n", context)
	if !ok {
		fmt.Fprintf(&b, "Для гражданства «%s» в countries.json нет правил. Добавьте правило с этим кодом или \"*\".\n", u.cur.Citizenship)
		return b.String()
	}
	rawRule := rule
	trips := u.cur.Trips[u.country.ID]
	var curTrip *Trip
	refD := dayNum(u.refDate)
	for i := range trips {
		en, valid := parseISO(trips[i].Entry)
		ex, hasEx := parseISO(trips[i].Exit)
		if valid && dayNum(en) <= refD && (!hasEx || dayNum(ex) >= refD) && (curTrip == nil || trips[i].Entry > curTrip.Entry) {
			curTrip = &trips[i]
		}
	}
	entryDate := u.refDate.Format(dateISO)
	if curTrip != nil {
		entryDate = curTrip.Entry
	}
	rule = rawRule.ForEntry(entryDate)

	// --- Правила ---
	b.WriteString("## Общая информация\n\n")
	if rule.VisaRequired {
		b.WriteString("- **Виза: требуется**")
	} else {
		b.WriteString("- Виза: не требуется")
	}
	if rule.VisaNote != "" {
		b.WriteString(" — " + rule.VisaNote)
	}
	b.WriteString("\n")
	if rule.Limits.unlimited() && rule.Registration.Required {
		b.WriteString("- Срок: определяется условиями и сроком действующей регистрации\n")
	} else {
		fmt.Fprintf(&b, "- Срок: %s\n", limitsText(rule.Limits))
	}

	vr := rule.VisaRun

	reg := rule.Registration
	if reg.Manual {
		b.WriteString("- Регистрация: условия требуют отдельной проверки")
	} else if reg.Required {
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
		fmt.Fprintf(&b, "- Источник: %s %s\n", sourceLinks(rule.Source), rule.Updated)
	}

	// --- Человек ---
	b.WriteString("\n---\n\n")
	lim := rule.Limits
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

	r := evaluate(trips, lim, resetGap(rule), u.refDate, rawRule)
	if r.Error == nil && r.Unlimited && regCap == nil && reg.Required {
		b.WriteString("**Срок пребывания — до окончания действующей регистрации.** Дата окончания регистрации в профиле не задана; точная дата выезда автоматически не определяется.\n\n")
	} else {
		stayDecision(&b, r, "вы", regCap)
	}
	if r.Error == nil {
		visaRunBlock(&b, rawRule.ForEntry(u.refDate.Format(dateISO)), r)
	}
	if rule.VisaRequired {
		b.WriteString("_Нужна виза: рассчитанный срок не может превышать срок, указанный в визе._\n\n")
	}
	returnPlans(&b, trips, rawRule, r, regCap)
	fmt.Fprintf(&b, "\n---\n\n## Пребывание на %s\n\n", fd(u.refDate))
	if r.Error != nil {
		b.WriteString("Исправьте даты поездок для отображения счётчиков.\n\n")
	} else {
		stayUsage(&b, r, lim, "вы", regCap)
		periodNote(&b, r, lim)
		violations(&b, r)
	}

	if r.Error == nil && curTrip != nil && reg.Required {
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
	if r.Error == nil && vr.MaxPerYear > 0 {
		mark := ""
		if r.EntriesInYear > vr.MaxPerYear {
			mark = " **⚠ превышено**"
		}
		fmt.Fprintf(&b, "- Въездов в %d году: %d из %d%s\n", u.refDate.Year(), r.EntriesInYear, vr.MaxPerYear, mark)
	}

	// --- Машина ---
	vehicleRule, minOutside := rule.Vehicle.forRegistration(u.cur.VehicleRegistrationCountry)
	vl := vehicleRule.Limits
	carTrips := u.cur.CarTrips[u.country.ID]
	hasCar := false
	for _, t := range carTrips {
		if t.Entry != "" || t.Exit != "" {
			hasCar = true
		}
	}
	if hasCar {
		b.WriteString("\n---\n\n## Автомобиль\n\n")
		fmt.Fprintf(&b, "Страна регистрации: **%s**. Туристическая поездка без ВНЖ.\n\n", vehicleCountryLabel(u.cur.VehicleRegistrationCountry))
		localRegistration := u.cur.VehicleRegistrationCountry == u.country.ID
		if localRegistration {
			vl = Limits{}
		}
		cr := Evaluate(carTrips, vl, 0, u.refDate)
		invalidEntry, earliest, vehicleHistory := vehicleReentry(carTrips, vl, minOutside, u.refDate)
		cr.Violations = append(cr.Violations, vehicleHistory...)
		sort.SliceStable(cr.Violations, func(i, j int) bool { return cr.Violations[i].Date.Before(cr.Violations[j].Date) })
		if !earliest.IsZero() {
			cr.EarliestEntry = &earliest
			cr.StayFromEarly = vehicleStayDeadline(earliest, vl)
			cr.EarlyForecastLimited = false
			if !cr.InCountry {
				cr.EntryBlocked = true
				cr.LatestReason = "обязательный перерыв после использования срока автомобиля"
			}
		}
		if localRegistration || vehicleRule.Manual {
			if localRegistration {
				b.WriteString("**Местная регистрация автомобиля.** Срок туристического ввоза для иностранных номеров здесь не применяется.\n\n")
			} else {
				if u.cur.VehicleRegistrationCountry == "" {
					b.WriteString("**Выберите страну регистрации во вкладке «Автомобиль».**\n\n")
				}
				b.WriteString("**Срок туристического ввоза зависит от условий страны.** Для точной даты вывоза нужны применимые условия и срок, предоставленный на границе. Автоматический расчёт пока не выполняется.\n\n")
			}
			if cr.Error != nil {
				fmt.Fprintf(&b, "Ошибка данных: %s.\n\n", cr.Error)
			} else {
				if cr.InCountry {
					fmt.Fprintf(&b, "- Машина в стране с %s (%d-й день поездки).\n", fd(cr.CurrentEntry), dayNum(cr.Ref)-dayNum(cr.CurrentEntry)+1)
				} else {
					fmt.Fprintf(&b, "- На %s машина вне страны.\n", fd(cr.Ref))
				}
				fmt.Fprintf(&b, "- Учтено дней в %d году: %d.\n\n", cr.Ref.Year(), cr.Used.InYear)
			}
		} else {
			if !invalidEntry.IsZero() {
				fmt.Fprintf(&b, "**Нарушен обязательный перерыв перед повторным въездом автомобиля %s.** После использования полного срока нужен вывоз на %d полных дней.\n\nПри выезде сегодня, %s, повторный въезд возможен не раньше **%s**.\n\n", fd(invalidEntry), minOutside, fd(u.refDate), fd(earliest))
				stayUsage(&b, cr, vl, "машина", nil)
				violations(&b, cr)
			} else {
				stayBlock(&b, cr, vl, "машина", nil)
			}
			fmt.Fprintf(&b, "Временный ввоз: %s.\n\n", limitsText(vl))
		}
		if !localRegistration {
			fmt.Fprintf(&b, "%s\n", vehicleRule.Note)
		}
		if !localRegistration && vehicleRule.Source != "" {
			fmt.Fprintf(&b, "\nИсточник: %s\n", sourceLinks(vehicleRule.Source))
		}
	}
	return b.String()
}
