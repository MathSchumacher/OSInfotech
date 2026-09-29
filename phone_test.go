package main

import "testing"

// TestMascaraNaoInventaTelefone é o caso que motivou a correção: cortar em 11
// dígitos sem olhar onde eles estão junta o fim de um telefone com o começo de
// outro e produz um número que parece válido e não é de ninguém.
func TestMascaraNaoInventaTelefone(t *testing.T) {
	casos := []struct{ entrada, quer string }{
		// Dois telefones no mesmo campo: fica o primeiro inteiro.
		{"5198104717 ou  51997077640", "(51) 9810-4717"},
		{"51 995379117     E 997067628", "(51) 99537-9117"},
		// Digitação normal, com os separadores que a própria máscara devolve.
		{"51999170194", "(51) 99917-0194"},
		{"(51) 99917-0194", "(51) 99917-0194"},
		{"51 99917-0194", "(51) 99917-0194"},
		{"5199917019", "(51) 9991-7019"},
		// Colado com código do país.
		{"+55 51 98765-4321", "(51) 98765-4321"},
		{"5551987654321", "(51) 98765-4321"},
		// Uma tecla a mais no campo já formatado: fica o número, sai o excedente.
		{"(51) 92004-00000", "(51) 92004-0000"},
		{"(55) 99876-54321", "(55) 99876-5432"},
		// Colar por cima de um número pela metade continua sem inventar outro.
		{"(51) 995351997067628", "(51"},
		// Lixo e vazio.
		{"", ""},
		{"abc", ""},
	}
	for _, caso := range casos {
		if got := maskPhone(caso.entrada); got != caso.quer {
			t.Errorf("maskPhone(%q) = %q, queria %q", caso.entrada, got, caso.quer)
		}
	}
}

// TestNumeroInventadoNaoChegaNoWhatsApp: o desfecho que importa é para onde a
// mensagem iria.
func TestNumeroInventadoNaoChegaNoWhatsApp(t *testing.T) {
	// Antes: "5198104717 ou 51997077640" virava 5551981047175, de um terceiro.
	got := whatsappNumber(maskPhone("5198104717 ou  51997077640"))
	if got != "555198104717" {
		t.Fatalf("a mensagem iria para %q, e o primeiro telefone do cliente é 5198104717", got)
	}
}

// TestAvisoDeTelefoneSobrando: guardar o primeiro número em silêncio esconde
// que havia um segundo.
func TestAvisoDeTelefoneSobrando(t *testing.T) {
	comSobra := []string{"5198104717 ou  51997077640", "51 995379117     E 997067628"}
	semSobra := []string{"51 99917-0194", "(51) 99917-0194", "51999170194", "+55 51 98765-4321", ""}
	for _, entrada := range comSobra {
		if !phoneHasExtraNumber(entrada) {
			t.Errorf("%q deveria avisar que sobrou telefone", entrada)
		}
	}
	for _, entrada := range semSobra {
		if phoneHasExtraNumber(entrada) {
			t.Errorf("%q não pode disparar aviso", entrada)
		}
	}
}

// TestAvisoDoTelefoneNoCadastro: o problema tem que aparecer na hora de
// cadastrar, com o cliente na frente do balcão, e não semanas depois quando
// alguém tenta cobrar.
func TestAvisoDoTelefoneNoCadastro(t *testing.T) {
	casos := []struct {
		entrada string
		avisa   bool
		motivo  string
	}{
		{"(51) 99917-0194", false, "número bom"},
		{"", false, "campo vazio ainda não é problema"},
		{"(51) 9991", true, "incompleto"},
		{"000000", true, "preenchimento"},
		{"(00) 0000-0000", true, "preenchimento com máscara"},
		{"5198104717 ou 51997077640", true, "dois telefones"},
	}
	for _, caso := range casos {
		texto, _ := phoneWarningFor(caso.entrada)
		if (texto != "") != caso.avisa {
			t.Errorf("%q (%s): aviso=%q, esperava avisar=%v", caso.entrada, caso.motivo, texto, caso.avisa)
		}
	}
}

// TestPreenchimentoNaoEContato: 14 OS da base têm "000000" como telefone.
func TestPreenchimentoNaoEContato(t *testing.T) {
	for _, filler := range []string{"000000", "0000000000", "99999999999", "111111"} {
		if !isFillerPhone(filler) {
			t.Errorf("%q deveria ser reconhecido como preenchimento", filler)
		}
	}
	for _, real := range []string{"51999170194", "5133334444", "12345"} {
		if isFillerPhone(real) {
			t.Errorf("%q não é preenchimento", real)
		}
	}
}

// TestContatoAlternativoDestravaACobranca: as 23 OS sem telefone utilizável
// estão justamente na fila que espera resposta. O segundo contato existe para
// destravar essas.
func TestContatoAlternativoDestravaACobranca(t *testing.T) {
	semTelefone := ServiceOrder{
		ID: 1, Status: statusPending, Phone: "000000", AmountCents: 20000,
	}
	if orderCanBeReached(semTelefone) || canFollowUp(semTelefone) {
		t.Fatal("sem telefone nem contato não há como cobrar")
	}
	if nota, tone := orderListNote(semTelefone); tone != toneDanger || nota != "sem como avisar" {
		t.Fatalf("a lista deveria dizer que não há como avisar: %q %v", nota, tone)
	}

	comContato := semTelefone
	comContato.ContactName = "Bruna (mãe)"
	comContato.ContactPhone = "(51) 99992-0119"
	if !orderCanBeReached(comContato) || !canFollowUp(comContato) {
		t.Fatal("com contato alternativo dá para cobrar")
	}
	if got := orderReachPhone(comContato); got != "5551999920119" {
		t.Fatalf("a mensagem deveria ir para o contato alternativo, e vai para %q", got)
	}

	// O telefone do cliente, quando serve, continua tendo preferência.
	comOsDois := comContato
	comOsDois.Phone = "(51) 99917-0194"
	if got := orderReachPhone(comOsDois); got != "5551999170194" {
		t.Fatalf("o telefone do cliente vem primeiro, e veio %q", got)
	}
}
