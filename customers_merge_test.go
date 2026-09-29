package main

import (
	"context"
	"testing"
)

// TestSameNameWritten separa "mesmo nome escrito diferente" de "pessoas
// diferentes que dividem o telefone".
func TestSameNameWritten(t *testing.T) {
	same := [][2]string{
		{"Juliana", "JULIANA DE MENESES"},
		{"matheus", "matheus Schumacher"},
		{"Patricia Botelho", "Patricia Botego"},
		{"CRISTIA CORREIA DOS SANTOS", "CRISTIAn CORREIA DOS SANTOS"},
		{"Jose", "José"},
	}
	for _, pair := range same {
		if !sameNameWritten(pair[0], pair[1]) {
			t.Errorf("%q e %q deveriam ser a mesma pessoa", pair[0], pair[1])
		}
	}
	different := [][2]string{
		{"josé", "josé a06"},     // a06 e o aparelho, nao sobrenome
		{"jose a13", "josé a06"}, // dois aparelhos do mesmo apelido
		{"KETLIM RODRIGUES", "luiz"},
		{"João Lirio", "VAGENER"},
		{"JEFERSON G22", "."},
		{"Ana", "Bia"},
	}
	for _, pair := range different {
		if sameNameWritten(pair[0], pair[1]) {
			t.Errorf("%q e %q nao deveriam ser unidos", pair[0], pair[1])
		}
	}
}

// TestPickSurvivorKeepsFullestName: o cadastro que fica e o de nome mais
// completo, porque as OS dos outros migram de qualquer forma.
func TestPickSurvivorKeepsFullestName(t *testing.T) {
	keep := pickSurvivor([]CustomerProfile{
		{Key: "matheus", Name: "matheus", Orders: 2},
		{Key: "matheus schumacher", Name: "matheus Schumacher", Orders: 0},
	})
	if keep.Name != "matheus Schumacher" {
		t.Fatalf("ficou %q", keep.Name)
	}
}

// TestPlanSeparatesAutoFromManual: mesmo CPF entra no automatico; telefone
// repetido com nomes diferentes fica para o operador.
func TestPlanSeparatesAutoFromManual(t *testing.T) {
	store := openTestStore(t)
	ctx := context.Background()
	seedCustomerWithOrder(t, store, "Matheus", "", "(51) 98138-1793")
	if err := store.SaveCustomerProfile(ctx, CustomerProfile{Name: "Matheus Schumacher", Phone: "51981381793"}); err != nil {
		t.Fatal(err)
	}
	seedCustomerWithOrder(t, store, "Ketlim Rodrigues", "", "(51) 99831-0635")
	if err := store.SaveCustomerProfile(ctx, CustomerProfile{Name: "Luiz", Phone: "51998310635"}); err != nil {
		t.Fatal(err)
	}

	plan, err := store.PlanDuplicateMerge(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(plan.Auto) != 1 || plan.Auto[0].Keep.Name != "Matheus Schumacher" {
		t.Fatalf("automatico = %+v", plan.Auto)
	}
	if len(plan.Manual) != 1 || len(plan.Manual[0].Profiles) != 2 {
		t.Fatalf("manual = %+v", plan.Manual)
	}

	merged, err := store.ApplyMergePlan(ctx, plan)
	if err != nil {
		t.Fatal(err)
	}
	if merged != 1 {
		t.Fatalf("uniu %d cadastros", merged)
	}
	clients, _ := store.ListCustomerProfiles(ctx, "")
	if len(clients) != 3 {
		names := []string{}
		for _, c := range clients {
			names = append(names, c.Name)
		}
		t.Fatalf("sobraram %d clientes: %v", len(clients), names)
	}
	// A dupla ambigua continua intacta.
	after, err := store.PlanDuplicateMerge(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(after.Auto) != 0 || len(after.Manual) != 1 {
		t.Fatalf("depois da uniao: auto=%d manual=%d", len(after.Auto), len(after.Manual))
	}
}

// TestPlanMergesBlankNameIntoRealName: "." nao identifica ninguem e vai para o
// cadastro que tem nome.
func TestPlanMergesBlankNameIntoRealName(t *testing.T) {
	store := openTestStore(t)
	ctx := context.Background()
	if err := store.SaveCustomerProfile(ctx, CustomerProfile{Name: "JEFERSON G22", Phone: "51995306043"}); err != nil {
		t.Fatal(err)
	}
	if err := store.SaveCustomerProfile(ctx, CustomerProfile{Name: ".", Phone: "(51) 99530-6043"}); err != nil {
		t.Fatal(err)
	}
	plan, err := store.PlanDuplicateMerge(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(plan.Auto) != 1 || len(plan.Manual) != 0 {
		t.Fatalf("plano inesperado: %+v", plan)
	}
	if plan.Auto[0].Keep.Name != "JEFERSON G22" {
		t.Fatalf("deveria ficar o cadastro com nome, ficou %q", plan.Auto[0].Keep.Name)
	}
}

// TestPlanMergesDeviceNotesIntoCleanName: "josé", "josé a06" e "jose a13" com o
// mesmo telefone sao a mesma pessoa; o aparelho ja esta na OS.
func TestPlanMergesDeviceNotesIntoCleanName(t *testing.T) {
	store := openTestStore(t)
	ctx := context.Background()
	for _, name := range []string{"josé", "josé a06", "jose a13"} {
		if err := store.SaveCustomerProfile(ctx, CustomerProfile{Name: name, Phone: "(51) 99868-9253"}); err != nil {
			t.Fatal(err)
		}
	}
	plan, err := store.PlanDuplicateMerge(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(plan.Auto) != 1 || len(plan.Manual) != 0 {
		t.Fatalf("plano inesperado: %+v", plan)
	}
	if plan.Auto[0].Keep.Name != "josé" || len(plan.Auto[0].Merge) != 2 {
		t.Fatalf("deveria ficar o nome limpo com os outros dois: %+v", plan.Auto[0])
	}

	merged, err := store.MergeObviousDuplicates(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if merged != 2 {
		t.Fatalf("uniu %d, queria 2", merged)
	}
	groups, _ := store.DuplicateCustomerGroups(ctx)
	if len(groups) != 0 {
		t.Fatalf("sobrou repetido: %+v", groups)
	}
}

// TestPlanKeepsDifferentPeopleApart: nomes diferentes dividindo o telefone
// continuam sendo dois clientes.
func TestPlanKeepsDifferentPeopleApart(t *testing.T) {
	store := openTestStore(t)
	ctx := context.Background()
	if err := store.SaveCustomerProfile(ctx, CustomerProfile{Name: "KETLIM RODRIGUES", Phone: "51988310635"}); err != nil {
		t.Fatal(err)
	}
	if err := store.SaveCustomerProfile(ctx, CustomerProfile{Name: "luiz", Phone: "(51) 98831-0635"}); err != nil {
		t.Fatal(err)
	}
	plan, err := store.PlanDuplicateMerge(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(plan.Auto) != 0 || len(plan.Manual) != 1 {
		t.Fatalf("nomes diferentes nao podem ser unidos sozinhos: %+v", plan)
	}
	if merged, err := store.MergeObviousDuplicates(ctx); err != nil || merged != 0 {
		t.Fatalf("uniu %d sem poder (err=%v)", merged, err)
	}
}
