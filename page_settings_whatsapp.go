package main

import (
	"context"
	"fmt"
	"image"
	"image/color"
	"sync/atomic"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/theme"
	"rsc.io/qr"
)

// ---------------------------------------------------------------------------
// Tela do WhatsApp da loja
//
// Parear e ler um QR, como no WhatsApp Web: a diferenca e que a sessao fica
// neste computador, dentro da pasta de dados do app, e nao no navegador.
// ---------------------------------------------------------------------------

// whatsappPoll numera as sessoes de atualizacao da tela, para a anterior parar
// quando o operador sai da secao.
var whatsappPoll atomic.Uint64

func (p *settingsPage) buildWhatsApp() fyne.CanvasObject {
	ui := p.ui
	headline := newText("Verificando…", sizeBody, true, inkPrimary).SetTruncate(true)
	detail := newText("", sizeCaption, false, inkMuted).SetTruncate(true)
	qrImage := canvas.NewImageFromImage(image.NewNRGBA(image.Rect(0, 0, 1, 1)))
	qrImage.FillMode = canvas.ImageFillContain
	qrImage.ScaleMode = canvas.ImageScaleFastest
	qrBox := container.NewStack(gap(220, 220), qrImage)
	qrBox.Hide()

	if !ui.whatsapp.Available() {
		return sectionCard("WhatsApp da loja",
			"O WhatsApp da loja é conectado no computador do balcão, que é onde a sessão fica guardada.",
			mutedLabel("Neste aparelho não dá para parear. Use o computador da loja."))
	}

	var connect, disconnect *dbsButton
	refresh := func(status whatsappStatus, err error) {
		if err != nil {
			headline.SetText("Não consegui falar com a ponte")
			detail.SetText(err.Error())
			qrBox.Hide()
			connect.Show()
			disconnect.Hide()
			return
		}
		headline.SetText(status.Headline())
		switch {
		case status.Connected:
			detail.SetText("A loja pode avisar os clientes direto pelo app.")
			qrBox.Hide()
			connect.Hide()
			disconnect.Show()
			_ = ui.store.SetSetting(context.Background(), whatsappPairedSetting, "1")
		case status.QR != "":
			detail.SetText("WhatsApp do celular > Aparelhos conectados > Conectar aparelho.")
			if img := renderQRCode(status.QR, 220); img != nil {
				qrImage.Image = img
				qrImage.Refresh()
				qrBox.Show()
			}
			connect.Hide()
			disconnect.Show()
		default:
			detail.SetText("")
			qrBox.Hide()
			connect.Show()
			disconnect.Hide()
		}
	}

	poll := func() {
		seq := whatsappPoll.Add(1)
		go func() {
			for i := 0; i < 150; i++ { // cerca de cinco minutos de pareamento
				if seq != whatsappPoll.Load() {
					return
				}
				ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
				status, err := ui.whatsapp.Status(ctx)
				cancel()
				if seq != whatsappPoll.Load() {
					return
				}
				fyne.Do(func() {
					if seq == whatsappPoll.Load() {
						refresh(status, err)
					}
				})
				if err == nil && status.Connected {
					return
				}
				time.Sleep(2 * time.Second)
			}
		}()
	}

	connect = primaryButton("Conectar o WhatsApp", theme.MailSendIcon(), func() {
		headline.SetText("Abrindo a ligação…")
		poll()
	})
	disconnect = dangerButton("Desconectar", theme.CancelIcon(), func() {
		message := "Desconectar o WhatsApp da loja? Para voltar a avisar clientes será preciso ler o QR de novo."
		dialog.ShowConfirm("Desconectar o WhatsApp", message, func(ok bool) {
			if !ok {
				return
			}
			go func() {
				ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
				defer cancel()
				err := ui.whatsapp.Disconnect(ctx)
				fyne.Do(func() {
					if err != nil {
						ui.notifyTone("Não consegui desconectar: "+err.Error(), toneDanger)
						return
					}
					_ = ui.store.SetSetting(context.Background(), whatsappPairedSetting, "")
					ui.notify("WhatsApp desconectado")
					poll()
				})
			}()
		}, ui.window)
	})
	disconnect.Hide()

	sent := newText("", sizeCaption, false, inkMuted)
	if count, err := ui.store.WhatsAppSentToday(context.Background()); err == nil {
		sent.SetText(fmt.Sprintf("Enviadas hoje: %d de %d", count, whatsappDailyCap))
	}

	// Sobe a ligacao sozinho quando a loja ja pareou antes.
	if ui.whatsappReady() {
		poll()
	}

	aviso := newText(
		"Use o número da loja, não o pessoal. O app só envia para clientes com OS aberta, "+
			"no máximo "+fmt.Sprint(whatsappDailyCap)+" mensagens por dia. Disparo para quem não pediu contato é o que faz o WhatsApp bloquear um número.",
		sizeCaption, false, inkMuted)

	return sectionCard("WhatsApp da loja",
		"Conecte o número da loja uma vez e avise os clientes direto da OS.",
		container.NewVBox(
			headline,
			detail,
			gap(1, 10),
			qrBox,
			gap(1, 10),
			container.NewHBox(connect, disconnect),
			gap(1, 8),
			sent,
			gap(1, 6),
			newGlassPanel(paddedTRBL(aviso, 8, 10, 8, 10), 0, tintGlass(toneWarning)),
		))
}

// renderQRCode desenha o QR em preto sobre branco. Mesmo no tema escuro ele
// precisa ser claro: leitor de QR espera fundo claro e modulo escuro.
func renderQRCode(text string, size int) image.Image {
	code, err := qr.Encode(text, qr.M)
	if err != nil {
		return nil
	}
	quiet := 4
	modules := code.Size + quiet*2
	scale := size / modules
	if scale < 1 {
		scale = 1
	}
	side := modules * scale
	img := image.NewNRGBA(image.Rect(0, 0, side, side))
	white := color.NRGBA{R: 255, G: 255, B: 255, A: 255}
	black := color.NRGBA{A: 255}
	for y := 0; y < side; y++ {
		for x := 0; x < side; x++ {
			img.SetNRGBA(x, y, white)
		}
	}
	for my := 0; my < code.Size; my++ {
		for mx := 0; mx < code.Size; mx++ {
			if !code.Black(mx, my) {
				continue
			}
			for y := 0; y < scale; y++ {
				for x := 0; x < scale; x++ {
					img.SetNRGBA((mx+quiet)*scale+x, (my+quiet)*scale+y, black)
				}
			}
		}
	}
	return img
}
