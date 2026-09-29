package main

import (
	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/theme"
)

// ---------------------------------------------------------------------------
// O telefone e o unico canal com o cliente
//
// Rodando a regra do proprio app contra a base: 23 das 220 OS nao conseguem
// receber WhatsApp. Catorze tem telefone de preenchimento ("000000"), e as
// outras estao incompletas ou misturadas.
//
// Isso trava justamente a fila de quem ja foi orcado e espera resposta: nao
// adianta cobrar quem nao tem como ser avisado. O problema so aparecia semanas
// depois, quando alguem tentava cobrar; agora aparece no cadastro, que e onde
// da para pedir outro numero ao cliente que esta na frente do balcao.
// ---------------------------------------------------------------------------

// refreshPhoneWarning avisa, no cadastro, quando o numero nao serve.
func (ui *mainUI) refreshPhoneWarning() {
	if ui.phoneWarning == nil || ui.phoneEntry == nil {
		return
	}
	texto, tone := phoneWarningFor(ui.phoneEntry.Text)
	if texto == "" {
		ui.phoneWarning.Hide()
		ui.phoneWarning.Refresh()
		return
	}
	label := newText(texto, sizeCaption, false, inkForTone(tone))
	icon := newToneIcon(theme.WarningIcon(), tone, 16)
	body := container.NewBorder(nil, nil, container.NewCenter(icon), nil, paddedTRBL(label, 0, 6, 0, 8))
	ui.phoneWarning.Objects = []fyne.CanvasObject{
		gap(1, 4),
		newGlassPanel(paddedTRBL(body, 6, 8, 6, 8), 10, tintGlass(tone)),
		gap(1, 6),
	}
	ui.phoneWarning.Show()
	ui.phoneWarning.Refresh()
}

// phoneWarningFor devolve o aviso do telefone digitado, e o tom dele. Texto
// vazio quer dizer numero utilizavel.
func phoneWarningFor(text string) (string, statusTone) {
	digits := onlyDigits(text)
	if digits == "" {
		return "", toneNeutral
	}
	if phoneHasExtraNumber(text) {
		return "Havia mais de um telefone aqui. Guardei o primeiro — use \"Telefone do contato\" para o outro.", toneWarning
	}
	if len(digits) < 10 {
		return "Número incompleto: faltam o DDD ou dígitos. Assim não dá para avisar o cliente depois.", toneWarning
	}
	if whatsappNumber(text) == "" {
		return "Este número não recebe WhatsApp. Sem ele, cobrar a aprovação depois vira ligação.", toneDanger
	}
	if isFillerPhone(digits) {
		return "Número de preenchimento. O cliente fica sem como ser avisado.", toneDanger
	}
	return "", toneNeutral
}

// isFillerPhone reconhece o numero digitado so para preencher o campo, que a
// base tem aos montes: 000000, 999999999, 111111111.
func isFillerPhone(digits string) bool {
	if len(digits) < 6 {
		return false
	}
	first := digits[0]
	for i := 0; i < len(digits); i++ {
		if digits[i] != first {
			return false
		}
	}
	return true
}

// orderReachPhone devolve o numero que serve para falar com esta OS: o do
// cliente, ou o do contato alternativo quando o primeiro nao serve.
func orderReachPhone(order ServiceOrder) string {
	if number := whatsappNumber(order.Phone); number != "" {
		return number
	}
	return whatsappNumber(order.ContactPhone)
}

// orderCanBeReached diz se ha como avisar o cliente desta OS.
func orderCanBeReached(order ServiceOrder) bool { return orderReachPhone(order) != "" }
