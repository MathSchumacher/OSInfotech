package main

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/go-pdf/fpdf"
)

const defaultTermType = "Compra e venda de equipamento usado"

func writeTermPDF(term ServiceTerm, dir string) (string, error) {
	if term.ID == 0 {
		return "", errors.New("salve o termo antes de gerar o PDF")
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}

	path := filepath.Join(dir, fmt.Sprintf("TERMO-%06d.pdf", term.ID))
	pdf := fpdf.New("P", "mm", "A4", "")
	pdf.SetTitle(fmt.Sprintf("Termo %06d", term.ID), false)
	pdf.SetAuthor(appDisplayName, false)
	pdf.SetCreator(appDisplayName, false)
	pdf.SetMargins(12, 10, 12)
	pdf.SetAutoPageBreak(true, 12)
	pdf.AddPage()

	tr := pdf.UnicodeTranslatorFromDescriptor("")
	writeTermPDFBody(pdf, tr, term)

	return path, pdf.OutputFileAndClose(path)
}

func writeTermPDFBody(pdf *fpdf.Fpdf, tr func(string) string, term ServiceTerm) {
	title := strings.ToUpper(strings.TrimSpace(term.TermType))
	if title == "" {
		title = strings.ToUpper(defaultTermType)
	}

	pdf.SetDrawColor(38, 50, 64)
	pdf.SetLineWidth(0.25)
	pdf.SetFont("Arial", "B", 15)
	pdf.CellFormat(0, 8, tr(title), "", 1, "C", false, 0, "")
	pdf.SetFont("Arial", "", 9)
	pdf.MultiCell(0, 5, tr("Pelo presente instrumento particular, as partes abaixo identificadas realizam a compra e venda do equipamento descrito, nas condicoes informadas neste termo."), "", "L", false)
	pdf.Ln(2)

	termPDFSection(pdf, tr, "VENDEDOR")
	termPDFLine(pdf, tr,
		termPDFField{Label: "Nome", Value: term.SellerName, Width: 106},
		termPDFField{Label: "CPF/CNPJ", Value: term.SellerCPF, Width: 80},
	)
	termPDFLine(pdf, tr,
		termPDFField{Label: "RG", Value: term.SellerRG, Width: 86},
		termPDFField{Label: "Telefone", Value: term.SellerPhone, Width: 100},
	)
	termPDFLine(pdf, tr, termPDFField{Label: "Endereco", Value: term.SellerAddress, Width: 186})
	pdf.Ln(3)

	termPDFSection(pdf, tr, "COMPRADOR")
	termPDFLine(pdf, tr,
		termPDFField{Label: "Empresa", Value: term.BuyerCompany, Width: 116},
		termPDFField{Label: "CNPJ/CPF", Value: term.BuyerDocument, Width: 70},
	)
	termPDFLine(pdf, tr, termPDFField{Label: "Endereco", Value: term.BuyerAddress, Width: 186})
	termPDFLine(pdf, tr, termPDFField{Label: "Telefone", Value: term.BuyerPhone, Width: 186})
	pdf.Ln(4)

	termPDFNumberedSection(pdf, tr, "1. DADOS DO EQUIPAMENTO")
	termPDFLine(pdf, tr,
		termPDFField{Label: "Tipo", Value: term.DeviceKind, Width: 54},
		termPDFField{Label: "Marca", Value: term.Brand, Width: 44},
		termPDFField{Label: "Modelo", Value: term.Model, Width: 56},
		termPDFField{Label: "Cor", Value: term.Color, Width: 32},
	)
	termPDFLine(pdf, tr,
		termPDFField{Label: "IMEI/Serie 1", Value: term.IMEI1, Width: 93},
		termPDFField{Label: "IMEI/Serie 2", Value: term.IMEI2, Width: 93},
	)
	termPDFLine(pdf, tr, termPDFField{Label: "Acessorios", Value: term.Accessories, Width: 186})
	pdf.Ln(4)

	termPDFNumberedSection(pdf, tr, "2. DECLARACAO")
	declaration := strings.TrimSpace(term.Declaration)
	if declaration == "" {
		declaration = defaultTermDeclaration()
	}
	pdf.SetFont("Arial", "", 10)
	pdf.MultiCell(0, 5.2, tr(declaration), "", "L", false)
	pdf.Ln(4)

	termPDFNumberedSection(pdf, tr, "3. VALOR DA VENDA")
	termPDFLine(pdf, tr,
		termPDFField{Label: "Valor", Value: formatBRL(term.AmountCents), Width: 88},
		termPDFField{Label: "Forma de pagamento", Value: term.PaymentMethod, Width: 98},
	)
	termPDFLine(pdf, tr,
		termPDFField{Label: "Cidade", Value: term.City, Width: 88},
		termPDFField{Label: "Data", Value: formatTermDate(term.TermDate), Width: 98},
	)

	pdf.Ln(14)
	y := pdf.GetY()
	if y > 248 {
		pdf.AddPage()
		y = pdf.GetY() + 10
	}
	pdf.Line(18, y, 88, y)
	pdf.Line(122, y, 192, y)
	pdf.SetFont("Arial", "", 9)
	pdf.SetXY(18, y+2)
	pdf.CellFormat(70, 5, tr("Assinatura do Vendedor"), "", 0, "C", false, 0, "")
	pdf.SetXY(122, y+2)
	pdf.CellFormat(70, 5, tr("Assinatura do Comprador"), "", 1, "C", false, 0, "")

	pdf.SetY(282)
	pdf.SetFont("Arial", "", 7.5)
	pdf.SetTextColor(95, 107, 122)
	pdf.CellFormat(0, 4, tr(fmt.Sprintf("Gerado pelo %s - Termo %06d", appDisplayName, term.ID)), "", 0, "C", false, 0, "")
	pdf.SetTextColor(0, 0, 0)
}

type termPDFField struct {
	Label string
	Value string
	Width float64
}

func termPDFSection(pdf *fpdf.Fpdf, tr func(string) string, title string) {
	pdf.SetFillColor(232, 238, 243)
	pdf.SetTextColor(24, 33, 43)
	pdf.SetFont("Arial", "B", 10)
	pdf.CellFormat(0, 7, tr(title), "1", 1, "L", true, 0, "")
}

func termPDFNumberedSection(pdf *fpdf.Fpdf, tr func(string) string, title string) {
	pdf.SetFont("Arial", "B", 10.5)
	pdf.CellFormat(0, 6, tr(title), "", 1, "L", false, 0, "")
}

func termPDFLine(pdf *fpdf.Fpdf, tr func(string) string, fields ...termPDFField) {
	for _, field := range fields {
		label := strings.TrimSpace(field.Label)
		value := strings.TrimSpace(field.Value)
		labelText := label + ": "
		labelWidth := pdf.GetStringWidth(tr(labelText)) + 2
		if labelWidth > field.Width*0.5 {
			labelWidth = field.Width * 0.5
		}

		pdf.SetFont("Arial", "B", 9)
		pdf.CellFormat(labelWidth, 6.4, tr(labelText), "L", 0, "L", false, 0, "")
		pdf.SetFont("Arial", "", 10)
		pdf.CellFormat(field.Width-labelWidth, 6.4, tr(value), "B", 0, "L", false, 0, "")
	}
	pdf.Ln(6.4)
}

func defaultTermDeclaration() string {
	return "O vendedor declara ser o legitimo proprietario do equipamento e afirma que o produto nao e proveniente de furto, roubo ou qualquer atividade ilicita. O comprador declara adquirir o equipamento no estado em que se encontra apos verificacao, estando ciente das condicoes, acessorios e identificadores descritos neste termo."
}

func formatTermDate(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return t.Local().Format("02/01/2006")
}
