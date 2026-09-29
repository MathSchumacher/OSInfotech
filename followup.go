package main

import (
	"context"
	"fmt"
	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/theme"
	"net/url"
	"strings"
	"time"
)

// ---------------------------------------------------------------------------
// Cobranca de aprovacao
//
// Metade das OS da loja fica parada em "aguardando aprovacao", e boa parte delas
// ja tem orcamento: falta falar com o cliente. O telefone esta em praticamente
// toda OS, mas o WhatsApp so existia na tela do cliente, a oito passos de onde o
// problema aparece, e sem texto pronto.
//
// Aqui a cobranca sai da propria OS, com a mensagem montada a partir dela, e a
// data do contato fica registrada para o operador saber de onde continuar.
// ---------------------------------------------------------------------------

// followUpMessage monta o texto da mensagem para o cliente, conforme o estado
// da OS.
func followUpMessage(order ServiceOrder) string {
	name := firstName(order.Customer)
	greeting := "Olá! Aqui é da " + appDisplayName + "."
	if name != "" {
		greeting = fmt.Sprintf("Olá, %s! Aqui é da %s.", name, appDisplayName)
	}
	device := strings.TrimSpace(order.Device)
	if model := strings.TrimSpace(order.BrandModel); model != "" {
		device = strings.TrimSpace(device + " " + model)
	}
	if device == "" {
		device = "seu aparelho"
	}
	reference := fmt.Sprintf("Sobre a OS %06d, do %s", order.ID, device)

	switch {
	case order.Status == statusPending && order.AmountCents > 0:
		return fmt.Sprintf("%s\n\n%s: o orçamento do serviço ficou em %s.\n\nPodemos seguir com o conserto?",
			greeting, reference, formatBRL(order.AmountCents))
	case order.Status == statusPending:
		return fmt.Sprintf("%s\n\n%s: estamos com ele em avaliação e em breve retornamos com o orçamento.",
			greeting, reference)
	case order.Status == statusApproved:
		return fmt.Sprintf("%s\n\n%s: o serviço foi aprovado e o aparelho já está em conserto. Avisamos assim que ficar pronto.",
			greeting, reference)
	case order.Status == statusReady:
		return readyMessage(order)
	case order.Status == statusNoRepair || order.Status == statusUnauthorized:
		return fmt.Sprintf("%s\n\n%s: o aparelho está disponível para retirada aqui na loja.", greeting, reference)
	default:
		return fmt.Sprintf("%s\n\n%s.", greeting, reference)
	}
}

// firstName devolve o primeiro nome, com a inicial em maiuscula.
func firstName(full string) string {
	fields := strings.Fields(strings.TrimSpace(full))
	if len(fields) == 0 {
		return ""
	}
	name := fields[0]
	runes := []rune(strings.ToLower(name))
	if len(runes) == 0 {
		return ""
	}
	runes[0] = []rune(strings.ToUpper(string(runes[0])))[0]
	return string(runes)
}

// whatsappMessageURL monta o link do WhatsApp com a mensagem ja escrita.
func whatsappMessageURL(phone string, message string) string {
	base := whatsappURL(phone)
	if base == "" || strings.TrimSpace(message) == "" {
		return base
	}
	// QueryEscape usa "+" para espaco; no caminho do wa.me o "+" chega literal.
	text := strings.ReplaceAll(url.QueryEscape(message), "+", "%20")
	return base + "?text=" + text
}

// canFollowUp diz se vale oferecer a cobranca para esta OS.
func canFollowUp(order ServiceOrder) bool {
	return order.ID != 0 && orderCanBeReached(order) && order.Status != statusDelivered
}

// openFollowUp abre o WhatsApp com a mensagem pronta e registra o contato.
func (ui *mainUI) openFollowUp(order ServiceOrder) {
	// Vai pelo contato alternativo quando o telefone do cliente nao serve: sao
	// 23 OS na base sem numero que receba WhatsApp, e elas estao justamente na
	// fila que espera resposta.
	link := whatsappMessageURL(orderReachPhone(order), followUpMessage(order))
	if link == "" {
		ui.notifyTone("Esta OS não tem um telefone com DDD para abrir o WhatsApp.", toneWarning)
		return
	}
	if err := openExternalURL(link); err != nil {
		ui.notifyTone("Não consegui abrir o WhatsApp: "+err.Error(), toneDanger)
		return
	}
	if err := ui.store.MarkOrderContacted(context.Background(), order.ID, time.Now().UTC()); err != nil {
		ui.notifyTone("WhatsApp aberto, mas não consegui anotar a cobrança: "+err.Error(), toneWarning)
		return
	}
	ui.refreshList()
	if ui.currentID == order.ID {
		ui.updateContactedLabel(order.ID)
	}
	ui.notify("Cobrança anotada em " + time.Now().Format("02/01 15:04"))
}

// orderAgeText descreve ha quanto tempo a OS esta aberta, e se ja passou do
// ponto de cobrar. Devolve texto vazio para OS ja entregues.
func orderAgeText(order ServiceOrder) (string, statusTone, bool) {
	if order.Status == statusDelivered || order.CreatedAt.IsZero() {
		return "", toneNeutral, false
	}
	days := int(time.Since(order.CreatedAt).Hours() / 24)
	switch {
	case days <= 0:
		return "aberta hoje", toneNeutral, false
	case days == 1:
		return "aberta ontem", toneNeutral, false
	case days < staleOrderDays:
		return fmt.Sprintf("aberta há %d dias", days), toneNeutral, false
	case days < 30:
		return fmt.Sprintf("parada há %d dias", days), toneWarning, true
	default:
		return fmt.Sprintf("parada há %d dias", days), toneDanger, true
	}
}

// orderListNote e o texto da ultima coluna da lista: o que falta nesta OS. Para
// OS aberta e ha quanto tempo esta parada; para OS entregue e o que o cliente
// ficou devendo, que antes nao aparecia em lugar nenhum.
func orderListNote(order ServiceOrder) (string, statusTone) {
	// Sem telefone utilizavel, cobrar e impossivel e a idade da OS deixa de ser
	// a informacao util: o que muda a acao e saber que nao ha como avisar.
	if orderQueue(order) == queueNeedsAnswer && !orderCanBeReached(order) {
		return "sem como avisar", toneDanger
	}
	if age, tone, _ := orderAgeText(order); age != "" {
		return age, tone
	}
	if owed := orderOwedCents(order); owed > 0 {
		return "devendo " + formatBRL(owed), toneDanger
	}
	return relativeDate(order.UpdatedAt), toneNeutral
}

// staleOrderDays e o limite a partir do qual uma OS em aberto vira cobranca
// atrasada. Sete dias e o ponto em que a loja perde o contato com o cliente.
const staleOrderDays = 7

// inkForTone escolhe a cor do texto conforme o tom do aviso.
func inkForTone(tone statusTone) inkLevel {
	switch tone {
	case toneWarning:
		return inkWarning
	case toneDanger:
		return inkDanger
	default:
		return inkMuted
	}
}

// refreshFollowUp mostra ou esconde o bloco de cobranca conforme a OS aberta.
func (ui *mainUI) refreshFollowUp(order ServiceOrder) {
	if ui.followUpBox == nil {
		return
	}
	ui.followUpBox.Objects = nil
	if !canFollowUp(order) {
		ui.followUpBox.Hide()
		ui.followUpBox.Refresh()
		return
	}
	ui.followUpBox.Add(gap(1, 8))
	ui.followUpBox.Add(ui.followUpBtn)
	ui.followUpBox.Add(gap(1, 4))
	ui.followUpBox.Add(ui.contactedLabel)
	ui.followUpBox.Show()
	ui.followUpBox.Refresh()
	ui.updateContactedLabel(order.ID)
}

// updateContactedLabel escreve quando o cliente foi cobrado pela ultima vez.
func (ui *mainUI) updateContactedLabel(id int64) {
	if ui.contactedLabel == nil {
		return
	}
	when, found, err := ui.store.OrderContactedAt(context.Background(), id)
	if err != nil || !found {
		ui.contactedLabel.SetText("Cliente ainda não foi cobrado")
		ui.contactedLabel.SetInk(inkMuted)
		return
	}
	ui.contactedLabel.SetText("Cobrado em " + when.Local().Format("02/01 às 15:04"))
	ui.contactedLabel.SetInk(inkSecondary)
}

// followUpCurrent cobra o cliente da OS aberta. Usa o que esta na tela, para a
// mensagem sair com o valor que o operador acabou de digitar.
func (ui *mainUI) followUpCurrent() {
	if ui.currentID == 0 {
		ui.notifyTone("Salve a OS antes de cobrar o cliente.", toneWarning)
		return
	}
	order, err := ui.collectOrder()
	if err != nil {
		saved, getErr := ui.store.Get(context.Background(), ui.currentID)
		if getErr != nil {
			ui.notifyTone(err.Error(), toneWarning)
			return
		}
		order = saved
	}
	order.ID = ui.currentID
	ui.openFollowUp(order)
}

// showStatusMenu troca o status direto da lista. Abrir a OS, mudar, salvar e
// voltar custava cinco cliques por aparelho.
func (ui *mainUI) showStatusMenu(id int64, anchor fyne.CanvasObject) {
	order, err := ui.store.Get(context.Background(), id)
	if err != nil {
		ui.notifyTone("Não consegui abrir esta OS: "+err.Error(), toneDanger)
		return
	}
	actions := make([]menuAction, 0, len(orderStatuses)+3)
	for _, status := range orderStatuses {
		if status == order.Status {
			continue
		}
		target := status
		actions = append(actions, menuAction{
			Label: target,
			Icon:  statusMenuIcon(target),
			Do:    func() { ui.applyStatusChange(order, target) },
		})
	}
	if canFollowUp(order) {
		actions = append(actions, menuAction{}, menuAction{
			Label: "Cobrar no WhatsApp",
			Hint:  "Mensagem pronta com o orçamento",
			Icon:  theme.MailSendIcon(),
			Do:    func() { ui.openFollowUp(order) },
		})
	}
	header := menuHeader(fmt.Sprintf("OS %06d", order.ID), firstNonEmpty(order.Customer, "Sem cliente"), order.Customer, "")
	ui.showActionMenu(anchor, header, actions)
}

func statusMenuIcon(status string) fyne.Resource {
	switch status {
	case statusDelivered:
		return theme.ConfirmIcon()
	case statusReady:
		return theme.MailSendIcon()
	case statusApproved:
		return theme.ContentAddIcon()
	case statusUnauthorized, statusNoRepair:
		return theme.CancelIcon()
	default:
		return theme.WarningIcon()
	}
}

// applyStatusChange grava o novo status e avisa o operador.
func (ui *mainUI) applyStatusChange(order ServiceOrder, status string) {
	if order.Status == status {
		return
	}
	order.Status = status
	if err := ui.store.Save(context.Background(), &order); err != nil {
		ui.notifyTone("Não consegui salvar: "+err.Error(), toneDanger)
		return
	}
	ui.refreshList()
	if ui.current == pageHome {
		ui.refreshHome()
	}
	if ui.currentID == order.ID {
		ui.loadOrder(order)
	}
	ui.syncD1Async(false)
	ui.notify(fmt.Sprintf("OS %06d agora está %s", order.ID, strings.ToLower(status)))
	if status == statusReady {
		ui.offerReadyNotice(order)
	}
	if status == statusDelivered {
		ui.offerDeliveryClose(order)
	}
}
