package main

import (
	"context"
	"testing"
)

// TestSenhaSaiDaObservacao roda sobre as anotações reais da loja: são elas que
// decidem se a extração serve ou mutila o trabalho de alguém.
func TestSenhaSaiDaObservacao(t *testing.T) {
	casos := []struct {
		nota    string
		senha   string
		sobra   string
		extraiu bool
	}{
		{"SENHA: 200508 SENHA: 091179", "200508 / 091179", "", true},
		{"TA COM CAPINHA SENHA: 155128", "155128", "TA COM CAPINHA", true},
		{"ORÇAR, SENHA 1307, JA FOI ABERTO PARA BOTAR BATERIA", "1307", "ORÇAR, JA FOI ABERTO PARA BOTAR BATERIA", true},
		{"senha 2580", "2580", "", true},
		{"SENHA: 1973555", "1973555", "", true},
		// A mesma senha escrita duas vezes não vira duas.
		{"senha 4321 confirmada, senha 4321", "4321", "confirmada", true},
		// Nada seguro a mover:
		{"sem senha", "", "sem senha", false},
		{"cliente não tem senha no aparelho", "", "cliente não tem senha no aparelho", false},
		{"PRA SEGUNDA, SENHA: L", "", "PRA SEGUNDA, SENHA: L", false},
		{"senha é o padrão de desenho", "", "senha é o padrão de desenho", false},
		{"trocar tela e bateria", "", "trocar tela e bateria", false},
		{"", "", "", false},
		// Senha alfanumérica não pode ser cortada pela metade: sem certeza,
		// a OS vai para a revisão humana com a anotação intacta.
		{"pelicula brinde, senha 140516a", "", "pelicula brinde, senha 140516a", false},
		{"senha; 041104", "041104", "", true},
	}
	for _, caso := range casos {
		senha, sobra, ok := extractDevicePassword(caso.nota)
		if ok != caso.extraiu || senha != caso.senha || sobra != caso.sobra {
			t.Errorf("%q\n  deu   senha=%q sobra=%q ok=%v\n  queria senha=%q sobra=%q ok=%v",
				caso.nota, senha, sobra, ok, caso.senha, caso.sobra, caso.extraiu)
		}
	}
}

// TestObservacaoQuePedeOlhoHumano: o que não sai sozinho não pode sumir da
// vista, senão a senha continua sendo impressa e ninguém sabe.
func TestObservacaoQuePedeOlhoHumano(t *testing.T) {
	precisa := []string{"PRA SEGUNDA, SENHA: L", "senha é o padrão de desenho", "desbloqueio por digital e senha"}
	naoPrecisa := []string{"sem senha", "não tem senha", "trocar tela", ""}
	for _, nota := range precisa {
		if !mentionsDevicePassword(nota) {
			t.Errorf("%q deveria pedir revisão", nota)
		}
	}
	for _, nota := range naoPrecisa {
		if mentionsDevicePassword(nota) {
			t.Errorf("%q não deveria pedir revisão", nota)
		}
	}
}

// TestMudancaNoBanco: a senha sai da observação e entra no campo que não é
// impresso, e a OS que já tem senha própria não é tocada.
func TestMudancaNoBanco(t *testing.T) {
	store := openTestStore(t)
	ctx := context.Background()

	vazando := ServiceOrder{
		Status: statusPending, Customer: "Cliente A", Phone: "51999998888",
		Device: "Celular", Issue: "tela", Notes: "TA COM CAPINHA SENHA: 155128",
	}
	jaTemCampo := ServiceOrder{
		Status: statusPending, Customer: "Cliente B", Phone: "51999997777",
		Device: "Celular", Issue: "bateria", Notes: "senha 4321", DeviceKey: "9999",
	}
	revisar := ServiceOrder{
		Status: statusPending, Customer: "Cliente C", Phone: "51999996666",
		Device: "Celular", Issue: "não liga", Notes: "PRA SEGUNDA, SENHA: L",
	}
	for _, o := range []*ServiceOrder{&vazando, &jaTemCampo, &revisar} {
		if err := store.Save(ctx, o); err != nil {
			t.Fatal(err)
		}
	}

	moved, err := store.MoveDevicePasswordsFromNotes(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if moved != 1 {
		t.Fatalf("deveria corrigir só a que vazava: %d", moved)
	}

	corrigida := mustGetOrder(t, store, vazando.ID)
	if corrigida.DeviceKey != "155128" {
		t.Fatalf("a senha não foi para o campo próprio: %q", corrigida.DeviceKey)
	}
	if corrigida.Notes != "TA COM CAPINHA" {
		t.Fatalf("o resto da anotação se perdeu: %q", corrigida.Notes)
	}

	intacta := mustGetOrder(t, store, jaTemCampo.ID)
	if intacta.DeviceKey != "9999" || intacta.Notes != "senha 4321" {
		t.Fatalf("OS com senha já preenchida não pode ser tocada: %+v", intacta)
	}

	// Rodar de novo não pode mexer em mais nada.
	if again, err := store.MoveDevicePasswordsFromNotes(ctx); err != nil || again != 0 {
		t.Fatalf("a passagem tem que ser idempotente: %d %v", again, err)
	}

	pendentes, err := store.DevicePasswordsNeedingReview(ctx)
	if err != nil {
		t.Fatal(err)
	}
	// A de senha já preenchida ainda cita senha na observação, então também
	// aparece para revisão — junto com a do padrão de desenho.
	ids := map[int64]bool{}
	for _, o := range pendentes {
		ids[o.ID] = true
	}
	if !ids[revisar.ID] {
		t.Fatal("a OS com padrão de desenho tem que aparecer para revisão")
	}
	if ids[corrigida.ID] {
		t.Fatal("a OS já corrigida não pode continuar na lista de revisão")
	}
}
