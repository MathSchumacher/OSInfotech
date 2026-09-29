package main

import (
	"context"
	"strings"
	"testing"
	"time"

	"fyne.io/fyne/v2/test"
)

// newSessionUI monta uma interface minima com um administrador cadastrado.
func newSessionUI(t *testing.T) (*mainUI, AppUser) {
	t.Helper()
	app := test.NewApp()
	t.Cleanup(app.Quit)
	store := openTestStore(t)
	user, err := store.BootstrapAdministrator(context.Background(), "Dona", "dona", "senha-local-1")
	if err != nil {
		t.Fatal(err)
	}
	if err := store.SetSetting(context.Background(), apiBaseURLSetting, "disabled"); err != nil {
		t.Fatal(err)
	}
	return &mainUI{store: store, app: app}, user
}

func TestAccessDefaultsAskEveryTime(t *testing.T) {
	ui, _ := newSessionUI(t)
	if ui.rememberLoginEnabled() || ui.keepSessionEnabled() {
		t.Fatal("por padrao nada deve ser lembrado")
	}
	if got := ui.savedLogin(); got != "" {
		t.Fatalf("login salvo = %q", got)
	}
	if user, notice := ui.restoreSession(); user.ID != 0 || notice != "" {
		t.Fatalf("nao deveria restaurar sessao: %+v %q", user, notice)
	}
}

func TestRememberLoginWithoutKeepingSession(t *testing.T) {
	ui, user := newSessionUI(t)
	ui.rememberAccess("dona", true, false, user)

	if !ui.rememberLoginEnabled() {
		t.Fatal("lembrar login deveria estar ligado")
	}
	if got := ui.savedLogin(); got != "dona" {
		t.Fatalf("login salvo = %q", got)
	}
	if ui.keepSessionEnabled() {
		t.Fatal("manter conectado nao foi pedido")
	}
	if restored, _ := ui.restoreSession(); restored.ID != 0 {
		t.Fatal("sem manter conectado, a sessao nao pode ser restaurada")
	}
	if summary := ui.accessSummary(); !strings.Contains(summary, "dona") {
		t.Fatalf("resumo = %q", summary)
	}
}

func TestKeepSessionRestoresUser(t *testing.T) {
	ui, user := newSessionUI(t)
	ui.rememberAccess("dona", false, true, user)

	// Manter conectado implica lembrar o login.
	if !ui.rememberLoginEnabled() || ui.savedLogin() != "dona" {
		t.Fatalf("login deveria ficar lembrado: %v %q", ui.rememberLoginEnabled(), ui.savedLogin())
	}
	restored, notice := ui.restoreSession()
	if restored.ID != user.ID || notice != "" {
		t.Fatalf("sessao nao restaurada: %+v %q", restored, notice)
	}
}

func TestKeepSessionExpires(t *testing.T) {
	ui, user := newSessionUI(t)
	ui.rememberAccess("dona", true, true, user)
	// Volta a validade para ontem.
	ui.prefs().SetString(prefSessionUntil, time.Now().Add(-24*time.Hour).UTC().Format(time.RFC3339))

	restored, notice := ui.restoreSession()
	if restored.ID != 0 {
		t.Fatal("sessao vencida nao pode entrar")
	}
	if notice == "" {
		t.Fatal("deveria avisar que a sessao expirou")
	}
	if ui.keepSessionEnabled() {
		t.Fatal("a sessao vencida deveria ter sido apagada")
	}
	if ui.savedLogin() != "dona" {
		t.Fatal("o login lembrado deve continuar preenchido")
	}
}

func TestDeactivatedUserCannotResume(t *testing.T) {
	ui, admin := newSessionUI(t)
	ctx := context.Background()
	tech := AppUser{Name: "Tecnico", Login: "tecnico", Role: roleTechnician, Active: true}
	if err := ui.store.SaveUserAsAdministrator(ctx, admin, &tech, "senha-tecnico-1"); err != nil {
		t.Fatal(err)
	}
	ui.rememberAccess("tecnico", true, true, tech)

	tech.Active = false
	if err := ui.store.SaveUserAsAdministrator(ctx, admin, &tech, ""); err != nil {
		t.Fatal(err)
	}
	restored, notice := ui.restoreSession()
	if restored.ID != 0 || notice == "" {
		t.Fatalf("usuario desativado nao pode entrar: %+v %q", restored, notice)
	}
}

func TestLockSessionKeepsRememberedLogin(t *testing.T) {
	ui, user := newSessionUI(t)
	ui.rememberAccess("dona", true, true, user)
	ui.currentUser = user

	ui.lockSession()

	if ui.keepSessionEnabled() {
		t.Fatal("trocar usuario deve cancelar a sessao mantida")
	}
	if ui.savedLogin() != "dona" {
		t.Fatal("trocar usuario deve manter o login lembrado")
	}
	if restored, _ := ui.restoreSession(); restored.ID != 0 {
		t.Fatal("nao deve mais restaurar a sessao")
	}
}

func TestForgetDeviceClearsEverything(t *testing.T) {
	ui, user := newSessionUI(t)
	ui.rememberAccess("dona", true, true, user)

	ui.forgetDevice()

	if ui.rememberLoginEnabled() || ui.keepSessionEnabled() || ui.savedLogin() != "" {
		t.Fatal("esquecer o aparelho deve apagar login e sessao")
	}
	if summary := ui.accessSummary(); !strings.Contains(summary, "toda vez") {
		t.Fatalf("resumo = %q", summary)
	}
}

func TestExpiredCloudTokenBlocksResume(t *testing.T) {
	ui, user := newSessionUI(t)
	ctx := context.Background()
	if err := ui.store.SetSetting(ctx, apiBaseURLSetting, "https://exemplo.invalido"); err != nil {
		t.Fatal(err)
	}
	if err := ui.store.SetSetting(ctx, apiTokenExpiry, time.Now().Add(-time.Hour).UTC().Format(time.RFC3339)); err != nil {
		t.Fatal(err)
	}
	ui.rememberAccess("dona", true, true, user)

	restored, notice := ui.restoreSession()
	if restored.ID != 0 {
		t.Fatal("token da nuvem vencido nao pode entrar direto")
	}
	if !strings.Contains(notice, "nuvem") {
		t.Fatalf("aviso = %q", notice)
	}
}

func TestStartupShowsLoginWithoutSession(t *testing.T) {
	app := test.NewApp()
	defer app.Quit()
	desktop := false
	forceMobileLayout = &desktop
	defer func() { forceMobileLayout = nil }()
	store := openTestStore(t)
	if _, err := store.BootstrapAdministrator(context.Background(), "Dona", "dona", "senha-local-1"); err != nil {
		t.Fatal(err)
	}
	if err := store.SetSetting(context.Background(), apiBaseURLSetting, "disabled"); err != nil {
		t.Fatal(err)
	}
	ui := newMainUI(app, store, test.NewWindow(nil), "x.db")

	content := ui.startupContent()
	if content == ui.content {
		t.Fatal("sem sessao mantida o app deve abrir na tela de acesso")
	}
	if ui.currentUser.ID != 0 {
		t.Fatal("nenhum usuario deveria estar logado")
	}
}
