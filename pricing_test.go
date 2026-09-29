package main

import (
	"context"
	"testing"
)

// TestReconhecimentoDeServico usa os defeitos como a loja realmente escreve.
func TestReconhecimentoDeServico(t *testing.T) {
	casos := map[string]string{
		"troca de tela":                  "Troca de tela",
		"TROCA DE TELA\n\nSEM SENHA":     "Troca de tela",
		"tela quebrada":                  "Troca de tela",
		"nao carrega":                    "Conector de carga",
		"dock de carga":                  "Conector de carga",
		"virus":                          "Formatação e vírus",
		"caiu na agua a 6 meses atras":   "Limpeza de oxidação",
		"NAO SAI SOM":                    "Áudio",
		"NÃO LIGA":                       "Não liga",
		"troca de bateria":               "Troca de bateria",
		"orçar peça sem descrição clara": "",
	}
	for defeito, esperado := range casos {
		if got := serviceFor(defeito); got != esperado {
			t.Fatalf("%q reconhecido como %q, esperava %q", defeito, got, esperado)
		}
	}
}

// TestMemoriaDePreco: a mesma troca de tela no mesmo aparelho, escrita de formas
// diferentes, tem que virar uma única memória de preço.
func TestMemoriaDePreco(t *testing.T) {
	store := openTestStore(t)
	ctx := context.Background()

	anteriores := []struct {
		modelo string
		valor  int64
		custo  int64
	}{
		{"Samsung Galaxy A14", 25000, 12000},
		{"a14", 35000, 18000},
	}
	for i, item := range anteriores {
		order := ServiceOrder{
			Status: statusDelivered, Customer: "Cliente", Phone: "(51) 98765-4321",
			Device: "Celular", BrandModel: item.modelo, Issue: "troca de tela",
			AmountCents: item.valor, CostCents: item.custo,
		}
		if err := store.Save(ctx, &order); err != nil {
			t.Fatalf("caso %d: %v", i, err)
		}
	}

	// A OS nova, com o modelo escrito de um terceiro jeito.
	nova := ServiceOrder{
		Status: statusPending, Customer: "Outro", Phone: "(51) 98765-4322",
		Device: "Celular", BrandModel: "Galaxy A14", Issue: "TELA QUEBRADA",
	}
	if err := store.Save(ctx, &nova); err != nil {
		t.Fatal(err)
	}

	points, err := store.RecentPrices(ctx, modelKeyFor(nova.BrandModel), serviceKeyFor(nova.Issue), nova.ID, 3)
	if err != nil {
		t.Fatal(err)
	}
	if len(points) != 2 {
		t.Fatalf("as duas cobranças anteriores deveriam aparecer, vieram %d", len(points))
	}
	if points[0].AmountCents != 35000 || points[0].CostCents != 18000 {
		t.Fatalf("a mais recente deveria vir primeiro: %+v", points[0])
	}

	// Serviço diferente no mesmo aparelho não entra.
	outros, err := store.RecentPrices(ctx, modelKeyFor("Galaxy A14"), serviceKeyFor("não carrega"), 0, 3)
	if err != nil {
		t.Fatal(err)
	}
	if len(outros) != 0 {
		t.Fatalf("conector de carga não tem histórico, vieram %d", len(outros))
	}
}

// TestPreencheChavesAntigas: as OS que já existiam ganham modelo e serviço sem
// ninguém digitar nada.
func TestPreencheChavesAntigas(t *testing.T) {
	store := openTestStore(t)
	ctx := context.Background()
	order := ServiceOrder{
		Status: statusDelivered, Customer: "Antiga", Phone: "(51) 98765-4321",
		Device: "Celular", BrandModel: "moto g54", Issue: "troca de tela", AmountCents: 30000,
	}
	if err := store.Save(ctx, &order); err != nil {
		t.Fatal(err)
	}
	// Simula a linha vinda da nuvem, sem as chaves derivadas.
	if _, err := store.db.ExecContext(ctx, `UPDATE service_orders SET model_key='', service_key='' WHERE id = ?`, order.ID); err != nil {
		t.Fatal(err)
	}
	filled, err := store.BackfillOrderKeys(ctx, 100)
	if err != nil {
		t.Fatal(err)
	}
	if filled != 1 {
		t.Fatalf("deveria ter preenchido 1 OS, preencheu %d", filled)
	}
	points, err := store.RecentPrices(ctx, modelKeyFor("g54"), serviceKeyFor("tela"), 0, 3)
	if err != nil {
		t.Fatal(err)
	}
	if len(points) != 1 || points[0].AmountCents != 30000 {
		t.Fatalf("a OS antiga deveria entrar na memória de preço: %+v", points)
	}
	// Roda de novo: não pode reprocessar o que já está pronto.
	again, err := store.BackfillOrderKeys(ctx, 100)
	if err != nil || again != 0 {
		t.Fatalf("segunda passada deveria não achar nada: %d %v", again, err)
	}
}
