package main

import (
	"image"
	"image/color"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/driver/desktop"
	"fyne.io/fyne/v2/driver/mobile"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
)

// ---------------------------------------------------------------------------
// Botoes DBS
//
// O botao do Fyne e um retangulo de uma cor so, e era isso que dava ao app cara
// de template pronto. Aqui o botao e desenhado no mesmo rasterizador dos
// paineis de vidro e carrega a assinatura da marca:
//
//   - gradiente na diagonal, luz batendo na aresta de cima e sombra interna na
//     base, que e o que faz a peca ter volume em vez de ser um retangulo chapado;
//   - um feixe de luz diagonal atravessando a superficie, no mesmo angulo dos
//     fachos da arte de fundo - e o detalhe que amarra o botao ao resto do
//     sistema visual;
//   - grao fino na superficie, para nao parecer plastico;
//   - sombra projetada colorida (deslocada para baixo), em vez de um halo
//     neon em volta.
//
// Cada estado e uma imagem propria, e a passagem de repouso para hover e
// animada em 130 ms: o feixe desliza e o brilho cresce. Pressionado afunda o
// conteudo 1 pt. O foco por teclado desenha um anel proprio.
// ---------------------------------------------------------------------------

type buttonVariant int

const (
	variantPrimary   buttonVariant = iota // acao principal da tela (vermelho solido)
	variantSecondary                      // acao de apoio (vidro com contorno)
	variantGhost                          // acao discreta (so texto, ganha corpo no hover)
	variantDanger                         // acao destrutiva (tingida, nunca solida em repouso)
	variantIconOnly                       // botao quadrado so com icone
)

const (
	buttonHeight      = 38
	buttonHeightTouch = 46
	buttonPadX        = 15
	buttonIconGap     = 9
	buttonIconSize    = 16
	buttonRadius      = radiusMd
	buttonGlowR       = 20
	buttonFadeMS      = 130 * time.Millisecond
)

type dbsButton struct {
	widget.BaseWidget
	text     string
	icon     fyne.Resource
	variant  buttonVariant
	OnTapped func()

	disabled bool
	hovered  bool
	pressed  bool
	focused  bool

	rest  *canvas.Image
	hot   *canvas.Image
	press *canvas.Image
	ring  *canvas.Image
	label *canvas.Text
	glyph *canvas.Image

	hotFade    float64
	anim       *fyne.Animation
	hotReady   bool
	pressReady bool

	lastW, lastH float32
	lastMode     themeMode
	lastScale    float32
	minSize      fyne.Size
}

func newDBSButton(variant buttonVariant, label string, icon fyne.Resource, tapped func()) *dbsButton {
	b := &dbsButton{text: label, icon: icon, variant: variant, OnTapped: tapped}
	b.rest = newLayerImage()
	b.hot = newLayerImage()
	b.press = newLayerImage()
	b.ring = newLayerImage()
	b.hot.Translucency = 1
	b.press.Translucency = 1
	b.ring.Translucency = 1
	b.label = canvas.NewText(label, color.White)
	b.label.TextSize = sizeBody
	b.label.TextStyle = fyne.TextStyle{Bold: true}
	b.glyph = canvas.NewImageFromResource(nil)
	b.glyph.FillMode = canvas.ImageFillContain
	b.ExtendBaseWidget(b)
	return b
}

func newLayerImage() *canvas.Image {
	img := canvas.NewImageFromImage(image.NewNRGBA(image.Rect(0, 0, 1, 1)))
	img.FillMode = canvas.ImageFillStretch
	// Suave, nao vizinho mais proximo: o tamanho em pixels quase nunca bate
	// exatamente com o retangulo na tela, e o vizinho mais proximo comia
	// pedacos da borda de 1 pixel, deixando a linha falhada.
	img.ScaleMode = canvas.ImageScaleSmooth
	return img
}

// ---- API compativel com widget.Button ----

func (b *dbsButton) SetText(text string) {
	if b.text == text {
		return
	}
	b.text = text
	b.Refresh()
}

func (b *dbsButton) SetIcon(icon fyne.Resource) {
	b.icon = icon
	b.Refresh()
}

func (b *dbsButton) Enable() {
	if !b.disabled {
		return
	}
	b.disabled = false
	b.invalidate()
	b.Refresh()
}

func (b *dbsButton) Disable() {
	if b.disabled {
		return
	}
	b.disabled = true
	b.hovered, b.pressed = false, false
	b.hotFade = 0
	b.hot.Translucency = 1
	b.press.Translucency = 1
	b.invalidate()
	b.Refresh()
}

func (b *dbsButton) Disabled() bool { return b.disabled }

// SetMinWidth iguala a largura de botoes que ficam lado a lado.
func (b *dbsButton) SetMinWidth(w float32) {
	b.minSize = fyne.NewSize(w, 0)
	b.Refresh()
}

// ---- geometria ----

func buttonHeightFor() float32 {
	if isMobileLayout() {
		return buttonHeightTouch
	}
	return buttonHeight
}

func (b *dbsButton) MinSize() fyne.Size {
	h := buttonHeightFor()
	if b.variant == variantIconOnly {
		return fyne.NewSize(h, h).Max(b.minSize)
	}
	w := float32(buttonPadX * 2)
	if b.icon != nil {
		w += buttonIconSize
		if b.text != "" {
			w += buttonIconGap
		}
	}
	if b.text != "" {
		w += fyne.MeasureText(b.text, sizeBody, fyne.TextStyle{Bold: true}).Width
	}
	return fyne.NewSize(w, h).Max(b.minSize)
}

// ---- interacao ----

func (b *dbsButton) Tapped(*fyne.PointEvent) {
	if b.disabled || b.OnTapped == nil {
		return
	}
	b.OnTapped()
}

func (b *dbsButton) MouseIn(*desktop.MouseEvent) {
	if b.disabled {
		return
	}
	b.hovered = true
	b.ensureLayer(b.hot, false)
	b.refreshInk()
	b.animateHover(1)
}

func (b *dbsButton) MouseMoved(*desktop.MouseEvent) {}

func (b *dbsButton) MouseOut() {
	if !b.hovered {
		return
	}
	b.hovered = false
	b.refreshInk()
	b.animateHover(0)
}

func (b *dbsButton) MouseDown(*desktop.MouseEvent) {
	if b.disabled {
		return
	}
	b.pressed = true
	b.ensureLayer(b.press, true)
	b.press.Translucency = 0
	b.layoutContent(b.Size())
	canvas.Refresh(b.press)
}

func (b *dbsButton) MouseUp(*desktop.MouseEvent) {
	if !b.pressed {
		return
	}
	b.pressed = false
	b.press.Translucency = 1
	b.layoutContent(b.Size())
	canvas.Refresh(b.press)
}

// O Fyne no celular nunca chama os eventos de mouse: sem isto o toque nao
// mostrava nada e o operador tocava duas vezes achando que nao pegou.
func (b *dbsButton) TouchDown(*mobile.TouchEvent) { b.MouseDown(nil) }
func (b *dbsButton) TouchUp(*mobile.TouchEvent)   { b.MouseUp(nil) }
func (b *dbsButton) TouchCancel(*mobile.TouchEvent) {
	b.MouseUp(nil)
}

func (b *dbsButton) Cursor() desktop.Cursor {
	if b.disabled {
		return desktop.DefaultCursor
	}
	return desktop.PointerCursor
}

// Foco por teclado: anel visivel e Espaco/Enter acionam o botao.
func (b *dbsButton) FocusGained() {
	b.focused = true
	b.ensureRing()
	b.ring.Translucency = 0
	canvas.Refresh(b.ring)
}

func (b *dbsButton) FocusLost() {
	b.focused = false
	b.ring.Translucency = 1
	canvas.Refresh(b.ring)
}

func (b *dbsButton) TypedRune(r rune) {
	if r == ' ' {
		b.Tapped(nil)
	}
}

func (b *dbsButton) TypedKey(event *fyne.KeyEvent) {
	if event != nil && (event.Name == fyne.KeyReturn || event.Name == fyne.KeyEnter) {
		b.Tapped(nil)
	}
}

func (b *dbsButton) animateHover(target float64) {
	if b.anim != nil {
		b.anim.Stop()
		b.anim = nil
	}
	start := b.hotFade
	if start == target {
		return
	}
	b.anim = fyne.NewAnimation(buttonFadeMS, func(f float32) {
		b.hotFade = start + (target-start)*float64(f)
		b.hot.Translucency = 1 - b.hotFade
		canvas.Refresh(b.hot)
	})
	b.anim.Curve = fyne.AnimationEaseOut
	b.anim.Start()
}

func (b *dbsButton) invalidate() { b.lastW, b.lastH = 0, 0 }

func (b *dbsButton) phase() buttonPhase {
	switch {
	case b.disabled:
		return phaseDisabled
	case b.pressed:
		return phasePress
	case b.hovered:
		return phaseHover
	default:
		return phaseRest
	}
}

// refreshInk acerta a cor do texto e do icone para o estado atual (o fundo e
// animado, mas o texto troca junto com o estado).
func (b *dbsButton) refreshInk() {
	ink, iconColor := buttonInk(b.variant, b.phase(), tk())
	b.label.Color = ink
	b.label.Refresh()
	if b.icon != nil {
		b.glyph.Resource = theme.NewColoredResource(b.icon, iconColor)
		b.glyph.Refresh()
	}
}

// ---------------------------------------------------------------------------
// Estilos por variante e estado.
// ---------------------------------------------------------------------------

// brandShadow e o vermelho da marca escurecido, usado como sombra projetada.
func brandShadow(t designTokens) color.NRGBA {
	return mixNRGBA(nrgba(t.Brand), shade(1), 0.5)
}

func white(a float64) color.NRGBA { return color.NRGBA{R: 255, G: 255, B: 255, A: alpha8(a)} }
func shade(a float64) color.NRGBA { return color.NRGBA{A: alpha8(a)} }

func alpha8(a float64) uint8 { return uint8(clamp01(a)*255 + 0.5) }

type buttonPhase int

const (
	phaseRest buttonPhase = iota
	phaseHover
	phasePress
	phaseDisabled
)

// buttonStyle devolve o desenho do botao para a variante e o estado.
func buttonStyle(v buttonVariant, phase buttonPhase) func(designTokens) glassStyle {
	return func(t designTokens) glassStyle {
		st := glassStyle{Radius: buttonRadius, BorderW: 1, GlowR: 14, Pad: buttonGlowR, Diagonal: true, AccentH: 1.4}
		// inset deixa a peca com cara de afundada: a luz do topo vira sombra e a
		// base perde a sombra. Escurecer o corpo inteiro, que era o caminho
		// obvio, so deixava tudo cinza no tema claro.
		inset := func(st *glassStyle) {
			st.Sheen, st.SheenH = shade(0.17), 0.55
			st.InnerShade, st.InnerShadeH = shade(0.05), 0.3
			st.TopAccent = color.NRGBA{}
			st.Beam = color.NRGBA{}
		}

		if phase == phaseDisabled {
			st.Fill = nrgbaAlpha(t.InkMuted, 0.10)
			st.Border = nrgbaAlpha(t.InkMuted, 0.16)
			return st
		}

		switch v {
		case variantPrimary:
			st.Fill, st.FillEnd = nrgba(t.Brand), nrgba(t.BrandPressed)
			st.Border = nrgbaAlpha(t.BrandPressed, 0.95)
			st.TopAccent = white(0.30)
			st.Sheen, st.SheenH = white(0.13), 0.50
			st.InnerShade, st.InnerShadeH = shade(0.16), 0.55
			st.Beam, st.BeamPos, st.BeamW = white(0.07), -0.17, 0.12
			st.Grain = 0.022
			st.Chamfer = 13
			// A sombra e vermelha escura, nao vermelha acesa: halo neon e o que
			// faz um botao parecer barato.
			st.Glow, st.GlowR, st.GlowOffsetY = nrgbaAlpha(brandShadow(t), 0.55), 12, 4
			switch phase {
			case phaseHover:
				st.Fill, st.FillEnd = nrgba(t.BrandHover), nrgba(t.Brand)
				st.TopAccent = white(0.48)
				st.Sheen = white(0.20)
				st.Beam, st.BeamPos, st.BeamW = white(0.17), 0.07, 0.15
				st.Glow, st.GlowR, st.GlowOffsetY = nrgbaAlpha(brandShadow(t), 0.72), 17, 5
			case phasePress:
				st.Fill, st.FillEnd = nrgba(t.BrandPressed), mixNRGBA(nrgba(t.BrandPressed), shade(1), 0.2)
				st.Glow, st.GlowR, st.GlowOffsetY = nrgbaAlpha(brandShadow(t), 0.30), 7, 1
				inset(&st)
				st.Sheen = shade(0.22)
			}

		case variantSecondary:
			st.Fill = t.Glass
			st.Border = t.GlassBorder
			st.TopAccent = white(0.10)
			st.Sheen, st.SheenH = white(0.06), 0.5
			st.InnerShade, st.InnerShadeH = shade(0.08), 0.4
			st.Grain = 0.014
			if !isDarkMode() {
				// No claro a peca precisa de sombra para descolar do fundo.
				st.Glow, st.GlowR, st.GlowOffsetY = nrgbaAlpha(t.Shadow, 0.55), 10, 3
			}
			switch phase {
			case phaseHover:
				st.Fill = mixNRGBA(t.Glass, nrgba(t.Brand), 0.12)
				st.Border = nrgbaAlpha(t.Brand, 0.60)
				st.TopAccent = nrgbaAlpha(t.Brand, 0.60)
				st.Beam, st.BeamPos, st.BeamW = white(0.06), 0.03, 0.16
				st.Glow, st.GlowR, st.GlowOffsetY = nrgbaAlpha(t.Brand, 0.26), 15, 2
			case phasePress:
				st.Fill = t.Glass
				st.Border = nrgbaAlpha(t.Brand, 0.45)
				st.Glow = color.NRGBA{}
				inset(&st)
			}

		case variantGhost, variantIconOnly:
			st.BorderW = 0
			switch phase {
			case phaseHover:
				fill := mixNRGBA(t.Glass, nrgba(t.Brand), 0.10)
				fill.A = uint8(float64(t.Glass.A) * 0.9)
				st.Fill = fill
				st.Border, st.BorderW = nrgbaAlpha(t.Brand, 0.42), 1
				st.TopAccent = nrgbaAlpha(t.Brand, 0.45)
				st.Beam, st.BeamPos, st.BeamW = white(0.05), 0.03, 0.16
				st.Glow, st.GlowR, st.GlowOffsetY = nrgbaAlpha(t.Brand, 0.16), 12, 2
			case phasePress:
				st.Fill = t.Glass
				st.Border, st.BorderW = nrgbaAlpha(t.Brand, 0.35), 1
				inset(&st)
			}

		case variantDanger:
			// Destrutivo nunca compete com a acao principal: em repouso e uma
			// peca tingida, e so ganha corpo quando o ponteiro chega nela.
			// O tom de alerta puro e salmao: misturado no vidro vira marrom. A
			// peca usa a versao escura dele, e quem carrega a cor viva e o
			// contorno com o texto.
			// Tingir o vidro com o tom de alerta deixava a peca marrom. Aqui o
			// corpo continua neutro e quem avisa e o contorno, o texto e o
			// brilho - que e como uma acao destrutiva deve se comportar: discreta
			// parada, inconfundivel quando o ponteiro chega.
			deep := mixNRGBA(nrgba(t.Danger), shade(1), 0.35)
			st.Fill = t.Glass
			st.Border = nrgbaAlpha(t.Danger, 0.55)
			st.TopAccent = nrgbaAlpha(t.Danger, 0.30)
			st.Sheen, st.SheenH = white(0.05), 0.5
			st.InnerShade, st.InnerShadeH = shade(0.10), 0.45
			st.Grain = 0.014
			switch phase {
			case phaseHover:
				st.Fill = mixNRGBA(t.Glass, deep, 0.18)
				st.Border = nrgbaAlpha(t.Danger, 1)
				st.TopAccent = nrgbaAlpha(t.Danger, 0.85)
				st.Beam, st.BeamPos, st.BeamW = white(0.05), 0.04, 0.16
				st.Glow, st.GlowR, st.GlowOffsetY = nrgbaAlpha(deep, 0.60), 14, 4
			case phasePress:
				st.Fill = t.Glass
				st.Border = nrgbaAlpha(t.Danger, 0.80)
				inset(&st)
			}
		}
		return st
	}
}

// buttonInk devolve a cor do texto e o nome de cor do icone.
func buttonInk(v buttonVariant, phase buttonPhase, t designTokens) (color.Color, fyne.ThemeColorName) {
	if phase == phaseDisabled {
		return t.InkMuted, theme.ColorNameDisabled
	}
	switch v {
	case variantPrimary:
		return t.InkOnBrand, theme.ColorNameForegroundOnPrimary
	case variantDanger:
		return t.Danger, theme.ColorNameError
	case variantGhost, variantIconOnly:
		if phase == phaseRest {
			return t.InkSecondary, theme.ColorNameForeground
		}
		return t.Ink, theme.ColorNameForeground
	default:
		return t.Ink, theme.ColorNameForeground
	}
}

// ---------------------------------------------------------------------------
// Renderer.
// ---------------------------------------------------------------------------

func (b *dbsButton) CreateRenderer() fyne.WidgetRenderer {
	r := &dbsButtonRenderer{b: b}
	r.Refresh()
	return r
}

type dbsButtonRenderer struct{ b *dbsButton }

func (r *dbsButtonRenderer) Layout(size fyne.Size) {
	b := r.b
	g := float32(buttonGlowR)
	for _, img := range []*canvas.Image{b.rest, b.hot, b.press} {
		img.Move(fyne.NewPos(-g, -g))
		img.Resize(fyne.NewSize(size.Width+2*g, size.Height+2*g))
	}
	b.ring.Move(fyne.NewPos(-3, -3))
	b.ring.Resize(fyne.NewSize(size.Width+6, size.Height+6))
	b.ensureRasters(size)
	b.layoutContent(size)
}

// layoutContent centraliza icone e texto, e afunda 1 pt quando pressionado.
func (b *dbsButton) layoutContent(size fyne.Size) {
	if size.IsZero() {
		return
	}
	drop := float32(0)
	if b.pressed {
		drop = 1
	}
	iconW := float32(0)
	if b.icon != nil {
		iconW = buttonIconSize
	}
	textW := float32(0)
	if b.text != "" {
		textW = fyne.MeasureText(b.text, sizeBody, fyne.TextStyle{Bold: true}).Width
	}
	gap := float32(0)
	if iconW > 0 && textW > 0 {
		gap = buttonIconGap
	}
	x := (size.Width - (iconW + gap + textW)) / 2
	if b.icon != nil {
		b.glyph.Resize(fyne.NewSize(buttonIconSize, buttonIconSize))
		b.glyph.Move(fyne.NewPos(x, (size.Height-buttonIconSize)/2+drop))
		x += iconW + gap
	}
	if b.text != "" {
		th := b.label.MinSize().Height
		b.label.Move(fyne.NewPos(x, (size.Height-th)/2+drop))
		b.label.Resize(fyne.NewSize(textW, th))
	}
}

func (r *dbsButtonRenderer) MinSize() fyne.Size { return r.b.MinSize() }

func (r *dbsButtonRenderer) Refresh() {
	b := r.b
	b.label.Text = b.text
	b.label.TextSize = sizeBody
	if b.text == "" {
		b.label.Hide()
	} else {
		b.label.Show()
	}
	if b.icon == nil {
		b.glyph.Hide()
	} else {
		b.glyph.Show()
	}
	b.refreshInk()
	if mode := activeThemeMode(); mode != b.lastMode {
		b.invalidate()
	}
	if size := b.Size(); !size.IsZero() {
		b.ensureRasters(size)
		b.layoutContent(size)
	}
	canvas.Refresh(b.rest)
}

func (r *dbsButtonRenderer) Objects() []fyne.CanvasObject {
	b := r.b
	return []fyne.CanvasObject{b.ring, b.rest, b.hot, b.press, b.glyph, b.label}
}

func (r *dbsButtonRenderer) Destroy() {
	if r.b.anim != nil {
		r.b.anim.Stop()
	}
}

// ensureRasters redesenha as camadas quando muda tamanho, tema ou escala. Hover
// e pressionado so sao desenhados na primeira vez que sao usados.
func (b *dbsButton) ensureRasters(size fyne.Size) {
	scale := canvasScale(b)
	mode := activeThemeMode()
	if size.Width <= 0 || size.Height <= 0 {
		return
	}
	if size.Width == b.lastW && size.Height == b.lastH && mode == b.lastMode && scale == b.lastScale {
		return
	}
	b.lastW, b.lastH, b.lastMode, b.lastScale = size.Width, size.Height, mode, scale

	phase := phaseRest
	if b.disabled {
		phase = phaseDisabled
	}
	b.rest.Image = renderGlass(size.Width, size.Height, buttonStyle(b.variant, phase)(tk()), scale)
	b.rest.Refresh()
	if b.hotReady {
		b.ensureLayer(b.hot, false)
	}
	if b.pressReady {
		b.ensureLayer(b.press, true)
	}
	if b.focused {
		b.ensureRing()
	}
}

func (b *dbsButton) ensureLayer(img *canvas.Image, pressed bool) {
	size := b.Size()
	if size.IsZero() || b.disabled {
		return
	}
	phase := phaseHover
	if pressed {
		phase = phasePress
		b.pressReady = true
	} else {
		b.hotReady = true
	}
	img.Image = renderGlass(size.Width, size.Height, buttonStyle(b.variant, phase)(tk()), canvasScale(b))
	img.Refresh()
}

func (b *dbsButton) ensureRing() {
	size := b.Size()
	if size.IsZero() {
		return
	}
	style := glassStyle{
		Border:  nrgbaAlpha(tk().Brand, 0.75),
		BorderW: 2,
		Radius:  buttonRadius + 3,
		Pad:     0,
	}
	b.ring.Image = renderGlass(size.Width+6, size.Height+6, style, canvasScale(b))
	b.ring.Refresh()
}

// ---------------------------------------------------------------------------
// Fabricas usadas pelas telas.
// ---------------------------------------------------------------------------

func primaryButton(label string, icon fyne.Resource, tapped func()) *dbsButton {
	return newDBSButton(variantPrimary, label, icon, tapped)
}

func secondaryButton(label string, icon fyne.Resource, tapped func()) *dbsButton {
	return newDBSButton(variantSecondary, label, icon, tapped)
}

func ghostButton(label string, icon fyne.Resource, tapped func()) *dbsButton {
	return newDBSButton(variantGhost, label, icon, tapped)
}

func dangerButton(label string, icon fyne.Resource, tapped func()) *dbsButton {
	return newDBSButton(variantDanger, label, icon, tapped)
}

func iconButton(icon fyne.Resource, tapped func()) *dbsButton {
	return newDBSButton(variantIconOnly, "", icon, tapped)
}
