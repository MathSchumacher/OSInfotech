package main

import (
	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
)

// loginScreen e a tela de acesso: a arte da DBS de um lado e o formulario da
// equipe do outro, sobre o fundo da marca. No celular a arte fica em cima.
func (ui *mainUI) loginScreen() fyne.CanvasObject {
	form, err := ui.loginForm()
	if err != nil {
		form = container.NewVBox(mutedLabel("Não foi possível preparar o acesso: " + err.Error()))
	}

	eyebrow := newEyebrowBrand("DBS Assistência Técnica")
	title := newText("Acesso da equipe", sizeTitle, true, inkPrimary)
	subtitle := mutedLabel("Entre com o usuário e a senha cadastrados pelo administrador.")
	version := newText("v"+appVersion, sizeCaption, false, inkMuted)
	formBlock := container.NewVBox(eyebrow, gap(1, 4), title, subtitle, gap(1, 12), form, gap(1, 8), version)

	if isMobileLayout() {
		hero := newBrandImage(brandHero, fyne.NewSize(240, 234))
		body := container.NewVBox(gap(1, 8), container.NewCenter(hero), cardPadded(formBlock, 16))
		return container.NewStack(newBackgroundArt(), container.NewVScroll(paddedTRBL(body, 12, mobilePadding, 24, mobilePadding)))
	}

	hero := newBrandImage(brandHero, fyne.NewSize(400, 390))
	left := newGlassPanel(container.NewCenter(hero), 0, func(t designTokens) glassStyle {
		st := sunkenGlass(t)
		st.Radius = radiusLg
		return st
	})
	right := container.NewStack(gap(380, 1), vcenter(paddedTRBL(formBlock, 24, 28, 24, 28)))
	panel := cardPadded(container.NewHBox(left, gap(8, 1), right), 12)
	return container.NewStack(newBackgroundArt(), container.NewCenter(panel))
}

// loginField = rotulo + campo, no mesmo estilo dos formularios do app.
func loginField(label string, entry fyne.CanvasObject) fyne.CanvasObject {
	return field(label, entry, "")
}

// loginSubmitRow deixa o botao principal com largura total.
func loginSubmitRow(button *dbsButton) fyne.CanvasObject {
	return container.NewGridWithColumns(1, button)
}
