package main

import (
	"context"
	"os"
	"testing"

	"fyne.io/fyne/v2/test"
)

// TestPerformLoginLocalWhenCloudDisabled: com a nuvem desligada, o login e local.
func TestPerformLoginLocalWhenCloudDisabled(t *testing.T) {
	app := test.NewApp()
	defer app.Quit()
	store := openTestStore(t)
	ctx := context.Background()
	if _, err := store.BootstrapAdministrator(ctx, "Dona", "dona", "senha-local-1"); err != nil {
		t.Fatal(err)
	}
	if err := store.SetSetting(ctx, apiBaseURLSetting, "disabled"); err != nil {
		t.Fatal(err)
	}
	ui := &mainUI{store: store, app: app}
	if _, err := ui.performLogin(ctx, "dona", "errada"); err == nil {
		t.Fatal("senha errada deveria falhar")
	}
	outcome, err := ui.performLogin(ctx, "dona", "senha-local-1")
	if err != nil {
		t.Fatal(err)
	}
	if outcome.Cloud || outcome.User.Login != "dona" {
		t.Fatalf("resultado inesperado: %+v", outcome)
	}
	if hint := ui.cloudModeHint(); hint != "Nuvem desligada neste aparelho: acesso com usuário local." {
		t.Fatalf("dica de modo = %q", hint)
	}
}

// TestPerformLoginFallsBackWithoutNetwork: nuvem configurada mas inacessivel
// cai para o usuario local (com aviso) e, sem usuario local, explica o motivo.
func TestPerformLoginFallsBackWithoutNetwork(t *testing.T) {
	app := test.NewApp()
	defer app.Quit()
	store := openTestStore(t)
	ctx := context.Background()
	if err := store.SetSetting(ctx, apiBaseURLSetting, "http://127.0.0.1:9"); err != nil {
		t.Fatal(err)
	}
	ui := &mainUI{store: store, app: app}
	if _, err := ui.performLogin(ctx, "alguem", "x"); err == nil {
		t.Fatal("sem nuvem e sem usuario local deveria falhar")
	}
	if _, err := store.BootstrapAdministrator(ctx, "Dona", "dona", "senha-local-1"); err != nil {
		t.Fatal(err)
	}
	outcome, err := ui.performLogin(ctx, "dona", "senha-local-1")
	if err != nil {
		t.Fatal(err)
	}
	if outcome.Cloud || outcome.Warning == "" {
		t.Fatalf("esperava modo local com aviso, veio %+v", outcome)
	}
}

// TestPerformLoginAgainstCloud roda so com DBS_CLOUD_LOGIN/DBS_CLOUD_PASSWORD
// definidos: entra na nuvem real (banco temporario) e confere o perfil.
func TestPerformLoginAgainstCloud(t *testing.T) {
	login, password := os.Getenv("DBS_CLOUD_LOGIN"), os.Getenv("DBS_CLOUD_PASSWORD")
	if login == "" || password == "" {
		t.Skip("DBS_CLOUD_LOGIN/DBS_CLOUD_PASSWORD nao definidos")
	}
	app := test.NewApp()
	defer app.Quit()
	store := openTestStore(t)
	ui := &mainUI{store: store, app: app}
	outcome, err := ui.performLogin(context.Background(), login, password)
	if err != nil {
		t.Fatalf("login na nuvem falhou: %v", err)
	}
	if !outcome.Cloud {
		t.Fatalf("esperava login pela nuvem, veio %+v", outcome)
	}
	t.Logf("nuvem OK: %s (%s) · %s", outcome.User.Name, outcome.User.Role, ui.cloudModeHint())
}
