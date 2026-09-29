package main

import (
	"bytes"
	"fmt"
	"strings"
	"time"
	"unicode"

	"golang.org/x/text/unicode/norm"
)

const escposLineWidth = 46

func buildESCPOSReceipt(order ServiceOrder) []byte {
	return buildESCPOSReceiptWithSettings(order, defaultThermalPrintSettings())
}

func buildESCPOSReceiptWithSettings(order ServiceOrder, settings ThermalPrintSettings) []byte {
	settings = settings.normalized()
	var out bytes.Buffer
	out.Write([]byte{0x1b, '@'})       // Initialize printer.
	out.Write([]byte{0x1b, 't', 0x02}) // CP850: common on Brazilian ESC/POS printers.

	escposAlign(&out, 1)
	escposTextStyle(&out, 0x30)
	for _, line := range escposWrap(settings.Title, escposLineWidth/2) {
		escposLine(&out, line)
	}
	escposTextStyle(&out, 0)
	for _, line := range escposWrap(settings.Subtitle, escposLineWidth) {
		escposLine(&out, line)
	}
	escposDivider(&out)

	escposTextStyle(&out, 0x08)
	escposLine(&out, fmt.Sprintf("OS %06d | %s", order.ID, order.Status))
	escposTextStyle(&out, 0)
	escposLine(&out, "Entrada: "+formatDate(order.CreatedAt))
	escposAlign(&out, 0)

	escposSection(&out, "CLIENTE")
	escposPrimary(&out, order.Customer)
	escposInline(&out, "CPF/CNPJ", order.CPF)
	escposInline(&out, "Fone", order.Phone)

	escposSection(&out, "EQUIPAMENTO")
	escposPrimary(&out, order.Device)
	escposInline(&out, "Marca/Mod.", order.BrandModel)
	escposInline(&out, "IMEI/Serie", order.Serial)

	escposSection(&out, "ATENDIMENTO")
	escposInline(&out, "Motivo", order.Issue)
	escposInline(&out, "Acess.", order.Accessories)
	// A anotacao da bancada NAO entra aqui. O formulario sempre prometeu que ela
	// "nao aparece para o cliente", e imprimia: das 94 OS com anotacao, 51
	// falavam em senha e 44 tinham o codigo de desbloqueio escrito. O que o
	// cliente precisa levar por escrito ja tem campo proprio (acessorios, valor,
	// garantia).
	escposInline(&out, "Tecnico", order.TechnicianName)

	escposDivider(&out)
	escposAlign(&out, 1)
	escposTextStyle(&out, 0x08)
	escposLine(&out, "VALOR DO SERVICO")
	escposTextStyle(&out, 0x30)
	escposLine(&out, formatBRL(order.AmountCents))
	escposTextStyle(&out, 0)
	// Quem deixou sinal precisa levar isso por escrito: e o comprovante dele de
	// que entregou o dinheiro, e o que evita discussao na retirada.
	if order.PaidCents > 0 {
		escposAlign(&out, 0)
		escposInline(&out, "Ja pago", formatBRL(order.PaidCents))
		if falta := order.AmountCents - order.PaidCents; falta > 0 {
			escposInline(&out, "Falta", formatBRL(falta))
		}
		escposAlign(&out, 1)
	}
	escposDivider(&out)
	escposLine(&out, "____________________________")
	escposLine(&out, "Assinatura do cliente")
	// Prazo de garantia e de retirada: e o que o cliente leva por escrito e o
	// que evita discussao no balcao meses depois.
	if footer := strings.TrimSpace(settings.Footer); footer != "" {
		escposDivider(&out)
		for _, line := range escposWrap(footer, escposLineWidth) {
			escposLine(&out, line)
		}
	}
	escposLine(&out, "Gerado em "+time.Now().Format("02/01/2006 15:04"))
	out.Write([]byte{0x1b, 'd', 0x03})    // Feed three lines only.
	out.Write([]byte{0x1d, 'V', 0x42, 0}) // Partial cut.
	return out.Bytes()
}

func escposAlign(out *bytes.Buffer, alignment byte) {
	out.Write([]byte{0x1b, 'a', alignment})
}

func escposTextStyle(out *bytes.Buffer, style byte) {
	out.Write([]byte{0x1b, '!', style})
}

func escposDivider(out *bytes.Buffer) {
	escposLine(out, strings.Repeat("-", escposLineWidth))
}

func escposSection(out *bytes.Buffer, title string) {
	escposDivider(out)
	escposTextStyle(out, 0x08)
	escposLine(out, title)
	escposTextStyle(out, 0)
}

func escposPrimary(out *bytes.Buffer, value string) {
	value = strings.TrimSpace(value)
	if value == "" {
		return
	}
	escposTextStyle(out, 0x08)
	for _, line := range escposWrap(value, escposLineWidth) {
		escposLine(out, line)
	}
	escposTextStyle(out, 0)
}

func escposInline(out *bytes.Buffer, label string, value string) {
	value = strings.TrimSpace(value)
	if value == "" {
		return
	}
	prefix := escposSafeText(label) + ": "
	plainValue := escposSafeText(value)
	if len(prefix)+len(plainValue) <= escposLineWidth {
		escposLine(out, prefix+plainValue)
		return
	}
	escposLine(out, prefix)
	for _, line := range escposWrap(value, escposLineWidth) {
		escposLine(out, line)
	}
}

func escposLine(out *bytes.Buffer, value string) {
	out.WriteString(escposSafeText(value))
	out.WriteByte('\n')
}

func escposWrap(value string, width int) []string {
	if width < 1 {
		return nil
	}
	var lines []string
	for _, paragraph := range strings.Split(value, "\n") {
		words := strings.Fields(escposSafeText(paragraph))
		if len(words) == 0 {
			continue
		}
		line := ""
		for _, word := range words {
			for len(word) > width {
				if line != "" {
					lines = append(lines, line)
					line = ""
				}
				lines = append(lines, word[:width])
				word = word[width:]
			}
			candidate := word
			if line != "" {
				candidate = line + " " + word
			}
			if len(candidate) > width && line != "" {
				lines = append(lines, line)
				line = word
				continue
			}
			line = candidate
		}
		if line != "" {
			lines = append(lines, line)
		}
	}
	return lines
}

func escposSafeText(value string) string {
	value = norm.NFD.String(value)
	var out strings.Builder
	for _, r := range value {
		if unicode.Is(unicode.Mn, r) {
			continue
		}
		if r == '\r' || r == '\n' || r == '\t' {
			out.WriteRune(r)
			continue
		}
		if r >= 32 && r <= 126 {
			out.WriteRune(r)
		} else {
			out.WriteByte('?')
		}
	}
	return out.String()
}
