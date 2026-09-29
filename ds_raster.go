package main

import (
	"image"
	"image/color"
	"math"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/widget"
)

// glassStyle descreve um painel "de vidro": preenchimento translucido, borda
// fina, brilho externo colorido e, opcionalmente, gradiente e filete no topo.
// E rasterizado em Go (SDF de retangulo arredondado) porque o Fyne nao tem
// sombras, brilho nem gradiente com cantos arredondados.
type glassStyle struct {
	Fill      color.NRGBA
	FillEnd   color.NRGBA // A > 0: gradiente horizontal de Fill ate FillEnd
	Border    color.NRGBA
	BorderW   float32
	Radius    float32
	Glow      color.NRGBA // A = intensidade do brilho externo
	GlowR     float32     // alcance do brilho, em pontos
	TopAccent color.NRGBA // filete de 2 pt no topo (A > 0 ativa)

	// Volume interno (usado pelos botoes): a luz bate no topo e a base recebe
	// sombra, que e o que faz a peca parecer solida em vez de um retangulo
	// chapado.
	Diagonal    bool        // gradiente na diagonal em vez de horizontal
	Sheen       color.NRGBA // brilho interno a partir do topo
	SheenH      float32     // altura do brilho, em fracao da altura (0..1)
	InnerShade  color.NRGBA // sombra interna a partir da base
	InnerShadeH float32     // altura da sombra, em fracao da altura (0..1)
	AccentH     float32     // espessura do filete do topo (padrao 2 pt)

	// Assinatura visual da marca: o feixe diagonal repete os facho de luz da
	// arte de fundo, e o grao tira o aspecto de plastico chapado.
	Beam        color.NRGBA // feixe de luz na diagonal (A > 0 ativa)
	BeamPos     float32     // posicao do feixe no eixo diagonal (-0.5 a 0.5)
	BeamW       float32     // largura do feixe (0.10 = estreito, 0.30 = largo)
	Grain       float64     // textura granulada, 0.02 a 0.05
	GlowOffsetY float32     // desloca o brilho para baixo: vira sombra projetada
	Pad         float32     // margem fixa da imagem, quando o brilho varia por estado
	Chamfer     float32     // corte de 45 graus no canto inferior direito, em pontos
}

// glassPad devolve a margem que a imagem precisa ter em volta da peca. Quando
// o brilho muda de tamanho entre estados, Pad mantem todas as camadas com a
// mesma geometria - sem isso uma camada e esticada na caixa da outra e a peca
// sai desalinhada, com o brilho virando um retangulo visivel.
func glassPad(st glassStyle) float32 {
	if st.Pad > 0 {
		return st.Pad
	}
	return st.GlowR
}

// glassPanel e o widget que desenha o painel e posiciona o conteudo dentro.
type glassPanel struct {
	widget.BaseWidget
	style   func(t designTokens) glassStyle
	content fyne.CanvasObject
	pad     float32
	minSize fyne.Size

	img       *canvas.Image
	lastW     float32
	lastH     float32
	lastMode  themeMode
	lastScale float32
}

func newGlassPanel(content fyne.CanvasObject, pad float32, style func(designTokens) glassStyle) *glassPanel {
	p := &glassPanel{style: style, content: content, pad: pad}
	p.img = canvas.NewImageFromImage(image.NewNRGBA(image.Rect(0, 0, 1, 1)))
	p.img.FillMode = canvas.ImageFillStretch
	// Suave, nao vizinho mais proximo: o tamanho em pixels quase nunca bate
	// exatamente com o retangulo na tela, e o vizinho mais proximo comia pedacos
	// da borda de 1 pixel, deixando a linha falhada.
	p.img.ScaleMode = canvas.ImageScaleSmooth
	p.ExtendBaseWidget(p)
	return p
}

func (p *glassPanel) SetMinSize(size fyne.Size) { p.minSize = size }

func (p *glassPanel) MinSize() fyne.Size {
	min := fyne.NewSize(p.pad*2, p.pad*2)
	if p.content != nil {
		min = p.content.MinSize().Add(fyne.NewSize(p.pad*2, p.pad*2))
	}
	return min.Max(p.minSize)
}

func (p *glassPanel) CreateRenderer() fyne.WidgetRenderer {
	r := &glassPanelRenderer{p: p}
	return r
}

type glassPanelRenderer struct {
	p *glassPanel
}

func (r *glassPanelRenderer) Layout(size fyne.Size) {
	p := r.p
	st := p.style(tk())
	g := glassPad(st)
	p.img.Move(fyne.NewPos(-g, -g))
	p.img.Resize(fyne.NewSize(size.Width+2*g, size.Height+2*g))
	r.ensureRaster(size, st)
	if p.content != nil {
		p.content.Move(fyne.NewPos(p.pad, p.pad))
		p.content.Resize(fyne.NewSize(size.Width-2*p.pad, size.Height-2*p.pad))
	}
}

// ensureRaster regenera a imagem quando o tamanho, o tema ou a escala mudam.
func (r *glassPanelRenderer) ensureRaster(size fyne.Size, st glassStyle) {
	p := r.p
	scale := canvasScale(p)
	mode := activeThemeMode()
	if size.Width == p.lastW && size.Height == p.lastH && mode == p.lastMode && scale == p.lastScale && p.img.Image != nil {
		return
	}
	if size.Width <= 0 || size.Height <= 0 {
		return
	}
	p.lastW, p.lastH, p.lastMode, p.lastScale = size.Width, size.Height, mode, scale
	p.img.Image = renderGlass(size.Width, size.Height, st, scale)
	p.img.Refresh()
}

func (r *glassPanelRenderer) MinSize() fyne.Size { return r.p.MinSize() }

func (r *glassPanelRenderer) Refresh() {
	p := r.p
	st := p.style(tk())
	if mode := activeThemeMode(); mode != p.lastMode && p.lastW > 0 {
		p.lastMode = mode
		p.img.Image = renderGlass(p.lastW, p.lastH, st, p.lastScale)
	}
	p.img.Refresh()
	if p.content != nil {
		p.content.Refresh()
	}
}

func (r *glassPanelRenderer) Objects() []fyne.CanvasObject {
	if r.p.content == nil {
		return []fyne.CanvasObject{r.p.img}
	}
	return []fyne.CanvasObject{r.p.img, r.p.content}
}

func (r *glassPanelRenderer) Destroy() {}

// canvasScale devolve a escala do canvas onde o objeto esta (1 se ainda nao esta).
func canvasScale(obj fyne.CanvasObject) float32 {
	app := fyne.CurrentApp()
	if app == nil || app.Driver() == nil {
		return 1
	}
	c := app.Driver().CanvasForObject(obj)
	if c == nil || c.Scale() <= 0 {
		return 1
	}
	s := c.Scale()
	if s > 3 {
		s = 3
	}
	return s
}

// renderGlass rasteriza o painel (w x h pontos, mais a margem do brilho).
func renderGlass(w, h float32, st glassStyle, scale float32) *image.NRGBA {
	if scale <= 0 {
		scale = 1
	}
	g := glassPad(st)
	pw := int(math.Ceil(float64((w + 2*g) * scale)))
	ph := int(math.Ceil(float64((h + 2*g) * scale)))
	if pw <= 0 || ph <= 0 {
		return image.NewNRGBA(image.Rect(0, 0, 1, 1))
	}
	img := image.NewNRGBA(image.Rect(0, 0, pw, ph))

	halfW := float64(w) / 2
	halfH := float64(h) / 2
	radius := float64(st.Radius)
	if radius > halfW {
		radius = halfW
	}
	if radius > halfH {
		radius = halfH
	}
	bw := float64(st.BorderW)
	glowR := float64(st.GlowR)
	glowA := float64(st.Glow.A) / 255
	hasGradient := st.FillEnd.A > 0
	accentH := 2.0
	if st.AccentH > 0 {
		accentH = float64(st.AccentH)
	}
	chamfer := float64(st.Chamfer)
	const invSqrt2 = 0.7071067811865476
	sheenH := float64(st.SheenH) * float64(h)
	shadeH := float64(st.InnerShadeH) * float64(h)

	fill := st.Fill
	fillEnd := st.FillEnd
	inv := 1 / float64(scale)
	aa := inv // um pixel do dispositivo, em pontos logicos

	for py := 0; py < ph; py++ {
		// coordenada logica relativa ao centro do painel
		y := (float64(py)+0.5)*inv - float64(g) - halfH
		qy := math.Abs(y) - (halfH - radius)
		for px := 0; px < pw; px++ {
			x := (float64(px)+0.5)*inv - float64(g) - halfW
			qx := math.Abs(x) - (halfW - radius)
			// distancia assinada ate a borda do retangulo arredondado
			mx, my := math.Max(qx, 0), math.Max(qy, 0)
			d := math.Sqrt(mx*mx+my*my) + math.Min(math.Max(qx, qy), 0) - radius
			// chanfro: corta o canto inferior direito num angulo de 45 graus
			if chamfer > 0 {
				dCut := (x + y - (halfW + halfH - chamfer)) * invSqrt2
				if dCut > d {
					d = dCut
				}
			}

			var out color.NRGBA
			// distancia para o brilho: deslocada para baixo vira sombra projetada
			dGlow := d
			if st.GlowOffsetY != 0 {
				gy := y - float64(st.GlowOffsetY)
				qgy := math.Abs(gy) - (halfH - radius)
				mgx, mgy := math.Max(qx, 0), math.Max(qgy, 0)
				dGlow = math.Sqrt(mgx*mgx+mgy*mgy) + math.Min(math.Max(qx, qgy), 0) - radius
				if chamfer > 0 {
					if cut := (x + gy - (halfW + halfH - chamfer)) * invSqrt2; cut > dGlow {
						dGlow = cut
					}
				}
			}
			// Brilho/sombra por fora da peca.
			if glowA > 0 && dGlow > 0 && dGlow < glowR {
				t := 1 - dGlow/glowR
				a := glowA * t * t * t
				out = color.NRGBA{R: st.Glow.R, G: st.Glow.G, B: st.Glow.B, A: uint8(a*255 + 0.5)}
			}

			// Cobertura da peca medida em pixels do dispositivo: a transicao tem
			// sempre 1 pixel, em qualquer escala de tela. Media em pontos
			// logicos, como era antes, a borda ficava grossa e mole em telas
			// 150% e dura demais em 100%.
			shape := clamp01(0.5 - d/aa)
			if shape <= 0 {
				img.SetNRGBA(px, py, out)
				continue
			}

			// Corpo: preenchimento (com gradiente opcional) e os efeitos internos.
			body := fill
			if hasGradient {
				t := clamp01((x + halfW) / float64(w))
				if st.Diagonal {
					t = clamp01(0.62*(x+halfW)/float64(w) + 0.38*(y+halfH)/float64(h))
				}
				body = mixNRGBA(fill, fillEnd, t)
			}
			if st.Sheen.A > 0 && sheenH > 0 {
				if top := y + halfH; top < sheenH {
					f := 1 - top/sheenH
					body = over(body, withAlpha(st.Sheen, f*f))
				}
			}
			if st.InnerShade.A > 0 && shadeH > 0 {
				if bottom := halfH - y; bottom < shadeH {
					f := 1 - bottom/shadeH
					body = over(body, withAlpha(st.InnerShade, f*f))
				}
			}
			if st.Beam.A > 0 && st.BeamW > 0 {
				u := x/float64(w) + 0.55*y/float64(h)
				k := (u - float64(st.BeamPos)) / float64(st.BeamW)
				body = over(body, withAlpha(st.Beam, math.Exp(-k*k)))
			}
			if st.Grain > 0 {
				if n := grainAt(px, py) * st.Grain; n > 0 {
					body = over(body, color.NRGBA{R: 255, G: 255, B: 255, A: uint8(n*255 + 0.5)})
				} else if n < 0 {
					body = over(body, color.NRGBA{A: uint8(-n*255 + 0.5)})
				}
			}
			if st.TopAccent.A > 0 {
				if top := y + halfH; top < accentH {
					body = over(body, withAlpha(st.TopAccent, clamp01((accentH-top)/aa)))
				}
			}
			// Borda: faixa entre -bw e 0, suavizada nas duas arestas. Antes a
			// aresta de dentro era um degrau seco, e era ela que serrilhava os
			// cantos arredondados.
			if bw > 0 {
				if band := 1 - clamp01(0.5-(d+bw)/aa); band > 0 {
					body = over(body, withAlpha(st.Border, band))
				}
			}
			out = over(out, withAlpha(body, shape))
			img.SetNRGBA(px, py, out)
		}
	}
	return img
}

func clamp01(v float64) float64 {
	if v < 0 {
		return 0
	}
	if v > 1 {
		return 1
	}
	return v
}

func withAlpha(c color.NRGBA, factor float64) color.NRGBA {
	c.A = uint8(clamp01(float64(c.A)/255*factor)*255 + 0.5)
	return c
}

func mixNRGBA(a, b color.NRGBA, t float64) color.NRGBA {
	mix := func(x, y uint8) uint8 { return uint8(float64(x)*(1-t) + float64(y)*t + 0.5) }
	return color.NRGBA{R: mix(a.R, b.R), G: mix(a.G, b.G), B: mix(a.B, b.B), A: mix(a.A, b.A)}
}

// over compoe src sobre dst (ambos NRGBA nao pre-multiplicados).
func over(dst, src color.NRGBA) color.NRGBA {
	sa := float64(src.A) / 255
	if sa <= 0 {
		return dst
	}
	da := float64(dst.A) / 255
	oa := sa + da*(1-sa)
	if oa <= 0 {
		return color.NRGBA{}
	}
	blend := func(s, d uint8) uint8 {
		v := (float64(s)*sa + float64(d)*da*(1-sa)) / oa
		return uint8(v + 0.5)
	}
	return color.NRGBA{R: blend(src.R, dst.R), G: blend(src.G, dst.G), B: blend(src.B, dst.B), A: uint8(oa*255 + 0.5)}
}

// nrgba converte qualquer cor para NRGBA.
func nrgba(c color.Color) color.NRGBA {
	return color.NRGBAModel.Convert(c).(color.NRGBA)
}

func nrgbaAlpha(c color.Color, alpha float64) color.NRGBA {
	n := nrgba(c)
	n.A = uint8(clamp01(alpha)*255 + 0.5)
	return n
}

// ---------------------------------------------------------------------------
// Estilos prontos.
// ---------------------------------------------------------------------------

// cardGlass e o cartao padrao das paginas.
func cardGlass(t designTokens) glassStyle {
	return glassStyle{
		Fill:    t.Glass,
		Border:  t.GlassBorder,
		BorderW: 1,
		Radius:  radiusLg,
		Glow:    t.GlassGlow,
		GlowR:   18,
	}
}

// sunkenGlass e a area rebaixada dentro de um cartao.
func sunkenGlass(t designTokens) glassStyle {
	return glassStyle{Fill: t.GlassSunken, Border: t.GlassBorder, BorderW: 1, Radius: radiusMd}
}

// toneGlass e o cartao com borda e brilho na cor do tom (tiles, destaques).
func toneGlass(tone statusTone, glowStrength float64) func(designTokens) glassStyle {
	return func(t designTokens) glassStyle {
		fg, _ := toneColors(tone, t)
		return glassStyle{
			Fill:    t.Glass,
			Border:  nrgbaAlpha(fg, 0.55),
			BorderW: 1,
			Radius:  radiusLg,
			Glow:    nrgbaAlpha(fg, glowStrength),
			GlowR:   22,
		}
	}
}

// tintGlass e a faixa tingida (cliente selecionado, avisos).
func tintGlass(tone statusTone) func(designTokens) glassStyle {
	return func(t designTokens) glassStyle {
		fg, soft := toneColors(tone, t)
		return glassStyle{Fill: nrgba(soft), Border: nrgbaAlpha(fg, 0.45), BorderW: 1, Radius: radiusMd}
	}
}

// brandPillGlass e a pilula vermelha com gradiente e brilho (item ativo, destaques).
func brandPillGlass(t designTokens) glassStyle {
	return glassStyle{
		Fill:    nrgba(t.Brand),
		FillEnd: nrgba(t.BrandPressed),
		Radius:  radiusMd,
		Glow:    nrgbaAlpha(t.Brand, 0.55),
		GlowR:   16,
	}
}

// heroGlass e o painel do banner de boas-vindas: mais transparente a direita
// para a arte e o fundo aparecerem.
func heroGlass(t designTokens) glassStyle {
	fill := t.Glass
	end := t.Glass
	end.A = uint8(float64(fill.A) * 0.35)
	return glassStyle{
		Fill:    fill,
		FillEnd: end,
		Border:  t.GlassBorder,
		BorderW: 1,
		Radius:  radiusLg,
		Glow:    nrgbaAlpha(t.Brand, 0.16),
		GlowR:   26,
	}
}

// grainAt devolve um ruido estavel entre -0,5 e 0,5 para o pixel. E calculado
// em pixels do dispositivo, entao o grao fica do mesmo tamanho em qualquer
// escala de tela.
func grainAt(px, py int) float64 {
	n := uint32(px)*374761393 + uint32(py)*668265263
	n = (n ^ (n >> 13)) * 1274126177
	n ^= n >> 16
	return float64(n&0xFFFF)/65535.0 - 0.5
}
