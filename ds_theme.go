package main

import (
	_ "embed"
	"image/color"
	"strings"
	"sync/atomic"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/theme"
)

// Identidade do produto. O identificador tecnico (app ID, pacote Android,
// pasta de dados) continua "osinfotech" de proposito: trocar isso separaria o
// app atualizado dos dados e da instalacao que ja estao nos aparelhos.
const (
	appDisplayName = "DBS Assistência Técnica"
	appShortName   = "DBS"
	appTagline     = "Assistência técnica"
)

// ---------------------------------------------------------------------------
// Tipografia: IBM Plex Sans (OFL). Uma unica familia; o "negrito" do Fyne mapeia
// para o SemiBold, que fica mais refinado em interfaces densas do que o Bold.
// ---------------------------------------------------------------------------

//go:embed assets/fonts/IBMPlexSans-Regular.ttf
var fontRegularBytes []byte

//go:embed assets/fonts/IBMPlexSans-Medium.ttf
var fontMediumBytes []byte

//go:embed assets/fonts/IBMPlexSans-SemiBold.ttf
var fontSemiBoldBytes []byte

//go:embed assets/fonts/IBMPlexSans-Italic.ttf
var fontItalicBytes []byte

var (
	fontRegular  = fyne.NewStaticResource("IBMPlexSans-Regular.ttf", fontRegularBytes)
	fontMedium   = fyne.NewStaticResource("IBMPlexSans-Medium.ttf", fontMediumBytes)
	fontSemiBold = fyne.NewStaticResource("IBMPlexSans-SemiBold.ttf", fontSemiBoldBytes)
	fontItalic   = fyne.NewStaticResource("IBMPlexSans-Italic.ttf", fontItalicBytes)
)

// ---------------------------------------------------------------------------
// Modo de aparencia (escuro e o padrao, seguindo a logo principal).
// ---------------------------------------------------------------------------

type themeMode int32

const (
	themeDark themeMode = iota
	themeLight
)

const appearanceModePreference = "appearance_mode"

var currentThemeMode atomic.Int32

func activeThemeMode() themeMode { return themeMode(currentThemeMode.Load()) }

func isDarkMode() bool { return activeThemeMode() == themeDark }

func parseThemeMode(value string) themeMode {
	if strings.EqualFold(strings.TrimSpace(value), "light") {
		return themeLight
	}
	return themeDark
}

func (m themeMode) String() string {
	if m == themeLight {
		return "light"
	}
	return "dark"
}

// loadThemeMode le a preferencia salva (padrao: escuro).
func loadThemeMode(app fyne.App) themeMode {
	if app == nil {
		return themeDark
	}
	return parseThemeMode(app.Preferences().StringWithFallback(appearanceModePreference, "dark"))
}

// applyThemeMode grava a preferencia e troca o tema do app inteiro. Todos os
// componentes do design system leem as cores em Refresh(), entao a troca e
// imediata e sem reconstruir telas.
func applyThemeMode(app fyne.App, mode themeMode) {
	if app == nil {
		currentThemeMode.Store(int32(mode))
		return
	}
	app.Preferences().SetString(appearanceModePreference, mode.String())
	app.Settings().SetTheme(newDBSTheme(mode))
}

// ---------------------------------------------------------------------------
// Tokens do design system. Sao a unica fonte de cor fora do tema do Fyne.
// ---------------------------------------------------------------------------

type designTokens struct {
	Canvas        color.Color // fundo da janela
	Surface       color.Color // cartoes, barra lateral, paineis
	SurfaceRaised color.Color // campos, botoes secundarios, linhas em hover
	SurfaceSunken color.Color // cabecalhos de tabela, areas rebaixadas

	Border       color.Color
	BorderSubtle color.Color
	BorderStrong color.Color

	Ink          color.Color // texto principal
	InkSecondary color.Color
	InkMuted     color.Color
	InkOnBrand   color.Color

	Brand        color.Color // vermelho DBS
	BrandHover   color.Color
	BrandPressed color.Color
	BrandSoft    color.Color // tinta a ~14% para fundos

	Success, SuccessSoft color.Color
	Warning, WarningSoft color.Color
	Danger, DangerSoft   color.Color
	Info, InfoSoft       color.Color
	Neutral, NeutralSoft color.Color

	Hover   color.Color // sobreposicao de hover em superficies
	Pressed color.Color
	Overlay color.Color // escurecimento atras de dialogos
	Shadow  color.Color

	// Superficies de vidro sobre a arte de fundo.
	Glass       color.NRGBA // cartoes e paineis translucidos
	GlassSunken color.NRGBA // areas rebaixadas dentro dos cartoes
	GlassBorder color.NRGBA // contorno fino dos paineis
	GlassGlow   color.NRGBA // brilho suave em volta dos cartoes
	GlassStrong color.NRGBA // paineis mais opacos (barra lateral, rodape)
}

func rgb(hex uint32) color.Color {
	return color.NRGBA{R: uint8(hex >> 16), G: uint8(hex >> 8), B: uint8(hex), A: 255}
}

func rgba(hex uint32, alpha float64) color.Color {
	return color.NRGBA{R: uint8(hex >> 16), G: uint8(hex >> 8), B: uint8(hex), A: uint8(alpha*255 + 0.5)}
}

var darkTokens = designTokens{
	Canvas:        rgb(0x121316),
	Surface:       rgb(0x1A1C20),
	SurfaceRaised: rgb(0x23262B),
	SurfaceSunken: rgb(0x0E0F11),

	Border:       rgb(0x2B2F35),
	BorderSubtle: rgb(0x23272C),
	BorderStrong: rgb(0x3D424A),

	Ink:          rgb(0xEEF0F3),
	InkSecondary: rgb(0xA3ABB5),
	InkMuted:     rgb(0x6F7881),
	InkOnBrand:   rgb(0xFFFFFF),

	Brand:        rgb(0xE5141C),
	BrandHover:   rgb(0xF42A32),
	BrandPressed: rgb(0xB80F16),
	BrandSoft:    rgba(0xE5141C, 0.16),

	Success: rgb(0x3DCB80), SuccessSoft: rgba(0x3DCB80, 0.14),
	Warning: rgb(0xF5B31B), WarningSoft: rgba(0xF5B31B, 0.14),
	Danger: rgb(0xFF6B57), DangerSoft: rgba(0xFF6B57, 0.14),
	Info: rgb(0x5AA9FF), InfoSoft: rgba(0x5AA9FF, 0.14),
	Neutral: rgb(0x9AA3AD), NeutralSoft: rgba(0x9AA3AD, 0.14),

	Hover:   rgba(0xFFFFFF, 0.06),
	Pressed: rgba(0xFFFFFF, 0.11),
	Overlay: rgba(0x000000, 0.58),
	Shadow:  rgba(0x000000, 0.5),

	Glass:       color.NRGBA{R: 16, G: 17, B: 21, A: 214},
	GlassSunken: color.NRGBA{R: 0, G: 0, B: 0, A: 96},
	GlassBorder: color.NRGBA{R: 255, G: 255, B: 255, A: 22},
	GlassGlow:   color.NRGBA{R: 0, G: 0, B: 0, A: 120},
	GlassStrong: color.NRGBA{R: 10, G: 10, B: 13, A: 236},
}

var lightTokens = designTokens{
	Canvas:        rgb(0xF3F4F6),
	Surface:       rgb(0xFFFFFF),
	SurfaceRaised: rgb(0xF6F7F9),
	SurfaceSunken: rgb(0xEBEDF0),

	Border:       rgb(0xE2E5E9),
	BorderSubtle: rgb(0xEDEFF2),
	BorderStrong: rgb(0xC9CED5),

	Ink:          rgb(0x14171B),
	InkSecondary: rgb(0x5B6570),
	InkMuted:     rgb(0x8B949E),
	InkOnBrand:   rgb(0xFFFFFF),

	Brand:        rgb(0xD31219),
	BrandHover:   rgb(0xE4191F),
	BrandPressed: rgb(0xA80C12),
	BrandSoft:    rgba(0xD31219, 0.10),

	Success: rgb(0x1D9A57), SuccessSoft: rgba(0x1D9A57, 0.12),
	Warning: rgb(0xB96F00), WarningSoft: rgba(0xF5B31B, 0.20),
	Danger: rgb(0xD93A2B), DangerSoft: rgba(0xD93A2B, 0.12),
	Info: rgb(0x1F6FEB), InfoSoft: rgba(0x1F6FEB, 0.12),
	Neutral: rgb(0x6B7480), NeutralSoft: rgba(0x6B7480, 0.12),

	Hover:   rgba(0x0F1218, 0.05),
	Pressed: rgba(0x0F1218, 0.10),
	Overlay: rgba(0x0F1218, 0.45),
	Shadow:  rgba(0x0F172A, 0.18),

	Glass:       color.NRGBA{R: 255, G: 255, B: 255, A: 222},
	GlassSunken: color.NRGBA{R: 15, G: 18, B: 24, A: 14},
	GlassBorder: color.NRGBA{R: 15, G: 18, B: 24, A: 26},
	GlassGlow:   color.NRGBA{R: 15, G: 23, B: 42, A: 40},
	GlassStrong: color.NRGBA{R: 255, G: 255, B: 255, A: 240},
}

// tk devolve os tokens do modo ativo.
func tk() designTokens {
	if isDarkMode() {
		return darkTokens
	}
	return lightTokens
}

// ---------------------------------------------------------------------------
// Tema Fyne construido em cima dos tokens.
// ---------------------------------------------------------------------------

type dbsTheme struct {
	mode themeMode
}

func newDBSTheme(mode themeMode) fyne.Theme {
	currentThemeMode.Store(int32(mode))
	return &dbsTheme{mode: mode}
}

func (t *dbsTheme) tokens() designTokens {
	if t.mode == themeLight {
		return lightTokens
	}
	return darkTokens
}

func (t *dbsTheme) Color(name fyne.ThemeColorName, _ fyne.ThemeVariant) color.Color {
	k := t.tokens()
	dark := t.mode == themeDark
	switch name {
	case theme.ColorNameBackground:
		return k.Canvas
	case theme.ColorNameForeground:
		return k.Ink
	case theme.ColorNameButton:
		if dark {
			return rgba(0xFFFFFF, 0.09)
		}
		return rgba(0x0F1218, 0.06)
	case theme.ColorNameDisabledButton:
		if dark {
			return rgba(0xFFFFFF, 0.04)
		}
		return rgba(0x0F1218, 0.03)
	case theme.ColorNameDisabled:
		return k.InkMuted
	case theme.ColorNamePlaceHolder:
		return k.InkMuted
	case theme.ColorNamePrimary:
		return k.Brand
	case theme.ColorNameForegroundOnPrimary:
		return k.InkOnBrand
	case theme.ColorNameInputBackground:
		if dark {
			return rgba(0xFFFFFF, 0.055)
		}
		return rgba(0x0F1218, 0.035)
	case theme.ColorNameInputBorder:
		if dark {
			return rgba(0xFFFFFF, 0.13)
		}
		return rgba(0x0F1218, 0.14)
	case theme.ColorNameMenuBackground, theme.ColorNameOverlayBackground:
		if dark {
			return k.SurfaceRaised
		}
		return k.Surface
	case theme.ColorNameHeaderBackground:
		return k.Surface
	case theme.ColorNameHover:
		return k.Hover
	case theme.ColorNamePressed:
		return k.Pressed
	case theme.ColorNameFocus:
		return rgbaOf(k.Brand, 0.38)
	case theme.ColorNameSelection:
		return rgbaOf(k.Brand, 0.30)
	case theme.ColorNameSeparator:
		return k.Border
	case theme.ColorNameShadow:
		return k.Shadow
	case theme.ColorNameScrollBar:
		return rgbaOf(k.Ink, 0.28)
	case theme.ColorNameScrollBarBackground:
		return color.Transparent
	case theme.ColorNameError:
		return k.Danger
	case theme.ColorNameSuccess:
		return k.Success
	case theme.ColorNameWarning:
		return k.Warning
	case theme.ColorNameHyperlink:
		return k.Brand
	case theme.ColorNameForegroundOnError, theme.ColorNameForegroundOnSuccess, theme.ColorNameForegroundOnWarning:
		if dark {
			return k.Canvas
		}
		return k.InkOnBrand
	}
	return theme.DefaultTheme().Color(name, t.variant())
}

func (t *dbsTheme) variant() fyne.ThemeVariant {
	if t.mode == themeLight {
		return theme.VariantLight
	}
	return theme.VariantDark
}

func (t *dbsTheme) Font(style fyne.TextStyle) fyne.Resource {
	if style.Monospace || style.Symbol {
		return theme.DefaultTheme().Font(style)
	}
	switch {
	case style.Bold:
		return fontSemiBold
	case style.Italic:
		return fontItalic
	default:
		return fontRegular
	}
}

func (t *dbsTheme) Icon(name fyne.ThemeIconName) fyne.Resource {
	return theme.DefaultTheme().Icon(name)
}

func (t *dbsTheme) Size(name fyne.ThemeSizeName) float32 {
	switch name {
	case theme.SizeNamePadding:
		return 6
	case theme.SizeNameInnerPadding:
		return 8
	case theme.SizeNameInlineIcon:
		return 18
	case theme.SizeNameText:
		return 13
	case theme.SizeNameHeadingText:
		return 20
	case theme.SizeNameSubHeadingText:
		return 15
	case theme.SizeNameCaptionText:
		return 11
	case theme.SizeNameInputRadius:
		return 8
	case theme.SizeNameSelectionRadius:
		return 8
	case theme.SizeNameScrollBar:
		return 10
	case theme.SizeNameScrollBarSmall:
		return 4
	case theme.SizeNameScrollBarRadius:
		return 4
	case theme.SizeNameSeparatorThickness:
		return 1
	case theme.SizeNameInputBorder:
		return 1
	case theme.SizeNameLineSpacing:
		return 4
	}
	return theme.DefaultTheme().Size(name)
}

// rgbaOf aplica um alpha a uma cor opaca.
func rgbaOf(c color.Color, alpha float64) color.Color {
	n := color.NRGBAModel.Convert(c).(color.NRGBA)
	n.A = uint8(alpha*255 + 0.5)
	return n
}

// mixColor mistura duas cores (t em [0,1]).
func mixColor(a, b color.Color, t float64) color.Color {
	ca := color.NRGBAModel.Convert(a).(color.NRGBA)
	cb := color.NRGBAModel.Convert(b).(color.NRGBA)
	mix := func(x, y uint8) uint8 { return uint8(float64(x)*(1-t) + float64(y)*t + 0.5) }
	return color.NRGBA{R: mix(ca.R, cb.R), G: mix(ca.G, cb.G), B: mix(ca.B, cb.B), A: mix(ca.A, cb.A)}
}
