package main

import (
	"context"
	"testing"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/test"
)

// campoComMascara monta o campo como os formulários montam, já numa janela.
func campoComMascara(t *testing.T, mask func(string) string) *dbsEntry {
	t.Helper()
	e := newNumericEntry()
	attachMask(e, mask, nil)
	test.NewTempWindow(t, e)
	return e
}

func cursorNoFim(e *dbsEntry) bool { return e.CursorColumn == len([]rune(e.Text)) }

// TestDigitarSemOlharNaoEmbaralha é o relato do balcão: digitando o telefone
// sem olhar a tela, o cursor voltava para trás a cada separador que a máscara
// punha, e 51920040000 virava (19) 04000-0025. CPF e valor sofriam igual.
func TestDigitarSemOlharNaoEmbaralha(t *testing.T) {
	test.NewTempApp(t)
	casos := []struct {
		nome   string
		mask   func(string) string
		teclas string
		quer   string
	}{
		{"celular", maskPhone, "51920040000", "(51) 92004-0000"},
		{"fixo", maskPhone, "5133334444", "(51) 3333-4444"},
		{"CPF", maskCPF, "12345678900", "123.456.789-00"},
		{"CNPJ", maskCPF, "44886268000148", "44.886.268/0001-48"},
		{"valor", maskMoney, "15000", "R$ 150,00"},
		{"centavos", maskMoney, "05", "R$ 0,05"},
		{"milhar", maskMoney, "150000", "R$ 1.500,00"},
	}
	for _, caso := range casos {
		e := campoComMascara(t, caso.mask)
		test.Type(e, caso.teclas)
		if e.Text != caso.quer {
			t.Errorf("%s: digitando %s ficou %q, queria %q", caso.nome, caso.teclas, e.Text, caso.quer)
		}
		if !cursorNoFim(e) {
			t.Errorf("%s: o cursor parou na coluna %d de %q; a próxima tecla entraria no meio", caso.nome, e.CursorColumn, e.Text)
		}
	}
}

// TestTeclaAMaisNaoApagaOTelefone: com o número completo, um dígito apertado a
// mais era lido como um segundo telefone e levava o fim do primeiro junto.
func TestTeclaAMaisNaoApagaOTelefone(t *testing.T) {
	test.NewTempApp(t)
	casos := []struct{ teclas, quer string }{
		{"519200400000", "(51) 92004-0000"},
		// DDD 55 não é código do país dentro do próprio campo.
		{"559987654321", "(55) 99876-5432"},
	}
	for _, caso := range casos {
		e := campoComMascara(t, maskPhone)
		test.Type(e, caso.teclas)
		if e.Text != caso.quer {
			t.Errorf("digitando %s ficou %q, queria %q", caso.teclas, e.Text, caso.quer)
		}
	}
}

// TestCursorAcompanhaAEdicao: corrigir um dígito no meio também precisa deixar o
// cursor onde o operador está mexendo.
func TestCursorAcompanhaAEdicao(t *testing.T) {
	app := test.NewTempApp(t)

	fone := campoComMascara(t, maskPhone)
	fone.SetText("(51) 9876-5432")
	fone.CursorColumn = len("(51) 9")
	fone.Refresh()
	fone.TypedRune('9')
	if fone.Text != "(51) 99876-5432" || fone.CursorColumn != len("(51) 99") {
		t.Fatalf("inserir no meio: %q coluna %d", fone.Text, fone.CursorColumn)
	}

	// Apagar o hífen: a máscara devolve o hífen, e o cursor passa para antes
	// dele para o próximo Backspace apagar um dígito, em vez de travar ali.
	fone.SetText("(51) 98765-4321")
	fone.CursorColumn = len("(51) 98765-")
	fone.Refresh()
	fone.TypedKey(&fyne.KeyEvent{Name: fyne.KeyBackspace})
	if fone.Text != "(51) 98765-4321" || fone.CursorColumn != len("(51) 98765") {
		t.Fatalf("Backspace no hífen: %q coluna %d", fone.Text, fone.CursorColumn)
	}
	fone.TypedKey(&fyne.KeyEvent{Name: fyne.KeyBackspace})
	if fone.Text != "(51) 9876-4321" || fone.CursorColumn != len("(51) 9876") {
		t.Fatalf("segundo Backspace: %q coluna %d", fone.Text, fone.CursorColumn)
	}

	// Colado com o código do país, o cursor fica no fim do número.
	fone.SetText("")
	app.Clipboard().SetContent("+55 51 99999-8888")
	fone.TypedShortcut(&fyne.ShortcutPaste{Clipboard: app.Clipboard()})
	if fone.Text != "(51) 99999-8888" || !cursorNoFim(fone) {
		t.Fatalf("colar com +55: %q coluna %d", fone.Text, fone.CursorColumn)
	}

	// No valor os dígitos entram pela direita, e o cursor acompanha o digitado.
	valor := campoComMascara(t, maskMoney)
	valor.SetText("R$ 1,50")
	valor.CursorColumn = len("R$ 1")
	valor.Refresh()
	valor.TypedRune('2')
	if valor.Text != "R$ 12,50" || valor.CursorColumn != len("R$ 12") {
		t.Fatalf("valor no meio: %q coluna %d", valor.Text, valor.CursorColumn)
	}
}

// TestCampoPreenchidoPeloAppDigitaNoFim: o que o app escreve sozinho (padrão do
// termo, cliente reconhecido, OS aberta) deixava o cursor no começo, e quem
// chegava pelo Tab digitava na frente do texto.
func TestCampoPreenchidoPeloAppDigitaNoFim(t *testing.T) {
	test.NewTempApp(t)

	valor := campoComMascara(t, maskMoney)
	valor.SetText(formatBRL(0))
	test.Type(valor, "15000")
	if valor.Text != "R$ 150,00" {
		t.Errorf("valor padrão do termo: digitando 15000 ficou %q", valor.Text)
	}

	nome := newFormEntry()
	test.NewTempWindow(t, nome)
	nome.SetText("Maria")
	test.Type(nome, " Souza")
	if nome.Text != "Maria Souza" {
		t.Errorf("nome preenchido: ficou %q", nome.Text)
	}

	defeito := newFormMultiLine()
	test.NewTempWindow(t, defeito)
	defeito.SetText("não liga\nsem carregador")
	test.Type(defeito, ".")
	if defeito.Text != "não liga\nsem carregador." {
		t.Errorf("defeito preenchido: ficou %q", defeito.Text)
	}
}

// TestDigitarNaOSENoTermo passa pelos campos reais das duas telas do relato.
func TestDigitarNaOSENoTermo(t *testing.T) {
	app := test.NewTempApp(t)
	desktop := false
	forceMobileLayout = &desktop
	defer func() { forceMobileLayout = nil }()
	app.Settings().SetTheme(newDBSTheme(themeDark))

	store := openTestStore(t)
	admin, err := store.BootstrapAdministrator(context.Background(), "Douglas", "douglas", "senha-segura")
	if err != nil {
		t.Fatal(err)
	}
	win := test.NewTempWindow(t, nil)
	win.Resize(fyne.NewSize(1200, 760))
	ui := newMainUI(app, store, win, "x.db")
	ui.currentUser = admin
	ui.applyRoleAccess()
	win.SetContent(ui.content)

	confere := func(tela string, e *dbsEntry, teclas, quer string) {
		t.Helper()
		test.Type(e, teclas)
		if e.Text != quer {
			t.Errorf("%s: digitando %q ficou %q, queria %q", tela, teclas, e.Text, quer)
		}
	}

	ui.newOrder()
	confere("OS nome", ui.nameEntry, "João da Silva", "João da Silva")
	confere("OS telefone", ui.phoneEntry, "51920040000", "(51) 92004-0000")
	confere("OS contato", ui.contactPhoneEntry, "51999170194", "(51) 99917-0194")
	confere("OS CPF", ui.cpfEntry, "12345678900", "123.456.789-00")
	confere("OS valor", ui.amountEntry, "15000", "R$ 150,00")

	ui.markOrderFormSaved()
	ui.newTerm()
	confere("termo nome", ui.termSellerNameEntry, "João da Silva", "João da Silva")
	confere("termo telefone", ui.termSellerPhoneEntry, "51920040000", "(51) 92004-0000")
	confere("termo CPF", ui.termSellerCPFEntry, "12345678900", "123.456.789-00")
	confere("termo valor", ui.termAmountEntry, "15000", "R$ 150,00")
}
