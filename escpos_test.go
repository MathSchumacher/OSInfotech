package main

import (
	"bytes"
	"strings"
	"testing"
)

func TestBuildESCPOSReceiptCutsPaper(t *testing.T) {
	data := buildESCPOSReceipt(ServiceOrder{
		ID:          19,
		Status:      statusDelivered,
		Customer:    "Joao da Silva",
		Phone:       "(51) 99999-0000",
		Device:      "Celular",
		Issue:       "Troca de tela",
		AmountCents: 12990,
	})
	if !bytes.Contains(data, []byte("OS 000019")) {
		t.Fatalf("receipt does not contain the order number: %q", data)
	}
	if !bytes.HasSuffix(data, []byte{0x1b, 'd', 0x03, 0x1d, 'V', 0x42, 0}) {
		t.Fatalf("receipt does not finish with feed and cut: %v", data[len(data)-7:])
	}
}

func TestBuildESCPOSReceiptUsesCustomStoreIdentity(t *testing.T) {
	data := buildESCPOSReceiptWithSettings(ServiceOrder{ID: 7}, ThermalPrintSettings{
		Title:    "DBS Assistencia",
		Subtitle: "Portao - RS",
	})
	if !bytes.Contains(data, []byte("DBS Assistencia")) {
		t.Fatalf("receipt does not contain the configured title: %q", data)
	}
	if !bytes.Contains(data, []byte("Portao - RS")) {
		t.Fatalf("receipt does not contain the configured subtitle: %q", data)
	}
}

func TestESCPOSWrapAndTextSafety(t *testing.T) {
	lines := escposWrap("Assistencia tecnica com acao e coracao", 12)
	for _, line := range lines {
		if len(line) > 12 {
			t.Fatalf("line %q exceeds width", line)
		}
	}
	if got := escposSafeText("acao, coracao e Joao"); got != "acao, coracao e Joao" {
		t.Fatalf("escposSafeText() = %q", got)
	}
}

// TestComprovanteNaoImprimeAnotacao: o formulário sempre prometeu que a
// anotação da bancada não aparece para o cliente, e ela era impressa. Era ali
// que a loja guardava a senha do aparelho — 44 OS da base tinham o código de
// desbloqueio escrito nesse campo.
func TestComprovanteNaoImprimeAnotacao(t *testing.T) {
	order := ServiceOrder{
		ID: 142, Status: statusPending, Customer: "Maria Souza", Phone: "51999998888",
		Device: "Celular", BrandModel: "Galaxy A14", Serial: "123456",
		Issue: "troca de tela", Accessories: "capinha e película",
		Notes:       "TA COM CAPINHA SENHA: 155128",
		DeviceKey:   "155128",
		AmountCents: 35000,
	}
	recibo := string(buildESCPOSReceipt(order))
	for _, proibido := range []string{"155128", "SENHA", "TA COM CAPINHA"} {
		if strings.Contains(recibo, proibido) {
			t.Fatalf("o comprovante do cliente não pode conter %q:\n%s", proibido, recibo)
		}
	}
	// O que o cliente precisa levar por escrito continua saindo. O termico
	// imprime sem acento, entao "pelicula" e a forma esperada aqui.
	for _, esperado := range []string{"Maria Souza", "Galaxy A14", "troca de tela", "capinha e pelicula"} {
		if !strings.Contains(recibo, esperado) {
			t.Fatalf("o comprovante deveria conter %q", esperado)
		}
	}
}

// TestComprovanteMostraSinal: quem deixou entrada precisa levar por escrito
// quanto pagou e quanto falta.
func TestComprovanteMostraSinal(t *testing.T) {
	order := ServiceOrder{
		ID: 7, Customer: "Maria", Device: "Celular", Issue: "tela",
		AmountCents: 22000, PaidCents: 10000,
	}
	recibo := string(buildESCPOSReceipt(order))
	for _, esperado := range []string{"Ja pago", "R$ 100,00", "Falta", "R$ 120,00"} {
		if !strings.Contains(recibo, esperado) {
			t.Fatalf("o comprovante deveria conter %q:\n%s", esperado, recibo)
		}
	}
	// Sem entrada, o comprovante não inventa linha nenhuma.
	order.PaidCents = 0
	if strings.Contains(string(buildESCPOSReceipt(order)), "Ja pago") {
		t.Fatal("sem entrada não pode aparecer linha de pagamento")
	}
}
