package main

import (
	"image/color"
	"strings"
	"time"
	"unicode"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/driver/desktop"
	"fyne.io/fyne/v2/driver/mobile"
	"fyne.io/fyne/v2/layout"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
)

// iconBell e o sino de alertas (o Fyne nao traz um).
var iconBell = fyne.NewStaticResource("dbs-bell.svg", []byte(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24"><path fill="#000000" d="M12 22c1.1 0 2-.9 2-2h-4c0 1.1.9 2 2 2zm6-6v-5c0-3.07-1.63-5.64-4.5-6.32V4c0-.83-.67-1.5-1.5-1.5s-1.5.67-1.5 1.5v.68C7.64 5.36 6 7.92 6 11v5l-2 2v1h16v-1l-2-2z"/></svg>`))

// Escala tipografica do design system (pontos).
const (
	sizeDisplay = 28
	sizeTitle   = 22
	sizeSection = 15
	sizeBody    = 13
	sizeCaption = 11
	sizeMicro   = 10
)

// Raio das superficies.
const (
	radiusSm   = 6
	radiusMd   = 10
	radiusLg   = 14
	radiusPill = 999
)

// ---------------------------------------------------------------------------
// Cores de texto por papel.
// ---------------------------------------------------------------------------

type inkLevel int

const (
	inkPrimary inkLevel = iota
	inkSecondary
	inkMuted
	inkBrand
	inkOnBrand
	inkSuccess
	inkWarning
	inkDanger
	inkInfo
)

func inkColor(level inkLevel, t designTokens) color.Color {
	switch level {
	case inkSecondary:
		return t.InkSecondary
	case inkMuted:
		return t.InkMuted
	case inkBrand:
		return t.Brand
	case inkOnBrand:
		return t.InkOnBrand
	case inkSuccess:
		return t.Success
	case inkWarning:
		return t.Warning
	case inkDanger:
		return t.Danger
	case inkInfo:
		return t.Info
	default:
		return t.Ink
	}
}

// ---------------------------------------------------------------------------
// themedRect: retangulo cujas cores seguem os tokens do tema ativo.
// ---------------------------------------------------------------------------

type themedRect struct {
	widget.BaseWidget
	radius      float32
	strokeWidth float32
	minSize     fyne.Size
	paint       func(designTokens) (fill color.Color, stroke color.Color)
}

func newThemedRect(radius float32, strokeWidth float32, paint func(designTokens) (color.Color, color.Color)) *themedRect {
	r := &themedRect{radius: radius, strokeWidth: strokeWidth, paint: paint}
	r.ExtendBaseWidget(r)
	return r
}

// newFill cria um retangulo so de preenchimento.
func newFill(radius float32, pick func(designTokens) color.Color) *themedRect {
	return newThemedRect(radius, 0, func(t designTokens) (color.Color, color.Color) {
		return pick(t), color.Transparent
	})
}

// hairline e uma linha divisoria de 1px.
func hairline() *themedRect {
	r := newFill(0, func(t designTokens) color.Color { return t.Border })
	r.minSize = fyne.NewSize(1, 1)
	return r
}

func (r *themedRect) SetMinSize(size fyne.Size) {
	r.minSize = size
	r.Refresh()
}

func (r *themedRect) MinSize() fyne.Size { return r.minSize }

func (r *themedRect) CreateRenderer() fyne.WidgetRenderer {
	rect := canvas.NewRectangle(color.Transparent)
	rect.CornerRadius = r.radius
	rect.StrokeWidth = r.strokeWidth
	rr := &themedRectRenderer{w: r, rect: rect}
	rr.Refresh()
	return rr
}

type themedRectRenderer struct {
	w    *themedRect
	rect *canvas.Rectangle
}

func (r *themedRectRenderer) Layout(size fyne.Size) { r.rect.Resize(size) }
func (r *themedRectRenderer) MinSize() fyne.Size    { return r.w.minSize }
func (r *themedRectRenderer) Refresh() {
	fill, stroke := r.w.paint(tk())
	r.rect.FillColor = fill
	r.rect.StrokeColor = stroke
	r.rect.CornerRadius = r.w.radius
	r.rect.StrokeWidth = r.w.strokeWidth
	r.rect.Refresh()
}
func (r *themedRectRenderer) Objects() []fyne.CanvasObject { return []fyne.CanvasObject{r.rect} }
func (r *themedRectRenderer) Destroy()                     {}

// ---------------------------------------------------------------------------
// themedText: texto de uma linha com tamanho livre e cor por papel.
// ---------------------------------------------------------------------------

type themedText struct {
	widget.BaseWidget
	text      *canvas.Text
	ink       inkLevel
	uppercase bool
	truncate  bool
	full      string
}

func newText(s string, size float32, bold bool, ink inkLevel) *themedText {
	t := &themedText{ink: ink}
	t.text = canvas.NewText(s, color.Black)
	t.text.TextSize = size
	t.text.TextStyle = fyne.TextStyle{Bold: bold}
	t.ExtendBaseWidget(t)
	return t
}

// newEyebrow e o rotulo curto em caixa alta usado em secoes, cabecalhos de
// tabela e tiles de indicadores.
func newEyebrow(s string) *themedText {
	t := newText(strings.ToUpper(s), sizeMicro, true, inkMuted)
	t.uppercase = true
	return t
}

// newEyebrowBrand e o rotulo em caixa alta na cor da marca (destaques).
func newEyebrowBrand(s string) *themedText {
	t := newText(strings.ToUpper(s), sizeMicro, true, inkBrand)
	t.uppercase = true
	return t
}

func (t *themedText) SetText(s string) {
	if t.uppercase {
		s = strings.ToUpper(s)
	}
	t.full = s
	t.text.Text = s
	t.Refresh()
}

// SetTruncate faz o texto caber na largura disponivel, com reticencias, em vez
// de exigir a largura inteira (colunas de tabela, faixas estreitas).
func (t *themedText) SetTruncate(on bool) *themedText {
	t.truncate = on
	t.full = t.text.Text
	return t
}

func (t *themedText) Text() string {
	if t.truncate {
		return t.full
	}
	return t.text.Text
}

// fitText encurta o texto para caber em width.
func (t *themedText) fitText(width float32) {
	if !t.truncate {
		return
	}
	full := t.full
	if width <= 0 {
		t.text.Text = full
		return
	}
	if fyne.MeasureText(full, t.text.TextSize, t.text.TextStyle).Width <= width {
		t.text.Text = full
		return
	}
	runes := []rune(full)
	lo, hi := 0, len(runes)
	for lo < hi {
		mid := (lo + hi + 1) / 2
		candidate := string(runes[:mid]) + "…"
		if fyne.MeasureText(candidate, t.text.TextSize, t.text.TextStyle).Width <= width {
			lo = mid
		} else {
			hi = mid - 1
		}
	}
	if lo == 0 {
		t.text.Text = "…"
		return
	}
	t.text.Text = strings.TrimRight(string(runes[:lo]), " ") + "…"
}

func (t *themedText) SetInk(level inkLevel) {
	t.ink = level
	t.Refresh()
}

func (t *themedText) SetAlignment(a fyne.TextAlign) {
	t.text.Alignment = a
	t.Refresh()
}

func (t *themedText) MinSize() fyne.Size {
	if t.truncate {
		h := fyne.MeasureText("Ag", t.text.TextSize, t.text.TextStyle).Height
		return fyne.NewSize(24, h)
	}
	return t.text.MinSize()
}

func (t *themedText) CreateRenderer() fyne.WidgetRenderer {
	r := &themedTextRenderer{w: t}
	r.Refresh()
	return r
}

type themedTextRenderer struct {
	w *themedText
}

func (r *themedTextRenderer) Layout(size fyne.Size) {
	r.w.fitText(size.Width)
	r.w.text.Resize(size)
}
func (r *themedTextRenderer) MinSize() fyne.Size { return r.w.MinSize() }
func (r *themedTextRenderer) Refresh() {
	r.w.text.Color = inkColor(r.w.ink, tk())
	if r.w.truncate {
		r.w.fitText(r.w.Size().Width)
	}
	r.w.text.Refresh()
}
func (r *themedTextRenderer) Objects() []fyne.CanvasObject { return []fyne.CanvasObject{r.w.text} }
func (r *themedTextRenderer) Destroy()                     {}

// bodyLabel e um texto com quebra de linha, cor secundaria.
func bodyLabel(s string) *widget.Label {
	l := widget.NewLabel(s)
	l.Wrapping = fyne.TextWrapWord
	return l
}

func mutedLabel(s string) *widget.Label {
	l := widget.NewLabel(s)
	l.Wrapping = fyne.TextWrapWord
	l.Importance = widget.LowImportance
	return l
}

// ---------------------------------------------------------------------------
// Superficies.
// ---------------------------------------------------------------------------

// card envolve o conteudo em um painel de vidro com padding de 16.
func card(content fyne.CanvasObject) fyne.CanvasObject {
	return cardPadded(content, 16)
}

func cardPadded(content fyne.CanvasObject, pad float32) fyne.CanvasObject {
	return newGlassPanel(content, pad, cardGlass)
}

// sunkenCard e a area rebaixada dentro de um cartao.
func sunkenCard(content fyne.CanvasObject, pad float32) fyne.CanvasObject {
	return newGlassPanel(content, pad, sunkenGlass)
}

// tintedCard e a faixa tingida na cor do tom (cliente selecionado, avisos).
func tintedCard(content fyne.CanvasObject, pad float32, tone statusTone) fyne.CanvasObject {
	return newGlassPanel(content, pad, tintGlass(tone))
}

func padded(content fyne.CanvasObject, pad float32) fyne.CanvasObject {
	return container.New(layout.NewCustomPaddedLayout(pad, pad, pad, pad), content)
}

func paddedTRBL(content fyne.CanvasObject, top, right, bottom, left float32) fyne.CanvasObject {
	return container.New(layout.NewCustomPaddedLayout(top, bottom, left, right), content)
}

// vcenter centraliza na vertical sem encolher a largura (ao contrario de
// container.NewCenter, que reduz o filho ao tamanho minimo).
func vcenter(obj fyne.CanvasObject) fyne.CanvasObject {
	return container.NewVBox(layout.NewSpacer(), obj, layout.NewSpacer())
}

// gap e um espacador fixo.
func gap(w, h float32) fyne.CanvasObject {
	r := canvas.NewRectangle(color.Transparent)
	r.SetMinSize(fyne.NewSize(w, h))
	return r
}

// ---------------------------------------------------------------------------
// Botoes.
// ---------------------------------------------------------------------------

// ---------------------------------------------------------------------------
// tapCard: superficie clicavel com hover (linhas de lista, tiles, sugestoes).
// ---------------------------------------------------------------------------

type tapCard struct {
	widget.BaseWidget
	content  fyne.CanvasObject
	OnTapped func()
	radius   float32
	pad      float32
	paint    func(t designTokens, hovered bool) (fill color.Color, stroke color.Color)
	hovered  bool
	minSize  fyne.Size
}

func newTapCard(content fyne.CanvasObject, radius float32, pad float32, onTap func()) *tapCard {
	c := &tapCard{content: content, OnTapped: onTap, radius: radius, pad: pad}
	c.paint = func(t designTokens, hovered bool) (color.Color, color.Color) {
		if hovered {
			return t.Hover, t.GlassBorder
		}
		return t.GlassSunken, t.GlassBorder
	}
	c.ExtendBaseWidget(c)
	return c
}

func (c *tapCard) Tapped(*fyne.PointEvent) {
	if c.OnTapped != nil {
		c.OnTapped()
	}
}

func (c *tapCard) MouseIn(*desktop.MouseEvent) { c.hovered = true; c.Refresh() }
func (c *tapCard) MouseMoved(*desktop.MouseEvent) {
}
func (c *tapCard) MouseOut()              { c.hovered = false; c.Refresh() }
func (c *tapCard) Cursor() desktop.Cursor { return desktop.PointerCursor }

// No celular nao existe hover: o toque acende a superficie enquanto o dedo
// esta nela, que e o unico retorno visual possivel.
func (c *tapCard) TouchDown(*mobile.TouchEvent) {
	c.hovered = true
	c.Refresh()
}

func (c *tapCard) TouchUp(*mobile.TouchEvent) {
	c.hovered = false
	c.Refresh()
}

func (c *tapCard) TouchCancel(*mobile.TouchEvent) {
	c.hovered = false
	c.Refresh()
}

func (c *tapCard) MinSize() fyne.Size {
	m := c.content.MinSize().Add(fyne.NewSize(c.pad*2, c.pad*2))
	return m.Max(c.minSize)
}

func (c *tapCard) CreateRenderer() fyne.WidgetRenderer {
	rect := canvas.NewRectangle(color.Transparent)
	rect.CornerRadius = c.radius
	rect.StrokeWidth = 1
	r := &tapCardRenderer{w: c, rect: rect}
	r.Refresh()
	return r
}

type tapCardRenderer struct {
	w    *tapCard
	rect *canvas.Rectangle
}

func (r *tapCardRenderer) Layout(size fyne.Size) {
	r.rect.Resize(size)
	pad := r.w.pad
	r.w.content.Move(fyne.NewPos(pad, pad))
	r.w.content.Resize(fyne.NewSize(size.Width-pad*2, size.Height-pad*2))
}
func (r *tapCardRenderer) MinSize() fyne.Size { return r.w.MinSize() }
func (r *tapCardRenderer) Refresh() {
	fill, stroke := r.w.paint(tk(), r.w.hovered)
	r.rect.FillColor = fill
	r.rect.StrokeColor = stroke
	r.rect.Refresh()
	r.w.content.Refresh()
}
func (r *tapCardRenderer) Objects() []fyne.CanvasObject {
	return []fyne.CanvasObject{r.rect, r.w.content}
}
func (r *tapCardRenderer) Destroy() {}

// ---------------------------------------------------------------------------
// Pilulas de status.
// ---------------------------------------------------------------------------

type statusTone int

const (
	toneNeutral statusTone = iota
	toneInfo
	toneSuccess
	toneWarning
	toneDanger
	toneBrand
)

func toneColors(tone statusTone, t designTokens) (fg color.Color, soft color.Color) {
	switch tone {
	case toneInfo:
		return t.Info, t.InfoSoft
	case toneSuccess:
		return t.Success, t.SuccessSoft
	case toneWarning:
		return t.Warning, t.WarningSoft
	case toneDanger:
		return t.Danger, t.DangerSoft
	case toneBrand:
		return t.Brand, t.BrandSoft
	default:
		return t.Neutral, t.NeutralSoft
	}
}

// orderStatusTone mapeia os status de OS para um tom. O vermelho da marca nunca
// representa status: vermelho no app sempre significa acao/identidade.
//
// A leitura e por quem esta esperando: amarelo espera o cliente responder, azul
// esta na bancada, verde esta pronto e esperando alguem vir buscar, cinza ja
// acabou. Entregue virou cinza quando "Pronto para retirada" entrou: com os dois
// em verde, o unico estado que pede acao sumia no meio de 96 OS fechadas.
func orderStatusTone(status string) statusTone {
	switch status {
	case statusPending:
		return toneWarning
	case statusApproved:
		return toneInfo
	case statusReady:
		return toneSuccess
	case statusDelivered:
		return toneNeutral
	case statusUnauthorized:
		return toneDanger
	case statusNoRepair:
		return toneNeutral
	default:
		return toneNeutral
	}
}

func termStatusTone(status string) statusTone {
	switch status {
	case termStatusSigned:
		return toneSuccess
	case termStatusGenerated:
		return toneInfo
	default:
		return toneWarning
	}
}

type statusPill struct {
	widget.BaseWidget
	label string
	tone  statusTone
	rect  *canvas.Rectangle
	dot   *canvas.Circle
	text  *canvas.Text
}

func newStatusPill(label string, tone statusTone) *statusPill {
	p := &statusPill{label: label, tone: tone}
	p.rect = canvas.NewRectangle(color.Transparent)
	p.rect.CornerRadius = pillHeight / 2
	p.dot = canvas.NewCircle(color.Transparent)
	p.text = canvas.NewText(label, color.Black)
	p.text.TextSize = sizeCaption
	p.text.TextStyle = fyne.TextStyle{Bold: true}
	p.ExtendBaseWidget(p)
	return p
}

func (p *statusPill) Set(label string, tone statusTone) {
	p.label = label
	p.tone = tone
	p.text.Text = label
	p.Refresh()
}

const (
	pillHeight = 22
	pillDot    = 6
	pillPadX   = 9
	pillGap    = 6
)

func (p *statusPill) MinSize() fyne.Size {
	w := pillPadX*2 + pillDot + pillGap + p.text.MinSize().Width
	return fyne.NewSize(w, pillHeight)
}

func (p *statusPill) CreateRenderer() fyne.WidgetRenderer {
	r := &statusPillRenderer{p: p}
	r.Refresh()
	return r
}

type statusPillRenderer struct {
	p *statusPill
}

func (r *statusPillRenderer) Layout(size fyne.Size) {
	p := r.p
	min := p.MinSize()
	// A pilula nao estica: fica alinhada a esquerda dentro do espaco dado.
	p.rect.Resize(fyne.NewSize(min.Width, pillHeight))
	p.rect.Move(fyne.NewPos(0, (size.Height-pillHeight)/2))
	p.dot.Resize(fyne.NewSize(pillDot, pillDot))
	p.dot.Move(fyne.NewPos(pillPadX, (size.Height-pillDot)/2))
	textSize := p.text.MinSize()
	p.text.Resize(textSize)
	p.text.Move(fyne.NewPos(pillPadX+pillDot+pillGap, (size.Height-textSize.Height)/2))
}
func (r *statusPillRenderer) MinSize() fyne.Size { return r.p.MinSize() }
func (r *statusPillRenderer) Refresh() {
	fg, soft := toneColors(r.p.tone, tk())
	r.p.rect.FillColor = soft
	r.p.dot.FillColor = fg
	r.p.text.Color = fg
	r.p.rect.Refresh()
	r.p.dot.Refresh()
	r.p.text.Refresh()
}
func (r *statusPillRenderer) Objects() []fyne.CanvasObject {
	return []fyne.CanvasObject{r.p.rect, r.p.dot, r.p.text}
}
func (r *statusPillRenderer) Destroy() {}

// ---------------------------------------------------------------------------
// Icone tingido pelo tema (segue a cor de texto ou a marca).
// ---------------------------------------------------------------------------

func themedIcon(res fyne.Resource) fyne.Resource { return theme.NewThemedResource(res) }

func brandIcon(res fyne.Resource) fyne.Resource {
	return theme.NewColoredResource(res, theme.ColorNamePrimary)
}

func mutedIcon(res fyne.Resource) fyne.Resource {
	return theme.NewColoredResource(res, theme.ColorNameDisabled)
}

func onBrandIcon(res fyne.Resource) fyne.Resource {
	return theme.NewColoredResource(res, theme.ColorNameForegroundOnPrimary)
}

// iconChip e o quadradinho tingido com um icone dentro (tiles e estados vazios).
func iconChip(res fyne.Resource, size float32, tone statusTone) fyne.CanvasObject {
	bg := newThemedRect(radiusMd, 0, func(t designTokens) (color.Color, color.Color) {
		_, soft := toneColors(tone, t)
		return soft, color.Transparent
	})
	bg.minSize = fyne.NewSize(size, size)
	return container.NewStack(bg, container.NewCenter(newToneIcon(res, tone, size*0.5)))
}

// toneIcon desenha um icone SVG na cor do tom, em um tamanho fixo.
type toneIconWidget struct {
	widget.BaseWidget
	res  fyne.Resource
	tone statusTone
	size float32
	img  *canvas.Image
}

func newToneIcon(res fyne.Resource, tone statusTone, size float32) *toneIconWidget {
	w := &toneIconWidget{res: res, tone: tone, size: size}
	w.img = canvas.NewImageFromResource(nil)
	w.img.FillMode = canvas.ImageFillContain
	w.ExtendBaseWidget(w)
	return w
}

func (w *toneIconWidget) SetResource(res fyne.Resource) {
	w.res = res
	w.Refresh()
}

func (w *toneIconWidget) MinSize() fyne.Size { return fyne.NewSize(w.size, w.size) }

func (w *toneIconWidget) CreateRenderer() fyne.WidgetRenderer {
	r := &toneIconRenderer{w: w}
	r.Refresh()
	return r
}

type toneIconRenderer struct{ w *toneIconWidget }

func (r *toneIconRenderer) Layout(size fyne.Size) {
	s := r.w.size
	r.w.img.Resize(fyne.NewSize(s, s))
	r.w.img.Move(fyne.NewPos((size.Width-s)/2, (size.Height-s)/2))
}
func (r *toneIconRenderer) MinSize() fyne.Size { return r.w.MinSize() }
func (r *toneIconRenderer) Refresh() {
	if r.w.res == nil {
		r.w.img.Resource = nil
	} else {
		r.w.img.Resource = theme.NewColoredResource(r.w.res, colorNameForTone(r.w.tone))
	}
	r.w.img.Refresh()
}
func (r *toneIconRenderer) Objects() []fyne.CanvasObject { return []fyne.CanvasObject{r.w.img} }
func (r *toneIconRenderer) Destroy()                     {}

func colorNameForTone(tone statusTone) fyne.ThemeColorName {
	switch tone {
	case toneBrand:
		return theme.ColorNamePrimary
	case toneSuccess:
		return theme.ColorNameSuccess
	case toneWarning:
		return theme.ColorNameWarning
	case toneDanger:
		return theme.ColorNameError
	case toneInfo:
		return theme.ColorNameHyperlink
	default:
		return theme.ColorNameDisabled
	}
}

// ---------------------------------------------------------------------------
// Tile de indicador (KPI).
// ---------------------------------------------------------------------------

// statTileGlass: no escuro, borda e brilho na cor do tom; no claro, filete
// colorido no topo e sombra suave.
func statTileGlass(tone statusTone) func(designTokens) glassStyle {
	return func(t designTokens) glassStyle {
		fg, _ := toneColors(tone, t)
		st := cardGlass(t)
		if isDarkMode() {
			st.Border = nrgbaAlpha(fg, 0.5)
			st.Glow = nrgbaAlpha(fg, 0.28)
			st.GlowR = 22
		} else {
			st.TopAccent = nrgbaAlpha(fg, 1)
		}
		return st
	}
}

type statTile struct {
	widget.BaseWidget
	value   *themedText
	label   *themedText
	sub     *themedText
	icon    *toneIconWidget
	chip    *themedRect
	tone    statusTone
	OnTap   func()
	hovered bool
	bg      *glassPanel
	arrow   *arrowButton
}

func newStatTile(label string, icon fyne.Resource, tone statusTone) *statTile {
	s := &statTile{tone: tone}
	s.value = newText("0", sizeDisplay, true, inkPrimary)
	s.label = newEyebrow(label).SetTruncate(true)
	s.sub = newText("", sizeCaption, false, inkMuted).SetTruncate(true)
	s.icon = newToneIcon(icon, tone, 22)
	s.chip = newThemedRect(radiusMd, 0, func(t designTokens) (color.Color, color.Color) {
		_, soft := toneColors(tone, t)
		return soft, color.Transparent
	})
	s.chip.minSize = fyne.NewSize(44, 44)
	s.bg = newGlassPanel(nil, 0, statTileGlass(tone))
	s.arrow = newArrowButton(tone, func() {
		if s.OnTap != nil {
			s.OnTap()
		}
	})
	if isMobileLayout() {
		s.arrow.Hide()
	}
	s.ExtendBaseWidget(s)
	return s
}

func (s *statTile) SetValue(v string) { s.value.SetText(v) }
func (s *statTile) SetSub(v string)   { s.sub.SetText(v) }

// SetLabel troca o titulo depois de montado. O resultado do periodo so pode se
// chamar "lucro" quando o custo de todas as OS foi lancado.
func (s *statTile) SetLabel(v string) { s.label.SetText(v) }

func (s *statTile) Tapped(*fyne.PointEvent) {
	if s.OnTap != nil {
		s.OnTap()
	}
}
func (s *statTile) MouseIn(*desktop.MouseEvent)    { s.hovered = true; s.Refresh() }
func (s *statTile) MouseMoved(*desktop.MouseEvent) {}
func (s *statTile) MouseOut()                      { s.hovered = false; s.Refresh() }
func (s *statTile) Cursor() desktop.Cursor {
	if s.OnTap != nil {
		return desktop.PointerCursor
	}
	return desktop.DefaultCursor
}

const (
	statTilePad    = 16
	statTileHeight = 116
)

func (s *statTile) MinSize() fyne.Size {
	// O valor e a seta ficam em alturas diferentes, entao a largura minima nao
	// precisa reservar espaco para a seta.
	w := statTilePad*2 + 44 + 14 + s.value.MinSize().Width + 4
	return fyne.NewSize(fyne.Max(w, 150), statTileHeight)
}

func (s *statTile) CreateRenderer() fyne.WidgetRenderer {
	r := &statTileRenderer{s: s}
	r.Refresh()
	return r
}

type statTileRenderer struct{ s *statTile }

func (r *statTileRenderer) Layout(size fyne.Size) {
	s := r.s
	s.bg.Resize(size)
	s.bg.Move(fyne.NewPos(0, 0))
	s.chip.Resize(fyne.NewSize(44, 44))
	s.chip.Move(fyne.NewPos(statTilePad, statTilePad+2))
	s.icon.Resize(fyne.NewSize(44, 44))
	s.icon.Move(fyne.NewPos(statTilePad, statTilePad+2))

	x := float32(statTilePad + 44 + 14)
	arrowSize := float32(30)
	reserve := arrowSize + 8
	if s.arrow.Hidden {
		reserve = 0
	}
	avail := size.Width - x - statTilePad - reserve
	labelSize := s.label.MinSize()
	s.label.Resize(fyne.NewSize(avail, labelSize.Height))
	s.label.Move(fyne.NewPos(x, statTilePad+2))
	valueSize := s.value.MinSize()
	s.value.Resize(valueSize)
	s.value.Move(fyne.NewPos(x, statTilePad+labelSize.Height+4))
	subSize := s.sub.MinSize()
	s.sub.Resize(fyne.NewSize(avail, subSize.Height))
	s.sub.Move(fyne.NewPos(x, statTilePad+labelSize.Height+valueSize.Height+8))
	s.arrow.Resize(fyne.NewSize(arrowSize, arrowSize))
	s.arrow.Move(fyne.NewPos(size.Width-statTilePad-arrowSize, size.Height-statTilePad-arrowSize))
}
func (r *statTileRenderer) MinSize() fyne.Size { return r.s.MinSize() }
func (r *statTileRenderer) Refresh() {
	s := r.s
	s.bg.Refresh()
	s.chip.Refresh()
	s.icon.Refresh()
	s.label.Refresh()
	s.value.Refresh()
	s.sub.Refresh()
	s.arrow.hovered = s.hovered
	s.arrow.Refresh()
}
func (r *statTileRenderer) Objects() []fyne.CanvasObject {
	s := r.s
	return []fyne.CanvasObject{s.bg, s.chip, s.icon, s.label, s.value, s.sub, s.arrow}
}
func (r *statTileRenderer) Destroy() {}

// ---------------------------------------------------------------------------
// Botao redondo de seta (cantos dos tiles e acoes de linha).
// ---------------------------------------------------------------------------

type arrowButton struct {
	widget.BaseWidget
	tone    statusTone
	onTap   func()
	hovered bool
	circle  *canvas.Circle
	icon    *canvas.Image
	size    float32
}

func newArrowButton(tone statusTone, onTap func()) *arrowButton {
	a := &arrowButton{tone: tone, onTap: onTap, size: 30}
	a.circle = canvas.NewCircle(color.Transparent)
	a.circle.StrokeWidth = 1
	a.icon = canvas.NewImageFromResource(nil)
	a.icon.FillMode = canvas.ImageFillContain
	a.ExtendBaseWidget(a)
	return a
}

func (a *arrowButton) Tapped(*fyne.PointEvent) {
	if a.onTap != nil {
		a.onTap()
	}
}
func (a *arrowButton) MouseIn(*desktop.MouseEvent)    { a.hovered = true; a.Refresh() }
func (a *arrowButton) MouseMoved(*desktop.MouseEvent) {}
func (a *arrowButton) MouseOut()                      { a.hovered = false; a.Refresh() }
func (a *arrowButton) Cursor() desktop.Cursor         { return desktop.PointerCursor }
func (a *arrowButton) MinSize() fyne.Size             { return fyne.NewSize(a.size, a.size) }

func (a *arrowButton) CreateRenderer() fyne.WidgetRenderer {
	r := &arrowButtonRenderer{a: a}
	r.Refresh()
	return r
}

type arrowButtonRenderer struct{ a *arrowButton }

func (r *arrowButtonRenderer) Layout(size fyne.Size) {
	s := r.a.size
	off := fyne.NewPos((size.Width-s)/2, (size.Height-s)/2)
	r.a.circle.Resize(fyne.NewSize(s, s))
	r.a.circle.Move(off)
	r.a.icon.Resize(fyne.NewSize(16, 16))
	r.a.icon.Move(off.Add(fyne.NewPos((s-16)/2, (s-16)/2)))
}
func (r *arrowButtonRenderer) MinSize() fyne.Size { return r.a.MinSize() }
func (r *arrowButtonRenderer) Refresh() {
	t := tk()
	fg, soft := toneColors(r.a.tone, t)
	if r.a.hovered {
		r.a.circle.FillColor = fg
		r.a.circle.StrokeColor = fg
		r.a.icon.Resource = theme.NewColoredResource(theme.NavigateNextIcon(), theme.ColorNameForegroundOnPrimary)
	} else {
		r.a.circle.FillColor = soft
		r.a.circle.StrokeColor = rgbaOf(fg, 0.45)
		r.a.icon.Resource = theme.NewColoredResource(theme.NavigateNextIcon(), colorNameForTone(r.a.tone))
	}
	r.a.circle.Refresh()
	r.a.icon.Refresh()
}
func (r *arrowButtonRenderer) Objects() []fyne.CanvasObject {
	return []fyne.CanvasObject{r.a.circle, r.a.icon}
}
func (r *arrowButtonRenderer) Destroy() {}

// ---------------------------------------------------------------------------
// Cabecalhos.
// ---------------------------------------------------------------------------

// pageTitleBlock monta titulo + subtitulo + acoes a direita, com regua embaixo.
func pageTitleBlock(title string, subtitle string, actions ...fyne.CanvasObject) (fyne.CanvasObject, *themedText, *themedText) {
	titleText := newText(title, sizeTitle, true, inkPrimary)
	subText := newText(subtitle, sizeBody, false, inkSecondary)
	texts := container.NewVBox(titleText, subText)
	var right fyne.CanvasObject
	if len(actions) > 0 {
		right = container.NewCenter(container.NewHBox(actions...))
	}
	row := container.NewBorder(nil, nil, nil, right, texts)
	block := container.NewVBox(row, gap(1, 10), hairline(), gap(1, 6))
	return block, titleText, subText
}

// sectionHeader e a regua com rotulo em caixa alta que separa blocos de um formulario.
func sectionHeader(title string) fyne.CanvasObject {
	label := newEyebrow(title)
	line := hairline()
	return container.NewBorder(nil, nil, label, nil, container.NewVBox(layout.NewSpacer(), container.NewPadded(line), layout.NewSpacer()))
}

// sectionHeaderWithAction adiciona um botao/acao a direita da regua.
func sectionHeaderWithAction(title string, action fyne.CanvasObject) fyne.CanvasObject {
	label := newEyebrow(title)
	line := hairline()
	return container.NewBorder(nil, nil, label, action, container.NewVBox(layout.NewSpacer(), container.NewPadded(line), layout.NewSpacer()))
}

// fillWidthLayout entrega ao filho a largura do container mesmo que o minimo
// dele seja maior (Selects com opcoes longas no celular).
type fillWidthLayout struct{}

func (fillWidthLayout) Layout(objects []fyne.CanvasObject, size fyne.Size) {
	for _, o := range objects {
		o.Move(fyne.NewPos(0, 0))
		o.Resize(size)
	}
}

func (fillWidthLayout) MinSize(objects []fyne.CanvasObject) fyne.Size {
	var h float32
	for _, o := range objects {
		if m := o.MinSize(); m.Height > h {
			h = m.Height
		}
	}
	return fyne.NewSize(40, h)
}

// field = rotulo pequeno em cima do campo + dica opcional embaixo.
func field(label string, input fyne.CanvasObject, hint string) fyne.CanvasObject {
	if _, isSelect := input.(*widget.Select); isSelect && isMobileLayout() {
		input = container.New(fillWidthLayout{}, input)
	}
	items := []fyne.CanvasObject{newText(label, sizeCaption, true, inkSecondary), input}
	if hint != "" {
		items = append(items, newText(hint, sizeCaption, false, inkMuted).SetTruncate(true))
	}
	return container.NewVBox(items...)
}

// fieldGrid coloca campos em 2 colunas no desktop e 1 no celular.
func fieldGrid(cols int, fields ...fyne.CanvasObject) fyne.CanvasObject {
	if isMobileLayout() || cols <= 1 {
		return container.NewVBox(fields...)
	}
	return container.NewGridWithColumns(cols, fields...)
}

// keyValue e a linha "rotulo ..... valor" usada em resumos laterais.
func keyValue(label string, value string) (fyne.CanvasObject, *themedText) {
	k := newText(label, sizeCaption, false, inkMuted)
	v := newText(value, sizeBody, true, inkPrimary)
	v.SetAlignment(fyne.TextAlignTrailing)
	return container.NewBorder(nil, nil, k, v, nil), v
}

// ---------------------------------------------------------------------------
// Layout de colunas proporcionais (tabelas).
// ---------------------------------------------------------------------------

type columnsLayout struct {
	weights []float32
	gap     float32
}

func newColumnsLayout(gap float32, weights ...float32) *columnsLayout {
	return &columnsLayout{weights: weights, gap: gap}
}

func (l *columnsLayout) Layout(objects []fyne.CanvasObject, size fyne.Size) {
	n := len(objects)
	if n == 0 {
		return
	}
	var total float32
	for i := 0; i < n; i++ {
		total += l.weight(i)
	}
	avail := size.Width - l.gap*float32(n-1)
	x := float32(0)
	for i, o := range objects {
		w := avail * l.weight(i) / total
		o.Move(fyne.NewPos(x, 0))
		o.Resize(fyne.NewSize(w, size.Height))
		x += w + l.gap
	}
}

func (l *columnsLayout) weight(i int) float32 {
	if i < len(l.weights) && l.weights[i] > 0 {
		return l.weights[i]
	}
	return 1
}

func (l *columnsLayout) MinSize(objects []fyne.CanvasObject) fyne.Size {
	var h float32
	var w float32
	for i, o := range objects {
		m := o.MinSize()
		if m.Height > h {
			h = m.Height
		}
		w += m.Width
		if i > 0 {
			w += l.gap
		}
	}
	return fyne.NewSize(w, h)
}

// tableHeader monta a linha de cabecalho em caixa alta sobre fundo rebaixado.
func tableHeader(gapSize float32, weights []float32, titles ...string) fyne.CanvasObject {
	cells := make([]fyne.CanvasObject, 0, len(titles))
	for _, t := range titles {
		cells = append(cells, newEyebrow(t))
	}
	row := container.New(newColumnsLayout(gapSize, weights...), cells...)
	bg := newFill(radiusSm, func(t designTokens) color.Color { return t.GlassSunken })
	return container.NewStack(bg, paddedTRBL(row, 8, 12, 8, 12))
}

// cellText e o texto de uma celula de tabela, com reticencias.
func cellText(s string, bold bool, ink inkLevel) *themedText {
	size := float32(sizeBody)
	if ink == inkMuted {
		size = sizeCaption
	}
	return newText(s, size, bold, ink).SetTruncate(true)
}

// ---------------------------------------------------------------------------
// Avatar com iniciais ou foto.
// ---------------------------------------------------------------------------

type avatar struct {
	widget.BaseWidget
	size     float32
	initials *canvas.Text
	circle   *canvas.Circle
	img      *canvas.Image
	name     string
	online   bool
	dot      *canvas.Circle
}

func newAvatar(size float32) *avatar {
	a := &avatar{size: size}
	a.circle = canvas.NewCircle(color.Transparent)
	a.initials = canvas.NewText("", color.White)
	a.initials.TextStyle = fyne.TextStyle{Bold: true}
	a.initials.TextSize = size * 0.38
	a.initials.Alignment = fyne.TextAlignCenter
	a.img = canvas.NewImageFromResource(nil)
	a.img.FillMode = canvas.ImageFillContain
	a.img.CornerRadius = size / 2
	a.img.Hide()
	a.dot = canvas.NewCircle(color.Transparent)
	a.dot.StrokeWidth = 2
	a.dot.Hide()
	a.ExtendBaseWidget(a)
	return a
}

// SetOnline mostra o ponto verde de "conectado" no canto do avatar.
func (a *avatar) SetOnline(on bool) {
	a.online = on
	if on {
		a.dot.Show()
	} else {
		a.dot.Hide()
	}
	a.Refresh()
}

func initialsOf(name string) string {
	parts := strings.Fields(strings.TrimSpace(name))
	if len(parts) == 0 {
		return "?"
	}
	first := []rune(parts[0])
	out := string(unicode.ToUpper(first[0]))
	if len(parts) > 1 {
		last := []rune(parts[len(parts)-1])
		out += string(unicode.ToUpper(last[0]))
	}
	return out
}

// SetName define as iniciais (e remove a foto).
func (a *avatar) SetName(name string) {
	a.name = name
	a.initials.Text = initialsOf(name)
	a.img.Image = nil
	a.img.Hide()
	a.initials.Show()
	a.circle.Show()
	a.Refresh()
}

// SetPhotoURL carrega a foto (com cache) e mostra quando chegar.
func (a *avatar) SetPhotoURL(rawURL string) {
	rawURL = strings.TrimSpace(rawURL)
	if rawURL == "" {
		return
	}
	loadImageCached(rawURL, func(img fyneImage) {
		fyne.Do(func() {
			a.img.Image = img
			a.img.Show()
			a.initials.Hide()
			a.circle.Hide()
			a.Refresh()
		})
	})
}

func (a *avatar) MinSize() fyne.Size { return fyne.NewSize(a.size, a.size) }

func (a *avatar) CreateRenderer() fyne.WidgetRenderer {
	r := &avatarRenderer{a: a}
	r.Refresh()
	return r
}

type avatarRenderer struct{ a *avatar }

func (r *avatarRenderer) Layout(size fyne.Size) {
	s := r.a.size
	off := fyne.NewPos((size.Width-s)/2, (size.Height-s)/2)
	r.a.circle.Resize(fyne.NewSize(s, s))
	r.a.circle.Move(off)
	r.a.img.Resize(fyne.NewSize(s, s))
	r.a.img.Move(off)
	r.a.initials.Resize(fyne.NewSize(s, s))
	r.a.initials.Move(off)
	d := s * 0.32
	r.a.dot.Resize(fyne.NewSize(d, d))
	r.a.dot.Move(off.Add(fyne.NewPos(s-d+1, s-d+1)))
}
func (r *avatarRenderer) MinSize() fyne.Size { return r.a.MinSize() }
func (r *avatarRenderer) Refresh() {
	t := tk()
	r.a.circle.FillColor = avatarColor(r.a.name, t)
	r.a.initials.Color = t.InkOnBrand
	r.a.dot.FillColor = t.Success
	r.a.dot.StrokeColor = t.Canvas
	r.a.circle.Refresh()
	r.a.initials.Refresh()
	r.a.img.Refresh()
	r.a.dot.Refresh()
}
func (r *avatarRenderer) Objects() []fyne.CanvasObject {
	return []fyne.CanvasObject{r.a.circle, r.a.img, r.a.initials, r.a.dot}
}
func (r *avatarRenderer) Destroy() {}

// avatarColor escolhe uma cor estavel por nome (paleta discreta, sem o vermelho
// da marca para nao competir com botoes).
func avatarColor(name string, t designTokens) color.Color {
	palette := []color.Color{
		rgb(0x4F6BED), rgb(0x0F9D8A), rgb(0xB8891C), rgb(0x8A5CF6), rgb(0xD9538A), rgb(0x2F8FBA), rgb(0x6B7480),
	}
	var h uint32
	for _, r := range strings.ToLower(name) {
		h = h*31 + uint32(r)
	}
	c := palette[int(h%uint32(len(palette)))]
	if isDarkMode() {
		return mixColor(c, t.Canvas, 0.15)
	}
	return c
}

// ---------------------------------------------------------------------------
// Estado vazio.
// ---------------------------------------------------------------------------

func emptyState(icon fyne.Resource, title string, body string, action fyne.CanvasObject) fyne.CanvasObject {
	chipGlass := newGlassPanel(container.NewCenter(newToneIcon(icon, toneBrand, 30)), 0, func(t designTokens) glassStyle {
		return glassStyle{Fill: nrgba(t.BrandSoft), Border: nrgbaAlpha(t.Brand, 0.5), BorderW: 1, Radius: radiusLg, Glow: nrgbaAlpha(t.Brand, 0.5), GlowR: 28}
	})
	chipBox := container.NewGridWrap(fyne.NewSize(64, 64), chipGlass)
	titleText := newText(title, sizeSection, true, inkPrimary)
	titleText.SetAlignment(fyne.TextAlignCenter)
	bodyText := widget.NewLabel(body)
	bodyText.Alignment = fyne.TextAlignCenter
	bodyText.Wrapping = fyne.TextWrapWord
	bodyText.Importance = widget.LowImportance
	items := []fyne.CanvasObject{
		container.NewCenter(chipBox),
		gap(1, 6),
		titleText,
		bodyText,
	}
	if action != nil {
		items = append(items, gap(1, 4), container.NewCenter(action))
	}
	return container.NewCenter(container.NewVBox(items...))
}

// ---------------------------------------------------------------------------
// Dica de atalho (ex.: Ctrl+K).
// ---------------------------------------------------------------------------

func kbdHint(text string) fyne.CanvasObject {
	bg := newThemedRect(radiusSm, 1, func(t designTokens) (color.Color, color.Color) {
		return t.SurfaceSunken, t.Border
	})
	label := newText(text, sizeMicro, true, inkMuted)
	return container.NewCenter(container.NewStack(bg, paddedTRBL(label, 2, 6, 2, 6)))
}

// ---------------------------------------------------------------------------
// Toast: aviso curto que some sozinho (feedback de salvar, sincronizar...).
// ---------------------------------------------------------------------------

type toastHost struct {
	widget.BaseWidget
	text    *canvas.Text
	rect    *canvas.Rectangle
	rail    *canvas.Rectangle
	tone    statusTone
	timer   *time.Timer
	message string
}

func newToastHost() *toastHost {
	h := &toastHost{}
	h.rect = canvas.NewRectangle(color.Transparent)
	h.rect.CornerRadius = radiusMd
	h.rect.StrokeWidth = 1
	h.rail = canvas.NewRectangle(color.Transparent)
	h.rail.CornerRadius = 2
	h.text = canvas.NewText("", color.White)
	h.text.TextSize = sizeBody
	h.text.TextStyle = fyne.TextStyle{Bold: true}
	h.ExtendBaseWidget(h)
	h.Hide()
	return h
}

// ShowMessage exibe a mensagem por alguns segundos.
func (h *toastHost) ShowMessage(message string, tone statusTone) {
	h.message = message
	h.tone = tone
	h.text.Text = message
	if h.timer != nil {
		h.timer.Stop()
	}
	h.BaseWidget.Show()
	h.Refresh()
	h.timer = time.AfterFunc(3200*time.Millisecond, func() {
		fyne.Do(func() { h.Hide() })
	})
}

func (h *toastHost) MinSize() fyne.Size {
	if h.Hidden {
		return fyne.NewSize(0, 0)
	}
	return fyne.NewSize(h.text.MinSize().Width+16*2+6, 40)
}

func (h *toastHost) CreateRenderer() fyne.WidgetRenderer {
	r := &toastRenderer{h: h}
	r.Refresh()
	return r
}

type toastRenderer struct{ h *toastHost }

func (r *toastRenderer) Layout(size fyne.Size) {
	r.h.rect.Resize(size)
	r.h.rail.Resize(fyne.NewSize(3, size.Height-16))
	r.h.rail.Move(fyne.NewPos(10, 8))
	ts := r.h.text.MinSize()
	r.h.text.Resize(ts)
	r.h.text.Move(fyne.NewPos(22, (size.Height-ts.Height)/2))
}
func (r *toastRenderer) MinSize() fyne.Size { return r.h.MinSize() }
func (r *toastRenderer) Refresh() {
	t := tk()
	fg, _ := toneColors(r.h.tone, t)
	if isDarkMode() {
		r.h.rect.FillColor = rgb(0x2A2D33)
	} else {
		r.h.rect.FillColor = rgb(0x1F2328)
	}
	r.h.rect.StrokeColor = rgbaOf(fg, 0.5)
	r.h.rail.FillColor = fg
	r.h.text.Color = rgb(0xF5F6F8)
	r.h.rect.Refresh()
	r.h.rail.Refresh()
	r.h.text.Refresh()
}
func (r *toastRenderer) Objects() []fyne.CanvasObject {
	return []fyne.CanvasObject{r.h.rect, r.h.rail, r.h.text}
}
func (r *toastRenderer) Destroy() {}
