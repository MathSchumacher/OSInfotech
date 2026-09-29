package main

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/test"
)

// dumpWide lista os objetos cuja largura minima passa do limite (diagnostico).
func dumpWide(t *testing.T, obj fyne.CanvasObject, depth int, limit float32, out *[]string) {
	if obj == nil {
		return
	}
	min := obj.MinSize()
	if min.Width > limit {
		extra := ""
		switch v := obj.(type) {
		case *canvas.Text:
			extra = " " + fmt.Sprintf("%q", v.Text)
		case *themedText:
			extra = " " + fmt.Sprintf("%q", v.Text())
		}
		*out = append(*out, fmt.Sprintf("%s%T min=%.0fx%.0f%s", strings.Repeat("  ", depth), obj, min.Width, min.Height, extra))
	}
	if c, ok := obj.(*fyne.Container); ok {
		for _, child := range c.Objects {
			dumpWide(t, child, depth+1, limit, out)
		}
		return
	}
	if w, ok := obj.(fyne.Widget); ok {
		r := test.TempWidgetRenderer(t, w)
		for _, child := range r.Objects() {
			dumpWide(t, child, depth+1, limit, out)
		}
	}
}

// TestMobilePagesFitViewport garante que nenhuma pagina do celular exige mais
// largura do que uma tela de 400px: conteudo mais largo que isso e cortado,
// porque a rolagem e so vertical.
func TestMobilePagesFitViewport(t *testing.T) {
	app := test.NewApp()
	defer app.Quit()
	mobile := true
	forceMobileLayout = &mobile
	defer func() { forceMobileLayout = nil }()
	store := openTestStore(t)
	order, customer, term := seedShotData(t, store)
	ui := newMainUI(app, store, test.NewWindow(nil), "x.db")
	user, err := store.Authenticate(ctxBackground(), "douglas", "senha-segura")
	if err != nil {
		t.Fatal(err)
	}
	ui.currentUser = user
	ui.applyRoleAccess()

	const viewport = 400
	pages := []pageKind{pageHome, pageOrders, pageOrderEditor, pageCustomers, pageCustomerEditor, pageTerms, pageTermEditor, pageReports, pageSettings, pageSearch}
	for _, kind := range pages {
		ui.showPage(kind)
		switch kind {
		case pageOrderEditor:
			ui.loadOrder(order)
		case pageCustomerEditor:
			ui.loadCustomerProfile(customer)
			time.Sleep(400 * time.Millisecond) // historico carrega em segundo plano
		case pageTermEditor:
			ui.loadTerm(term)
		}
		page := ui.pages[kind]
		if w := page.MinSize().Width; w > viewport {
			var wide []string
			dumpWide(t, page, 0, viewport-60, &wide)
			t.Errorf("pagina %d exige %.0fpx de largura (limite %d):\n%s", kind, w, viewport, strings.Join(wide, "\n"))
		}
	}
	if w := ui.loginScreen().MinSize().Width; w > viewport {
		t.Errorf("tela de login exige %.0fpx de largura", w)
	}
}
