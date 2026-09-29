package main

import (
	"context"
	"testing"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/test"
)

// TestGuardaDeCadastro: o app precisa saber quando há texto digitado a perder.
func TestGuardaDeCadastro(t *testing.T) {
	app := test.NewApp()
	defer app.Quit()
	desktop := false
	forceMobileLayout = &desktop
	defer func() { forceMobileLayout = nil }()
	app.Settings().SetTheme(newDBSTheme(themeDark))

	store := openTestStore(t)
	ctx := context.Background()
	admin, err := store.BootstrapAdministrator(ctx, "Douglas", "douglas", "senha-segura")
	if err != nil {
		t.Fatal(err)
	}
	win := test.NewWindow(nil)
	win.Resize(fyne.NewSize(1200, 760))
	ui := newMainUI(app, store, win, "x.db")
	ui.currentUser = admin
	ui.applyRoleAccess()
	win.SetContent(ui.content)
	ui.showPage(pageOrderEditor)
	ui.clearForm()

	if ui.orderFormDirty() {
		t.Fatal("formulário recém-limpo não pode contar como alterado")
	}

	ui.nameEntry.SetText("Cliente do Balcão")
	ui.deviceEntry.SetText("Celular")
	if !ui.orderFormDirty() {
		t.Fatal("texto digitado deveria contar como alteração pendente")
	}

	// Sair para outra tela não pode acontecer direto: o app pergunta antes.
	ui.showPage(pageCustomers)
	if ui.current != pageOrderEditor {
		t.Fatalf("a navegação deveria ter parado no editor, foi para %v", ui.current)
	}

	// Depois de salvar, sair é livre.
	ui.phoneEntry.SetText("(51) 98765-4321")
	ui.issueEntry.SetText("troca de tela")
	if !ui.persistOrder() {
		t.Fatal("a OS deveria ter sido salva")
	}
	if ui.orderFormDirty() {
		t.Fatal("depois de salvar não há nada pendente")
	}
	ui.showPage(pageCustomers)
	if ui.current != pageCustomers {
		t.Fatalf("depois de salvar a navegação deveria seguir, ficou em %v", ui.current)
	}
	win.Close()
}
