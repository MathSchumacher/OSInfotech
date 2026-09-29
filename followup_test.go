package main

import (
	"context"
	"fmt"
	"fyne.io/fyne/v2"
	"strings"
	"testing"
	"time"
)

// TestMensagemDeCobranca: a mensagem precisa sair pronta, com numero da OS,
// aparelho e valor, para o operador so apertar enviar.
func TestMensagemDeCobranca(t *testing.T) {
	order := ServiceOrder{
		ID: 137, Status: statusPending, Customer: "JOSÉ da silva", Phone: "(51) 98765-4321",
		Device: "Celular", BrandModel: "Galaxy A14", Issue: "troca de tela", AmountCents: 45000,
	}
	msg := followUpMessage(order)
	for _, want := range []string{"José", "000137", "Galaxy A14", "R$ 450,00", "Podemos seguir"} {
		if !strings.Contains(msg, want) {
			t.Fatalf("a mensagem não cita %q:\n%s", want, msg)
		}
	}

	link := whatsappMessageURL(order.Phone, msg)
	if !strings.HasPrefix(link, "https://wa.me/5551987654321?text=") {
		t.Fatalf("link inesperado: %s", link)
	}
	if strings.Contains(link, " ") || strings.Contains(link, "+") {
		t.Fatalf("o texto não foi codificado para URL: %s", link)
	}

	// Sem orcamento a conversa e outra.
	order.AmountCents = 0
	if msg := followUpMessage(order); !strings.Contains(msg, "avaliação") {
		t.Fatalf("sem valor a mensagem deveria falar em avaliação: %s", msg)
	}
	// Pronto para retirada.
	order.Status = statusNoRepair
	if msg := followUpMessage(order); !strings.Contains(msg, "retirada") {
		t.Fatalf("sem conserto deveria avisar para retirar: %s", msg)
	}
}

// TestQuandoOferecerCobranca: entregue nao se cobra, e sem telefone nao dá.
func TestQuandoOferecerCobranca(t *testing.T) {
	base := ServiceOrder{ID: 1, Status: statusPending, Phone: "(51) 98765-4321"}
	if !canFollowUp(base) {
		t.Fatal("OS em aberto com telefone deveria permitir cobrança")
	}
	entregue := base
	entregue.Status = statusDelivered
	if canFollowUp(entregue) {
		t.Fatal("OS entregue não deve oferecer cobrança")
	}
	semTelefone := base
	semTelefone.Phone = "123"
	if canFollowUp(semTelefone) {
		t.Fatal("telefone incompleto não deve oferecer cobrança")
	}
}

// TestIdadeDaOS: a lista precisa destacar o que está parado.
func TestIdadeDaOS(t *testing.T) {
	novo := ServiceOrder{Status: statusPending, CreatedAt: time.Now().Add(-2 * time.Hour)}
	if text, _, atrasada := orderAgeText(novo); atrasada || text != "aberta hoje" {
		t.Fatalf("OS de hoje: %q atrasada=%v", text, atrasada)
	}
	parada := ServiceOrder{Status: statusPending, CreatedAt: time.Now().AddDate(0, 0, -12)}
	text, tone, atrasada := orderAgeText(parada)
	if !atrasada || tone != toneWarning || !strings.Contains(text, "12 dias") {
		t.Fatalf("OS de 12 dias: %q tom=%v atrasada=%v", text, tone, atrasada)
	}
	antiga := ServiceOrder{Status: statusPending, CreatedAt: time.Now().AddDate(0, 0, -40)}
	if _, tone, _ := orderAgeText(antiga); tone != toneDanger {
		t.Fatalf("OS de 40 dias deveria ser tom de alerta, veio %v", tone)
	}
	entregue := ServiceOrder{Status: statusDelivered, CreatedAt: time.Now().AddDate(0, 0, -40)}
	if text, _, _ := orderAgeText(entregue); text != "" {
		t.Fatalf("OS entregue não tem idade a mostrar: %q", text)
	}
}

// TestRegistroDaCobranca: a data do contato fica gravada e o sino conta só o
// que passou do prazo.
func TestRegistroDaCobranca(t *testing.T) {
	store := openTestStore(t)
	ctx := context.Background()
	recente := ServiceOrder{Status: statusPending, Customer: "Ana", Phone: "(51) 98765-4321", Device: "Celular", Issue: "tela"}
	if err := store.Save(ctx, &recente); err != nil {
		t.Fatal(err)
	}
	antiga := ServiceOrder{Status: statusPending, Customer: "Bruno", Phone: "(51) 98765-4322", Device: "Celular", Issue: "tela"}
	if err := store.Save(ctx, &antiga); err != nil {
		t.Fatal(err)
	}
	velha := encodeTime(time.Now().UTC().AddDate(0, 0, -20))
	if _, err := store.db.ExecContext(ctx, "UPDATE service_orders SET created_at = ? WHERE id = ?", velha, antiga.ID); err != nil {
		t.Fatal(err)
	}

	if _, found, _ := store.OrderContactedAt(ctx, antiga.ID); found {
		t.Fatal("OS nova não pode nascer cobrada")
	}
	when := time.Now().UTC()
	if err := store.MarkOrderContacted(ctx, antiga.ID, when); err != nil {
		t.Fatal(err)
	}
	got, found, err := store.OrderContactedAt(ctx, antiga.ID)
	if err != nil || !found {
		t.Fatalf("a cobrança não ficou registrada: %v %v", found, err)
	}
	if got.Sub(when) > time.Second || when.Sub(got) > time.Second {
		t.Fatalf("data errada: %v contra %v", got, when)
	}

	stale, err := store.StaleOrderCount(ctx, staleOrderDays)
	if err != nil {
		t.Fatal(err)
	}
	if stale != 1 {
		t.Fatalf("só a OS de 20 dias está atrasada, o sino contou %d", stale)
	}
}

// TestAtalhoFuncionaDentroDoCampo: o Tab anda e o atalho do app não é engolido.
func TestAtalhoFuncionaDentroDoCampo(t *testing.T) {
	entry := newFormMultiLine()
	if entry.AcceptsTab() {
		t.Fatal("campo de várias linhas não pode engolir o Tab")
	}
	chamou := false
	globalShortcutHandler = func(fyne.Shortcut) bool { chamou = true; return true }
	defer func() { globalShortcutHandler = nil }()
	entry.TypedShortcut(&fyne.ShortcutCopy{})
	if !chamou {
		t.Fatal("o atalho não chegou ao app")
	}
}

// TestSenhaDoAparelho: o campo guarda a senha e ela nunca vai para o papel.
func TestSenhaDoAparelho(t *testing.T) {
	store := openTestStore(t)
	ctx := context.Background()
	order := ServiceOrder{
		Status: statusPending, Customer: "Ana", Phone: "(51) 98765-4321",
		Device: "Celular", Issue: "troca de tela", DeviceKey: "1234",
	}
	if err := store.Save(ctx, &order); err != nil {
		t.Fatal(err)
	}
	got, err := store.Get(ctx, order.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.DeviceKey != "1234" {
		t.Fatalf("a senha não foi guardada: %q", got.DeviceKey)
	}
	recibo := string(buildESCPOSReceipt(got))
	if strings.Contains(recibo, "1234") {
		t.Fatal("a senha do aparelho não pode sair no comprovante do cliente")
	}
}

// TestSugestoesDoHistorico: o campo oferece o que a loja já escreveu antes, sem
// carregar junto a senha que alguém digitou embaixo do defeito.
func TestSugestoesDoHistorico(t *testing.T) {
	store := openTestStore(t)
	ctx := context.Background()
	for i, issue := range []string{
		"troca de tela", "troca de tela", "troca de tela\n\nsenha 9988", "não carrega",
	} {
		order := ServiceOrder{
			Status: statusPending, Customer: fmt.Sprintf("Cliente %d", i), Phone: "(51) 98765-4321",
			Device: "Celular", Issue: issue,
		}
		if err := store.Save(ctx, &order); err != nil {
			t.Fatal(err)
		}
	}
	values, err := store.SuggestOrderValues(ctx, "issue", "tro", 4)
	if err != nil {
		t.Fatal(err)
	}
	if len(values) == 0 || values[0] != "troca de tela" {
		t.Fatalf("a sugestão mais usada deveria vir primeiro: %v", values)
	}
	for _, v := range values {
		if strings.Contains(v, "senha") || strings.Contains(v, "\n") {
			t.Fatalf("a sugestão não pode levar a senha de outro cliente: %q", v)
		}
	}
	if _, err := store.SuggestOrderValues(ctx, "notes", "x", 4); err == nil {
		t.Fatal("coluna fora da lista deveria ser recusada")
	}
}
