package main

import (
	"errors"
	"fmt"
	"net/url"
	"path/filepath"
	"strings"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/layout"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
)

type UI struct {
	app   fyne.App
	win   fyne.Window
	store *Store

	countries *CountriesFile
	profiles  []*Profile
	cur       *Profile
	country   *Country
	refDate   time.Time

	profileSel  *widget.Select
	countrySel  *widget.Select
	citizenship *widget.Entry
	refEntry    *widget.Entry
	tripsBox    *fyne.Container
	carBox      *fyne.Container
	result      *widget.RichText
}

func runUI(a fyne.App, store *Store) {
	newUI(a, store).win.ShowAndRun()
}

func newUI(a fyne.App, store *Store) *UI {
	u := &UI{app: a, store: store, refDate: today()}
	u.win = a.NewWindow("Калькулятор пребывания")
	u.win.Resize(fyne.NewSize(1150, 720))

	cf, err := store.LoadCountries()
	if err != nil {
		dialog.ShowError(fmt.Errorf("countries.json: %w\nИсправьте файл в редакторе правил.", err), u.win)
		cf = &CountriesFile{}
	}
	u.countries = cf

	u.build()
	u.reloadProfiles("")
	return u
}

func (u *UI) build() {
	u.profileSel = widget.NewSelect(nil, func(name string) {
		for _, p := range u.profiles {
			if p.Name == name {
				u.selectProfile(p)
				return
			}
		}
	})
	u.profileSel.PlaceHolder = "— создайте профиль —"

	newBtn := widget.NewButtonWithIcon("Новый", theme.ContentAddIcon(), u.newProfileDialog)
	delBtn := widget.NewButtonWithIcon("", theme.DeleteIcon(), u.deleteProfile)

	u.citizenship = widget.NewEntry()
	u.citizenship.SetPlaceHolder("RU")
	u.citizenship.OnChanged = func(s string) {
		if u.cur == nil {
			return
		}
		u.cur.Citizenship = strings.ToUpper(strings.TrimSpace(s))
		u.save()
		u.recalc()
	}

	u.countrySel = widget.NewSelect(nil, func(name string) {
		for i := range u.countries.Countries {
			if u.countries.Countries[i].Name == name {
				u.country = &u.countries.Countries[i]
			}
		}
		if u.cur != nil && u.country != nil && u.cur.LastCountry != u.country.ID {
			u.cur.LastCountry = u.country.ID
			u.save()
		}
		u.rebuildTrips()
		u.recalc()
	})
	u.countrySel.PlaceHolder = "— выберите страну —"
	u.fillCountries()

	refField, refEntry := u.dateField(u.refDate.Format(dateISO), func(iso string) {
		if t, ok := parseISO(iso); ok {
			u.refDate = t
			u.recalc()
		}
	})
	u.refEntry = refEntry
	todayBtn := widget.NewButton("Сегодня", func() { u.refEntry.SetText(today().Format(dateUI)) })

	rulesBtn := widget.NewButtonWithIcon("Правила стран", theme.SettingsIcon(), u.rulesEditor)

	top := container.NewVBox(
		container.NewHBox(
			widget.NewIcon(theme.AccountIcon()), widget.NewLabel("Профиль:"),
			sized(u.profileSel, 200), newBtn, delBtn,
			widget.NewSeparator(),
			widget.NewLabel("Гражданство:"), sized(u.citizenship, 60),
			layout.NewSpacer(), rulesBtn,
		),
		container.NewHBox(
			widget.NewLabel("Страна:"), sized(u.countrySel, 220),
			widget.NewSeparator(),
			widget.NewLabel("Расчёт на дату:"), refField, todayBtn,
		),
		widget.NewSeparator(),
	)

	u.tripsBox = container.NewVBox()
	u.carBox = container.NewVBox()
	addTrip := widget.NewButtonWithIcon("Добавить поездку", theme.ContentAddIcon(), func() { u.addRow(false) })
	addCar := widget.NewButtonWithIcon("Добавить въезд машины", theme.ContentAddIcon(), func() { u.addRow(true) })
	copyCar := widget.NewButton("Скопировать даты из поездок", u.copyTripsToCar)

	hint := widget.NewLabel("Даты: ДД.ММ.ГГГГ. Пустая дата выезда — вы ещё в стране.\nДень въезда и день выезда считаются днями пребывания.")
	hint.Importance = widget.LowImportance

	tabs := container.NewAppTabs(
		container.NewTabItem("Пребывание", container.NewBorder(nil, container.NewVBox(addTrip, hint), nil, nil,
			container.NewVScroll(u.tripsBox))),
		container.NewTabItem("Автомобиль", container.NewBorder(nil, container.NewHBox(addCar, copyCar), nil, nil,
			container.NewVScroll(u.carBox))),
	)

	u.result = widget.NewRichText()
	u.result.Wrapping = fyne.TextWrapWord

	split := container.NewHSplit(tabs, container.NewVScroll(container.NewPadded(u.result)))
	split.Offset = 0.5

	u.win.SetContent(container.NewBorder(top, nil, nil, nil, split))
}

func sized(o fyne.CanvasObject, w float32) fyne.CanvasObject {
	return container.NewGridWrap(fyne.NewSize(w, o.MinSize().Height), o)
}

// dateField — поле ДД.ММ.ГГГГ с кнопкой календаря. onValid вызывается с ISO-датой
// (или "" при очистке поля); некорректный ввод игнорируется до исправления.
func (u *UI) dateField(iso string, onValid func(string)) (fyne.CanvasObject, *widget.Entry) {
	e := widget.NewEntry()
	e.SetPlaceHolder("ДД.ММ.ГГГГ")
	e.SetText(isoToUI(iso))
	e.Validator = func(s string) error {
		if strings.TrimSpace(s) == "" {
			return nil
		}
		if _, ok := parseUserDate(s); !ok {
			return errors.New("формат ДД.ММ.ГГГГ")
		}
		return nil
	}
	e.OnChanged = func(s string) {
		if strings.TrimSpace(s) == "" {
			onValid("")
			return
		}
		if t, ok := parseUserDate(s); ok && len(strings.TrimSpace(s)) >= 8 {
			onValid(t.Format(dateISO))
		}
	}
	var btn *widget.Button
	btn = widget.NewButtonWithIcon("", theme.CalendarIcon(), func() {
		start := today()
		if t, ok := parseUserDate(e.Text); ok {
			start = t
		}
		var pop *widget.PopUp
		cal := widget.NewCalendar(start, func(t time.Time) {
			e.SetText(t.Format(dateUI))
			pop.Hide()
		})
		pop = widget.NewPopUp(cal, u.win.Canvas())
		pos := fyne.CurrentApp().Driver().AbsolutePositionForObject(btn)
		pop.ShowAtPosition(pos.Add(fyne.NewPos(0, btn.Size().Height)))
	})
	return container.NewHBox(sized(e, 135), btn), e
}

// ---------- Профили ----------

func (u *UI) reloadProfiles(selectFile string) {
	ps, err := u.store.ListProfiles()
	if err != nil {
		dialog.ShowError(err, u.win)
	}
	u.profiles = ps
	var names []string
	for _, p := range ps {
		names = append(names, p.Name)
	}
	u.profileSel.Options = names
	u.profileSel.Refresh()

	switch {
	case len(ps) == 0:
		u.cur = nil
		u.profileSel.ClearSelected()
		u.rebuildTrips()
		u.recalc()
	case selectFile != "":
		for _, p := range ps {
			if p.file == selectFile {
				u.profileSel.SetSelected(p.Name)
				return
			}
		}
		fallthrough
	default:
		u.profileSel.SetSelected(ps[0].Name)
	}
}

func (u *UI) selectProfile(p *Profile) {
	u.cur = nil // чтобы OnChanged гражданства не перезаписал профиль
	u.citizenship.SetText(p.Citizenship)
	u.cur = p
	if p.LastCountry != "" {
		for _, c := range u.countries.Countries {
			if c.ID == p.LastCountry {
				u.countrySel.SetSelected(c.Name)
				return // SetSelected вызовет rebuild/recalc
			}
		}
	}
	if u.country == nil && len(u.countries.Countries) > 0 {
		u.countrySel.SetSelected(u.countries.Countries[0].Name)
		return
	}
	u.rebuildTrips()
	u.recalc()
}

func (u *UI) newProfileDialog() {
	name := widget.NewEntry()
	name.SetPlaceHolder("Например: Иван")
	cz := widget.NewEntry()
	cz.SetPlaceHolder("RU")
	if u.cur != nil {
		cz.SetText(u.cur.Citizenship)
	}
	dialog.ShowForm("Новый профиль (член семьи)", "Создать", "Отмена", []*widget.FormItem{
		widget.NewFormItem("Имя", name),
		widget.NewFormItem("Гражданство (код)", cz),
	}, func(ok bool) {
		if !ok {
			return
		}
		p, err := u.store.NewProfile(name.Text, cz.Text)
		if err != nil {
			dialog.ShowError(err, u.win)
			return
		}
		if u.country != nil {
			p.LastCountry = u.country.ID
			u.store.SaveProfile(p)
		}
		u.reloadProfiles(p.file)
	}, u.win)
}

func (u *UI) deleteProfile() {
	if u.cur == nil {
		return
	}
	p := u.cur
	dialog.ShowConfirm("Удалить профиль", fmt.Sprintf("Удалить профиль «%s» со всеми поездками?", p.Name), func(ok bool) {
		if !ok {
			return
		}
		if err := u.store.DeleteProfile(p); err != nil {
			dialog.ShowError(err, u.win)
		}
		u.reloadProfiles("")
	}, u.win)
}

func (u *UI) save() {
	if u.cur == nil {
		return
	}
	if err := u.store.SaveProfile(u.cur); err != nil {
		dialog.ShowError(err, u.win)
	}
}

// ---------- Поездки ----------

func (u *UI) tripsMap(car bool) map[string][]Trip {
	if car {
		return u.cur.CarTrips
	}
	return u.cur.Trips
}

func (u *UI) ensureEmptyRow(car bool) {
	m := u.tripsMap(car)
	list := m[u.country.ID]
	if n := len(list); n == 0 || list[n-1].Exit != "" {
		m[u.country.ID] = append(list, Trip{})
	}
}

func (u *UI) rebuildTrips() {
	u.tripsBox.RemoveAll()
	u.carBox.RemoveAll()
	if u.cur == nil || u.country == nil {
		u.tripsBox.Add(widget.NewLabel("Создайте профиль и выберите страну."))
		return
	}
	for _, car := range []bool{false, true} {
		box := u.tripsBox
		if car {
			box = u.carBox
		}
		u.ensureEmptyRow(car)
		list := u.tripsMap(car)[u.country.ID]
		for i := range list {
			box.Add(u.tripRow(car, i))
		}
	}
}

func (u *UI) tripRow(car bool, i int) fyne.CanvasObject {
	cid := u.country.ID
	get := func() *Trip {
		l := u.tripsMap(car)[cid]
		if i < len(l) {
			return &l[i]
		}
		return &Trip{}
	}
	t := get()
	changed := func() {
		u.save()
		u.recalc()
	}

	entryF, _ := u.dateField(t.Entry, func(iso string) {
		get().Entry = iso
		changed()
	})
	exitF, _ := u.dateField(t.Exit, func(iso string) {
		get().Exit = iso
		l := u.tripsMap(car)[cid]
		// заполнили выезд в последней строке — добавляем новую пару полей
		if iso != "" && i == len(l)-1 && get().Entry != "" {
			u.tripsMap(car)[cid] = append(l, Trip{})
			box := u.tripsBox
			if car {
				box = u.carBox
			}
			box.Add(u.tripRow(car, i+1))
		}
		changed()
	})

	note := widget.NewEntry()
	note.SetPlaceHolder("заметка")
	note.SetText(t.Note)
	note.OnChanged = func(s string) { get().Note = s; u.save() }

	del := widget.NewButtonWithIcon("", theme.DeleteIcon(), func() {
		l := u.tripsMap(car)[cid]
		if i < len(l) {
			u.tripsMap(car)[cid] = append(l[:i:i], l[i+1:]...)
		}
		u.save()
		u.rebuildTrips()
		u.recalc()
	})

	items := []fyne.CanvasObject{
		widget.NewLabel(fmt.Sprintf("%d.", i+1)),
		widget.NewLabel("Въезд"), entryF,
		widget.NewLabel("Выезд"), exitF,
	}
	if !car {
		reg := widget.NewCheck("Регистрация", func(b bool) { get().Registered = b; changed() })
		reg.Checked = t.Registered
		items = append(items, reg)
	}
	items = append(items, sized(note, 140), del)
	return container.NewHBox(items...)
}

func (u *UI) addRow(car bool) {
	if u.cur == nil || u.country == nil {
		return
	}
	m := u.tripsMap(car)
	m[u.country.ID] = append(m[u.country.ID], Trip{})
	u.rebuildTrips()
}

func (u *UI) copyTripsToCar() {
	if u.cur == nil || u.country == nil {
		return
	}
	var l []Trip
	for _, t := range u.cur.Trips[u.country.ID] {
		if t.Entry != "" {
			l = append(l, Trip{Entry: t.Entry, Exit: t.Exit})
		}
	}
	u.cur.CarTrips[u.country.ID] = l
	u.save()
	u.rebuildTrips()
	u.recalc()
}

// ---------- Страны ----------

func (u *UI) fillCountries() {
	var names []string
	for _, c := range u.countries.Countries {
		names = append(names, c.Name)
	}
	u.countrySel.Options = names
	u.countrySel.Refresh()
}

func (u *UI) rulesEditor() {
	raw, err := u.store.LoadCountriesRaw()
	if err != nil {
		dialog.ShowError(err, u.win)
		return
	}
	w := u.app.NewWindow("Правила стран — countries.json")
	w.Resize(fyne.NewSize(900, 700))

	ed := widget.NewMultiLineEntry()
	ed.TextStyle = fyne.TextStyle{Monospace: true}
	ed.SetText(string(raw))
	status := widget.NewLabel(u.store.countriesPath())
	status.Wrapping = fyne.TextWrapWord

	apply := func() {
		cf, err := ParseCountries([]byte(ed.Text))
		if err != nil {
			dialog.ShowError(err, w)
			return
		}
		if err := u.store.SaveCountriesRaw([]byte(ed.Text)); err != nil {
			dialog.ShowError(err, w)
			return
		}
		u.applyCountries(cf)
		status.SetText("Сохранено " + time.Now().Format("15:04:05"))
	}
	save := widget.NewButtonWithIcon("Сохранить", theme.DocumentSaveIcon(), apply)
	save.Importance = widget.HighImportance
	check := widget.NewButton("Проверить", func() {
		if _, err := ParseCountries([]byte(ed.Text)); err != nil {
			status.SetText("Ошибка: " + err.Error())
		} else {
			status.SetText("Файл корректен")
		}
	})
	reload := widget.NewButton("Перечитать файл", func() {
		b, err := u.store.LoadCountriesRaw()
		if err != nil {
			dialog.ShowError(err, w)
			return
		}
		ed.SetText(string(b))
		if cf, err := ParseCountries(b); err == nil {
			u.applyCountries(cf)
		}
		status.SetText("Перечитано с диска")
	})
	external := widget.NewButton("Открыть во внешнем редакторе", func() {
		p, _ := filepath.Abs(u.store.countriesPath())
		fu := &url.URL{Scheme: "file", Path: "/" + filepath.ToSlash(p)}
		if err := u.app.OpenURL(fu); err != nil {
			dialog.ShowError(err, w)
		}
	})
	help := widget.NewLabel("0 = не ограничено. max_consecutive — дней подряд; max_in_period/period_days — " +
		"скользящий период (90/180); max_per_year + year_mode (calendar|rolling); visa_run.allowed/min_days_outside — " +
		"обнуляет ли выезд счётчик «подряд» (min_days_outside 0 — можно въехать в день выезда); registration.extends_stay + extended — лимиты при регистрации; vehicle — лимиты для машины.")
	help.Wrapping = fyne.TextWrapWord

	w.SetContent(container.NewBorder(help,
		container.NewVBox(container.NewHBox(save, check, reload, external), status),
		nil, nil, ed))
	w.Show()
}

func (u *UI) applyCountries(cf *CountriesFile) {
	prev := ""
	if u.country != nil {
		prev = u.country.ID
	}
	u.countries = cf
	u.country = nil
	u.fillCountries()
	for i := range cf.Countries {
		if cf.Countries[i].ID == prev {
			u.country = &cf.Countries[i]
			u.countrySel.Selected = cf.Countries[i].Name
			u.countrySel.Refresh()
		}
	}
	if u.country == nil {
		u.countrySel.ClearSelected()
	}
	u.rebuildTrips()
	u.recalc()
}

// ---------- Расчёт ----------

func resetGap(r *Rule) int {
	vr := r.VisaRun
	if vr.Allowed {
		return vr.MinDaysOutside // 0 — визаран в тот же день
	}
	if vr.MinDaysOutside > 0 {
		return vr.MinDaysOutside
	}
	return max(1, r.PeriodDays, r.MaxConsecutive)
}

func (u *UI) recalc() {
	md := u.report()
	u.result.ParseMarkdown(md)
}
