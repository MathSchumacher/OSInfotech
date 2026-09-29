package main

import (
	"context"
	"log"
	"net/url"
	"path/filepath"
	"strings"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/app"
)

var appVersion = "dev"

func main() {
	// Modo ponte: o mesmo executavel mantem a ligacao com o WhatsApp num
	// processo separado, para uma falha do protocolo nao derrubar o balcao.
	if runWhatsAppBridge() {
		return
	}

	dbPath, err := defaultDatabasePath()
	if err != nil {
		log.Fatal(err)
	}

	store, err := OpenStore(dbPath)
	if err != nil {
		log.Fatal(err)
	}
	defer store.Close()
	_ = store.SeedDefaultChecklistTemplates(context.Background())

	// O ID tecnico segue "osinfotech": e a chave das preferencias, da pasta de
	// dados e do pacote Android instalado nos aparelhos.
	a := app.NewWithID("br.com.osinfotech.desktop")
	if packagedVersion := strings.TrimSpace(a.Metadata().Version); packagedVersion != "" && packagedVersion != "0.0.1" {
		appVersion = packagedVersion
	}
	a.Settings().SetTheme(newDBSTheme(loadThemeMode(a)))
	a.SetIcon(appIcon)

	win := a.NewWindow(appDisplayName)
	if !fyne.CurrentDevice().IsMobile() {
		win.Resize(fyne.NewSize(1280, 800))
		win.SetMaster()
	}
	win.SetIcon(appIcon)

	ui := newMainUI(a, store, win, dbPath)
	win.SetContent(ui.startupContent())
	// A ponte do WhatsApp e um processo filho: fechar o app fecha ela junto, em
	// vez de deixar um processo solto segurando a sessao.
	win.SetOnClosed(func() { ui.whatsapp.Stop() })
	win.ShowAndRun()
}

func firstLine(s string) string {
	s = strings.TrimSpace(s)
	if i := strings.IndexAny(s, "\r\n"); i >= 0 {
		return strings.TrimSpace(s[:i])
	}
	return s
}

func fileURL(path string) (*url.URL, error) {
	abs, err := filepath.Abs(path)
	if err != nil {
		return nil, err
	}
	return url.Parse("file:///" + strings.ReplaceAll(filepath.ToSlash(abs), " ", "%20"))
}
