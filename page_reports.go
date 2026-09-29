package main

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/theme"
)

var reportColumns = []float32{0.9, 2.4, 2.4, 1.1, 1.1, 1.2}

// reportPage e a secao de relatorios financeiros (somente administrador).
type reportPage struct {
	ui       *mainUI
	root     fyne.CanvasObject
	mode     string
	modeChip *chipGroup
	dateEnt  *dbsEntry
	period   *themedText
	status   *themedText
	tiles    map[string]*statTile
	rows     *fyne.Container
	current  FinancialReport
	restrict fyne.CanvasObject
	body     fyne.CanvasObject
}

func newReportPage(ui *mainUI) *reportPage {
	p := &reportPage{ui: ui, mode: "Semanal", tiles: map[string]*statTile{}}
	header, _, _ := ui.pageTitle("Gestão", "Relatórios", "Faturamento, custos e lucro por período", "", nil,
		ghostButton("PDF", theme.DocumentIcon(), func() { p.export("pdf") }),
		ghostButton("Word", theme.DocumentCreateIcon(), func() { p.export("docx") }),
		primaryButton("Exportar Excel", theme.FileApplicationIcon(), func() { p.export("xlsx") }),
	)

	p.modeChip = newChipGroup(func(value string) {
		p.mode = value
		p.refresh()
	})
	p.modeChip.Add("Semanal", "Semanal")
	p.modeChip.Add("Mensal", "Mensal")
	p.dateEnt = newFormEntry()
	p.dateEnt.SetText(time.Now().Format("02/01/2006"))
	p.dateEnt.SetPlaceHolder("dd/mm/aaaa")
	p.dateEnt.OnSubmitted = func(string) { p.refresh() }
	p.period = newText("", sizeBody, true, inkPrimary)
	p.status = newText("", sizeCaption, false, inkMuted)
	controls := cardPadded(container.NewVBox(
		fieldGrid(3,
			field("Período", p.modeChip.Object(), ""),
			field("Data base", container.NewBorder(nil, nil, nil, ghostButton("Atualizar", theme.ViewRefreshIcon(), func() { p.refresh() }), p.dateEnt), "Qualquer dia dentro da semana ou do mês"),
			field("Intervalo", p.period, ""),
		),
		p.status,
	), 14)

	// Quatro ladrilhos, como antes: cinco nao cabem na linha sem espremer o
	// valor, que e o que se le de longe. A contagem de OS virou subtitulo do
	// faturamento porque ela ja aparece na lista logo abaixo, enquanto o que o
	// cliente ficou devendo nao aparecia em lugar nenhum.
	p.tiles["gross"] = newStatTile("Faturamento bruto", theme.DocumentIcon(), toneBrand)
	p.tiles["cost"] = newStatTile("Custos", theme.WarningIcon(), toneWarning)
	p.tiles["net"] = newStatTile("Lucro líquido", theme.ListIcon(), toneInfo)
	p.tiles["unpaid"] = newStatTile("A receber", theme.WarningIcon(), toneDanger)
	order := []fyne.CanvasObject{p.tiles["gross"], p.tiles["cost"], p.tiles["net"], p.tiles["unpaid"]}
	var tiles fyne.CanvasObject
	if isMobileLayout() {
		tiles = container.NewVBox(order...)
	} else {
		tiles = container.NewGridWithColumns(4, order...)
	}

	p.rows = container.NewVBox()
	var table fyne.CanvasObject
	if isMobileLayout() {
		table = p.rows
	} else {
		table = container.NewVBox(tableHeader(12, reportColumns, "Nº", "Cliente", "Aparelho", "Valor", "Custo", "Entregue em"), p.rows)
	}
	listCard := card(container.NewVBox(sectionHeader("OS entregues no período"), table))

	p.body = container.NewVBox(controls, tiles, listCard)
	p.restrict = cardPadded(emptyState(theme.WarningIcon(), "Acesso restrito",
		"Somente o administrador consulta faturamento, custos e lucro.", nil), 32)
	p.root = pageScaffold(header, container.NewStack(p.body, p.restrict), true)
	return p
}

// costCoverageText diz em quantas OS do periodo o custo foi lancado. Sem isso o
// total de custos parece baixo por eficiencia, e nao por falta de registro.
func costCoverageText(report FinancialReport) string {
	total := len(report.Orders)
	if total == 0 {
		return ""
	}
	if report.MissingCostOrders == 0 {
		return "custo lançado em todas"
	}
	return fmt.Sprintf("%d de %d OS sem custo", report.MissingCostOrders, total)
}

func (p *reportPage) Object() fyne.CanvasObject { return p.root }

func (p *reportPage) refresh() {
	if !p.ui.currentUser.IsAdmin() {
		p.body.Hide()
		p.restrict.Show()
		return
	}
	p.restrict.Hide()
	p.body.Show()
	baseDate, err := parseTermDate(p.dateEnt.Text)
	if err != nil {
		p.status.SetText(err.Error())
		return
	}
	report, err := buildFinancialReport(context.Background(), p.ui.store, p.mode, baseDate)
	if err != nil {
		p.status.SetText(err.Error())
		return
	}
	p.current = report
	p.period.SetText(report.PeriodText)
	p.status.SetText("")
	p.tiles["gross"].SetValue(formatBRL(report.GrossCents))
	p.tiles["gross"].SetSub(fmt.Sprintf("%d OS entregues", len(report.Orders)))
	p.tiles["cost"].SetValue(formatBRL(report.CostCents))
	p.tiles["cost"].SetSub(costCoverageText(report))
	p.tiles["net"].SetValue(formatBRL(report.NetCents))
	// Com custo faltando, o numero nao e lucro: e bruto menos os custos que
	// alguem lembrou de lancar. O titulo diz isso em vez de esconder.
	if report.CostIsPartial() {
		p.tiles["net"].SetLabel("Resultado parcial")
		p.tiles["net"].SetSub("Não é lucro: falta custo")
	} else {
		p.tiles["net"].SetLabel("Lucro líquido")
		p.tiles["net"].SetSub("")
	}
	p.tiles["unpaid"].SetValue(formatBRL(report.UnpaidCents))
	if report.UnpaidOrders == 0 {
		p.tiles["unpaid"].SetSub("Tudo recebido")
	} else {
		p.tiles["unpaid"].SetSub(fmt.Sprintf("%d OS sem pagamento", report.UnpaidOrders))
	}

	p.rows.Objects = nil
	if len(report.Orders) == 0 {
		p.rows.Add(paddedTRBL(newText("Nenhuma OS entregue neste período.", sizeCaption, false, inkMuted), 8, 0, 4, 0))
	}
	for _, o := range report.Orders {
		order := o
		var row fyne.CanvasObject
		if isMobileLayout() {
			row = container.NewVBox(
				container.NewBorder(nil, nil, newText(fmt.Sprintf("%06d", order.ID), sizeBody, true, inkPrimary), newText(formatBRL(order.AmountCents), sizeBody, true, inkPrimary), nil),
				newText(order.Customer, sizeBody, false, inkPrimary),
				newText(strings.TrimSpace(order.Device+" "+order.BrandModel)+" · "+formatDate(order.DeliveredAt), sizeCaption, false, inkSecondary),
			)
		} else {
			row = container.New(newColumnsLayout(12, reportColumns...),
				newText(fmt.Sprintf("%06d", order.ID), sizeBody, true, inkPrimary),
				cellText(order.Customer, false, inkPrimary),
				cellText(strings.TrimSpace(order.Device+" "+order.BrandModel), false, inkSecondary),
				cellText(formatBRL(order.AmountCents), true, inkPrimary),
				cellText(formatBRL(order.CostCents), false, inkSecondary),
				cellText(formatDate(order.DeliveredAt), false, inkMuted),
			)
		}
		p.rows.Add(newTapCard(paddedTRBL(row, 2, 6, 2, 6), radiusSm, 4, func() { p.ui.openOrder(order) }))
	}
	p.rows.Refresh()
}

func (p *reportPage) export(kind string) {
	if !p.ui.requireAdmin("exportar relatórios") {
		return
	}
	if p.current.Start.IsZero() {
		p.refresh()
	}
	outDir := financialReportOutputDir(p.ui.dbPath)
	var (
		path string
		err  error
	)
	switch strings.ToLower(kind) {
	case "pdf":
		path, err = writeFinancialReportPDF(p.current, outDir)
	case "docx":
		path, err = writeFinancialReportDOCX(p.current, outDir)
	default:
		path, err = writeFinancialReportXLSX(p.current, outDir)
	}
	if err != nil {
		dialog.ShowError(err, p.ui.window)
		return
	}
	p.status.SetText("Exportado: " + filepath.Base(path))
	p.ui.showGeneratedFileDialog("Relatório exportado", "O arquivo foi salvo na pasta de relatórios.", path, "")
}
