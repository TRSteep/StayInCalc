package main

import (
	"image/color"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
)

// Оттенок карточки следует светлой/тёмной теме и обновляется вместе с ней.
type reportPanel struct {
	widget.BaseWidget
	content fyne.CanvasObject
}

func newReportPanel(content fyne.CanvasObject) *reportPanel {
	p := &reportPanel{content: content}
	p.ExtendBaseWidget(p)
	return p
}

func (p *reportPanel) CreateRenderer() fyne.WidgetRenderer {
	background := canvas.NewRectangle(color.Transparent)
	background.CornerRadius = 6
	r := &reportPanelRenderer{panel: p, background: background}
	r.WidgetRenderer = widget.NewSimpleRenderer(container.NewStack(background, widget.NewCard("", "", p.content)))
	r.Refresh()
	return r
}

type reportPanelRenderer struct {
	fyne.WidgetRenderer
	panel      *reportPanel
	background *canvas.Rectangle
}

func (r *reportPanelRenderer) Refresh() {
	t := r.panel.Theme()
	v := fyne.CurrentApp().Settings().ThemeVariant()
	br, bg, bb, _ := t.Color(theme.ColorNameBackground, v).RGBA()
	fr, fg, fb, _ := t.Color(theme.ColorNameForeground, v).RGBA()
	// 8% цвета текста: светлее в тёмной теме, темнее в светлой.
	r.background.FillColor = color.NRGBA{R: uint8((br*92 + fr*8) / 100 >> 8), G: uint8((bg*92 + fg*8) / 100 >> 8), B: uint8((bb*92 + fb*8) / 100 >> 8), A: 255}
	r.WidgetRenderer.Refresh()
}
