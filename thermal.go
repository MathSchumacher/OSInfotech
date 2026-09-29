package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
	"github.com/go-pdf/fpdf"
)

const thermalReceiptWidthMM = 80.0

const (
	thermalPrintTitleSetting    = "thermal_print_title"
	thermalPrintSubtitleSetting = "thermal_print_subtitle"
	thermalPrintFooterSetting   = "thermal_print_footer"
)

// ThermalPrintSettings identifies the store on every 80 mm receipt.
// It is stored locally so each installation can use its own shop identity.
type ThermalPrintSettings struct {
	Title    string
	Subtitle string
	// Footer sai no pe do recibo. E onde a loja avisa prazo de garantia e prazo
	// de retirada, que e o que evita discussao no balcao meses depois.
	Footer string
}

func defaultThermalPrintSettings() ThermalPrintSettings {
	return ThermalPrintSettings{
		Title:    appDisplayName,
		Subtitle: "Assistencia tecnica",
		Footer:   "Garantia de 90 dias sobre o servico executado. Prazo para retirada do aparelho: 90 dias.",
	}
}

func (settings ThermalPrintSettings) normalized() ThermalPrintSettings {
	defaults := defaultThermalPrintSettings()
	settings.Title = strings.TrimSpace(settings.Title)
	settings.Subtitle = strings.TrimSpace(settings.Subtitle)
	settings.Footer = strings.TrimSpace(settings.Footer)
	if settings.Title == "" {
		settings.Title = defaults.Title
	}
	return settings
}

func writeThermalReceiptPDF(order ServiceOrder, dir string) (string, error) {
	return writeThermalReceiptPDFWithSettings(order, dir, defaultThermalPrintSettings())
}

func writeThermalReceiptPDFWithSettings(order ServiceOrder, dir string, settings ThermalPrintSettings) (string, error) {
	if order.ID == 0 {
		return "", errors.New("salve a OS antes de imprimir")
	}
	settings = settings.normalized()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}

	path := filepath.Join(dir, fmt.Sprintf("OS-%06d-80mm.pdf", order.ID))
	pdf := fpdf.NewCustom(&fpdf.InitType{
		OrientationStr: "P",
		UnitStr:        "mm",
		Size: fpdf.SizeType{
			Wd: thermalReceiptWidthMM,
			Ht: thermalReceiptHeightWithSettings(order, settings),
		},
	})
	pdf.SetTitle(fmt.Sprintf("OS %06d - 80mm", order.ID), false)
	pdf.SetAuthor(settings.Title, false)
	pdf.SetCreator(settings.Title, false)
	pdf.SetMargins(4, 4, 4)
	pdf.SetAutoPageBreak(false, 0)
	pdf.AddPage()

	tr := pdf.UnicodeTranslatorFromDescriptor("")
	writeThermalReceiptBody(pdf, tr, order, settings)
	return path, pdf.OutputFileAndClose(path)
}

func thermalReceiptHeight(order ServiceOrder) float64 {
	return thermalReceiptHeightWithSettings(order, defaultThermalPrintSettings())
}

func thermalReceiptHeightWithSettings(order ServiceOrder, settings ThermalPrintSettings) float64 {
	fields := []string{
		order.Customer,
		order.CPF,
		order.Phone,
		order.Device,
		order.BrandModel,
		order.Serial,
		order.Issue,
		order.Accessories,
		order.Notes,
		order.TechnicianName,
		settings.Footer,
	}
	lines := 0
	for _, value := range fields {
		lines += thermalLineEstimate(value)
	}
	height := 88.0 + float64(lines)*3.9
	if height < 132 {
		return 132
	}
	if height > 280 {
		return 280
	}
	return height
}

func thermalLineEstimate(value string) int {
	value = strings.TrimSpace(value)
	if value == "" {
		return 1
	}
	count := 0
	for _, line := range strings.Split(value, "\n") {
		chars := len([]rune(strings.TrimSpace(line)))
		if chars == 0 {
			count++
			continue
		}
		count += (chars + 34) / 35
	}
	return count
}

func writeThermalReceiptBody(pdf *fpdf.Fpdf, tr func(string) string, order ServiceOrder, settings ThermalPrintSettings) {
	settings = settings.normalized()
	pdf.SetTextColor(18, 27, 38)
	pdf.SetFont("Arial", "B", 13)
	pdf.CellFormat(0, 6, tr(settings.Title), "", 1, "C", false, 0, "")
	pdf.SetFont("Arial", "", 7.5)
	if settings.Subtitle != "" {
		pdf.CellFormat(0, 4, tr(settings.Subtitle), "", 1, "C", false, 0, "")
	}
	thermalRule(pdf)

	pdf.SetFont("Arial", "B", 9.5)
	pdf.CellFormat(0, 5, tr(fmt.Sprintf("OS %06d | %s", order.ID, order.Status)), "", 1, "C", false, 0, "")
	pdf.SetFont("Arial", "", 7.5)
	pdf.CellFormat(0, 4, tr("Entrada: "+formatDate(order.CreatedAt)), "", 1, "C", false, 0, "")

	thermalSection(pdf, tr, "CLIENTE")
	thermalPrimary(pdf, tr, order.Customer)
	thermalInline(pdf, tr, "CPF/CNPJ", order.CPF)
	thermalInline(pdf, tr, "Fone", order.Phone)

	thermalSection(pdf, tr, "EQUIPAMENTO")
	thermalPrimary(pdf, tr, order.Device)
	thermalInline(pdf, tr, "Marca/Mod.", order.BrandModel)
	thermalInline(pdf, tr, "IMEI/Serie", order.Serial)

	thermalSection(pdf, tr, "ATENDIMENTO")
	thermalInline(pdf, tr, "Motivo", order.Issue)
	thermalInline(pdf, tr, "Acess.", order.Accessories)
	thermalInline(pdf, tr, "Obs.", order.Notes)
	thermalInline(pdf, tr, "Tecnico", order.TechnicianName)

	thermalRule(pdf)
	pdf.SetFont("Arial", "B", 8)
	pdf.CellFormat(0, 4, tr("VALOR DO SERVICO"), "", 1, "C", false, 0, "")
	pdf.SetFont("Arial", "B", 12)
	pdf.CellFormat(0, 6, tr(formatBRL(order.AmountCents)), "", 1, "C", false, 0, "")
	thermalRule(pdf)

	pdf.Ln(9)
	pdf.Line(7, pdf.GetY(), 73, pdf.GetY())
	pdf.SetFont("Arial", "", 7.5)
	pdf.CellFormat(0, 4, tr("Assinatura do cliente"), "", 1, "C", false, 0, "")
	if footer := strings.TrimSpace(settings.Footer); footer != "" {
		pdf.Ln(2)
		pdf.SetTextColor(86, 99, 114)
		pdf.SetFont("Arial", "", 6.5)
		pdf.MultiCell(0, 3.2, tr(footer), "", "C", false)
		pdf.SetTextColor(0, 0, 0)
	}
	pdf.Ln(3)
	pdf.SetTextColor(86, 99, 114)
	pdf.SetFont("Arial", "", 6.5)
	pdf.CellFormat(0, 3.5, tr("Gerado em "+time.Now().Format("02/01/2006 15:04")), "", 1, "C", false, 0, "")
	pdf.SetTextColor(0, 0, 0)
}

func thermalRule(pdf *fpdf.Fpdf) {
	pdf.Ln(1.5)
	y := pdf.GetY()
	pdf.SetDrawColor(110, 123, 138)
	pdf.Line(4, y, 76, y)
	pdf.Ln(2)
}

func thermalSection(pdf *fpdf.Fpdf, tr func(string) string, title string) {
	thermalRule(pdf)
	pdf.SetFont("Arial", "B", 8)
	pdf.SetTextColor(20, 86, 82)
	pdf.CellFormat(0, 4.5, tr(title), "", 1, "L", false, 0, "")
	pdf.SetTextColor(0, 0, 0)
}

func thermalPrimary(pdf *fpdf.Fpdf, tr func(string) string, value string) {
	value = strings.TrimSpace(value)
	if value == "" {
		return
	}
	pdf.SetFont("Arial", "B", 8.5)
	pdf.MultiCell(0, 3.9, tr(value), "", "L", false)
}

func thermalInline(pdf *fpdf.Fpdf, tr func(string) string, label string, value string) {
	value = strings.TrimSpace(value)
	if value == "" {
		return
	}
	pdf.SetFont("Arial", "", 7.7)
	text := label + ": " + value
	pdf.MultiCell(0, 3.7, tr(text), "", "L", false)
}

func (ui *mainUI) thermalPrintSettings(ctx context.Context) (ThermalPrintSettings, error) {
	settings := defaultThermalPrintSettings()
	if title, found, err := ui.store.GetSetting(ctx, thermalPrintTitleSetting); err != nil {
		return settings, err
	} else if found {
		settings.Title = title
	}
	if subtitle, found, err := ui.store.GetSetting(ctx, thermalPrintSubtitleSetting); err != nil {
		return settings, err
	} else if found {
		settings.Subtitle = subtitle
	}
	if footer, found, err := ui.store.GetSetting(ctx, thermalPrintFooterSetting); err != nil {
		return settings, err
	} else if found {
		settings.Footer = footer
	}
	return settings.normalized(), nil
}

func (ui *mainUI) saveThermalPrintSettings(ctx context.Context, settings ThermalPrintSettings) error {
	settings = settings.normalized()
	if err := ui.store.SetSetting(ctx, thermalPrintTitleSetting, settings.Title); err != nil {
		return err
	}
	if err := ui.store.SetSetting(ctx, thermalPrintSubtitleSetting, settings.Subtitle); err != nil {
		return err
	}
	return ui.store.SetSetting(ctx, thermalPrintFooterSetting, settings.Footer)
}

// thermalPrinterSetting guarda a impressora escolhida da ultima vez.
const thermalPrinterSetting = "thermal_printer"

func (ui *mainUI) generateThermalReceipt() {
	if !ui.ensureSavedForPrint() {
		return
	}
	order, err := ui.store.Get(context.Background(), ui.currentID)
	if err != nil {
		dialog.ShowError(err, ui.window)
		return
	}

	outDir := filepath.Join(filepath.Dir(ui.dbPath), "recibos", "80mm")
	settings, err := ui.thermalPrintSettings(context.Background())
	if err != nil {
		dialog.ShowError(err, ui.window)
		return
	}
	path, err := writeThermalReceiptPDFWithSettings(order, outDir, settings)
	if err != nil {
		dialog.ShowError(err, ui.window)
		return
	}

	open := ghostButton("Abrir PDF", theme.FolderOpenIcon(), func() {
		uri, err := fileURL(path)
		if err != nil {
			dialog.ShowError(err, ui.window)
			return
		}
		if err := fyne.CurrentApp().OpenURL(uri); err != nil {
			dialog.ShowError(err, ui.window)
		}
	})

	content := []fyne.CanvasObject{
		widget.NewLabel("Recibo da OS pronto."),
	}
	if rawThermalPrintingAvailable() {
		printers, defaultPrinter, err := thermalPrinterNames()
		if err != nil {
			dialog.ShowError(err, ui.window)
			return
		}
		if len(printers) > 0 {
			printerSelect := widget.NewSelect(printers, nil)
			// A loja imprime na mesma impressora todo dia: a escolhida da ultima
			// vez vem selecionada, e so cai na padrao do Windows se ela sumiu.
			if saved, found, err := ui.store.GetSetting(context.Background(), thermalPrinterSetting); err == nil && found {
				for _, name := range printers {
					if name == saved {
						printerSelect.SetSelected(saved)
						break
					}
				}
			}
			if printerSelect.Selected == "" && defaultPrinter != "" {
				printerSelect.SetSelected(defaultPrinter)
			}
			if printerSelect.Selected == "" {
				printerSelect.SetSelected(printers[0])
			}
			status := widget.NewLabel("")
			status.Importance = widget.LowImportance
			printButton := primaryButton("Imprimir 80 mm", theme.DocumentPrintIcon(), nil)
			printButton.OnTapped = func() {
				printer := strings.TrimSpace(printerSelect.Selected)
				if printer == "" {
					return
				}
				printButton.Disable()
				_ = ui.store.SetSetting(context.Background(), thermalPrinterSetting, printer)
				status.SetText("Enviando para " + printer + "...")
				go func() {
					err := printThermalRaw(printer, order, settings)
					fyne.Do(func() {
						if err != nil {
							printButton.Enable()
							status.SetText("Nao foi possivel imprimir.")
							dialog.ShowError(err, ui.window)
							return
						}
						status.SetText("Recibo enviado para a impressora.")
					})
				}()
			}
			content = append(content,
				container.NewBorder(nil, nil, widget.NewLabel("Impressora"), nil, printerSelect),
				printButton,
				status,
			)
		}
	}
	content = append(content, open, widget.NewLabel(path))
	dialog.ShowCustom("Recibo termico 80 mm", "Fechar", container.NewVBox(content...), ui.window)
}
