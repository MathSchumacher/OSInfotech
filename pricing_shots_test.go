package main

import (
	"context"
	"os"
	"testing"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/test"
)

// TestShotCatalogoEPreco registra o catalogo de modelos e a memoria de preco
// funcionando juntos: digitar "a14" oferece o Galaxy A14, e o defeito "tela
// quebrada" traz o que a loja cobrou das ultimas vezes nesse aparelho.
func TestShotCatalogoEPreco(t *testing.T) {
	dir := os.Getenv("DBS_SHOTS_DIR")
	if dir == "" {
		t.Skip()
	}
	desktop := false
	forceMobileLayout = &desktop
	defer func() { forceMobileLayout = nil }()
	app := test.NewApp()
	defer app.Quit()
	app.Settings().SetTheme(newDBSTheme(themeDark))
	currentThemeMode.Store(int32(themeDark))

	store := openTestStore(t)
	ctx := context.Background()
	admin, err := store.BootstrapAdministrator(ctx, "Douglas Silva", "douglas", "senha-segura")
	if err != nil {
		t.Fatal(err)
	}
	for _, item := range []struct {
		modelo string
		valor  int64
		custo  int64
	}{{"Samsung Galaxy A14", 25000, 12000}, {"a14", 35000, 18000}} {
		o := ServiceOrder{
			Status: statusDelivered, Customer: "Cliente Antigo", Phone: "(51) 98765-4321",
			Device: "Celular", BrandModel: item.modelo, Issue: "troca de tela",
			AmountCents: item.valor, CostCents: item.custo,
		}
		if err := store.Save(ctx, &o); err != nil {
			t.Fatal(err)
		}
	}

	win := test.NewWindow(nil)
	win.Resize(fyne.NewSize(1366, 1120))
	ui := newMainUI(app, store, win, "x.db")
	ui.currentUser = admin
	ui.applyRoleAccess()
	win.SetContent(ui.content)
	ui.showPage(pageOrderEditor)
	ui.clearForm()

	ui.nameEntry.SetText("Maria Souza")
	ui.phoneEntry.SetText("(51) 99888-7766")
	ui.deviceEntry.SetText("Celular")
	ui.modelEntry.SetText("a14")
	ui.issueEntry.SetText("tela quebrada")
	time.Sleep(700 * time.Millisecond)
	ui.content.Refresh()
	time.Sleep(200 * time.Millisecond)
	saveShot(t, dir, "catalogo-e-preco", win.Canvas().Capture())

	t.Logf("modelo=%q servico=%q", canonicalModel(ui.modelEntry.Text), serviceFor(ui.issueEntry.Text))
	win.Close()
}
