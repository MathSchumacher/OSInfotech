package main

import (
	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/driver/mobile"
	"fyne.io/fyne/v2/widget"
)

// ---------------------------------------------------------------------------
// Campo de texto do app
//
// O campo do Fyne tem dois comportamentos que travavam o cadastro:
//
//  1. um campo de varias linhas engole o Tab e insere uma tabulacao, em vez de
//     pular para o proximo campo. Como "Defeito relatado" e de varias linhas e
//     obrigatorio, toda OS batia nessa parede e obrigava a voltar para o mouse;
//  2. o atalho vai primeiro para o widget focado, e o campo descarta em silencio
//     o que nao conhece. Ou seja, Ctrl+S, Ctrl+N e Ctrl+K ficavam mortos
//     exatamente enquanto alguem digitava, que e quando serviriam.
//
// Este campo desliga as duas coisas. O Tab sempre anda, e o atalho que o app
// registrou e executado mesmo com o cursor dentro do campo; copiar, colar e
// recortar continuam indo para o proprio campo.
// ---------------------------------------------------------------------------

// globalShortcutHandler executa os atalhos do app. Devolve true quando o atalho
// era dele. E preenchido por installShortcuts.
var globalShortcutHandler func(fyne.Shortcut) bool

type dbsEntry struct {
	widget.Entry
	// keyboard escolhe o teclado que o Android abre. Sem isso, CPF, telefone e
	// valor abriam o teclado de letras e o operador trocava de aba em todo campo.
	keyboard mobile.KeyboardType
}

// Keyboard informa ao Android qual teclado abrir para este campo.
func (e *dbsEntry) Keyboard() mobile.KeyboardType {
	if e.keyboard != mobile.DefaultKeyboard {
		return e.keyboard
	}
	return e.Entry.Keyboard()
}

// AcceptsTab mantem o Tab navegando entre campos, inclusive nos de varias
// linhas. Ninguem precisa digitar tabulacao dentro de um defeito de OS.
func (e *dbsEntry) AcceptsTab() bool { return false }

func (e *dbsEntry) TypedShortcut(shortcut fyne.Shortcut) {
	if globalShortcutHandler != nil && globalShortcutHandler(shortcut) {
		return
	}
	e.Entry.TypedShortcut(shortcut)
}

// SetText troca o texto e deixa o cursor no fim dele.
//
// O Fyne mantem o cursor na coluna em que estava. Num campo que o app preenche
// sozinho (cliente reconhecido, OS aberta, valores padrao do termo) ele ficava
// no comeco, e quem entrava no campo pelo Tab digitava na frente do que ja
// estava escrito: 15000 no "R$ 0,00" do termo virava R$ 150.000,00.
func (e *dbsEntry) SetText(text string) {
	if text != e.Text {
		// Uma posicao alem do fim: o proprio SetText traz o cursor de volta para
		// o ultimo caractere, inclusive quando o texto quebra em varias linhas.
		beyond := len([]rune(text)) + 1
		e.CursorRow, e.CursorColumn = beyond, beyond
	}
	e.Entry.SetText(text)
}

func newFormEntry() *dbsEntry {
	e := &dbsEntry{}
	e.ExtendBaseWidget(e)
	return e
}

func newFormMultiLine() *dbsEntry {
	e := &dbsEntry{}
	e.MultiLine = true
	e.Wrapping = fyne.TextWrapWord
	e.ExtendBaseWidget(e)
	return e
}

// newNumericEntry e o campo que so recebe numero: CPF, telefone, valor.
func newNumericEntry() *dbsEntry {
	e := &dbsEntry{keyboard: mobile.NumberKeyboard}
	e.ExtendBaseWidget(e)
	return e
}

func newFormPassword() *dbsEntry {
	e := &dbsEntry{}
	e.Password = true
	e.ExtendBaseWidget(e)
	return e
}
