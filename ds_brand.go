package main

import (
	_ "embed"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/widget"
)

// Arte da marca em duas versoes (escura = padrao, clara). Os arquivos de
// assets/brand sao gerados por um script a partir de assets/logo.png e
// assets/lightlogo.png (fundo, banner, lockup com bordas suaves).

//go:embed assets/brand/logo-dark-lockup.png
var brandLockupDarkBytes []byte

//go:embed assets/brand/logo-light-lockup.png
var brandLockupLightBytes []byte

//go:embed assets/brand/logo-dark-hero.png
var brandHeroDarkBytes []byte

//go:embed assets/brand/logo-light-hero.png
var brandHeroLightBytes []byte

//go:embed assets/brand/hero-art-dark.png
var brandArtDarkBytes []byte

//go:embed assets/brand/hero-art-light.png
var brandArtLightBytes []byte

//go:embed assets/brand/mark-dark.png
var brandMarkDarkBytes []byte

//go:embed assets/brand/mark-light.png
var brandMarkLightBytes []byte

//go:embed assets/brand/bg-dark.png
var brandBackgroundDarkBytes []byte

//go:embed assets/brand/bg-light.png
var brandBackgroundLightBytes []byte

var (
	brandLockupDark      = fyne.NewStaticResource("dbs-lockup-dark.png", brandLockupDarkBytes)
	brandLockupLight     = fyne.NewStaticResource("dbs-lockup-light.png", brandLockupLightBytes)
	brandHeroDark        = fyne.NewStaticResource("dbs-hero-dark.png", brandHeroDarkBytes)
	brandHeroLight       = fyne.NewStaticResource("dbs-hero-light.png", brandHeroLightBytes)
	brandArtDark         = fyne.NewStaticResource("dbs-art-dark.png", brandArtDarkBytes)
	brandArtLight        = fyne.NewStaticResource("dbs-art-light.png", brandArtLightBytes)
	brandMarkDark        = fyne.NewStaticResource("dbs-mark-dark.png", brandMarkDarkBytes)
	brandMarkLight       = fyne.NewStaticResource("dbs-mark-light.png", brandMarkLightBytes)
	brandBackgroundDark  = fyne.NewStaticResource("dbs-bg-dark.png", brandBackgroundDarkBytes)
	brandBackgroundLight = fyne.NewStaticResource("dbs-bg-light.png", brandBackgroundLightBytes)
)

type brandKind int

const (
	brandWide       brandKind = iota // emblema + "DBS" com bordas suaves (barra lateral)
	brandHero                        // arte completa da logo (tela de acesso)
	brandMark                        // so o emblema (barra do celular)
	brandArt                         // aparelhos + controle + chave (banner do inicio)
	brandBackground                  // fundo da janela
)

func brandResource(kind brandKind) fyne.Resource {
	dark := isDarkMode()
	pick := func(d, l fyne.Resource) fyne.Resource {
		if dark {
			return d
		}
		return l
	}
	switch kind {
	case brandHero:
		return pick(brandHeroDark, brandHeroLight)
	case brandMark:
		return pick(brandMarkDark, brandMarkLight)
	case brandArt:
		return pick(brandArtDark, brandArtLight)
	case brandBackground:
		return pick(brandBackgroundDark, brandBackgroundLight)
	default:
		return pick(brandLockupDark, brandLockupLight)
	}
}

// brandImage e uma imagem que troca de versao (escura/clara) quando o tema muda.
type brandImage struct {
	widget.BaseWidget
	kind brandKind
	img  *canvas.Image
	min  fyne.Size
}

func newBrandImage(kind brandKind, min fyne.Size) *brandImage {
	b := &brandImage{kind: kind, min: min}
	b.img = canvas.NewImageFromResource(brandResource(kind))
	b.img.FillMode = canvas.ImageFillContain
	b.img.ScaleMode = canvas.ImageScaleSmooth
	b.ExtendBaseWidget(b)
	return b
}

// newBackgroundArt e o fundo da janela: a arte esticada para cobrir tudo.
func newBackgroundArt() *brandImage {
	b := newBrandImage(brandBackground, fyne.NewSize(1, 1))
	b.img.FillMode = canvas.ImageFillStretch
	b.img.ScaleMode = canvas.ImageScaleSmooth
	return b
}

func (b *brandImage) MinSize() fyne.Size { return b.min }

func (b *brandImage) CreateRenderer() fyne.WidgetRenderer {
	return &brandImageRenderer{b: b}
}

type brandImageRenderer struct {
	b *brandImage
}

func (r *brandImageRenderer) Layout(size fyne.Size) { r.b.img.Resize(size) }
func (r *brandImageRenderer) MinSize() fyne.Size    { return r.b.min }
func (r *brandImageRenderer) Refresh() {
	res := brandResource(r.b.kind)
	if r.b.img.Resource != res {
		r.b.img.Resource = res
	}
	r.b.img.Refresh()
}
func (r *brandImageRenderer) Objects() []fyne.CanvasObject { return []fyne.CanvasObject{r.b.img} }
func (r *brandImageRenderer) Destroy()                     {}
