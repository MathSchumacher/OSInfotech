package main

import (
	"image/color"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/driver/desktop"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
)

// ---------------------------------------------------------------------------
// Menu de acoes (conta do usuario e afins)
//
// O menu pronto do Fyne e uma lista cinza com texto: fora do sistema visual do
// app. Este aqui e uma folha de vidro rasterizada, com sombra projetada,
// cabecalho de identificacao e linhas que acendem com uma barra na lateral -
// o mesmo vocabulario dos botoes.
// ---------------------------------------------------------------------------

type menuAction struct {
	Label string
	Hint  string // linha de apoio, opcional
	Icon  fyne.Resource
	Tone  statusTone // toneDanger deixa a linha em tom de alerta
	Do    func()
}

const (
	menuRowHeight = 36
	menuWidth     = 248
	menuBarW      = 3
)

// menuGlass e a folha do menu: mais opaca que um cartao e com sombra real.
func menuGlass(t designTokens) glassStyle {
	shadow := shade(0.55)
	if !isDarkMode() {
		shadow = nrgbaAlpha(t.Shadow, 0.6)
	}
	return glassStyle{
		// Quase opaco: um menu precisa ser lido por cima de qualquer coisa, e o
		// vidro translucido deixava o botao de tras aparecer no meio do texto.
		Fill:      nrgbaAlpha(t.Surface, 0.975),
		Border:    t.GlassBorder,
		BorderW:   1,
		Radius:    radiusLg,
		TopAccent: white(0.07),
		AccentH:   1.2,
		Grain:     0.010,
		Glow:      shadow,
		GlowR:     26,
		Pad:       26,
	}
}

// ---- linha do menu ----

type menuRow struct {
	widget.BaseWidget
	action  menuAction
	onPick  func()
	hovered bool

	bg    *canvas.Rectangle
	bar   *canvas.Rectangle
	icon  *canvas.Image
	label *canvas.Text
	hint  *canvas.Text
}

func newMenuRow(action menuAction, onPick func()) *menuRow {
	r := &menuRow{action: action, onPick: onPick}
	r.bg = canvas.NewRectangle(color.Transparent)
	r.bg.CornerRadius = radiusSm
	r.bar = canvas.NewRectangle(color.Transparent)
	r.bar.CornerRadius = menuBarW / 2
	r.icon = canvas.NewImageFromResource(nil)
	r.icon.FillMode = canvas.ImageFillContain
	r.label = canvas.NewText(action.Label, color.White)
	r.label.TextSize = sizeBody
	r.hint = canvas.NewText(action.Hint, color.White)
	r.hint.TextSize = sizeCaption
	r.ExtendBaseWidget(r)
	return r
}

func (r *menuRow) MinSize() fyne.Size {
	h := float32(menuRowHeight)
	if r.action.Hint != "" {
		h = 48
	}
	return fyne.NewSize(menuWidth, h)
}

func (r *menuRow) Tapped(*fyne.PointEvent) {
	if r.onPick != nil {
		r.onPick()
	}
	if r.action.Do != nil {
		r.action.Do()
	}
}

func (r *menuRow) MouseIn(*desktop.MouseEvent)    { r.hovered = true; r.Refresh() }
func (r *menuRow) MouseMoved(*desktop.MouseEvent) {}
func (r *menuRow) MouseOut()                      { r.hovered = false; r.Refresh() }
func (r *menuRow) Cursor() desktop.Cursor         { return desktop.PointerCursor }

func (r *menuRow) CreateRenderer() fyne.WidgetRenderer {
	rr := &menuRowRenderer{r: r}
	rr.Refresh()
	return rr
}

type menuRowRenderer struct{ r *menuRow }

func (rr *menuRowRenderer) Layout(size fyne.Size) {
	r := rr.r
	r.bg.Resize(size)
	r.bg.Move(fyne.NewPos(0, 0))
	barH := size.Height - 14
	r.bar.Resize(fyne.NewSize(menuBarW, barH))
	r.bar.Move(fyne.NewPos(0, (size.Height-barH)/2))
	x := float32(14)
	if r.action.Icon != nil {
		r.icon.Resize(fyne.NewSize(16, 16))
		r.icon.Move(fyne.NewPos(x, (size.Height-16)/2))
		x += 16 + 10
	}
	if r.action.Hint == "" {
		th := r.label.MinSize().Height
		r.label.Move(fyne.NewPos(x, (size.Height-th)/2))
		return
	}
	th := r.label.MinSize().Height
	hh := r.hint.MinSize().Height
	top := (size.Height - th - hh - 2) / 2
	r.label.Move(fyne.NewPos(x, top))
	r.hint.Move(fyne.NewPos(x, top+th+2))
}

func (rr *menuRowRenderer) MinSize() fyne.Size { return rr.r.MinSize() }

func (rr *menuRowRenderer) Refresh() {
	r := rr.r
	t := tk()
	ink := t.Ink
	iconName := theme.ColorNameForeground
	if r.action.Tone == toneDanger {
		ink = t.Danger
		iconName = theme.ColorNameError
	}
	if r.hovered {
		if r.action.Tone == toneDanger {
			r.bg.FillColor = nrgbaAlpha(t.Danger, 0.14)
			r.bar.FillColor = nrgba(t.Danger)
		} else {
			r.bg.FillColor = nrgbaAlpha(t.Brand, 0.16)
			r.bar.FillColor = nrgba(t.Brand)
		}
	} else {
		r.bg.FillColor = color.Transparent
		r.bar.FillColor = color.Transparent
		if r.action.Tone != toneDanger {
			ink = t.InkSecondary
		}
	}
	r.label.Text = r.action.Label
	r.label.Color = ink
	r.label.TextStyle = fyne.TextStyle{Bold: r.hovered}
	r.hint.Text = r.action.Hint
	r.hint.Color = t.InkMuted
	if r.action.Icon != nil {
		r.icon.Resource = theme.NewColoredResource(r.action.Icon, iconName)
		r.icon.Show()
	} else {
		r.icon.Hide()
	}
	if r.action.Hint == "" {
		r.hint.Hide()
	} else {
		r.hint.Show()
	}
	r.bg.Refresh()
	r.bar.Refresh()
	r.icon.Refresh()
	r.label.Refresh()
	r.hint.Refresh()
	rr.Layout(r.Size())
}

func (rr *menuRowRenderer) Objects() []fyne.CanvasObject {
	r := rr.r
	return []fyne.CanvasObject{r.bg, r.bar, r.icon, r.label, r.hint}
}

func (rr *menuRowRenderer) Destroy() {}

// ---- folha do menu ----

// showActionMenu abre a folha ancorada no objeto informado. header e opcional.
func (ui *mainUI) showActionMenu(anchor fyne.CanvasObject, header fyne.CanvasObject, actions []menuAction) {
	if ui.window == nil || anchor == nil {
		return
	}
	c := ui.window.Canvas()
	if c == nil {
		return
	}
	var overlay *fyne.Container
	dismiss := func() {
		if overlay != nil {
			c.Overlays().Remove(overlay)
			overlay = nil
		}
	}

	items := container.NewVBox()
	if header != nil {
		items.Add(header)
		items.Add(menuDivider())
	}
	for _, action := range actions {
		if action.Label == "" {
			items.Add(menuDivider())
			continue
		}
		items.Add(newMenuRow(action, dismiss))
	}
	sheet := newGlassPanel(items, 8, menuGlass)
	size := sheet.MinSize()
	size.Width = fyne.Max(size.Width, menuWidth+16)

	// Posiciona abaixo da ancora, alinhado a direita quando nao cabe.
	pos := fyne.CurrentApp().Driver().AbsolutePositionForObject(anchor)
	canvasSize := c.Size()
	x := pos.X
	y := pos.Y + anchor.Size().Height + 8
	// Ancora grande (no celular o menu e ancorado na area de conteudo inteira):
	// "abaixo da ancora" cairia fora da tela, entao vale o topo dela.
	if anchor.Size().Height > canvasSize.Height/2 {
		y = pos.Y + 8
	}
	if x+size.Width > canvasSize.Width-8 {
		x = pos.X + anchor.Size().Width - size.Width
	}
	if y+size.Height > canvasSize.Height-8 {
		y = pos.Y - size.Height - 8
	}
	// Limites da tela por ultimo: sem isto a folha podia terminar acima do topo
	// e o menu ficava inalcancavel.
	x = clampFloat(x, 8, canvasSize.Width-size.Width-8)
	y = clampFloat(y, 8, canvasSize.Height-size.Height-8)

	scrim := newTapCard(canvas.NewRectangle(color.Transparent), 0, 0, dismiss)
	scrim.paint = func(designTokens, bool) (color.Color, color.Color) {
		return color.Transparent, color.Transparent
	}
	scrim.Resize(canvasSize)
	scrim.Move(fyne.NewPos(0, 0))
	sheet.Resize(size)
	sheet.Move(fyne.NewPos(x, y-6))

	overlay = container.NewWithoutLayout(scrim, sheet)
	overlay.Resize(canvasSize)
	c.Overlays().Add(overlay)

	// Entrada curta: a folha desce 6 pt ate o lugar.
	anim := fyne.NewAnimation(120*time.Millisecond, func(f float32) {
		sheet.Move(fyne.NewPos(x, y-6+6*f))
	})
	anim.Curve = fyne.AnimationEaseOut
	anim.Start()
}

// clampFloat prende v ao intervalo, e devolve o minimo quando o intervalo e
// invalido (folha maior que a tela).
func clampFloat(v, min, max float32) float32 {
	if max < min {
		return min
	}
	if v < min {
		return min
	}
	if v > max {
		return max
	}
	return v
}

// menuDivider e o fio que separa grupos de acoes.
func menuDivider() fyne.CanvasObject {
	line := newFill(0, func(t designTokens) color.Color { return t.GlassBorder })
	line.SetMinSize(fyne.NewSize(menuWidth, 1))
	return paddedTRBL(line, 5, 4, 5, 4)
}

// menuHeader e o cabecalho com quem esta logado.
func menuHeader(name string, role string, avatarName string, avatarURL string) fyne.CanvasObject {
	av := newAvatar(34)
	av.SetName(firstNonEmpty(avatarName, name))
	if avatarURL != "" {
		av.SetPhotoURL(avatarURL)
	}
	title := newText(name, sizeBody, true, inkPrimary).SetTruncate(true)
	sub := newText(role, sizeCaption, false, inkMuted).SetTruncate(true)
	body := container.NewBorder(nil, nil, container.NewCenter(av), nil,
		paddedTRBL(container.NewVBox(title, sub), 0, 0, 0, 10))
	return paddedTRBL(body, 4, 6, 2, 6)
}
