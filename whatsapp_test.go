package main

import (
	"context"
	"strings"
	"testing"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/test"
)

// TestNumeroParaWhatsApp: o número tem que sair em E.164 sem o mais, e número
// incompleto não pode virar envio.
func TestNumeroParaWhatsApp(t *testing.T) {
	casos := map[string]string{
		"(51) 98765-4321": "5551987654321",
		"51 98765-4321":   "5551987654321",
		"5551987654321":   "5551987654321",
		"+55 51 98765432": "555198765432",
		"98765-4321":      "",
		"123":             "",
		"":                "",
		"55 11 3333-4444": "551133334444",
	}
	for entrada, esperado := range casos {
		if got := whatsappNumber(entrada); got != esperado {
			t.Fatalf("%q virou %q, esperava %q", entrada, got, esperado)
		}
	}
}

// TestMensagemDePronto: o cliente precisa saber qual aparelho, qual OS e quanto
// custa, sem abrir nada.
func TestMensagemDePronto(t *testing.T) {
	order := ServiceOrder{
		ID: 142, Status: statusReady, Customer: "maria souza", Phone: "(51) 98765-4321",
		Device: "Celular", BrandModel: "Samsung Galaxy A14", AmountCents: 35000,
	}
	msg := readyMessage(order)
	for _, want := range []string{"Maria", "000142", "Galaxy A14", "R$ 350,00", "retirado"} {
		if !strings.Contains(msg, want) {
			t.Fatalf("a mensagem não cita %q:\n%s", want, msg)
		}
	}
	// Sem valor lançado não inventa preço.
	order.AmountCents = 0
	if strings.Contains(readyMessage(order), "R$") {
		t.Fatal("sem valor a mensagem não pode falar em preço")
	}
	// E o status pronto usa essa mensagem no fluxo de cobrança também.
	if !strings.Contains(followUpMessage(order), "retirado") {
		t.Fatal("status pronto deveria usar a mensagem de retirada")
	}
}

// TestHorarioComercial: madrugada e domingo não são hora de mandar mensagem.
func TestHorarioComercial(t *testing.T) {
	quarta := time.Date(2026, 9, 23, 10, 0, 0, 0, time.Local)
	if !businessHours(quarta) {
		t.Fatal("quarta às 10h é horário comercial")
	}
	madrugada := time.Date(2026, 9, 23, 3, 0, 0, 0, time.Local)
	if businessHours(madrugada) {
		t.Fatal("3h da manhã não é horário comercial")
	}
	domingo := time.Date(2026, 9, 27, 10, 0, 0, 0, time.Local)
	if businessHours(domingo) {
		t.Fatal("domingo não é horário comercial")
	}
}

// TestTetoDiario: o registro alimenta o limite que protege o número.
func TestTetoDiario(t *testing.T) {
	store := openTestStore(t)
	ctx := context.Background()
	if count, err := store.WhatsAppSentToday(ctx); err != nil || count != 0 {
		t.Fatalf("base nova deveria ter 0 envios: %d %v", count, err)
	}
	for i := 0; i < 3; i++ {
		if err := store.LogWhatsApp(ctx, int64(i+1), "5551987654321", "pronto"); err != nil {
			t.Fatal(err)
		}
	}
	count, err := store.WhatsAppSentToday(ctx)
	if err != nil || count != 3 {
		t.Fatalf("deveria contar 3 envios de hoje: %d %v", count, err)
	}
	when, found, err := store.WhatsAppLastSent(ctx, 2)
	if err != nil || !found || time.Since(when) > time.Minute {
		t.Fatalf("o envio da OS 2 deveria estar registrado: %v %v %v", when, found, err)
	}
	if _, found, _ := store.WhatsAppLastSent(ctx, 99); found {
		t.Fatal("OS sem envio não pode aparecer no registro")
	}
}

// TestArgumentosDaPonte: o modo ponte é o mesmo executável com uma flag, então
// a leitura dos argumentos não pode depender do flag global do Fyne.
func TestArgumentosDaPonte(t *testing.T) {
	args, found := parseBridgeArgs([]string{"--whatsapp-bridge", "--wa-store", `c:\dados\wa.db`, "--wa-addr", "127.0.0.1:9000", "--wa-token", "segredo"})
	if !found || args.Store != `c:\dados\wa.db` || args.Addr != "127.0.0.1:9000" || args.Token != "segredo" {
		t.Fatalf("argumentos lidos errado: %+v %v", args, found)
	}
	if _, found := parseBridgeArgs([]string{"-alguma-coisa"}); found {
		t.Fatal("execução normal do app não pode virar modo ponte")
	}
}

// TestQRDesenhado: o código precisa sair claro, com borda, senão o celular não lê.
func TestQRDesenhado(t *testing.T) {
	img := renderQRCode("2@abc123,def456,ghi789", 220)
	if img == nil {
		t.Fatal("não desenhou o QR")
	}
	bounds := img.Bounds()
	if bounds.Dx() < 100 || bounds.Dx() != bounds.Dy() {
		t.Fatalf("tamanho estranho: %v", bounds)
	}
	// O canto é zona de silêncio: tem que ser branco.
	r, g, b, _ := img.At(2, 2).RGBA()
	if r < 60000 || g < 60000 || b < 60000 {
		t.Fatalf("a borda do QR não está branca: %d %d %d", r, g, b)
	}
}

// collectButtonLabels desce a arvore de objetos juntando o texto dos botoes.
func collectButtonLabels(t *testing.T, obj fyne.CanvasObject, out *[]string) {
	if b, ok := obj.(*dbsButton); ok {
		*out = append(*out, b.text)
		return
	}
	if c, ok := obj.(*fyne.Container); ok {
		for _, child := range c.Objects {
			collectButtonLabels(t, child, out)
		}
		return
	}
	if w, ok := obj.(fyne.Widget); ok {
		for _, child := range test.TempWidgetRenderer(t, w).Objects() {
			collectButtonLabels(t, child, out)
		}
	}
}

// TestAvisoDeProntoTemSaida: a caixa que oferece avisar o cliente precisa ter
// como sair. O SetButtons do Fyne troca o botao de dispensar, entao sem um
// "Agora não" proprio o atendente fica preso na caixa.
func TestAvisoDeProntoTemSaida(t *testing.T) {
	app := test.NewApp()
	defer app.Quit()
	desktop := false
	forceMobileLayout = &desktop
	defer func() { forceMobileLayout = nil }()

	store := openTestStore(t)
	ctx := context.Background()
	admin, err := store.BootstrapAdministrator(ctx, "Douglas Silva", "douglas", "senha-segura")
	if err != nil {
		t.Fatal(err)
	}
	order := ServiceOrder{
		Status: statusReady, Customer: "Maria Souza", Phone: "(51) 99888-7766",
		Device: "Celular", BrandModel: "Samsung Galaxy A14", AmountCents: 35000,
	}
	if err := store.Save(ctx, &order); err != nil {
		t.Fatal(err)
	}

	win := test.NewWindow(nil)
	defer win.Close()
	win.Resize(fyne.NewSize(1200, 800))
	ui := newMainUI(app, store, win, "x.db")
	ui.currentUser = admin
	ui.applyRoleAccess()
	win.SetContent(ui.content)

	ui.offerReadyNotice(order)
	overlay := win.Canvas().Overlays().Top()
	if overlay == nil {
		t.Fatal("a OS pronta com telefone deveria oferecer o aviso")
	}
	var labels []string
	collectButtonLabels(t, overlay, &labels)
	joined := strings.Join(labels, " | ")
	if !strings.Contains(joined, "Agora não") {
		t.Fatalf("a caixa precisa de um jeito de sair, botões: %s", joined)
	}
	if !strings.Contains(joined, "Abrir o WhatsApp") {
		t.Fatalf("faltou a saída pelo WhatsApp do celular, botões: %s", joined)
	}
	win.Canvas().Overlays().Remove(overlay)

	// Sem telefone com DDD nao ha o que oferecer: avisa e nao abre caixa.
	semFone := order
	semFone.Phone = "1234"
	ui.offerReadyNotice(semFone)
	if win.Canvas().Overlays().Top() != nil {
		t.Fatal("OS sem telefone não pode abrir a caixa de aviso")
	}
}
