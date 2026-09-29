package main

import (
	"context"
	"os"
	"testing"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/test"
	"fyne.io/fyne/v2/theme"
)

// Capturas do sistema de botoes e do menu de acoes. Servem para revisar o
// desenho sem abrir o app: DBS_SHOTS_DIR=<pasta> go test -run TestShot.
//
// forceState deixa o botao numa aparencia fixa para a captura.
func forceState(b *dbsButton, phase buttonPhase) {
	switch phase {
	case phaseHover:
		b.hovered = true
		b.ensureLayer(b.hot, false)
		b.hotFade = 1
		b.hot.Translucency = 0
	case phasePress:
		b.pressed = true
		b.ensureLayer(b.press, true)
		b.press.Translucency = 0
	case phaseDisabled:
		b.Disable()
	}
	b.refreshInk()
	b.Refresh()
}

func buttonRow(phase buttonPhase, label string) fyne.CanvasObject {
	save := primaryButton("Salvar cliente", theme.DocumentSaveIcon(), nil)
	novo := secondaryButton("Nova OS para o cliente", theme.ContentAddIcon(), nil)
	excluir := dangerButton("Excluir", theme.DeleteIcon(), nil)
	zap := ghostButton("WhatsApp", theme.MailSendIcon(), nil)
	only := iconButton(theme.SearchIcon(), nil)
	row := container.NewHBox(zap, novo, excluir, save, only)
	for _, b := range []*dbsButton{save, novo, excluir, zap, only} {
		b.Resize(b.MinSize())
		forceState(b, phase)
	}
	tag := newText(label, sizeCaption, true, inkMuted)
	return container.NewVBox(tag, row, gap(1, 10))
}

func TestShotButtons(t *testing.T) {
	dir := os.Getenv("DBS_SHOTS_DIR")
	if dir == "" {
		t.Skip()
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	desktopMode := false
	forceMobileLayout = &desktopMode
	defer func() { forceMobileLayout = nil }()

	for _, mode := range []themeMode{themeDark, themeLight} {
		app := test.NewApp()
		app.Settings().SetTheme(newDBSTheme(mode))
		currentThemeMode.Store(int32(mode))
		win := test.NewWindow(nil)
		win.Resize(fyne.NewSize(1000, 460))
		body := container.NewVBox(
			buttonRow(phaseRest, "REPOUSO"),
			buttonRow(phaseHover, "HOVER"),
			buttonRow(phasePress, "PRESSIONADO"),
			buttonRow(phaseDisabled, "DESABILITADO"),
		)
		bg := canvas.NewRectangle(tk().Canvas)
		win.SetContent(container.NewStack(bg, paddedTRBL(body, 24, 24, 24, 24)))
		time.Sleep(250 * time.Millisecond)
		win.Content().Refresh()
		name := "botoes-escuro"
		if mode == themeLight {
			name = "botoes-claro"
		}
		saveShot(t, dir, name, win.Canvas().Capture())
		win.Close()
		app.Quit()
	}
}

// findMenuRows procura as linhas do menu dentro da sobreposicao.
func findMenuRows(obj fyne.CanvasObject) []*menuRow {
	var rows []*menuRow
	switch v := obj.(type) {
	case *menuRow:
		rows = append(rows, v)
	case *fyne.Container:
		for _, child := range v.Objects {
			rows = append(rows, findMenuRows(child)...)
		}
	case *glassPanel:
		rows = append(rows, findMenuRows(v.content)...)
	}
	return rows
}

func TestShotUserMenu(t *testing.T) {
	dir := os.Getenv("DBS_SHOTS_DIR")
	if dir == "" {
		t.Skip()
	}
	desktopMode := false
	forceMobileLayout = &desktopMode
	defer func() { forceMobileLayout = nil }()

	for _, mode := range []themeMode{themeDark, themeLight} {
		app := test.NewApp()
		app.Settings().SetTheme(newDBSTheme(mode))
		currentThemeMode.Store(int32(mode))
		store := openTestStore(t)
		admin, err := store.BootstrapAdministrator(context.Background(), "Douglas Silva", "douglas", "senha-segura")
		if err != nil {
			t.Fatal(err)
		}
		win := test.NewWindow(nil)
		win.Resize(fyne.NewSize(1100, 640))
		ui := newMainUI(app, store, win, "x.db")
		ui.currentUser = admin
		ui.applyRoleAccess()
		win.SetContent(ui.content)
		ui.showPage(pageCustomers)
		time.Sleep(200 * time.Millisecond)
		ui.showUserMenu(ui.userChip)
		time.Sleep(200 * time.Millisecond)
		// deixa a segunda linha em hover para a captura mostrar o estado
		for _, overlay := range win.Canvas().Overlays().List() {
			rows := findMenuRows(overlay)
			if len(rows) > 1 {
				rows[1].MouseIn(nil)
			}
		}
		win.Content().Refresh()
		time.Sleep(150 * time.Millisecond)
		name := "menu-escuro"
		if mode == themeLight {
			name = "menu-claro"
		}
		saveShot(t, dir, name, win.Canvas().Capture())
		win.Close()
		app.Quit()
	}
}
