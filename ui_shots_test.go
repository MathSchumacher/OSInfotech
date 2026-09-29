package main

import (
	"context"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"testing"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/test"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
)

// Harness de screenshots headless. Roda apenas quando DBS_SHOTS_DIR aponta para
// uma pasta: ali ficam os PNGs de cada tela, em ambos os temas e layouts, para
// revisao visual do design. Em `go test` normal e ignorado.
func shotsDir(t *testing.T) string {
	t.Helper()
	dir := os.Getenv("DBS_SHOTS_DIR")
	if dir == "" {
		t.Skip("DBS_SHOTS_DIR nao definido")
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	return dir
}

func saveShot(t *testing.T, dir string, name string, img image.Image) {
	t.Helper()
	f, err := os.Create(filepath.Join(dir, name+".png"))
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	if err := png.Encode(f, img); err != nil {
		t.Fatal(err)
	}
}

// renderShot monta a janela de teste com o tema pedido e captura a imagem.
func renderShot(t *testing.T, app fyne.App, mode themeMode, size fyne.Size, build func() fyne.CanvasObject) image.Image {
	t.Helper()
	app.Settings().SetTheme(newDBSTheme(mode))
	content := build()
	w := test.NewWindow(content)
	defer w.Close()
	w.Resize(size)
	content.Refresh()
	return w.Canvas().Capture()
}

// seedShotData cria clientes, OS e termos variados para as telas terem vida.
func seedShotData(t *testing.T, store *Store) (ServiceOrder, CustomerProfile, ServiceTerm) {
	t.Helper()
	ctx := context.Background()
	admin, err := store.BootstrapAdministrator(ctx, "Douglas Silva", "douglas", "senha-segura")
	if err != nil {
		t.Fatal(err)
	}
	orders := []ServiceOrder{
		{Status: statusPending, Customer: "José da Silva", CPF: "123.456.789-00", Phone: "(11) 98765-4321", Device: "Celular", BrandModel: "iPhone 11", Issue: "Tela quebrada, não responde ao toque", AmountCents: 45000, CostCents: 21000},
		{Status: statusApproved, Customer: "Maria Souza", CPF: "555.444.333-22", Phone: "(21) 3333-2222", Device: "Notebook", BrandModel: "Dell Inspiron 15", Issue: "Não liga", AmountCents: 38000, CostCents: 12000},
		{Status: statusDelivered, Customer: "Antônio Gonçalves", Phone: "(31) 99888-7766", Device: "Console", BrandModel: "PlayStation 5", Issue: "Superaquecimento e ruído", AmountCents: 25000, CostCents: 6000},
		{Status: statusUnauthorized, Customer: "Carla Menezes", Phone: "(41) 98877-6655", Device: "TV", BrandModel: "Samsung 55\"", Issue: "Sem imagem", AmountCents: 0},
		{Status: statusNoRepair, Customer: "Rafael Lima", Phone: "(51) 97766-5544", Device: "Celular", BrandModel: "Moto G9", Issue: "Molhou", AmountCents: 0},
		{Status: statusDelivered, Customer: "José da Silva", CPF: "123.456.789-00", Phone: "(11) 98765-4321", Device: "Notebook", BrandModel: "Acer Aspire", Issue: "Troca de SSD", AmountCents: 52000, CostCents: 30000},
		{Status: statusPending, Customer: "Beatriz Rocha", Phone: "(61) 98111-2233", Device: "Celular", BrandModel: "Galaxy S21", Issue: "Bateria não carrega", AmountCents: 22000},
	}
	var first ServiceOrder
	for i, o := range orders {
		o.TechnicianID = admin.ID
		o.TechnicianName = admin.Name
		if err := store.Save(ctx, &o); err != nil {
			t.Fatal(err)
		}
		if i == 0 {
			first = o
		}
	}
	term := ServiceTerm{SellerName: "Antônio Gonçalves", SellerCPF: "111.222.333-44", SellerPhone: "(31) 99888-7766",
		BuyerCompany: "DBS Assistência Técnica", BuyerDocument: "12.345.678/0001-90",
		DeviceKind: "Celular", Brand: "Samsung", Model: "Galaxy S21", IMEI1: "356938035643809", AmountCents: 90000, PaymentMethod: "Pix", City: "Belo Horizonte/MG"}
	if err := store.SaveTerm(ctx, &term); err != nil {
		t.Fatal(err)
	}
	term2 := ServiceTerm{SellerName: "Maria Souza", DeviceKind: "Notebook", Brand: "Dell", Model: "Inspiron", AmountCents: 120000, Status: termStatusSigned}
	if err := store.SaveTerm(ctx, &term2); err != nil {
		t.Fatal(err)
	}
	profiles, err := store.FindCustomerProfiles(ctx, "jose", 3)
	if err != nil || len(profiles) == 0 {
		t.Fatalf("perfil do cliente nao encontrado: %v", err)
	}
	return first, profiles[0], term
}

type shotUI struct {
	ui       *mainUI
	order    ServiceOrder
	customer CustomerProfile
	term     ServiceTerm
}

func newShotUI(t *testing.T, app fyne.App, mode themeMode, mobile bool) shotUI {
	t.Helper()
	forceMobileLayout = &mobile
	app.Settings().SetTheme(newDBSTheme(mode))
	store := openTestStore(t)
	order, customer, term := seedShotData(t, store)
	win := test.NewWindow(nil)
	ui := newMainUI(app, store, win, filepath.Join(t.TempDir(), "dbs.db"))
	user, err := store.Authenticate(context.Background(), "douglas", "senha-segura")
	if err != nil {
		t.Fatal(err)
	}
	ui.currentUser = user
	ui.updateSessionLabel()
	ui.applyRoleAccess()
	return shotUI{ui: ui, order: order, customer: customer, term: term}
}

func TestShotScreens(t *testing.T) {
	dir := shotsDir(t)
	app := test.NewApp()
	defer app.Quit()
	defer func() { forceMobileLayout = nil }()

	layouts := []struct {
		name   string
		mobile bool
		size   fyne.Size
	}{
		{"desktop", false, fyne.NewSize(1366, 820)},
		{"mobile", true, fyne.NewSize(400, 820)},
	}
	for _, layout := range layouts {
		for _, mode := range []themeMode{themeDark, themeLight} {
			prefix := fmt.Sprintf("%s-%s", layout.name, mode)
			// tela de login
			mobile := layout.mobile
			forceMobileLayout = &mobile
			app.Settings().SetTheme(newDBSTheme(mode))
			s := newShotUI(t, app, mode, layout.mobile)
			login := s.ui.loginScreen()
			w := test.NewWindow(login)
			w.Resize(layout.size)
			saveShot(t, dir, prefix+"-00-login", w.Canvas().Capture())
			w.Close()

			w = test.NewWindow(s.ui.content)
			w.Resize(layout.size)
			shoot := func(name string, kind pageKind, prepare func()) {
				s.ui.showPage(kind)
				if prepare != nil {
					prepare()
				}
				s.ui.content.Refresh()
				saveShot(t, dir, prefix+"-"+name, w.Canvas().Capture())
				// O driver de teste executa fyne.Do na propria goroutine; esperar
				// as cargas em segundo plano evita corrida com a proxima captura.
				time.Sleep(150 * time.Millisecond)
			}
			shoot("01-home", pageHome, nil)
			if !layout.mobile {
				w.Resize(fyne.NewSize(1366, 1240))
				s.ui.showPage(pageHome)
				s.ui.content.Refresh()
				saveShot(t, dir, prefix+"-01b-home-tall", w.Canvas().Capture())
				w.Resize(layout.size)
			}
			shoot("02-orders", pageOrders, nil)
			shoot("03-order-edit", pageOrderEditor, func() { s.ui.loadOrder(s.order) })
			shoot("04-order-new", pageOrderEditor, func() { s.ui.clearForm() })
			shoot("05-customers", pageCustomers, nil)
			shoot("06-customer", pageCustomerEditor, func() { s.ui.loadCustomerProfile(s.customer) })
			shoot("07-terms", pageTerms, nil)
			shoot("08-term-edit", pageTermEditor, func() { s.ui.loadTerm(s.term) })
			shoot("09-reports", pageReports, nil)
			shoot("10-settings", pageSettings, nil)
			shoot("11-settings-team", pageSettings, func() { s.ui.openSettingsSection(settingsTeam) })
			shoot("11b-settings-access", pageSettings, func() { s.ui.openSettingsSection(settingsAccess) })
			if !layout.mobile {
				shoot("12-settings-cloud", pageSettings, func() { s.ui.openSettingsSection(settingsCloud) })
			}
			w.Close()
		}
	}
	// Da tempo para goroutines de carga (historico, equipe, nuvem) terminarem
	// antes de encerrar o app de teste.
	time.Sleep(300 * time.Millisecond)
}

func TestShotComponentSheet(t *testing.T) {
	dir := shotsDir(t)
	app := test.NewApp()
	defer app.Quit()

	build := func() fyne.CanvasObject {
		tiles := container.NewGridWithColumns(4,
			newStatTile("Aguardando aprovação", theme.WarningIcon(), toneWarning),
			newStatTile("Em bancada", theme.ComputerIcon(), toneInfo),
			newStatTile("Entregues no mês", theme.ConfirmIcon(), toneSuccess),
			newStatTile("Faturamento", theme.DocumentIcon(), toneBrand),
		)
		pills := container.NewHBox(
			newStatusPill(statusPending, orderStatusTone(statusPending)),
			newStatusPill(statusApproved, orderStatusTone(statusApproved)),
			newStatusPill(statusDelivered, orderStatusTone(statusDelivered)),
			newStatusPill(statusUnauthorized, orderStatusTone(statusUnauthorized)),
			newStatusPill(statusNoRepair, orderStatusTone(statusNoRepair)),
		)
		buttons := container.NewHBox(
			primaryButton("Salvar OS", theme.DocumentSaveIcon(), nil),
			secondaryButton("Imprimir 80 mm", theme.DocumentPrintIcon(), nil),
			ghostButton("Comprovante", theme.DocumentIcon(), nil),
			dangerButton("Excluir", theme.DeleteIcon(), nil),
			iconButton(theme.MoreVerticalIcon(), nil),
		)
		entry := widget.NewEntry()
		entry.SetPlaceHolder("Nome do cliente")
		sel := widget.NewSelect(orderStatuses, nil)
		sel.SetSelected(statusPending)
		form := card(container.NewVBox(
			sectionHeader("Cliente"),
			fieldGrid(2,
				field("Nome", entry, ""),
				field("Status", sel, "Muda o andamento da OS"),
			),
		))
		titleBlock, _, _ := pageTitleBlock("Ordens de serviço", "128 ordens · 12 aguardando aprovação", primaryButton("Nova OS", theme.ContentAddIcon(), nil))
		empty := emptyState(theme.SearchIcon(), "Nenhuma OS encontrada", "Ajuste a busca ou limpe os filtros para ver todas as ordens.", ghostButton("Limpar filtros", theme.ContentClearIcon(), nil))
		content := container.NewVBox(titleBlock, tiles, gap(1, 8), pills, buttons, form, cardPadded(empty, 24))
		bg := newFill(0, func(t designTokens) color.Color { return t.Canvas })
		return container.NewStack(bg, padded(content, 24))
	}
	saveShot(t, dir, "components-dark", renderShot(t, app, themeDark, fyne.NewSize(1180, 760), build))
	saveShot(t, dir, "components-light", renderShot(t, app, themeLight, fyne.NewSize(1180, 760), build))
}

// TestShotWhatsAppNotice retrata as duas telas novas do WhatsApp da loja: o
// aviso de retirada que sai da OS e o pareamento nos ajustes.
func TestShotWhatsAppNotice(t *testing.T) {
	dir := shotsDir(t)
	app := test.NewApp()
	defer app.Quit()
	defer func() { forceMobileLayout = nil }()

	shot := newShotUI(t, app, themeDark, false)
	ui := shot.ui
	win := ui.window
	win.Resize(fyne.NewSize(1200, 800))
	win.SetContent(ui.content)
	ui.showPage(pageOrders)
	time.Sleep(200 * time.Millisecond)

	order := shot.order
	order.Status = statusReady
	ui.offerReadyNotice(order)
	time.Sleep(300 * time.Millisecond)
	win.Content().Refresh()
	saveShot(t, dir, "whatsapp-aviso-pronto", win.Canvas().Capture())
	if top := win.Canvas().Overlays().Top(); top != nil {
		win.Canvas().Overlays().Remove(top)
	}
	win.Close()
}

// TestShotFechamento retrata a caixa de fechamento da OS e o relatório com o
// resultado parcial.
func TestShotFechamento(t *testing.T) {
	dir := shotsDir(t)
	app := test.NewApp()
	defer app.Quit()
	defer func() { forceMobileLayout = nil }()

	shot := newShotUI(t, app, themeDark, false)
	ui := shot.ui
	win := ui.window
	// Largura de notebook comum: e onde a linha de ladrilhos aperta.
	win.Resize(fyne.NewSize(1440, 860))
	win.SetContent(ui.content)
	ui.showPage(pageOrders)
	time.Sleep(200 * time.Millisecond)

	order := shot.order
	order.Status = statusDelivered
	order.AmountCents = 35000
	ui.offerDeliveryClose(order)
	time.Sleep(300 * time.Millisecond)
	win.Content().Refresh()
	saveShot(t, dir, "fechamento-da-os", win.Canvas().Capture())
	if top := win.Canvas().Overlays().Top(); top != nil {
		win.Canvas().Overlays().Remove(top)
	}

	ui.showPage(pageReports)
	time.Sleep(300 * time.Millisecond)
	win.Content().Refresh()
	saveShot(t, dir, "relatorio-parcial", win.Canvas().Capture())
	win.Close()
}

// TestShotPadrao retrata a caixa do padrão de desbloqueio.
func TestShotPadrao(t *testing.T) {
	dir := shotsDir(t)
	app := test.NewApp()
	defer app.Quit()
	defer func() { forceMobileLayout = nil }()

	shot := newShotUI(t, app, themeDark, false)
	ui := shot.ui
	win := ui.window
	win.Resize(fyne.NewSize(1200, 800))
	win.SetContent(ui.content)
	ui.showPage(pageOrders)
	time.Sleep(200 * time.Millisecond)

	ui.showPatternDialog("padrao:1-2-3-6-9", func(string) {})
	time.Sleep(300 * time.Millisecond)
	win.Content().Refresh()
	saveShot(t, dir, "padrao-de-desbloqueio", win.Canvas().Capture())
	if top := win.Canvas().Overlays().Top(); top != nil {
		win.Canvas().Overlays().Remove(top)
	}
	win.Close()
}
