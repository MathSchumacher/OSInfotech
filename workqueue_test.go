package main

import (
	"context"
	"testing"
	"time"
)

// TestFilaSeparaQuemEsperaOQue: a distinção é derivada do valor lançado, e é
// ela que transforma um monte de 92 OS em duas listas com dono diferente.
func TestFilaSeparaQuemEsperaOQue(t *testing.T) {
	semPreco := ServiceOrder{Status: statusPending}
	comPreco := ServiceOrder{Status: statusPending, AmountCents: 25000}
	emConserto := ServiceOrder{Status: statusApproved, AmountCents: 25000}

	if orderQueue(semPreco) != queueNeedsQuote {
		t.Fatal("OS pendente sem valor é trabalho da loja")
	}
	if orderQueue(comPreco) != queueNeedsAnswer {
		t.Fatal("OS pendente já orçada espera o cliente")
	}
	if orderQueue(emConserto) != queueNone {
		t.Fatal("OS aprovada não está em nenhuma das duas filas")
	}
}

// TestFiltroDaFilaNaLista: os dois filtros novos precisam devolver exatamente
// as OS daquela fila, sem tocar nos filtros de status que já existiam.
func TestFiltroDaFilaNaLista(t *testing.T) {
	store := openTestStore(t)
	ctx := context.Background()

	orcar := ServiceOrder{
		Status: statusPending, Customer: "Sem preço", Phone: "51999998888",
		Device: "Celular", Issue: "tela",
	}
	responder := ServiceOrder{
		Status: statusPending, Customer: "Já orçado", Phone: "51999997777",
		Device: "Celular", Issue: "bateria", AmountCents: 30000,
	}
	bancada := ServiceOrder{
		Status: statusApproved, Customer: "Em conserto", Phone: "51999996666",
		Device: "Celular", Issue: "conector", AmountCents: 20000,
	}
	for _, o := range []*ServiceOrder{&orcar, &responder, &bancada} {
		if err := store.Save(ctx, o); err != nil {
			t.Fatal(err)
		}
	}

	casos := []struct {
		filtro string
		quer   string
	}{
		{filterNeedsQuote, "Sem preço"},
		{filterNeedsAnswer, "Já orçado"},
	}
	for _, caso := range casos {
		lista, err := store.List(ctx, caso.filtro, "")
		if err != nil {
			t.Fatal(err)
		}
		if len(lista) != 1 || lista[0].Customer != caso.quer {
			t.Fatalf("filtro %q devolveu %d OS, esperava só %q", caso.filtro, len(lista), caso.quer)
		}
	}

	// O filtro por status continua como era.
	if lista, err := store.List(ctx, statusPending, ""); err != nil || len(lista) != 2 {
		t.Fatalf("o filtro de status mudou de comportamento: %d OS, %v", len(lista), err)
	}

	counts, err := store.Counts(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if counts[filterNeedsQuote] != 1 || counts[filterNeedsAnswer] != 1 {
		t.Fatalf("contagem das filas errada: %+v", counts)
	}
	if counts[statusPending] != 2 {
		t.Fatalf("a contagem por status não pode ter mudado: %d", counts[statusPending])
	}
}

// TestResumoDaFila: o ladrilho precisa dizer há quanto tempo a mais antiga
// espera e quanto dinheiro está parado — o número sozinho não move ninguém.
func TestResumoDaFila(t *testing.T) {
	store := openTestStore(t)
	ctx := context.Background()

	antiga := ServiceOrder{
		Status: statusPending, Customer: "Antiga", Phone: "51999998888",
		Device: "Celular", Issue: "tela",
	}
	if err := store.Save(ctx, &antiga); err != nil {
		t.Fatal(err)
	}
	// Envelhece a OS sem passar pelo Save, que carimbaria a data de hoje.
	if _, err := store.db.ExecContext(ctx, "UPDATE service_orders SET created_at = ? WHERE id = ?",
		encodeTime(time.Now().UTC().AddDate(0, 0, -40)), antiga.ID); err != nil {
		t.Fatal(err)
	}
	orcada := ServiceOrder{
		Status: statusPending, Customer: "Orçada", Phone: "51999997777",
		Device: "Celular", Issue: "bateria", AmountCents: 45000,
	}
	if err := store.Save(ctx, &orcada); err != nil {
		t.Fatal(err)
	}

	needsQuote, needsAnswer, err := store.QueueStats(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if needsQuote.Orders != 1 || needsQuote.Overdue != 1 || needsQuote.OldestDay < 39 {
		t.Fatalf("fila de orçar errada: %+v", needsQuote)
	}
	if needsAnswer.Orders != 1 || needsAnswer.Cents != 45000 {
		t.Fatalf("fila de resposta errada: %+v", needsAnswer)
	}
	if got := needsAnswer.Summary(true); got != "R$ 450,00 parados" {
		t.Fatalf("o resumo precisa citar o dinheiro parado: %q", got)
	}
	if got := needsQuote.Summary(false); got != "mais antiga há 40 dias" {
		t.Fatalf("o resumo da fila de orçar precisa citar a idade: %q", got)
	}
	vazia := QueueStat{}
	if vazia.Summary(true) != "Nada esperando" {
		t.Fatal("fila vazia não pode inventar aviso")
	}
}

// TestRotuloDoFiltroVaiEVolta: o Select do celular mostra rótulo e guarda
// valor interno; trocar um pelo outro deixaria o filtro mudo.
func TestRotuloDoFiltroVaiEVolta(t *testing.T) {
	for _, valor := range []string{"", filterNeedsQuote, filterNeedsAnswer, statusDelivered} {
		if got := filterValueFromLabel(filterLabelFromValue(valor)); got != valor {
			t.Fatalf("%q virou %q na ida e volta", valor, got)
		}
	}
}
