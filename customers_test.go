package main

import (
	"context"
	"errors"
	"testing"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/test"
)

func seedCustomerWithOrder(t *testing.T, store *Store, name, cpf, phone string) ServiceOrder {
	t.Helper()
	ctx := context.Background()
	order := ServiceOrder{
		Status: statusPending, Customer: name, CPF: cpf, Phone: phone,
		Device: "Celular", BrandModel: "Galaxy S21", Issue: "Não liga",
	}
	if err := store.Save(ctx, &order); err != nil {
		t.Fatal(err)
	}
	return order
}

// TestRenameKeepsSingleCustomer: editar o nome renomeia o cadastro; nao pode
// sobrar um segundo cliente nem OS orfas.
func TestRenameKeepsSingleCustomer(t *testing.T) {
	store := openTestStore(t)
	ctx := context.Background()
	order := seedCustomerWithOrder(t, store, "Matheus", "037.528.350-10", "(54) 99912-3344")

	// O vinculo com a nuvem precisa acompanhar a renomeacao.
	mapping, err := store.syncMapping(ctx, syncEntityCustomer, "matheus")
	if err != nil {
		t.Fatal(err)
	}

	profile := CustomerProfile{Name: "Matheus Schumacher", CPF: "037.528.350-10", Phone: "(54) 99912-3344"}
	if err := store.RenameCustomerProfile(ctx, "matheus", profile); err != nil {
		t.Fatal(err)
	}

	clients, err := store.ListCustomerProfiles(ctx, "")
	if err != nil {
		t.Fatal(err)
	}
	if len(clients) != 1 {
		names := make([]string, 0, len(clients))
		for _, c := range clients {
			names = append(names, c.Name)
		}
		t.Fatalf("deveria haver 1 cliente, ha %d: %v", len(clients), names)
	}
	if clients[0].Name != "Matheus Schumacher" {
		t.Fatalf("nome = %q", clients[0].Name)
	}
	if clients[0].Orders != 1 {
		t.Fatalf("a OS deveria seguir o cliente, veio %d", clients[0].Orders)
	}

	updated, err := store.Get(ctx, order.ID)
	if err != nil {
		t.Fatal(err)
	}
	if updated.Customer != "Matheus Schumacher" {
		t.Fatalf("a OS ficou com %q", updated.Customer)
	}

	moved, found, err := store.existingSyncMapping(ctx, syncEntityCustomer, "matheus schumacher")
	if err != nil {
		t.Fatal(err)
	}
	if !found || moved.RemoteID != mapping.RemoteID {
		t.Fatalf("o vinculo com a nuvem nao acompanhou: %+v (queria %s)", moved, mapping.RemoteID)
	}
	if _, stillThere, _ := store.existingSyncMapping(ctx, syncEntityCustomer, "matheus"); stillThere {
		t.Fatal("o vinculo antigo continuou no banco")
	}
}

// TestRenameRefusesExistingName: renomear para um nome ja usado precisa avisar,
// nunca sobrescrever o outro cadastro.
func TestRenameRefusesExistingName(t *testing.T) {
	store := openTestStore(t)
	ctx := context.Background()
	seedCustomerWithOrder(t, store, "Ana Paula", "111.222.333-44", "(11) 98888-7777")
	seedCustomerWithOrder(t, store, "Bruno Costa", "555.666.777-88", "(11) 97777-6666")

	err := store.RenameCustomerProfile(ctx, "bruno costa", CustomerProfile{Name: "Ana Paula"})
	if !errors.Is(err, ErrCustomerNameTaken) {
		t.Fatalf("erro = %v, queria ErrCustomerNameTaken", err)
	}
	clients, _ := store.ListCustomerProfiles(ctx, "")
	if len(clients) != 2 {
		t.Fatalf("os dois cadastros deveriam continuar, ha %d", len(clients))
	}
}

// TestDocumentConflictIsDetected: o mesmo CPF (ou telefone) em outro cadastro e
// sempre um conflito.
func TestDocumentConflictIsDetected(t *testing.T) {
	store := openTestStore(t)
	ctx := context.Background()
	seedCustomerWithOrder(t, store, "Carlos Dias", "037.528.350-10", "(54) 99912-3344")

	conflict, found, err := store.FindCustomerConflict(ctx, "", "03752835010", "")
	if err != nil {
		t.Fatal(err)
	}
	if !found || conflict.Profile.Name != "Carlos Dias" || conflict.Field != "CPF" {
		t.Fatalf("conflito de CPF nao detectado: %+v %v", conflict, found)
	}

	conflict, found, err = store.FindCustomerConflict(ctx, "", "", "5554999123344")
	if err != nil {
		t.Fatal(err)
	}
	if !found || conflict.Field != "telefone" {
		t.Fatalf("conflito de telefone nao detectado: %+v %v", conflict, found)
	}

	// O proprio cadastro nunca conflita consigo mesmo.
	if _, found, _ := store.FindCustomerConflict(ctx, "carlos dias", "037.528.350-10", "(54) 99912-3344"); found {
		t.Fatal("o cliente conflitou com ele mesmo")
	}
	// Documento incompleto nao identifica ninguem.
	if _, found, _ := store.FindCustomerConflict(ctx, "", "037", ""); found {
		t.Fatal("CPF incompleto nao deveria conflitar")
	}
}

// TestFillerNumbersAreIgnored: telefones e CPFs de preenchimento (0000000000)
// aparecem em varios cadastros e nao podem bloquear um cliente novo.
func TestFillerNumbersAreIgnored(t *testing.T) {
	store := openTestStore(t)
	ctx := context.Background()
	if err := store.SaveCustomerProfile(ctx, CustomerProfile{Name: "Cliente Um", Phone: "0000000000", CPF: "00000000000"}); err != nil {
		t.Fatal(err)
	}
	if _, found, _ := store.FindCustomerConflict(ctx, "", "00000000000", "0000000000"); found {
		t.Fatal("numero de preenchimento nao pode conflitar")
	}
	if err := store.SaveCustomerProfile(ctx, CustomerProfile{Name: "Cliente Dois", Phone: "0000000000", CPF: "00000000000"}); err != nil {
		t.Fatal(err)
	}
	groups, err := store.DuplicateCustomerGroups(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(groups) != 0 {
		t.Fatalf("numeros de preenchimento viraram duplicados: %+v", groups)
	}
}

// TestOrderDoesNotCreateSecondCustomerForSameDocument: salvar uma OS com o CPF
// de um cliente ja cadastrado nao pode abrir um segundo cadastro.
func TestOrderDoesNotCreateSecondCustomerForSameDocument(t *testing.T) {
	store := openTestStore(t)
	ctx := context.Background()
	seedCustomerWithOrder(t, store, "Matheus Schumacher", "037.528.350-10", "(54) 99912-3344")
	seedCustomerWithOrder(t, store, "Matheus", "037.528.350-10", "(54) 99912-3344")

	clients, err := store.ListCustomerProfiles(ctx, "")
	if err != nil {
		t.Fatal(err)
	}
	if len(clients) != 1 {
		names := make([]string, 0, len(clients))
		for _, c := range clients {
			names = append(names, c.Name)
		}
		t.Fatalf("o mesmo CPF gerou %d cadastros: %v", len(clients), names)
	}
	if clients[0].Orders != 2 {
		t.Fatalf("as duas OS deveriam ficar no mesmo cliente, veio %d", clients[0].Orders)
	}
}

// TestMergeCustomers: unificar move as OS e apaga o cadastro extra.
func TestMergeCustomers(t *testing.T) {
	store := openTestStore(t)
	ctx := context.Background()
	seedCustomerWithOrder(t, store, "Matheus Schumacher", "037.528.350-10", "")
	orderShort := seedCustomerWithOrder(t, store, "Matheus", "", "(54) 99912-3344")

	if err := store.MergeCustomerProfiles(ctx, "matheus schumacher", "matheus"); err != nil {
		t.Fatal(err)
	}
	clients, _ := store.ListCustomerProfiles(ctx, "")
	if len(clients) != 1 || clients[0].Name != "Matheus Schumacher" {
		t.Fatalf("clientes apos unificar: %+v", clients)
	}
	if clients[0].Orders != 2 {
		t.Fatalf("OS no cliente = %d", clients[0].Orders)
	}
	if clients[0].Phone == "" {
		t.Fatal("o telefone do cadastro extra deveria ter sido aproveitado")
	}
	moved, err := store.Get(ctx, orderShort.ID)
	if err != nil {
		t.Fatal(err)
	}
	if moved.Customer != "Matheus Schumacher" {
		t.Fatalf("a OS ficou com %q", moved.Customer)
	}
}

// TestDuplicateGroupsFindsRepeated: a revisao lista os cadastros repetidos.
func TestDuplicateGroupsFindsRepeated(t *testing.T) {
	store := openTestStore(t)
	ctx := context.Background()
	// Dois cadastros com o mesmo telefone (documento diferente para escapar da
	// protecao automatica e simular a base ja existente).
	if err := store.SaveCustomerProfile(ctx, CustomerProfile{Name: "Matheus", Phone: "(54) 99912-3344"}); err != nil {
		t.Fatal(err)
	}
	if err := store.SaveCustomerProfile(ctx, CustomerProfile{Name: "Matheus Schumacher", Phone: "54999123344"}); err != nil {
		t.Fatal(err)
	}
	if err := store.SaveCustomerProfile(ctx, CustomerProfile{Name: "Outra Pessoa", Phone: "(11) 91111-2222"}); err != nil {
		t.Fatal(err)
	}

	groups, err := store.DuplicateCustomerGroups(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(groups) != 1 {
		t.Fatalf("grupos = %d, queria 1: %+v", len(groups), groups)
	}
	if len(groups[0].Profiles) != 2 || groups[0].Field != "telefone" {
		t.Fatalf("grupo inesperado: %+v", groups[0])
	}
}

// TestShotDuplicateCustomers registra o aviso e a revisao de repetidos.
func TestShotDuplicateCustomers(t *testing.T) {
	dir := shotsDir(t)
	app := test.NewApp()
	defer app.Quit()
	desktop := false
	forceMobileLayout = &desktop
	defer func() { forceMobileLayout = nil }()
	app.Settings().SetTheme(newDBSTheme(themeDark))

	store := openTestStore(t)
	ctx := context.Background()
	admin, err := store.BootstrapAdministrator(ctx, "Douglas Silva", "douglas", "senha-segura")
	if err != nil {
		t.Fatal(err)
	}
	seedCustomerWithOrder(t, store, "Matheus", "", "(51) 98138-1793")
	if err := store.SaveCustomerProfile(ctx, CustomerProfile{Name: "Matheus Schumacher", Phone: "51981381793"}); err != nil {
		t.Fatal(err)
	}
	seedCustomerWithOrder(t, store, "Juliana", "", "(51) 98540-4381")
	if err := store.SaveCustomerProfile(ctx, CustomerProfile{Name: "Juliana de Meneses", Phone: "51985404381"}); err != nil {
		t.Fatal(err)
	}

	win := test.NewWindow(nil)
	win.Resize(fyne.NewSize(1200, 760))
	ui := newMainUI(app, store, win, "x.db")
	ui.currentUser = admin
	ui.applyRoleAccess()
	win.SetContent(ui.content)
	ui.showPage(pageCustomers)
	time.Sleep(400 * time.Millisecond)
	ui.content.Refresh()
	saveShot(t, dir, "customers-duplicates", win.Canvas().Capture())

	ui.showDuplicateCustomers()
	time.Sleep(200 * time.Millisecond)
	saveShot(t, dir, "customers-duplicates-review", win.Canvas().Capture())
	win.Close()
}

// TestFillerPhoneWithAreaCode: (51) 99999-9999 e numero de preenchimento e nao
// pode transformar clientes diferentes em repetidos.
func TestFillerPhoneWithAreaCode(t *testing.T) {
	for _, filler := range []string{"5199999999", "51 99999-9999", "(51) 00000-0000"} {
		if phoneKey(filler) != "" {
			t.Fatalf("%q deveria ser tratado como preenchimento", filler)
		}
	}
	if phoneKey("51988310635") == "" {
		t.Fatal("telefone real foi descartado")
	}
}

// TestMarkCustomersDistinctHidesGroup: quando o operador confirma que sao duas
// pessoas dividindo o telefone, o par sai do aviso de repetidos.
func TestMarkCustomersDistinctHidesGroup(t *testing.T) {
	store := openTestStore(t)
	ctx := context.Background()
	if err := store.SaveCustomerProfile(ctx, CustomerProfile{Name: "Ketlim Rodrigues", Phone: "(51) 98831-0635"}); err != nil {
		t.Fatal(err)
	}
	if err := store.SaveCustomerProfile(ctx, CustomerProfile{Name: "Luiz", Phone: "51988310635"}); err != nil {
		t.Fatal(err)
	}
	groups, err := store.DuplicateCustomerGroups(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(groups) != 1 {
		t.Fatalf("o telefone repetido deveria aparecer: %+v", groups)
	}
	if err := store.MarkCustomersDistinct(ctx, []string{"ketlim rodrigues", "luiz"}); err != nil {
		t.Fatal(err)
	}
	groups, err = store.DuplicateCustomerGroups(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(groups) != 0 {
		t.Fatalf("o par conferido continuou no aviso: %+v", groups)
	}
}
