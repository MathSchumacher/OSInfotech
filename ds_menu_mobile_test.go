package main

import (
	"context"
	"os"
	"testing"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/test"
)

// TestShotMenuCelular confere que o menu da conta cabe na tela do celular.
//
// No celular o menu e ancorado na area de conteudo inteira, e sem limite de
// tela a folha era desenhada acima do topo: o usuario ficava sem sair da conta,
// sem sincronizar e sem trocar o tema.
func TestShotMenuCelular(t *testing.T) {
	dir := os.Getenv("DBS_SHOTS_DIR")
	if dir == "" {
		t.Skip()
	}
	mobile := true
	forceMobileLayout = &mobile
	defer func() { forceMobileLayout = nil }()

	app := test.NewApp()
	defer app.Quit()
	app.Settings().SetTheme(newDBSTheme(themeDark))
	currentThemeMode.Store(int32(themeDark))
	store := openTestStore(t)
	admin, err := store.BootstrapAdministrator(context.Background(), "Douglas Silva", "douglas", "senha-segura")
	if err != nil {
		t.Fatal(err)
	}
	win := test.NewWindow(nil)
	win.Resize(fyne.NewSize(380, 720))
	ui := newMainUI(app, store, win, "x.db")
	ui.currentUser = admin
	ui.applyRoleAccess()
	win.SetContent(ui.content)
	ui.showPage(pageHome)
	time.Sleep(250 * time.Millisecond)
	ui.showUserMenu(ui.pageHost)
	time.Sleep(200 * time.Millisecond)

	// Confere a posicao da folha dentro da tela.
	for _, overlay := range win.Canvas().Overlays().List() {
		cont, ok := overlay.(*fyne.Container)
		if !ok {
			continue
		}
		for _, obj := range cont.Objects {
			panel, ok := obj.(*glassPanel)
			if !ok {
				continue
			}
			p, s := panel.Position(), panel.Size()
			t.Logf("folha do menu: x=%.0f y=%.0f largura=%.0f altura=%.0f (tela 380x720)", p.X, p.Y, s.Width, s.Height)
			if p.Y < 0 || p.X < 0 {
				t.Fatalf("o menu ficou fora da tela: x=%.0f y=%.0f", p.X, p.Y)
			}
			if p.Y+s.Height > 720 || p.X+s.Width > 380 {
				t.Fatalf("o menu passou da borda: termina em x=%.0f y=%.0f", p.X+s.Width, p.Y+s.Height)
			}
		}
	}
	win.Content().Refresh()
	saveShot(t, dir, "menu-celular", win.Canvas().Capture())
	win.Close()
}
