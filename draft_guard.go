package main

import (
	"strings"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/dialog"
)

// ---------------------------------------------------------------------------
// Protecao do cadastro pela metade
//
// Nao havia rascunho, aviso nem desfazer: qualquer clique na barra lateral com
// o cliente na frente do balcao apagava a OS inteira que estava sendo digitada.
// Aqui a saida so acontece depois de o operador decidir o que fazer.
// ---------------------------------------------------------------------------

// orderFormSignature resume o que esta digitado no formulario da OS.
func (ui *mainUI) orderFormSignature() string {
	if ui.nameEntry == nil || ui.statusSelect == nil {
		return ""
	}
	parts := []string{
		ui.nameEntry.Text, ui.phoneEntry.Text, ui.cpfEntry.Text,
		ui.deviceEntry.Text, ui.modelEntry.Text, ui.serialEntry.Text,
		ui.deviceKeyEntry.Text, ui.issueEntry.Text, ui.accessEntry.Text,
		ui.notesEntry.Text, ui.amountEntry.Text, ui.costEntry.Text,
		ui.statusSelect.Selected,
	}
	return strings.Join(parts, "\x1f")
}

// markOrderFormSaved guarda o estado atual como "sem alteracoes".
func (ui *mainUI) markOrderFormSaved() {
	ui.orderFormBaseline = ui.orderFormSignature()
}

// orderFormDirty diz se ha texto digitado que ainda nao foi gravado.
func (ui *mainUI) orderFormDirty() bool {
	if ui.nameEntry == nil {
		return false
	}
	current := ui.orderFormSignature()
	if current == ui.orderFormBaseline {
		return false
	}
	// Formulario novo ainda vazio nao conta como perdido.
	return strings.TrimSpace(strings.ReplaceAll(current, "\x1f", "")) != strings.TrimSpace(statusPending)
}

// guardedPages sao as telas de navegacao: sair para uma delas com a OS pela
// metade pede confirmacao. Fluxos internos do proprio editor passam direto.
var guardedPages = map[pageKind]bool{
	pageHome:      true,
	pageOrders:    true,
	pageCustomers: true,
	pageTerms:     true,
	pageReports:   true,
	pageSettings:  true,
	pageSearch:    true,
}

// confirmLeaveOrder pergunta o que fazer antes de abandonar a OS digitada.
func (ui *mainUI) confirmLeaveOrder(next pageKind) {
	message := "Esta OS tem informação digitada que ainda não foi salva. O que você quer fazer?"
	var leaveDialog *dialog.CustomDialog
	save := primaryButton("Salvar e sair", nil, func() {
		leaveDialog.Hide()
		if ui.persistOrder() {
			ui.markOrderFormSaved()
			ui.showPage(next)
		}
	})
	discard := dangerButton("Sair sem salvar", nil, func() {
		leaveDialog.Hide()
		ui.markOrderFormSaved()
		ui.showPage(next)
	})
	content := paddedTRBL(mutedLabel(message), 0, 4, 8, 4)
	leaveDialog = dialog.NewCustom("OS não salva", "Continuar editando", content, ui.window)
	leaveDialog.Resize(fyne.NewSize(460, 210))
	leaveDialog.SetButtons([]fyne.CanvasObject{discard, save})
	leaveDialog.Show()
}

// scheduleOrderSearch espera o operador parar de digitar antes de refazer a
// lista. Sem isso e uma varredura completa por tecla.
func (ui *mainUI) scheduleOrderSearch() {
	seq := ui.orderSearchSeq.Add(1)
	time.AfterFunc(searchDebounce, func() {
		if seq != ui.orderSearchSeq.Load() {
			return
		}
		fyne.Do(func() {
			if seq != ui.orderSearchSeq.Load() {
				return
			}
			ui.refreshList()
		})
	})
}

// searchDebounce e curto o bastante para parecer instantaneo e longo o bastante
// para pular as teclas do meio de uma palavra.
const searchDebounce = 180 * time.Millisecond
