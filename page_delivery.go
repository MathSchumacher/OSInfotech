package main

import (
	"context"
	"fmt"
	"strings"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
)

// ---------------------------------------------------------------------------
// Fechamento da OS
//
// "Entregue" respondia uma pergunta e deixava duas sem resposta: quanto custou
// a peca, e se o cliente pagou.
//
// O custo existe em 33 das 96 OS entregues, e em setembro, em nenhuma. Mesmo
// assim o painel estampava "Lucro liquido" com bruto menos esse quase-nada, o
// que erra sempre para mais, justo no numero que serve para formar preco. E
// pagamento nao existia em coluna nenhuma: entregue nao queria dizer pago, e
// nao havia como saber quem levou o aparelho sem pagar.
//
// As duas perguntas saem aqui, no unico momento em que as respostas sao
// conhecidas: na hora de entregar. Enquanto o custo foi um campo opcional no
// meio do cadastro de entrada, quando a peca nem tinha sido comprada, ele
// simplesmente nao era preenchido.
// ---------------------------------------------------------------------------

// paymentPending e a forma de pagamento que significa "levou e nao pagou".
const paymentPending = "Ficou devendo"

var paymentMethods = []string{"Dinheiro", "Pix", "Débito", "Crédito", paymentPending}

// offerDeliveryClose pergunta custo e pagamento logo depois de a OS virar
// entregue. Vem depois da gravacao, e nao antes, para o operador nunca ficar
// preso: fechar a caixa deixa a OS entregue do mesmo jeito, so que sem os dois
// numeros.
func (ui *mainUI) offerDeliveryClose(order ServiceOrder) {
	costEntry := newNumericEntry()
	costEntry.SetPlaceHolder("R$ 0,00")
	attachMask(costEntry, maskMoney, nil)
	if order.CostCents > 0 {
		costEntry.SetText(formatBRL(order.CostCents))
	}

	// Quanto o cliente entregou agora. Vem preenchido com o que falta, que e o
	// caso comum; mexer nele e o que registra sinal e pagamento parcial, hoje
	// escritos em texto livre ("100 de entrada", "foi pago 100 dos 220").
	faltando := order.AmountCents - order.PaidCents
	if faltando < 0 {
		faltando = 0
	}
	paidEntry := newNumericEntry()
	paidEntry.SetPlaceHolder("R$ 0,00")
	attachMask(paidEntry, maskMoney, nil)
	if faltando > 0 {
		paidEntry.SetText(formatBRL(faltando))
	}

	methods := newChipGroupWrapped(nil, fyne.NewSize(132, 34))
	for _, method := range paymentMethods {
		methods.Add(method, method)
	}
	methods.Select(firstNonEmpty(order.PaymentMethod, paymentMethods[0]))
	// "Ficou devendo" e o mesmo que receber zero: uma coisa so para o operador.
	methods.OnChanged = func(value string) {
		if value == paymentPending {
			paidEntry.SetText("")
			return
		}
		if strings.TrimSpace(paidEntry.Text) == "" && faltando > 0 {
			paidEntry.SetText(formatBRL(faltando))
		}
	}

	body := container.NewVBox(
		mutedLabel(fmt.Sprintf("Valor cobrado do cliente: %s", formatBRL(order.AmountCents))),
		gap(1, 10),
	)
	// O tecnico nao ve custo, pela mesma razao de o campo do formulario ser so
	// do administrador.
	if ui.currentUser.IsAdmin() {
		body.Add(newText("Quanto custou para a loja?", sizeBody, true, inkPrimary))
		body.Add(newText("Peças e serviço de terceiros. É o que falta para o resultado do mês ser real.", sizeCaption, false, inkMuted))
		body.Add(gap(1, 6))
		body.Add(costEntry)
		body.Add(gap(1, 14))
	}
	body.Add(newText("Como o cliente pagou?", sizeBody, true, inkPrimary))
	body.Add(gap(1, 6))
	body.Add(methods.Object())
	body.Add(gap(1, 12))
	body.Add(newText("Quanto recebeu agora?", sizeBody, true, inkPrimary))
	if order.PaidCents > 0 {
		body.Add(newText(fmt.Sprintf("Já havia %s pago nesta OS.", formatBRL(order.PaidCents)), sizeCaption, false, inkMuted))
	} else {
		body.Add(newText("Menos que o total registra entrada ou pagamento parcial.", sizeCaption, false, inkMuted))
	}
	body.Add(gap(1, 6))
	body.Add(paidEntry)

	var closeDialog *dialog.CustomDialog
	confirm := primaryButton("Confirmar entrega", theme.ConfirmIcon(), func() {
		closeDialog.Hide()
		ui.applyDeliveryClose(order, costEntry.Text, methods.value, paidEntry.Text)
	})
	later := ghostButton("Depois", nil, func() { closeDialog.Hide() })

	closeDialog = dialog.NewCustom(fmt.Sprintf("Fechar a OS %06d", order.ID), "", body, ui.window)
	closeDialog.Resize(fyne.NewSize(540, 430))
	closeDialog.SetButtons([]fyne.CanvasObject{later, confirm})
	closeDialog.Show()
}

// applyDeliveryClose grava o custo e o pagamento da entrega.
func (ui *mainUI) applyDeliveryClose(order ServiceOrder, costText string, method string, paidText string) {
	if ui.currentUser.IsAdmin() {
		cost, err := parseBRL(costText)
		if err != nil {
			ui.notifyTone("Custo inválido: "+err.Error(), toneWarning)
			return
		}
		order.CostCents = cost
	}
	recebido, err := parseBRL(paidText)
	if err != nil {
		ui.notifyTone("Valor recebido inválido: "+err.Error(), toneWarning)
		return
	}
	if method == paymentPending {
		recebido = 0
	}
	order.PaymentMethod = method
	order.PaidCents += recebido
	if order.PaidCents > order.AmountCents && order.AmountCents > 0 {
		order.PaidCents = order.AmountCents
	}
	if order.PaidCents > 0 {
		order.PaidAt = time.Now().UTC().Truncate(time.Second)
	} else {
		order.PaidAt = time.Time{}
	}
	if err := ui.store.Save(context.Background(), &order); err != nil {
		ui.notifyTone("Não consegui salvar o fechamento: "+err.Error(), toneDanger)
		return
	}
	ui.refreshList()
	if ui.current == pageHome {
		ui.refreshHome()
	}
	if ui.currentID == order.ID {
		ui.currentCostCents = order.CostCents
		if ui.currentUser.IsAdmin() {
			ui.costEntry.SetText(formatBRL(order.CostCents))
		}
		ui.updateNetAmountPreview()
		ui.refreshDelivery(order)
		ui.markOrderFormSaved()
	}
	ui.syncD1Async(false)
	if owed := orderOwedCents(order); owed > 0 {
		ui.notifyTone(fmt.Sprintf("OS %06d entregue com %s em aberto", order.ID, formatBRL(owed)), toneWarning)
		return
	}
	ui.notify(fmt.Sprintf("OS %06d fechada · %s", order.ID, method))
}

// deliveryLine descreve o fechamento da OS em uma linha.
func deliveryLine(order ServiceOrder) (string, statusTone, bool) {
	if order.Status != statusDelivered || order.DeliveredAt.IsZero() {
		return "", toneNeutral, false
	}
	when := order.DeliveredAt.Local().Format("02/01")
	switch {
	case order.PaymentMethod == "":
		return "Entregue em " + when + " · pagamento não registrado", toneWarning, true
	case orderOwedCents(order) > 0 && order.PaidCents > 0:
		return fmt.Sprintf("Entregue em %s · pago %s, faltam %s",
			when, formatBRL(order.PaidCents), formatBRL(orderOwedCents(order))), toneDanger, true
	case orderOwedCents(order) > 0:
		return fmt.Sprintf("Entregue em %s · %s (%s)", when, paymentPending, formatBRL(orderOwedCents(order))), toneDanger, true
	default:
		return fmt.Sprintf("Entregue em %s · pago em %s", when, order.PaymentMethod), toneSuccess, true
	}
}

// importanceForTone traduz o tom do aviso para o realce do widget.Label.
func importanceForTone(tone statusTone) widget.Importance {
	switch tone {
	case toneDanger:
		return widget.DangerImportance
	case toneWarning:
		return widget.WarningImportance
	case toneSuccess:
		return widget.SuccessImportance
	default:
		return widget.MediumImportance
	}
}

// refreshDelivery mostra o fechamento no resumo da OS aberta.
func (ui *mainUI) refreshDelivery(order ServiceOrder) {
	if ui.deliveryBox == nil {
		return
	}
	text, tone, show := deliveryLine(order)
	if !show {
		ui.deliveryBox.Hide()
		ui.deliveryBox.Refresh()
		return
	}
	ui.deliveryLabel.SetText(text)
	ui.deliveryLabel.Importance = importanceForTone(tone)
	ui.deliveryLabel.Refresh()
	ui.deliveryBox.Show()
	ui.deliveryBox.Refresh()
}
