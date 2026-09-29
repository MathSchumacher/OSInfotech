package main

import (
	"context"
	"testing"
	"time"
)

// TestEntregaCarimbaData: a data de entrega e gravada quando o status vira
// Entregue, nao muda em edicoes seguintes, e some se a entrega for desfeita.
func TestEntregaCarimbaData(t *testing.T) {
	store := openTestStore(t)
	ctx := context.Background()

	order := ServiceOrder{
		Status: statusApproved, Customer: "Maria", Phone: "51999998888",
		Device: "Celular", Issue: "tela", AmountCents: 35000,
	}
	if err := store.Save(ctx, &order); err != nil {
		t.Fatal(err)
	}
	if !order.DeliveredAt.IsZero() {
		t.Fatal("OS aprovada não pode ter data de entrega")
	}

	order.Status = statusDelivered
	if err := store.Save(ctx, &order); err != nil {
		t.Fatal(err)
	}
	entrega := order.DeliveredAt
	if entrega.IsZero() {
		t.Fatal("marcar como entregue tem que carimbar a data")
	}

	// Uma correcao qualquer, depois. A data da entrega nao pode andar: era
	// exatamente isso que movia receita de mes quando o relatorio usava
	// updated_at.
	order.Phone = "51988887777"
	if err := store.Save(ctx, &order); err != nil {
		t.Fatal(err)
	}
	if !order.DeliveredAt.Equal(entrega) {
		t.Fatalf("a data de entrega andou: %v -> %v", entrega, order.DeliveredAt)
	}

	// Mesmo quando quem salva nao carrega o campo, como o formulario.
	semCampo, err := store.Get(ctx, order.ID)
	if err != nil {
		t.Fatal(err)
	}
	semCampo.DeliveredAt = time.Time{}
	semCampo.Notes = "conferido"
	if err := store.Save(ctx, &semCampo); err != nil {
		t.Fatal(err)
	}
	if !semCampo.DeliveredAt.Equal(entrega) {
		t.Fatalf("o Save deveria recuperar a data já gravada: %v", semCampo.DeliveredAt)
	}

	// Desfez a entrega: a OS voltou para a bancada.
	semCampo.Status = statusApproved
	if err := store.Save(ctx, &semCampo); err != nil {
		t.Fatal(err)
	}
	if !semCampo.DeliveredAt.IsZero() {
		t.Fatal("OS que voltou para conserto não pode ficar com data de entrega")
	}
}

// TestRelatorioNaoMudaDeMes e a razao de existir deste lote: um relatorio
// fechado nao pode mudar porque alguem corrigiu um telefone depois.
func TestRelatorioNaoMudaDeMes(t *testing.T) {
	store := openTestStore(t)
	ctx := context.Background()

	julho := time.Date(2026, 7, 10, 15, 0, 0, 0, time.UTC)
	order := ServiceOrder{
		Status: statusDelivered, Customer: "Cliente de julho", Phone: "51999998888",
		Device: "Celular", Issue: "tela", AmountCents: 30000, CostCents: 10000,
		DeliveredAt: julho,
	}
	if err := store.Save(ctx, &order); err != nil {
		t.Fatal(err)
	}

	periodo := func(ano int, mes time.Month) FinancialReport {
		t.Helper()
		inicio := time.Date(ano, mes, 1, 0, 0, 0, 0, time.UTC)
		report, err := store.FinancialReport(ctx, inicio, inicio.AddDate(0, 1, 0), "mensal")
		if err != nil {
			t.Fatal(err)
		}
		return report
	}

	if got := periodo(2026, time.July); len(got.Orders) != 1 || got.GrossCents != 30000 {
		t.Fatalf("julho deveria ter a OS: %d OS, bruto %d", len(got.Orders), got.GrossCents)
	}

	// Correcao feita em setembro. Com updated_at, esta OS saía de julho e
	// entrava em setembro.
	order.Phone = "51988887777"
	if err := store.Save(ctx, &order); err != nil {
		t.Fatal(err)
	}
	if got := periodo(2026, time.July); len(got.Orders) != 1 {
		t.Fatalf("a OS saiu do relatório de julho depois de uma correção: %d OS", len(got.Orders))
	}
	if got := periodo(2026, time.September); len(got.Orders) != 0 {
		t.Fatalf("a OS de julho apareceu em setembro: %d OS", len(got.Orders))
	}
}

// TestResultadoParcialNaoEChamadoDeLucro: sem custo lancado o periodo nao pode
// dizer lucro, e o numero precisa se declarar incompleto.
func TestResultadoParcialNaoEChamadoDeLucro(t *testing.T) {
	store := openTestStore(t)
	ctx := context.Background()
	quando := time.Date(2026, 7, 10, 15, 0, 0, 0, time.UTC)

	comCusto := ServiceOrder{
		Status: statusDelivered, Customer: "Com custo", Phone: "51999998888",
		Device: "Celular", Issue: "tela", AmountCents: 30000, CostCents: 10000,
		DeliveredAt: quando, PaymentMethod: "Pix", PaidAt: quando, PaidCents: 30000,
	}
	semCusto := ServiceOrder{
		Status: statusDelivered, Customer: "Sem custo", Phone: "51999997777",
		Device: "Celular", Issue: "bateria", AmountCents: 20000,
		DeliveredAt: quando, PaymentMethod: paymentPending,
	}
	for _, o := range []*ServiceOrder{&comCusto, &semCusto} {
		if err := store.Save(ctx, o); err != nil {
			t.Fatal(err)
		}
	}

	inicio := time.Date(2026, 7, 1, 0, 0, 0, 0, time.UTC)
	report, err := store.FinancialReport(ctx, inicio, inicio.AddDate(0, 1, 0), "mensal")
	if err != nil {
		t.Fatal(err)
	}
	if !report.CostIsPartial() || report.MissingCostOrders != 1 {
		t.Fatalf("o período deveria se declarar incompleto: %d sem custo", report.MissingCostOrders)
	}
	if netLabel(report) == "Lucro liquido" {
		t.Fatal("com custo faltando o resultado não pode se chamar lucro")
	}
	if report.UnpaidOrders != 1 || report.UnpaidCents != 20000 {
		t.Fatalf("a OS em aberto deveria estar no a receber: %d OS, %d", report.UnpaidOrders, report.UnpaidCents)
	}

	// Pagamento parcial: o que entra no "a receber" é o que falta, e não o
	// valor cheio. A loja escreve isso em texto livre hoje ("foi pago 100 dos
	// 220, falta 150").
	parcial := ServiceOrder{
		Status: statusDelivered, Customer: "Deu entrada", Phone: "51999996666",
		Device: "Celular", Issue: "conector", AmountCents: 22000,
		DeliveredAt: quando, PaymentMethod: "Dinheiro", PaidAt: quando, PaidCents: 10000,
	}
	if err := store.Save(ctx, &parcial); err != nil {
		t.Fatal(err)
	}
	if got := orderOwedCents(parcial); got != 12000 {
		t.Fatalf("faltam R$ 120,00 e a conta deu %d", got)
	}
	report, err = store.FinancialReport(ctx, inicio, inicio.AddDate(0, 1, 0), "mensal")
	if err != nil {
		t.Fatal(err)
	}
	if report.UnpaidOrders != 2 || report.UnpaidCents != 32000 {
		t.Fatalf("a receber deveria somar só o que falta: %d OS, %d", report.UnpaidOrders, report.UnpaidCents)
	}

	// Sem registro de pagamento nenhum não é dívida, é registro faltando.
	semRegistro := ServiceOrder{Status: statusDelivered, AmountCents: 50000}
	if orderOwedCents(semRegistro) != 0 {
		t.Fatal("OS sem forma de pagamento não pode virar cobrança")
	}
}

// TestSemPagamentoNaoEDivida: as OS entregues antes desta versao nao tem forma
// de pagamento, e tratar todas como divida inventaria uma cobranca.
func TestSemPagamentoNaoEDivida(t *testing.T) {
	antiga := ServiceOrder{Status: statusDelivered, AmountCents: 15000}
	if orderIsOwing(antiga) {
		t.Fatal("OS sem forma de pagamento não é dívida, é registro faltando")
	}
	if nota, tone := orderListNote(antiga); tone == toneDanger {
		t.Fatalf("a lista não pode acusar dívida numa OS antiga: %q", nota)
	}
	devendo := ServiceOrder{Status: statusDelivered, AmountCents: 15000, PaymentMethod: paymentPending}
	if !orderIsOwing(devendo) {
		t.Fatal("quem foi marcado como devendo tem que contar como dívida")
	}
	nota, tone := orderListNote(devendo)
	if tone != toneDanger || nota == "" {
		t.Fatalf("a lista deveria mostrar a dívida: %q %v", nota, tone)
	}
}

// TestBackfillDaDataDeEntrega: as OS entregues antes desta versao recebem a
// melhor estimativa disponivel, e o formato tem que ordenar como texto.
func TestBackfillDaDataDeEntrega(t *testing.T) {
	store := openTestStore(t)
	ctx := context.Background()
	// Uma OS gravada como as antigas: sem delivered_at, com fracao de segundo
	// no updated_at, que e o formato que nao ordena direito.
	if _, err := store.db.ExecContext(ctx, `
INSERT INTO service_orders(status, cpf, customer, phone, device, brand_model, serial, issue,
	accessories, notes, amount_cents, cost_cents, technician_id, technician_name, created_at, updated_at)
VALUES (?, '', 'Antiga', '', 'Celular', '', '', 'tela', '', '', 10000, 0, 0, '',
	'2026-07-10T12:00:00.4372Z', '2026-07-10T18:30:45.4372Z')`, statusDelivered); err != nil {
		t.Fatal(err)
	}
	if err := store.migrate(ctx); err != nil {
		t.Fatal(err)
	}
	var delivered string
	if err := store.db.QueryRowContext(ctx,
		"SELECT delivered_at FROM service_orders WHERE customer = 'Antiga'").Scan(&delivered); err != nil {
		t.Fatal(err)
	}
	if delivered != "2026-07-10T18:30:45Z" {
		t.Fatalf("data de entrega herdada errada: %q", delivered)
	}
}
