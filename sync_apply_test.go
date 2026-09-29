package main

import (
	"context"
	"encoding/json"
	"strconv"
	"testing"
	"time"
)

// remoteOrderFrom monta a versao "da nuvem" de uma OS local.
func remoteOrderFrom(order ServiceOrder, remoteID string, revision int64) remoteServiceOrder {
	return remoteServiceOrder{
		ID: remoteID, Status: order.Status, CustomerName: order.Customer,
		CustomerDocument: order.CPF, CustomerPhone: order.Phone, Device: order.Device,
		BrandModel: order.BrandModel, Issue: order.Issue, GrossCents: order.AmountCents,
		CostCents: order.CostCents, Revision: revision,
	}
}

// TestRemoteOrderKeepsTechnician: a OS que volta da nuvem nao pode perder o
// tecnico responsavel; era isso que fazia a OS trocar de dono sozinha.
func TestRemoteOrderKeepsTechnician(t *testing.T) {
	store := openTestStore(t)
	ctx := context.Background()
	tech, err := store.BootstrapAdministrator(ctx, "Bruno Tecnico", "bruno", "senha-segura")
	if err != nil {
		t.Fatal(err)
	}
	if err := store.saveSyncMapping(ctx, "user", strconv.FormatInt(tech.ID, 10), "u-remoto", 0); err != nil {
		t.Fatal(err)
	}
	order := ServiceOrder{
		Status: statusPending, Customer: "Cliente Teste", Device: "Celular", Issue: "Não liga",
		TechnicianID: tech.ID, TechnicianName: tech.Name, AmountCents: 45000,
	}
	if err := store.Save(ctx, &order); err != nil {
		t.Fatal(err)
	}
	if err := store.saveSyncMapping(ctx, syncEntityOrder, strconv.FormatInt(order.ID, 10), "os-remota", 1); err != nil {
		t.Fatal(err)
	}

	// A nuvem devolve a OS com o tecnico conhecido.
	remote := remoteOrderFrom(order, "os-remota", 2)
	remote.TechnicianID = "u-remoto"
	remote.Notes = "editada em outro aparelho"
	if err := store.applyRemoteOrder(ctx, remote); err != nil {
		t.Fatal(err)
	}
	updated, err := store.Get(ctx, order.ID)
	if err != nil {
		t.Fatal(err)
	}
	if updated.TechnicianID != tech.ID || updated.TechnicianName != tech.Name {
		t.Fatalf("o tecnico se perdeu: id=%d nome=%q", updated.TechnicianID, updated.TechnicianName)
	}
	if updated.Notes != "editada em outro aparelho" {
		t.Fatalf("a edicao da nuvem nao chegou: %q", updated.Notes)
	}

	// Tecnico desconhecido neste aparelho: mantem o que ja estava.
	remote.TechnicianID = "u-de-outro-aparelho"
	remote.Revision = 3
	if err := store.applyRemoteOrder(ctx, remote); err != nil {
		t.Fatal(err)
	}
	updated, err = store.Get(ctx, order.ID)
	if err != nil {
		t.Fatal(err)
	}
	if updated.TechnicianID != tech.ID {
		t.Fatalf("tecnico desconhecido apagou o responsavel: %d", updated.TechnicianID)
	}
}

// TestRemoteOrderReturnsAfterLocalDelete: se a exclusao local foi recusada pela
// nuvem, a OS precisa voltar em vez de sumir do aparelho.
func TestRemoteOrderReturnsAfterLocalDelete(t *testing.T) {
	store := openTestStore(t)
	ctx := context.Background()
	order := ServiceOrder{Status: statusApproved, Customer: "Joana Ribeiro", Device: "Notebook", Issue: "Não carrega"}
	if err := store.Save(ctx, &order); err != nil {
		t.Fatal(err)
	}
	if err := store.saveSyncMapping(ctx, syncEntityOrder, strconv.FormatInt(order.ID, 10), "os-remota", 1); err != nil {
		t.Fatal(err)
	}
	// Exclusao local (o vinculo com a nuvem continua ate a nuvem confirmar).
	if _, err := store.db.ExecContext(ctx, "DELETE FROM service_orders WHERE id = ?", order.ID); err != nil {
		t.Fatal(err)
	}
	remote := remoteOrderFrom(order, "os-remota", 2)
	if err := store.applyRemoteOrder(ctx, remote); err != nil {
		t.Fatal(err)
	}
	orders, err := store.List(ctx, "Todos", "Joana Ribeiro")
	if err != nil {
		t.Fatal(err)
	}
	if len(orders) != 1 {
		t.Fatalf("a OS deveria ter voltado, encontrei %d", len(orders))
	}
	if orders[0].Device != "Notebook" {
		t.Fatalf("voltou errada: %+v", orders[0])
	}
}

// TestDeletePushClearsMapping: depois que a nuvem confirma a exclusao, o vinculo
// local nao pode continuar apontando para uma linha que nao existe mais.
func TestDeletePushClearsMapping(t *testing.T) {
	store := openTestStore(t)
	ctx := context.Background()
	order := ServiceOrder{Status: statusPending, Customer: "Cliente Sumiu", Device: "TV", Issue: "Sem imagem"}
	if err := store.Save(ctx, &order); err != nil {
		t.Fatal(err)
	}
	localKey := strconv.FormatInt(order.ID, 10)
	if err := store.saveSyncMapping(ctx, syncEntityOrder, localKey, "os-remota", 1); err != nil {
		t.Fatal(err)
	}
	if err := store.Delete(ctx, order.ID); err != nil {
		t.Fatal(err)
	}
	change := SyncQueueEntry{EntityType: syncEntityOrder, EntityRemote: "os-remota", LocalKey: localKey, Action: "delete"}
	if err := store.completeSyncChange(ctx, change, 2); err != nil {
		t.Fatal(err)
	}
	if _, found, _ := store.existingSyncMapping(ctx, syncEntityOrder, localKey); found {
		t.Fatal("o vinculo com a nuvem continuou depois da exclusao confirmada")
	}
}

// TestFechamentoViajaNaSincronizacao: os campos do fechamento e os que ficavam
// presos num computador so precisam ir e voltar inteiros.
func TestFechamentoViajaNaSincronizacao(t *testing.T) {
	store := openTestStore(t)
	ctx := context.Background()
	entregue := time.Date(2026, 9, 20, 13, 0, 0, 0, time.UTC)

	order := ServiceOrder{
		Status: statusDelivered, Customer: "Maria", Phone: "51999998888",
		Device: "Celular", Issue: "tela", DeviceKey: "1234",
		AmountCents: 35000, CostCents: 12000,
		DeliveredAt: entregue, PaidAt: entregue, PaymentMethod: "Pix",
	}
	if err := store.Save(ctx, &order); err != nil {
		t.Fatal(err)
	}
	if err := store.MarkOrderContacted(ctx, order.ID, entregue); err != nil {
		t.Fatal(err)
	}
	if err := store.queueOrder(ctx, mustGetOrder(t, store, order.ID)); err != nil {
		t.Fatal(err)
	}

	pending, err := store.PendingSyncChanges(ctx, 50)
	if err != nil {
		t.Fatal(err)
	}
	var payload syncOrderPayload
	found := false
	for _, item := range pending {
		if item.EntityType != syncEntityOrder {
			continue
		}
		if err := json.Unmarshal(item.Payload, &payload); err != nil {
			t.Fatal(err)
		}
		found = true
	}
	if !found {
		t.Fatal("a OS não entrou na fila de envio")
	}
	if payload.DeviceKey != "1234" || payload.PaymentMethod != "Pix" {
		t.Fatalf("o envio perdeu campos: %+v", payload)
	}
	if payload.DeliveredAt != "2026-09-20T13:00:00Z" || payload.PaidAt != "2026-09-20T13:00:00Z" {
		t.Fatalf("datas do fechamento erradas: %+v", payload)
	}
	if payload.ContactedAt != "2026-09-20T13:00:00Z" {
		t.Fatalf("a data da cobrança não foi enviada: %q", payload.ContactedAt)
	}
}

// TestNuvemVaziaNaoApagaFechamento: enquanto o servidor antigo nao conhecer os
// campos novos ele responde vazio para todos, e uma leitura nao pode limpar a
// senha do aparelho nem o fechamento que ja existe aqui.
func TestNuvemVaziaNaoApagaFechamento(t *testing.T) {
	store := openTestStore(t)
	ctx := context.Background()
	entregue := time.Date(2026, 9, 20, 13, 0, 0, 0, time.UTC)

	order := ServiceOrder{
		Status: statusDelivered, Customer: "Maria", Phone: "51999998888",
		Device: "Celular", Issue: "tela", DeviceKey: "1234",
		AmountCents: 35000, DeliveredAt: entregue, PaidAt: entregue, PaymentMethod: "Pix",
	}
	if err := store.Save(ctx, &order); err != nil {
		t.Fatal(err)
	}
	if err := store.saveSyncMapping(ctx, syncEntityOrder, strconv.FormatInt(order.ID, 10), "os-remota", 1); err != nil {
		t.Fatal(err)
	}

	remote := remoteOrderFrom(order, "os-remota", 2)
	remote.Notes = "editada em outro aparelho"
	// Servidor antigo: nenhum dos campos novos vem preenchido.
	if err := store.applyRemoteOrder(ctx, remote); err != nil {
		t.Fatal(err)
	}
	updated := mustGetOrder(t, store, order.ID)
	if updated.Notes != "editada em outro aparelho" {
		t.Fatalf("a edição da nuvem não chegou: %q", updated.Notes)
	}
	if updated.DeviceKey != "1234" {
		t.Fatal("a senha do aparelho foi apagada pela sincronização")
	}
	if updated.PaymentMethod != "Pix" || updated.DeliveredAt.IsZero() || updated.PaidAt.IsZero() {
		t.Fatalf("o fechamento foi apagado pela sincronização: %+v", updated)
	}
}

func mustGetOrder(t *testing.T, store *Store, id int64) ServiceOrder {
	t.Helper()
	order, err := store.Get(context.Background(), id)
	if err != nil {
		t.Fatal(err)
	}
	return order
}
