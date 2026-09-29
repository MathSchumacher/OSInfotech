package main

import (
	"context"
	"fmt"
	"testing"
	"time"

	"fyne.io/fyne/v2/test"
)

func TestFoldSearchTextRemovesAccentsAndCase(t *testing.T) {
	cases := map[string]string{
		"José":            "jose",
		"JOSÉ DA SILVA":   "jose da silva",
		"  Conserto  TV ": "conserto tv",
		"Não liga":        "nao liga",
		"Inspiron":        "inspiron",
	}
	for in, want := range cases {
		if got := foldSearchText(in); got != want {
			t.Fatalf("foldSearchText(%q) = %q, quero %q", in, got, want)
		}
	}
}

func TestParseSearchTokensDropsOrderPrefix(t *testing.T) {
	tokens := parseSearchTokens("OS #42")
	if len(tokens) != 1 {
		t.Fatalf("parseSearchTokens devolveu %d termos: %+v", len(tokens), tokens)
	}
	if tokens[0].text != "42" || !tokens[0].numeric || tokens[0].digits != "42" {
		t.Fatalf("token inesperado: %+v", tokens[0])
	}

	if tokens := parseSearchTokens("os"); len(tokens) != 1 || tokens[0].text != "os" {
		t.Fatalf("termo unico nao pode ser descartado: %+v", tokens)
	}
	if tokens := parseSearchTokens("   "); tokens != nil {
		t.Fatalf("busca vazia deveria devolver nil, veio %+v", tokens)
	}
}

func TestBuildSearchFilterIgnoresDigitColumnsWithoutDigits(t *testing.T) {
	clause, args, ok := buildSearchFilter("tela", orderSearchSpec)
	if !ok {
		t.Fatal("buildSearchFilter deveria montar a clausula")
	}
	if len(args) != len(orderSearchSpec.TextCols) {
		t.Fatalf("busca sem digitos deveria consultar apenas colunas de texto, veio %d args: %s", len(args), clause)
	}
	if _, _, ok := buildSearchFilter("  ", orderSearchSpec); ok {
		t.Fatal("busca vazia nao deveria filtrar")
	}
}

func seedSearchOrders(t *testing.T, store *Store) (int64, int64) {
	t.Helper()
	ctx := context.Background()
	first := ServiceOrder{
		Status: statusPending, Customer: "José da Silva", CPF: "123.456.789-00",
		Phone: "(11) 98765-4321", Device: "Celular", BrandModel: "iPhone 11",
		Issue: "Tela quebrada", Notes: "Cliente aguarda orcamento",
	}
	if err := store.Save(ctx, &first); err != nil {
		t.Fatalf("Save() error = %v", err)
	}
	second := ServiceOrder{
		Status: statusDelivered, Customer: "Maria Souza", CPF: "555.444.333-22",
		Phone: "(21) 3333-2222", Device: "Notebook", BrandModel: "Dell Inspiron",
		Issue: "Nao liga",
	}
	if err := store.Save(ctx, &second); err != nil {
		t.Fatalf("Save() error = %v", err)
	}
	return first.ID, second.ID
}

func TestListSearch(t *testing.T) {
	store := openTestStore(t)
	ctx := context.Background()
	firstID, secondID := seedSearchOrders(t, store)

	cases := []struct {
		name   string
		status string
		query  string
		want   []int64
	}{
		{"sem busca", "", "", []int64{secondID, firstID}},
		{"texto sem digitos nao pode devolver a base inteira", "", "tela", []int64{firstID}},
		{"acento no cadastro", "", "jose", []int64{firstID}},
		{"acento na busca", "", "JOSÉ", []int64{firstID}},
		{"termos em campos diferentes", "", "jose iphone", []int64{firstID}},
		{"telefone parcial", "", "98765", []int64{firstID}},
		{"cpf com pontuacao", "", "123.456.789-00", []int64{firstID}},
		{"numero da os", "", fmt.Sprintf("#%d", secondID), []int64{secondID}},
		{"defeito com acento diferente", "", "não liga", []int64{secondID}},
		{"status combinado com busca", statusDelivered, "maria", []int64{secondID}},
		{"status exclui o resultado", statusPending, "maria", nil},
		{"sem resultado", "", "zzzzz", nil},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			orders, err := store.List(ctx, tc.status, tc.query)
			if err != nil {
				t.Fatalf("List() error = %v", err)
			}
			if len(orders) != len(tc.want) {
				t.Fatalf("List(%q, %q) devolveu %d OS, quero %d", tc.status, tc.query, len(orders), len(tc.want))
			}
			for i, id := range tc.want {
				if orders[i].ID != id {
					t.Fatalf("List(%q, %q)[%d] = OS %d, quero %d", tc.status, tc.query, i, orders[i].ID, id)
				}
			}
		})
	}
}

func TestFindCustomerProfilesIgnoresAccents(t *testing.T) {
	store := openTestStore(t)
	ctx := context.Background()
	seedSearchOrders(t, store)

	profiles, err := store.FindCustomerProfiles(ctx, "jose", 8)
	if err != nil {
		t.Fatalf("FindCustomerProfiles() error = %v", err)
	}
	if len(profiles) != 1 || profiles[0].Name != "José da Silva" {
		t.Fatalf("FindCustomerProfiles(jose) = %+v", profiles)
	}

	profiles, err = store.FindCustomerProfiles(ctx, "silva iphone", 8)
	if err != nil {
		t.Fatalf("FindCustomerProfiles() error = %v", err)
	}
	if len(profiles) != 1 {
		t.Fatalf("busca por nome + aparelho = %+v", profiles)
	}

	profiles, err = store.FindCustomerProfiles(ctx, "quebrada", 8)
	if err != nil {
		t.Fatalf("FindCustomerProfiles() error = %v", err)
	}
	if len(profiles) != 0 {
		t.Fatalf("defeito nao e campo de cliente, veio %+v", profiles)
	}
}

func TestListTermsSearch(t *testing.T) {
	store := openTestStore(t)
	ctx := context.Background()
	term := ServiceTerm{
		SellerName: "Antônio Gonçalves", SellerCPF: "111.222.333-44",
		BuyerCompany: "OS Infotech", BuyerDocument: "12.345.678/0001-90",
		DeviceKind: "Celular", Brand: "Samsung", Model: "Galaxy S21", IMEI1: "356938035643809",
	}
	if err := store.SaveTerm(ctx, &term); err != nil {
		t.Fatalf("SaveTerm() error = %v", err)
	}
	other := ServiceTerm{SellerName: "Maria Souza", DeviceKind: "Notebook", Brand: "Dell"}
	if err := store.SaveTerm(ctx, &other); err != nil {
		t.Fatalf("SaveTerm() error = %v", err)
	}

	cases := map[string]int{
		"antonio":         1,
		"ANTÔNIO":         1,
		"samsung galaxy":  1,
		"356938035643809": 1,
		"12345678000190":  1,
		"dell":            1,
		"zzzzz":           0,
		"celular":         1,
		"notebook":        1,
	}
	for query, want := range cases {
		terms, err := store.ListTerms(ctx, query, termFilterAll)
		if err != nil {
			t.Fatalf("ListTerms(%q) error = %v", query, err)
		}
		if len(terms) != want {
			t.Fatalf("ListTerms(%q) devolveu %d termos, quero %d", query, len(terms), want)
		}
	}
}

// TestSearchSummaryReflectsFilters monta a interface inteira e confere o resumo
// de resultados e o estado vazio da lista de OS - tambem serve de smoke test
// para a construcao das paginas.
func TestSearchSummaryReflectsFilters(t *testing.T) {
	app := test.NewApp()
	defer app.Quit()
	desktop := false
	forceMobileLayout = &desktop
	defer func() { forceMobileLayout = nil }()

	store := openTestStore(t)
	seedSearchOrders(t, store)

	ui := newMainUI(app, store, app.NewWindow("teste"), "teste.db")
	ui.showPage(pageOrders)
	if got := ui.listSummary.Text(); got != "2 OS no total" {
		t.Fatalf("resumo sem filtro = %q", got)
	}
	if ui.ordersEmpty.Visible() {
		t.Fatal("estado vazio nao deveria aparecer com resultados")
	}

	// A busca espera o operador parar de digitar antes de refazer a lista.
	esperarBusca := func() { time.Sleep(searchDebounce + 250*time.Millisecond) }

	ui.searchEntry.SetText("jose")
	esperarBusca()
	if got := ui.listSummary.Text(); got != "1 resultado(s)" {
		t.Fatalf("resumo com busca = %q", got)
	}

	ui.searchEntry.SetText("zzzzz")
	esperarBusca()
	if got := ui.listSummary.Text(); got != "Nenhum resultado para os filtros atuais" {
		t.Fatalf("resumo sem resultados = %q", got)
	}
	if !ui.ordersEmpty.Visible() {
		t.Fatal("estado vazio deveria aparecer sem resultados")
	}

	ui.clearOrderFilters()
	if got := ui.listSummary.Text(); got != "2 OS no total" {
		t.Fatalf("resumo apos limpar = %q", got)
	}

	ui.openOrdersWithStatus(statusDelivered)
	if got := ui.listSummary.Text(); got != "1 resultado(s)" {
		t.Fatalf("resumo com filtro de status = %q", got)
	}
	ui.clearOrderFilters()
	if got := ui.listSummary.Text(); got != "2 OS no total" {
		t.Fatalf("resumo apos limpar o status = %q", got)
	}

	// As demais paginas precisam montar sem panico.
	for _, kind := range []pageKind{pageHome, pageCustomers, pageCustomerEditor, pageTerms, pageTermEditor, pageReports, pageSettings, pageOrderEditor} {
		ui.showPage(kind)
	}
}
