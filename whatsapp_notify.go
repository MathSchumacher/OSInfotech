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
// Aviso de "pode vir buscar"
//
// E o momento que mais vale uma mensagem: o aparelho esta pronto, o cliente nao
// sabe, e cada dia parado na prateleira e dinheiro que nao entrou e espaco que
// nao tem. Marcar a OS como pronta ja oferece o aviso, e o envio sai pelo
// WhatsApp da propria loja quando ele esta conectado.
//
// Dois freios propositais. O envio so acontece para quem tem OS aberta na loja,
// nunca para uma lista. E ha um teto diario: disparo em volume e o que faz o
// WhatsApp bloquear um numero.
// ---------------------------------------------------------------------------

// readyMessage e o texto do aviso de retirada.
func readyMessage(order ServiceOrder) string {
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
	text := fmt.Sprintf("%s\n\nBoa notícia: o serviço da OS %06d, do %s, está pronto e o aparelho já pode ser retirado.",
		greeting, order.ID, device)
	if order.AmountCents > 0 {
		text += fmt.Sprintf("\n\nValor do serviço: %s.", formatBRL(order.AmountCents))
	}
	return text + "\n\nEstamos à disposição no horário de atendimento."
}

// businessHours diz se agora e uma hora razoavel para mandar mensagem.
func businessHours(now time.Time) bool {
	if now.Weekday() == time.Sunday {
		return false
	}
	hour := now.Hour()
	return hour >= 8 && hour < 20
}

// offerReadyNotice pergunta se avisa o cliente, mostrando o texto que vai sair.
func (ui *mainUI) offerReadyNotice(order ServiceOrder) {
	number := orderReachPhone(order)
	if number == "" {
		ui.notifyTone("Esta OS não tem telefone que receba WhatsApp, nem contato alternativo.", toneWarning)
		return
	}
	text := readyMessage(order)

	preview := widget.NewLabel(text)
	preview.Wrapping = fyne.TextWrapWord
	body := container.NewVBox(
		mutedLabel("O cliente recebe esta mensagem no WhatsApp:"),
		gap(1, 6),
		sunkenCard(paddedTRBL(preview, 10, 12, 10, 12), 10),
	)
	if !businessHours(time.Now()) {
		body.Add(gap(1, 6))
		body.Add(newText("Fora do horário comercial. Dá para enviar assim mesmo, mas o cliente recebe agora.", sizeCaption, false, inkWarning))
	}

	var noticeDialog *dialog.CustomDialog
	sendNow := primaryButton("Enviar pelo WhatsApp da loja", theme.MailSendIcon(), func() {
		noticeDialog.Hide()
		ui.sendReadyNotice(order, number, text)
	})
	openApp := secondaryButton("Abrir o WhatsApp", theme.ComputerIcon(), func() {
		noticeDialog.Hide()
		ui.openFollowUp(order)
	})
	if !ui.whatsappReady() {
		sendNow = primaryButton("Conectar o WhatsApp da loja", theme.MailSendIcon(), func() {
			noticeDialog.Hide()
			ui.openSettingsSection(settingsWhatsApp)
		})
	}

	// SetButtons troca o botao de dispensar, entao ele volta aqui: sem saida a
	// caixa so fecha com Esc, e o atendente fica preso.
	later := ghostButton("Agora não", nil, func() { noticeDialog.Hide() })
	noticeDialog = dialog.NewCustom(fmt.Sprintf("Avisar o cliente da OS %06d", order.ID), "", body, ui.window)
	noticeDialog.Resize(fyne.NewSize(560, 430))
	noticeDialog.SetButtons([]fyne.CanvasObject{later, openApp, sendNow})
	noticeDialog.Show()
}

// whatsappReady diz se a loja ja pareou o WhatsApp neste computador.
func (ui *mainUI) whatsappReady() bool {
	if ui.whatsapp == nil || !ui.whatsapp.Available() {
		return false
	}
	value, found, err := ui.store.GetSetting(context.Background(), whatsappPairedSetting)
	return err == nil && found && value == "1"
}

// sendReadyNotice envia de verdade, respeitando o teto diario.
func (ui *mainUI) sendReadyNotice(order ServiceOrder, number, text string) {
	ctx := context.Background()
	sentToday, err := ui.store.WhatsAppSentToday(ctx)
	if err == nil && sentToday >= whatsappDailyCap {
		ui.notifyTone(fmt.Sprintf("Limite de %d mensagens por dia atingido. Amanhã libera.", whatsappDailyCap), toneWarning)
		return
	}

	ui.notify("Enviando o aviso…")
	go func() {
		sendCtx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
		defer cancel()
		sendErr := ui.whatsapp.Send(sendCtx, number, text)
		fyne.Do(func() {
			if sendErr != nil {
				ui.notifyTone("Não consegui enviar: "+sendErr.Error(), toneDanger)
				return
			}
			background := context.Background()
			_ = ui.store.LogWhatsApp(background, order.ID, number, "pronto")
			_ = ui.store.MarkOrderContacted(background, order.ID, time.Now().UTC())
			if ui.currentID == order.ID {
				ui.updateContactedLabel(order.ID)
			}
			ui.refreshList()
			ui.notify("Cliente avisado pelo WhatsApp")
		})
	}()
}
