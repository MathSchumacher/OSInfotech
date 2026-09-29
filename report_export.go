package main

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/xml"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/go-pdf/fpdf"
)

const reportDirName = "relatorios"

func writeFinancialReportPDF(report FinancialReport, dir string) (string, error) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	path := filepath.Join(dir, safeReportFileName(report, "pdf"))
	pdf := fpdf.New("P", "mm", "A4", "")
	pdf.SetTitle(report.Title, false)
	pdf.SetAuthor(appDisplayName, false)
	pdf.SetCreator(appDisplayName, false)
	pdf.SetMargins(10, 10, 10)
	pdf.SetAutoPageBreak(true, 12)
	pdf.AddPage()
	tr := pdf.UnicodeTranslatorFromDescriptor("")

	pdf.SetTextColor(20, 31, 43)
	pdf.SetFont("Arial", "B", 16)
	pdf.CellFormat(0, 8, tr(report.Title), "", 1, "L", false, 0, "")
	pdf.SetFont("Arial", "", 9)
	pdf.SetTextColor(88, 101, 115)
	pdf.CellFormat(0, 5, tr(report.PeriodText), "", 1, "L", false, 0, "")
	pdf.Ln(3)

	writeReportPDFKPI(pdf, tr, "Faturamento bruto", report.GrossCents, 10)
	writeReportPDFKPI(pdf, tr, "Gastos / custos", report.CostCents, 75)
	writeReportPDFKPI(pdf, tr, tr(netLabel(report)), report.NetCents, 140)
	pdf.Ln(16)

	pdf.SetFont("Arial", "B", 8)
	pdf.SetFillColor(229, 235, 242)
	headers := []string{"Data", "OS", "Cliente", "Dispositivo", "Bruto", "Gasto", "Lucro"}
	widths := []float64{22, 16, 46, 44, 22, 22, 22}
	for i, header := range headers {
		pdf.CellFormat(widths[i], 7, tr(header), "1", 0, "L", true, 0, "")
	}
	pdf.Ln(7)
	pdf.SetFont("Arial", "", 7.5)
	if len(report.Orders) == 0 {
		pdf.CellFormat(194, 7, tr("Nenhuma OS entregue no periodo."), "1", 1, "L", false, 0, "")
	} else {
		for _, order := range report.Orders {
			pdf.CellFormat(widths[0], 6, tr(order.DeliveredAt.Local().Format("02/01/2006")), "1", 0, "L", false, 0, "")
			pdf.CellFormat(widths[1], 6, tr(fmt.Sprintf("%06d", order.ID)), "1", 0, "L", false, 0, "")
			pdf.CellFormat(widths[2], 6, tr(trimForReport(order.Customer, 28)), "1", 0, "L", false, 0, "")
			pdf.CellFormat(widths[3], 6, tr(trimForReport(firstNonEmpty(order.Device, order.BrandModel), 26)), "1", 0, "L", false, 0, "")
			pdf.CellFormat(widths[4], 6, tr(formatBRL(order.AmountCents)), "1", 0, "R", false, 0, "")
			pdf.CellFormat(widths[5], 6, tr(formatBRL(order.CostCents)), "1", 0, "R", false, 0, "")
			pdf.CellFormat(widths[6], 6, tr(formatBRL(order.NetCents())), "1", 1, "R", false, 0, "")
		}
	}
	pdf.SetFont("Arial", "B", 8)
	pdf.CellFormat(128, 7, tr("TOTAL"), "1", 0, "R", true, 0, "")
	pdf.CellFormat(22, 7, tr(formatBRL(report.GrossCents)), "1", 0, "R", true, 0, "")
	pdf.CellFormat(22, 7, tr(formatBRL(report.CostCents)), "1", 0, "R", true, 0, "")
	pdf.CellFormat(22, 7, tr(formatBRL(report.NetCents)), "1", 1, "R", true, 0, "")
	return path, pdf.OutputFileAndClose(path)
}

func writeReportPDFKPI(pdf *fpdf.Fpdf, tr func(string) string, label string, cents int64, x float64) {
	y := pdf.GetY()
	pdf.SetXY(x, y)
	pdf.SetFillColor(246, 248, 251)
	pdf.SetDrawColor(204, 213, 221)
	pdf.Rect(x, y, 58, 16, "DF")
	pdf.SetXY(x+3, y+3)
	pdf.SetFont("Arial", "", 7.5)
	pdf.SetTextColor(88, 101, 115)
	pdf.CellFormat(52, 4, tr(label), "", 1, "L", false, 0, "")
	pdf.SetXY(x+3, y+8)
	pdf.SetFont("Arial", "B", 11)
	pdf.SetTextColor(20, 31, 43)
	pdf.CellFormat(52, 5, tr(formatBRL(cents)), "", 1, "L", false, 0, "")
}

func writeFinancialReportXLSX(report FinancialReport, dir string) (string, error) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	path := filepath.Join(dir, safeReportFileName(report, "xlsx"))
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	files := map[string]string{
		"[Content_Types].xml":        xlsxContentTypes(),
		"_rels/.rels":                xlsxRootRels(),
		"docProps/app.xml":           xlsxAppProps(),
		"docProps/core.xml":          coreProps(report.Title),
		"xl/workbook.xml":            xlsxWorkbook(),
		"xl/_rels/workbook.xml.rels": xlsxWorkbookRels(),
		"xl/styles.xml":              xlsxStyles(),
		"xl/worksheets/sheet1.xml":   xlsxSheet(report),
	}
	for name, content := range files {
		w, err := zw.Create(name)
		if err != nil {
			_ = zw.Close()
			return "", err
		}
		if _, err := w.Write([]byte(content)); err != nil {
			_ = zw.Close()
			return "", err
		}
	}
	if err := zw.Close(); err != nil {
		return "", err
	}
	return path, os.WriteFile(path, buf.Bytes(), 0o644)
}

func writeFinancialReportDOCX(report FinancialReport, dir string) (string, error) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	path := filepath.Join(dir, safeReportFileName(report, "docx"))
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	files := map[string]string{
		"[Content_Types].xml":          docxContentTypes(),
		"_rels/.rels":                  docxRootRels(),
		"docProps/app.xml":             docxAppProps(),
		"docProps/core.xml":            coreProps(report.Title),
		"word/_rels/document.xml.rels": `<?xml version="1.0" encoding="UTF-8" standalone="yes"?><Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships"/>`,
		"word/styles.xml":              docxStyles(),
		"word/document.xml":            docxDocument(report),
	}
	for name, content := range files {
		w, err := zw.Create(name)
		if err != nil {
			_ = zw.Close()
			return "", err
		}
		if _, err := w.Write([]byte(content)); err != nil {
			_ = zw.Close()
			return "", err
		}
	}
	if err := zw.Close(); err != nil {
		return "", err
	}
	return path, os.WriteFile(path, buf.Bytes(), 0o644)
}

func financialReportOutputDir(dbPath string) string {
	return filepath.Join(filepath.Dir(dbPath), reportDirName)
}

// netLabel nomeia o resultado do periodo conforme o custo esteja completo ou
// nao. Um PDF que sai da loja chamando de lucro um bruto menos custo pela
// metade e um numero que alguem vai usar para formar preco.
func netLabel(report FinancialReport) string {
	if report.CostIsPartial() {
		return "Resultado parcial (falta custo)"
	}
	return "Lucro liquido"
}

func buildFinancialReport(ctx context.Context, store *Store, mode string, baseDate time.Time) (FinancialReport, error) {
	start, end, title := financialReportPeriod(mode, baseDate)
	return store.FinancialReport(ctx, start, end, title)
}

func financialReportPeriod(mode string, baseDate time.Time) (time.Time, time.Time, string) {
	if baseDate.IsZero() {
		baseDate = time.Now()
	}
	local := baseDate.Local()
	switch strings.ToLower(strings.TrimSpace(mode)) {
	case "mensal":
		start := time.Date(local.Year(), local.Month(), 1, 0, 0, 0, 0, time.Local)
		end := start.AddDate(0, 1, 0)
		return start, end, "Relatorio financeiro mensal"
	default:
		weekday := int(local.Weekday())
		if weekday == 0 {
			weekday = 7
		}
		start := time.Date(local.Year(), local.Month(), local.Day(), 0, 0, 0, 0, time.Local).AddDate(0, 0, -(weekday - 1))
		end := start.AddDate(0, 0, 7)
		return start, end, "Relatorio financeiro semanal"
	}
}

func safeReportFileName(report FinancialReport, ext string) string {
	start := report.Start.Local().Format("2006-01-02")
	end := report.End.Local().Add(-time.Nanosecond).Format("2006-01-02")
	prefix := "RELATORIO-FINANCEIRO"
	if strings.Contains(strings.ToLower(report.Title), "mensal") {
		prefix = "RELATORIO-MENSAL"
	} else if strings.Contains(strings.ToLower(report.Title), "semanal") {
		prefix = "RELATORIO-SEMANAL"
	}
	return fmt.Sprintf("%s-%s-a-%s.%s", prefix, start, end, ext)
}

func trimForReport(value string, max int) string {
	runes := []rune(strings.TrimSpace(value))
	if len(runes) <= max {
		return string(runes)
	}
	if max <= 1 {
		return string(runes[:max])
	}
	return string(runes[:max-1]) + "."
}

func centsAsDecimal(cents int64) string {
	return fmt.Sprintf("%.2f", float64(cents)/100)
}

func xmlText(value string) string {
	var buf bytes.Buffer
	_ = xml.EscapeText(&buf, []byte(value))
	return buf.String()
}

func coreProps(title string) string {
	now := time.Now().UTC().Format(time.RFC3339)
	return `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>` +
		`<cp:coreProperties xmlns:cp="http://schemas.openxmlformats.org/package/2006/metadata/core-properties" xmlns:dc="http://purl.org/dc/elements/1.1/" xmlns:dcterms="http://purl.org/dc/terms/" xmlns:dcmitype="http://purl.org/dc/dcmitype/" xmlns:xsi="http://www.w3.org/2001/XMLSchema-instance">` +
		`<dc:title>` + xmlText(title) + `</dc:title><dc:creator>DBS Assistencia Tecnica</dc:creator><cp:lastModifiedBy>DBS Assistencia Tecnica</cp:lastModifiedBy>` +
		`<dcterms:created xsi:type="dcterms:W3CDTF">` + now + `</dcterms:created><dcterms:modified xsi:type="dcterms:W3CDTF">` + now + `</dcterms:modified></cp:coreProperties>`
}

func xlsxSheet(report FinancialReport) string {
	var b strings.Builder
	lastDataRow := 4 + len(report.Orders)
	if len(report.Orders) == 0 {
		lastDataRow = 5
	}
	totalRow := lastDataRow + 1
	b.WriteString(`<?xml version="1.0" encoding="UTF-8" standalone="yes"?><worksheet xmlns="http://schemas.openxmlformats.org/spreadsheetml/2006/main"><sheetViews><sheetView workbookViewId="0"><pane ySplit="4" topLeftCell="A5" activePane="bottomLeft" state="frozen"/></sheetView></sheetViews><cols>`)
	widths := []int{13, 10, 26, 22, 14, 14, 14, 14}
	for i, width := range widths {
		b.WriteString(fmt.Sprintf(`<col min="%d" max="%d" width="%d" customWidth="1"/>`, i+1, i+1, width))
	}
	b.WriteString(`</cols><sheetData>`)
	b.WriteString(xlsxRow(1, []string{xlsxStringCell("A1", report.Title, 1)}))
	b.WriteString(xlsxRow(2, []string{xlsxStringCell("A2", report.PeriodText, 0)}))
	b.WriteString(xlsxRow(4, []string{
		xlsxStringCell("A4", "Data", 3), xlsxStringCell("B4", "OS", 3), xlsxStringCell("C4", "Cliente", 3), xlsxStringCell("D4", "Dispositivo", 3),
		xlsxStringCell("E4", "Status", 3), xlsxStringCell("F4", "Faturamento bruto", 3), xlsxStringCell("G4", "Gastos", 3), xlsxStringCell("H4", netLabel(report), 3),
	}))
	if len(report.Orders) == 0 {
		b.WriteString(xlsxRow(5, []string{
			xlsxStringCell("A5", "", 0), xlsxStringCell("B5", "", 0), xlsxStringCell("C5", "Nenhuma OS entregue no periodo.", 0), xlsxStringCell("D5", "", 0),
			xlsxStringCell("E5", "", 0), xlsxNumberCell("F5", "0", 2), xlsxNumberCell("G5", "0", 2), xlsxNumberCell("H5", "0", 2),
		}))
	} else {
		for i, order := range report.Orders {
			row := 5 + i
			b.WriteString(xlsxRow(row, []string{
				xlsxStringCell(fmt.Sprintf("A%d", row), order.DeliveredAt.Local().Format("02/01/2006"), 0),
				xlsxNumberCell(fmt.Sprintf("B%d", row), fmt.Sprintf("%d", order.ID), 0),
				xlsxStringCell(fmt.Sprintf("C%d", row), order.Customer, 0),
				xlsxStringCell(fmt.Sprintf("D%d", row), firstNonEmpty(order.Device, order.BrandModel), 0),
				xlsxStringCell(fmt.Sprintf("E%d", row), order.Status, 0),
				xlsxNumberCell(fmt.Sprintf("F%d", row), centsAsDecimal(order.AmountCents), 2),
				xlsxNumberCell(fmt.Sprintf("G%d", row), centsAsDecimal(order.CostCents), 2),
				xlsxNumberCell(fmt.Sprintf("H%d", row), centsAsDecimal(order.NetCents()), 2),
			}))
		}
	}
	b.WriteString(xlsxRow(totalRow, []string{
		xlsxStringCell(fmt.Sprintf("E%d", totalRow), "TOTAL", 3),
		xlsxFormulaCell(fmt.Sprintf("F%d", totalRow), fmt.Sprintf("SUM(F5:F%d)", lastDataRow), 2),
		xlsxFormulaCell(fmt.Sprintf("G%d", totalRow), fmt.Sprintf("SUM(G5:G%d)", lastDataRow), 2),
		xlsxFormulaCell(fmt.Sprintf("H%d", totalRow), fmt.Sprintf("SUM(H5:H%d)", lastDataRow), 2),
	}))
	b.WriteString(`</sheetData><autoFilter ref="A4:H` + fmt.Sprint(lastDataRow) + `"/><pageMargins left="0.4" right="0.4" top="0.6" bottom="0.6" header="0.3" footer="0.3"/></worksheet>`)
	return b.String()
}

func xlsxRow(index int, cells []string) string {
	return fmt.Sprintf(`<row r="%d">%s</row>`, index, strings.Join(cells, ""))
}

func xlsxStringCell(ref string, value string, style int) string {
	return fmt.Sprintf(`<c r="%s" s="%d" t="inlineStr"><is><t>%s</t></is></c>`, ref, style, xmlText(value))
}

func xlsxNumberCell(ref string, value string, style int) string {
	return fmt.Sprintf(`<c r="%s" s="%d"><v>%s</v></c>`, ref, style, value)
}

func xlsxFormulaCell(ref string, formula string, style int) string {
	return fmt.Sprintf(`<c r="%s" s="%d"><f>%s</f></c>`, ref, style, xmlText(formula))
}

func xlsxContentTypes() string {
	return `<?xml version="1.0" encoding="UTF-8" standalone="yes"?><Types xmlns="http://schemas.openxmlformats.org/package/2006/content-types"><Default Extension="rels" ContentType="application/vnd.openxmlformats-package.relationships+xml"/><Default Extension="xml" ContentType="application/xml"/><Override PartName="/xl/workbook.xml" ContentType="application/vnd.openxmlformats-officedocument.spreadsheetml.sheet.main+xml"/><Override PartName="/xl/worksheets/sheet1.xml" ContentType="application/vnd.openxmlformats-officedocument.spreadsheetml.worksheet+xml"/><Override PartName="/xl/styles.xml" ContentType="application/vnd.openxmlformats-officedocument.spreadsheetml.styles+xml"/><Override PartName="/docProps/core.xml" ContentType="application/vnd.openxmlformats-package.core-properties+xml"/><Override PartName="/docProps/app.xml" ContentType="application/vnd.openxmlformats-officedocument.extended-properties+xml"/></Types>`
}

func xlsxRootRels() string {
	return `<?xml version="1.0" encoding="UTF-8" standalone="yes"?><Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships"><Relationship Id="rId1" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/officeDocument" Target="xl/workbook.xml"/><Relationship Id="rId2" Type="http://schemas.openxmlformats.org/package/2006/relationships/metadata/core-properties" Target="docProps/core.xml"/><Relationship Id="rId3" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/extended-properties" Target="docProps/app.xml"/></Relationships>`
}

func xlsxWorkbook() string {
	return `<?xml version="1.0" encoding="UTF-8" standalone="yes"?><workbook xmlns="http://schemas.openxmlformats.org/spreadsheetml/2006/main" xmlns:r="http://schemas.openxmlformats.org/officeDocument/2006/relationships"><sheets><sheet name="Relatorio" sheetId="1" r:id="rId1"/></sheets><calcPr calcMode="auto"/></workbook>`
}

func xlsxWorkbookRels() string {
	return `<?xml version="1.0" encoding="UTF-8" standalone="yes"?><Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships"><Relationship Id="rId1" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/worksheet" Target="worksheets/sheet1.xml"/><Relationship Id="rId2" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/styles" Target="styles.xml"/></Relationships>`
}

func xlsxAppProps() string {
	return `<?xml version="1.0" encoding="UTF-8" standalone="yes"?><Properties xmlns="http://schemas.openxmlformats.org/officeDocument/2006/extended-properties" xmlns:vt="http://schemas.openxmlformats.org/officeDocument/2006/docPropsVTypes"><Application>DBS Assistencia Tecnica</Application></Properties>`
}

func xlsxStyles() string {
	return `<?xml version="1.0" encoding="UTF-8" standalone="yes"?><styleSheet xmlns="http://schemas.openxmlformats.org/spreadsheetml/2006/main"><numFmts count="1"><numFmt numFmtId="164" formatCode="&quot;R$&quot; #,##0.00"/></numFmts><fonts count="3"><font><sz val="11"/><name val="Calibri"/></font><font><b/><sz val="16"/><color rgb="FF14202B"/><name val="Calibri"/></font><font><b/><color rgb="FFFFFFFF"/><name val="Calibri"/></font></fonts><fills count="3"><fill><patternFill patternType="none"/></fill><fill><patternFill patternType="gray125"/></fill><fill><patternFill patternType="solid"><fgColor rgb="FF0F766E"/><bgColor indexed="64"/></patternFill></fill></fills><borders count="2"><border><left/><right/><top/><bottom/><diagonal/></border><border><left style="thin"><color rgb="FFCCD5DD"/></left><right style="thin"><color rgb="FFCCD5DD"/></right><top style="thin"><color rgb="FFCCD5DD"/></top><bottom style="thin"><color rgb="FFCCD5DD"/></bottom><diagonal/></border></borders><cellStyleXfs count="1"><xf numFmtId="0" fontId="0" fillId="0" borderId="0"/></cellStyleXfs><cellXfs count="4"><xf numFmtId="0" fontId="0" fillId="0" borderId="1" xfId="0"/><xf numFmtId="0" fontId="1" fillId="0" borderId="0" xfId="0"/><xf numFmtId="164" fontId="0" fillId="0" borderId="1" xfId="0" applyNumberFormat="1"/><xf numFmtId="0" fontId="2" fillId="2" borderId="1" xfId="0" applyFill="1" applyFont="1"/></cellXfs><cellStyles count="1"><cellStyle name="Normal" xfId="0" builtinId="0"/></cellStyles></styleSheet>`
}

func docxDocument(report FinancialReport) string {
	var b strings.Builder
	b.WriteString(`<?xml version="1.0" encoding="UTF-8" standalone="yes"?><w:document xmlns:w="http://schemas.openxmlformats.org/wordprocessingml/2006/main"><w:body>`)
	b.WriteString(docxParagraph(report.Title, "Title"))
	b.WriteString(docxParagraph(report.PeriodText, "Normal"))
	b.WriteString(docxParagraph("Resumo", "Heading1"))
	b.WriteString(docxSummaryTable(report))
	b.WriteString(docxParagraph("Ordens entregues no periodo", "Heading1"))
	b.WriteString(docxOrdersTable(report))
	b.WriteString(`<w:sectPr><w:pgSz w:w="11906" w:h="16838"/><w:pgMar w:top="850" w:right="720" w:bottom="850" w:left="720" w:header="360" w:footer="360" w:gutter="0"/></w:sectPr></w:body></w:document>`)
	return b.String()
}

func docxSummaryTable(report FinancialReport) string {
	rows := [][]string{
		{"Faturamento bruto", formatBRL(report.GrossCents)},
		{"Gastos / custos", formatBRL(report.CostCents)},
		{netLabel(report), formatBRL(report.NetCents)},
		{"OS entregues", fmt.Sprint(len(report.Orders))},
	}
	return docxTable(rows, true)
}

func docxOrdersTable(report FinancialReport) string {
	rows := [][]string{{"Data", "OS", "Cliente", "Dispositivo", "Bruto", "Gasto", "Lucro"}}
	if len(report.Orders) == 0 {
		rows = append(rows, []string{"", "", "Nenhuma OS entregue no periodo.", "", "R$ 0,00", "R$ 0,00", "R$ 0,00"})
	} else {
		for _, order := range report.Orders {
			rows = append(rows, []string{
				order.DeliveredAt.Local().Format("02/01/2006"),
				fmt.Sprintf("%06d", order.ID),
				order.Customer,
				firstNonEmpty(order.Device, order.BrandModel),
				formatBRL(order.AmountCents),
				formatBRL(order.CostCents),
				formatBRL(order.NetCents()),
			})
		}
	}
	return docxTable(rows, true)
}

func docxTable(rows [][]string, header bool) string {
	var b strings.Builder
	b.WriteString(`<w:tbl><w:tblPr><w:tblW w:w="0" w:type="auto"/><w:tblBorders><w:top w:val="single" w:sz="4" w:color="CCD5DD"/><w:left w:val="single" w:sz="4" w:color="CCD5DD"/><w:bottom w:val="single" w:sz="4" w:color="CCD5DD"/><w:right w:val="single" w:sz="4" w:color="CCD5DD"/><w:insideH w:val="single" w:sz="4" w:color="CCD5DD"/><w:insideV w:val="single" w:sz="4" w:color="CCD5DD"/></w:tblBorders></w:tblPr>`)
	for r, row := range rows {
		b.WriteString(`<w:tr>`)
		for _, cell := range row {
			if header && r == 0 {
				b.WriteString(`<w:tc><w:tcPr><w:shd w:fill="0F766E"/></w:tcPr>`)
				b.WriteString(docxRunParagraph(cell, true, "FFFFFF"))
				b.WriteString(`</w:tc>`)
			} else {
				b.WriteString(`<w:tc>`)
				b.WriteString(docxRunParagraph(cell, false, "14202B"))
				b.WriteString(`</w:tc>`)
			}
		}
		b.WriteString(`</w:tr>`)
	}
	b.WriteString(`</w:tbl>`)
	return b.String()
}

func docxParagraph(text string, style string) string {
	styleXML := ""
	if style != "" && style != "Normal" {
		styleXML = `<w:pPr><w:pStyle w:val="` + xmlText(style) + `"/></w:pPr>`
	}
	return `<w:p>` + styleXML + `<w:r><w:t>` + xmlText(text) + `</w:t></w:r></w:p>`
}

func docxRunParagraph(text string, bold bool, color string) string {
	props := `<w:color w:val="` + color + `"/>`
	if bold {
		props += `<w:b/>`
	}
	return `<w:p><w:r><w:rPr>` + props + `</w:rPr><w:t>` + xmlText(text) + `</w:t></w:r></w:p>`
}

func docxContentTypes() string {
	return `<?xml version="1.0" encoding="UTF-8" standalone="yes"?><Types xmlns="http://schemas.openxmlformats.org/package/2006/content-types"><Default Extension="rels" ContentType="application/vnd.openxmlformats-package.relationships+xml"/><Default Extension="xml" ContentType="application/xml"/><Override PartName="/word/document.xml" ContentType="application/vnd.openxmlformats-officedocument.wordprocessingml.document.main+xml"/><Override PartName="/word/styles.xml" ContentType="application/vnd.openxmlformats-officedocument.wordprocessingml.styles+xml"/><Override PartName="/docProps/core.xml" ContentType="application/vnd.openxmlformats-package.core-properties+xml"/><Override PartName="/docProps/app.xml" ContentType="application/vnd.openxmlformats-officedocument.extended-properties+xml"/></Types>`
}

func docxRootRels() string {
	return `<?xml version="1.0" encoding="UTF-8" standalone="yes"?><Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships"><Relationship Id="rId1" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/officeDocument" Target="word/document.xml"/><Relationship Id="rId2" Type="http://schemas.openxmlformats.org/package/2006/relationships/metadata/core-properties" Target="docProps/core.xml"/><Relationship Id="rId3" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/extended-properties" Target="docProps/app.xml"/></Relationships>`
}

func docxAppProps() string {
	return `<?xml version="1.0" encoding="UTF-8" standalone="yes"?><Properties xmlns="http://schemas.openxmlformats.org/officeDocument/2006/extended-properties" xmlns:vt="http://schemas.openxmlformats.org/officeDocument/2006/docPropsVTypes"><Application>DBS Assistencia Tecnica</Application></Properties>`
}

func docxStyles() string {
	return `<?xml version="1.0" encoding="UTF-8" standalone="yes"?><w:styles xmlns:w="http://schemas.openxmlformats.org/wordprocessingml/2006/main"><w:style w:type="paragraph" w:default="1" w:styleId="Normal"><w:name w:val="Normal"/><w:rPr><w:rFonts w:ascii="Calibri" w:hAnsi="Calibri"/><w:sz w:val="22"/></w:rPr></w:style><w:style w:type="paragraph" w:styleId="Title"><w:name w:val="Title"/><w:rPr><w:b/><w:color w:val="14202B"/><w:sz w:val="32"/></w:rPr></w:style><w:style w:type="paragraph" w:styleId="Heading1"><w:name w:val="Heading 1"/><w:pPr><w:spacing w:before="240" w:after="120"/></w:pPr><w:rPr><w:b/><w:color w:val="0F766E"/><w:sz w:val="24"/></w:rPr></w:style></w:styles>`
}
