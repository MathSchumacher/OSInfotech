package main

import (
	"context"
	"fmt"
	"strings"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/layout"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
)

var termColumns = []float32{0.9, 2.4, 2.4, 1.5, 2.2, 1.1}

// ---------------------------------------------------------------------------
// Widgets do formulario de termo.
// ---------------------------------------------------------------------------

func (ui *mainUI) buildTermForm() {
	ui.termPreview = newTermPreview()
	ui.termDocumentFilterValue = termFilterAll

	ui.termSearchEntry = newFormEntry()
	if isMobileLayout() {
		ui.termSearchEntry.SetPlaceHolder("Buscar termo, vendedor, CPF, IMEI…")
	} else {
		ui.termSearchEntry.SetPlaceHolder("Buscar por nº do termo, vendedor, comprador, CPF/CNPJ, equipamento ou IMEI")
	}
	ui.termSearchEntry.ActionItem = widget.NewIcon(theme.SearchIcon())
	ui.termSearchEntry.OnChanged = func(string) { ui.refreshTerms() }
	ui.termSummary = newText("", sizeCaption, false, inkMuted)
	ui.termDocumentFilter = widget.NewSelect(termDocumentFilters, func(value string) {
		ui.termDocumentFilterValue = value
		ui.refreshTerms()
	})
	ui.termDocumentFilter.Selected = termFilterAll
	ui.termChips = newChipGroup(func(value string) {
		ui.termDocumentFilterValue = value
		ui.refreshTerms()
	})
	for _, f := range termDocumentFilters {
		ui.termChips.Add(f, f)
	}

	ui.termList = widget.NewList(
		func() int { return len(ui.terms) },
		func() fyne.CanvasObject { return newTermRow() },
		func(id widget.ListItemID, obj fyne.CanvasObject) {
			if id < 0 || id >= len(ui.terms) {
				return
			}
			obj.(*termRow).update(ui.terms[id])
		},
	)
	ui.termList.OnSelected = func(id widget.ListItemID) {
		if ui.suppressSelect || id < 0 || id >= len(ui.terms) {
			return
		}
		ui.openTerm(ui.terms[id])
	}

	ui.termTypeSelect = widget.NewSelect(salesTermTypes, func(string) { ui.refreshTermPreview() })
	ui.termStatusSelect = widget.NewSelect(termStatuses, func(status string) {
		if ui.termDetailStatus != nil {
			ui.termDetailStatus.Set(status, termStatusTone(status))
		}
		ui.refreshTermPreview()
	})
	ui.termDetailStatus = newStatusPill(termStatusDraft, termStatusTone(termStatusDraft))

	ui.termSellerPicker = newCustomerPicker(ui, "Buscar cliente cadastrado (nome, CPF ou telefone)",
		func(profile CustomerProfile) { ui.applyTermSellerProfile(profile, true) },
		func() {})

	ui.termSellerNameEntry = newFormEntry()
	ui.termSellerNameEntry.SetPlaceHolder("Nome do vendedor")
	ui.termSellerCPFEntry = newMaskedEntry("000.000.000-00 ou CNPJ", maskCPF)
	ui.termSellerRGEntry = newFormEntry()
	ui.termSellerRGEntry.SetPlaceHolder("RG ou documento complementar")
	ui.termSellerPhoneEntry = newMaskedEntry("(00) 00000-0000", maskPhone)
	ui.termSellerAddressEntry = newFormEntry()
	ui.termSellerAddressEntry.SetPlaceHolder("Endereço do vendedor")

	ui.termBuyerCompanyEntry = newFormEntry()
	ui.termBuyerCompanyEntry.SetPlaceHolder("Nome da empresa ou comprador")
	ui.termBuyerDocumentEntry = newMaskedEntry("CNPJ ou CPF", maskCPF)
	ui.termBuyerAddressEntry = newFormEntry()
	ui.termBuyerAddressEntry.SetPlaceHolder("Endereço do comprador")
	ui.termBuyerPhoneEntry = newMaskedEntry("(00) 00000-0000", maskPhone)

	ui.termDeviceKindEntry = newFormEntry()
	ui.termDeviceKindEntry.SetPlaceHolder("Celular, notebook, PC, TV…")
	ui.termBrandEntry = newFormEntry()
	ui.termBrandEntry.SetPlaceHolder("Marca")
	ui.termModelEntry = newFormEntry()
	ui.termModelEntry.SetPlaceHolder("Modelo")
	ui.termColorEntry = newFormEntry()
	ui.termColorEntry.SetPlaceHolder("Cor")
	ui.termIMEI1Entry = newFormEntry()
	ui.termIMEI1Entry.SetPlaceHolder("IMEI, série ou patrimônio")
	ui.termIMEI2Entry = newFormEntry()
	ui.termIMEI2Entry.SetPlaceHolder("IMEI 2 ou série secundária")
	ui.termAccessoriesEntry = newFormMultiLine()
	ui.termAccessoriesEntry.SetPlaceHolder("Acessórios inclusos")
	ui.termAccessoriesEntry.SetMinRowsVisible(2)
	ui.termDeclarationEntry = newFormMultiLine()
	ui.termDeclarationEntry.SetPlaceHolder("Declaração do termo")
	ui.termDeclarationEntry.SetMinRowsVisible(5)
	ui.termAmountEntry = newNumericEntry()
	ui.termAmountEntry.SetPlaceHolder("R$ 0,00")
	ui.termPaymentMethodEntry = newFormEntry()
	ui.termPaymentMethodEntry.SetPlaceHolder("Pix, dinheiro, cartão…")
	ui.termCityEntry = newFormEntry()
	ui.termCityEntry.SetPlaceHolder("Cidade/UF")
	ui.termDateEntry = newFormEntry()
	ui.termDateEntry.SetPlaceHolder("dd/mm/aaaa")

	ui.termPDFStatus = widget.NewLabel("PDF não gerado")
	ui.termPDFStatus.Importance = widget.LowImportance
	ui.termPDFStatus.Wrapping = fyne.TextWrapWord
	ui.termSignedStatus = widget.NewLabel("Assinado não anexado")
	ui.termSignedStatus.Importance = widget.LowImportance
	ui.termSignedStatus.Wrapping = fyne.TextWrapWord
	ui.deleteTermBtn = dangerButton("Excluir", theme.DeleteIcon(), func() { ui.deleteTerm() })

	ui.bindTermPreviewUpdates()
	attachMask(ui.termAmountEntry, maskMoney, func(string) { ui.refreshTermPreview() })
	ui.termSellerNameEntry.OnChanged = func(string) { ui.refreshTermPreview() }

	defaults, err := ui.loadTermDefaults(context.Background())
	if err != nil {
		defaults = defaultTermDefaults()
	}
	ui.applyTermDefaultsToForm(defaults)
	ui.termStatusSelect.SetSelected(termStatusDraft)
	ui.termDateEntry.SetText(time.Now().Format("02/01/2006"))
	ui.refreshTermPreview()
}

func (ui *mainUI) bindTermPreviewUpdates() {
	entries := []*dbsEntry{
		ui.termSellerRGEntry, ui.termSellerAddressEntry, ui.termBuyerCompanyEntry,
		ui.termBuyerAddressEntry, ui.termDeviceKindEntry, ui.termBrandEntry, ui.termModelEntry,
		ui.termColorEntry, ui.termIMEI1Entry, ui.termIMEI2Entry, ui.termAccessoriesEntry,
		ui.termDeclarationEntry, ui.termPaymentMethodEntry, ui.termCityEntry, ui.termDateEntry,
	}
	for _, entry := range entries {
		entry.OnChanged = func(string) { ui.refreshTermPreview() }
	}
	// Campos com mascara mantem a mascara e ganham a atualizacao da previa.
	for _, entry := range []*dbsEntry{ui.termSellerCPFEntry, ui.termBuyerDocumentEntry} {
		attachMask(entry, maskCPF, func(string) { ui.refreshTermPreview() })
	}
	for _, entry := range []*dbsEntry{ui.termSellerPhoneEntry, ui.termBuyerPhoneEntry} {
		attachMask(entry, maskPhone, func(string) { ui.refreshTermPreview() })
	}
}

func (ui *mainUI) applyTermSellerProfile(profile CustomerProfile, forceName bool) {
	ui.suppressTermLookup = true
	defer func() { ui.suppressTermLookup = false }()
	if forceName || strings.TrimSpace(ui.termSellerNameEntry.Text) == "" {
		ui.termSellerNameEntry.SetText(profile.Name)
	}
	if profile.CPF != "" && canReplaceProfileField(ui.termSellerCPFEntry.Text, ui.lastTermAutoCPF) {
		ui.termSellerCPFEntry.SetText(profile.CPF)
		ui.lastTermAutoCPF = profile.CPF
	}
	if profile.Phone != "" && canReplaceProfileField(ui.termSellerPhoneEntry.Text, ui.lastTermAutoPhone) {
		ui.termSellerPhoneEntry.SetText(profile.Phone)
		ui.lastTermAutoPhone = profile.Phone
	}
	ui.refreshTermPreview()
}

// ---------------------------------------------------------------------------
// Pagina: lista de termos.
// ---------------------------------------------------------------------------

func (ui *mainUI) buildTermsPage() fyne.CanvasObject {
	header, _, sub := ui.pageTitle("Documentos", "Termos", "Compra e venda de equipamentos usados", "", nil,
		primaryButton("Novo termo", theme.ContentAddIcon(), func() { ui.newTerm() }))
	ui.termsTitleSub = sub

	var filters fyne.CanvasObject
	if isMobileLayout() {
		filters = container.NewVBox(ui.termSearchEntry, ui.termDocumentFilter, ui.termSummary)
	} else {
		filters = container.NewVBox(ui.termSearchEntry, gap(1, 4),
			container.NewBorder(nil, nil, nil, ui.termSummary, container.NewHScroll(ui.termChips.Object())))
	}
	ui.termsEmpty = cardPadded(emptyState(theme.DocumentCreateIcon(), "Nenhum termo encontrado",
		"Os termos de compra e venda ficam aqui, com PDF gerado e documento assinado anexado.",
		ghostButton("Limpar filtros", theme.ContentClearIcon(), func() { ui.clearTermFilters() })), 32)
	ui.termsEmpty.Hide()

	var table fyne.CanvasObject
	if isMobileLayout() {
		table = ui.termList
	} else {
		table = container.NewBorder(tableHeader(12, termColumns, "Nº", "Vendedor", "Equipamento", "Status", "Documentos", "Valor"), nil, nil, nil, ui.termList)
	}
	ui.termsTable = cardPadded(table, 8)
	body := container.NewBorder(container.NewVBox(cardPadded(filters, 12), gap(1, 10)), nil, nil, nil, container.NewStack(ui.termsTable, ui.termsEmpty))
	return pageScaffold(header, body, false)
}

func (ui *mainUI) termFiltersActive() bool {
	if strings.TrimSpace(ui.termSearchEntry.Text) != "" {
		return true
	}
	return ui.termDocumentFilterValue != "" && ui.termDocumentFilterValue != termFilterAll
}

func (ui *mainUI) clearTermFilters() {
	ui.termSearchEntry.SetText("")
	ui.termDocumentFilterValue = termFilterAll
	if ui.termChips != nil {
		ui.termChips.Select(termFilterAll)
	}
	if ui.termDocumentFilter != nil {
		ui.termDocumentFilter.Selected = termFilterAll
		ui.termDocumentFilter.Refresh()
	}
	ui.refreshTerms()
}

func (ui *mainUI) refreshTerms() {
	if ui.termList == nil {
		return
	}
	filter := firstNonEmpty(ui.termDocumentFilterValue, termFilterAll)
	terms, err := ui.store.ListTerms(context.Background(), ui.termSearchEntry.Text, filter)
	if err != nil {
		dialog.ShowError(err, ui.window)
		return
	}
	ui.terms = terms
	ui.termList.Refresh()
	if ui.termsEmpty != nil {
		if len(terms) == 0 {
			ui.termsEmpty.Show()
			ui.termsTable.Hide()
		} else {
			ui.termsEmpty.Hide()
			ui.termsTable.Show()
		}
	}
	switch {
	case ui.termFiltersActive() && len(terms) == 0:
		ui.termSummary.SetText("Nenhum resultado")
	case ui.termFiltersActive():
		ui.termSummary.SetText(fmt.Sprintf("%d resultado(s)", len(terms)))
	default:
		ui.termSummary.SetText(fmt.Sprintf("%d termos", len(terms)))
	}
	if ui.termsTitleSub != nil && !ui.termFiltersActive() {
		ui.termsTitleSub.SetText(fmt.Sprintf("%d termos registrados", len(terms)))
	}
	if ui.currentTermID != 0 {
		for i, term := range ui.terms {
			if term.ID == ui.currentTermID {
				ui.selectSilently(ui.termList, i)
				return
			}
		}
	}
	ui.selectSilently(ui.termList, -1)
}

type termRow struct {
	widget.BaseWidget
	number *themedText
	seller *themedText
	kind   *themedText
	device *themedText
	status *statusPill
	docs   *themedText
	amount *themedText
	root   fyne.CanvasObject
}

func newTermRow() *termRow {
	r := &termRow{}
	r.number = newText("000000", sizeBody, true, inkPrimary)
	r.seller = cellText("Vendedor", true, inkPrimary)
	r.kind = cellText("", false, inkMuted)
	r.device = cellText("", false, inkPrimary)
	r.status = newStatusPill(termStatusDraft, toneWarning)
	r.docs = cellText("", false, inkMuted)
	r.amount = cellText("R$ 0,00", true, inkPrimary)
	r.amount.SetAlignment(fyne.TextAlignTrailing)
	if isMobileLayout() {
		top := container.NewBorder(nil, nil, r.number, r.status, nil)
		r.root = paddedTRBL(container.NewVBox(top, gap(1, 2), r.seller, r.device), 8, 4, 8, 4)
	} else {
		r.root = paddedTRBL(container.New(newColumnsLayout(12, termColumns...),
			container.NewCenter(r.number),
			container.NewVBox(layout.NewSpacer(), r.seller, r.kind, layout.NewSpacer()),
			vcenter(r.device),
			container.NewCenter(container.NewHBox(r.status, layout.NewSpacer())),
			vcenter(r.docs),
			vcenter(r.amount),
		), 6, 12, 6, 12)
	}
	r.ExtendBaseWidget(r)
	return r
}

func (r *termRow) update(term ServiceTerm) {
	r.number.SetText(fmt.Sprintf("%06d", term.ID))
	r.seller.SetText(firstNonEmpty(term.SellerName, "Sem vendedor"))
	r.kind.SetText(firstNonEmpty(term.TermType, defaultTermType))
	device := strings.TrimSpace(term.DeviceKind)
	model := strings.TrimSpace(strings.Join([]string{term.Brand, term.Model}, " "))
	if model != "" {
		device = strings.TrimSpace(device + " · " + model)
	}
	r.device.SetText(firstNonEmpty(device, "—"))
	r.status.Set(firstNonEmpty(term.Status, termStatusDraft), termStatusTone(term.Status))
	r.docs.SetText(termDocumentSummary(term))
	r.amount.SetText(formatBRL(term.AmountCents))
}

func (r *termRow) CreateRenderer() fyne.WidgetRenderer { return widget.NewSimpleRenderer(r.root) }

// ---------------------------------------------------------------------------
// Pagina: editor de termo.
// ---------------------------------------------------------------------------

func (ui *mainUI) buildTermEditorPage() fyne.CanvasObject {
	save := primaryButton("Salvar termo", theme.DocumentSaveIcon(), func() { ui.saveTerm() })
	var actions []fyne.CanvasObject
	if isMobileLayout() {
		actions = []fyne.CanvasObject{ui.deleteTermBtn}
	} else {
		actions = []fyne.CanvasObject{ui.deleteTermBtn, gap(6, 1), save}
	}
	header, title, meta := ui.pageTitle("Termo", "Novo termo", "", "Termos", func() { ui.showPage(pageTerms) }, actions...)
	ui.termDetailTitle = title
	ui.termDetailMeta = meta

	docCard := card(container.NewVBox(
		sectionHeader("Documento"),
		fieldGrid(2,
			field("Tipo de termo", ui.termTypeSelect, ""),
			field("Status", ui.termStatusSelect, ""),
		),
		fieldGrid(2,
			field("Data", ui.termDateEntry, "dd/mm/aaaa"),
			field("Cidade", ui.termCityEntry, ""),
		),
	))
	sellerCard := card(container.NewVBox(
		sectionHeader("Vendedor"),
		ui.termSellerPicker.Object(),
		gap(1, 4),
		field("Nome", ui.termSellerNameEntry, ""),
		fieldGrid(2,
			field("CPF / CNPJ", ui.cnpjLookupField(ui.termSellerCPFEntry, func() { ui.lookupTermCNPJ("seller") }), "CNPJ com 14 dígitos pode ser consultado"),
			field("RG", ui.termSellerRGEntry, ""),
		),
		fieldGrid(2,
			field("Telefone", ui.termSellerPhoneEntry, ""),
			field("Endereço", ui.termSellerAddressEntry, ""),
		),
	))
	buyerCard := card(container.NewVBox(
		sectionHeader("Comprador"),
		field("Empresa ou comprador", ui.termBuyerCompanyEntry, ""),
		fieldGrid(2,
			field("CNPJ / CPF", ui.cnpjLookupField(ui.termBuyerDocumentEntry, func() { ui.lookupTermCNPJ("buyer") }), ""),
			field("Telefone", ui.termBuyerPhoneEntry, ""),
		),
		field("Endereço", ui.termBuyerAddressEntry, ""),
	))
	deviceCard := card(container.NewVBox(
		sectionHeader("Equipamento"),
		fieldGrid(2,
			field("Tipo", ui.termDeviceKindEntry, ""),
			field("Cor", ui.termColorEntry, ""),
		),
		fieldGrid(2,
			field("Marca", ui.termBrandEntry, ""),
			field("Modelo", ui.termModelEntry, ""),
		),
		fieldGrid(2,
			field("IMEI / Série 1", ui.termIMEI1Entry, ""),
			field("IMEI / Série 2", ui.termIMEI2Entry, ""),
		),
		field("Acessórios", ui.termAccessoriesEntry, ""),
	))
	valuesCard := card(container.NewVBox(
		sectionHeader("Valores"),
		fieldGrid(2,
			field("Valor", ui.termAmountEntry, ""),
			field("Forma de pagamento", ui.termPaymentMethodEntry, ""),
		),
		field("Declaração", ui.termDeclarationEntry, "Texto que vai no corpo do termo"),
	))

	genPDF := primaryButton("Gerar PDF", theme.DocumentCreateIcon(), func() { ui.generateTermPDF() })
	openPDF := ghostButton("Abrir PDF", theme.FolderOpenIcon(), func() { ui.openTermPDF() })
	attach := secondaryButton("Anexar assinado", theme.UploadIcon(), func() { ui.attachSignedTerm() })
	openSigned := ghostButton("Abrir assinado", theme.ConfirmIcon(), func() { ui.openSignedTerm() })
	pdfRow := fyne.CanvasObject(container.NewGridWithColumns(2, genPDF, openPDF))
	signedRow := fyne.CanvasObject(container.NewGridWithColumns(2, attach, openSigned))
	if isMobileLayout() {
		pdfRow = container.New(layout.NewRowWrapLayout(), genPDF, openPDF)
		signedRow = container.New(layout.NewRowWrapLayout(), attach, openSigned)
	}
	docsCard := card(container.NewVBox(
		sectionHeader("Documentos"),
		container.NewHBox(ui.termDetailStatus),
		ui.termPDFStatus,
		pdfRow,
		ui.termSignedStatus,
		signedRow,
	))
	previewCard := card(container.NewVBox(
		sectionHeader("Prévia do termo"),
		ui.termPreview.CanvasObject(),
	))

	if isMobileLayout() {
		preview := container.NewHScroll(ui.termPreview.CanvasObject())
		body := container.NewVBox(docsCard, docCard, sellerCard, buyerCard, deviceCard, valuesCard,
			widget.NewAccordion(widget.NewAccordionItem("Prévia do documento", preview)), gap(1, 60))
		return container.NewBorder(nil, ui.mobileActionBar(save), nil, nil, pageScaffold(header, body, true))
	}
	main := container.NewVBox(docCard, sellerCard, buyerCard, deviceCard, valuesCard)
	rail := container.NewStack(gap(360, 1), container.NewVBox(docsCard, previewCard))
	return pageScaffold(header, container.NewBorder(nil, nil, nil, rail, main), true)
}

func (ui *mainUI) newTerm() {
	ui.showPage(pageTermEditor)
	ui.clearTermForm()
}

func (ui *mainUI) openTerm(term ServiceTerm) {
	ui.showPage(pageTermEditor)
	ui.loadTerm(term)
}

func (ui *mainUI) clearTermForm() {
	ui.currentTermID = 0
	ui.suppressTermLookup = true
	defaults, err := ui.loadTermDefaults(context.Background())
	if err != nil {
		defaults = defaultTermDefaults()
	}
	ui.applyTermDefaultsToForm(defaults)
	ui.termStatusSelect.SetSelected(termStatusDraft)
	ui.termDeviceKindEntry.SetText("")
	ui.termBrandEntry.SetText("")
	ui.termModelEntry.SetText("")
	ui.termColorEntry.SetText("")
	ui.termIMEI1Entry.SetText("")
	ui.termIMEI2Entry.SetText("")
	ui.termAccessoriesEntry.SetText("")
	ui.termDateEntry.SetText(time.Now().Format("02/01/2006"))
	if ui.termDetailTitle != nil {
		ui.termDetailTitle.SetText("Novo termo")
		ui.termDetailMeta.SetText("Preencha vendedor, comprador e equipamento. Gere o PDF para assinatura ou anexe o documento assinado.")
	}
	ui.termPDFStatus.SetText("PDF não gerado")
	ui.termSignedStatus.SetText("Assinado não anexado")
	if ui.termList != nil {
		ui.termList.UnselectAll()
	}
	ui.lastTermAutoCPF = ""
	ui.lastTermAutoPhone = ""
	if ui.termSellerPicker != nil {
		ui.termSellerPicker.Reset()
	}
	ui.suppressTermLookup = false
	ui.refreshTermPreview()
}

func (ui *mainUI) loadTerm(term ServiceTerm) {
	ui.currentTermID = term.ID
	ui.suppressTermLookup = true
	ensureSelectOption(ui.termTypeSelect, term.TermType)
	ensureSelectOption(ui.termStatusSelect, term.Status)
	ui.termTypeSelect.SetSelected(firstNonEmpty(term.TermType, defaultTermType))
	ui.termStatusSelect.SetSelected(firstNonEmpty(term.Status, termStatusDraft))
	ui.termSellerNameEntry.SetText(term.SellerName)
	ui.termSellerCPFEntry.SetText(term.SellerCPF)
	ui.termSellerRGEntry.SetText(term.SellerRG)
	ui.termSellerPhoneEntry.SetText(term.SellerPhone)
	ui.termSellerAddressEntry.SetText(term.SellerAddress)
	ui.termBuyerCompanyEntry.SetText(term.BuyerCompany)
	ui.termBuyerDocumentEntry.SetText(term.BuyerDocument)
	ui.termBuyerAddressEntry.SetText(term.BuyerAddress)
	ui.termBuyerPhoneEntry.SetText(term.BuyerPhone)
	ui.termDeviceKindEntry.SetText(term.DeviceKind)
	ui.termBrandEntry.SetText(term.Brand)
	ui.termModelEntry.SetText(term.Model)
	ui.termColorEntry.SetText(term.Color)
	ui.termIMEI1Entry.SetText(term.IMEI1)
	ui.termIMEI2Entry.SetText(term.IMEI2)
	ui.termAccessoriesEntry.SetText(term.Accessories)
	ui.termDeclarationEntry.SetText(firstNonEmpty(term.Declaration, defaultTermDeclaration()))
	ui.termAmountEntry.SetText(formatBRL(term.AmountCents))
	ui.termPaymentMethodEntry.SetText(term.PaymentMethod)
	ui.termCityEntry.SetText(term.City)
	ui.termDateEntry.SetText(formatTermDate(term.TermDate))
	if ui.termDetailTitle != nil {
		ui.termDetailTitle.SetText(fmt.Sprintf("Termo %06d", term.ID))
		ui.termDetailMeta.SetText(fmt.Sprintf("Criado em %s · atualizado %s", formatDate(term.CreatedAt), relativeDate(term.UpdatedAt)))
	}
	ui.updateTermDocumentLabels(term)
	ui.lastTermAutoCPF = ""
	ui.lastTermAutoPhone = ""
	if ui.termSellerPicker != nil {
		ui.termSellerPicker.Reset()
		if strings.TrimSpace(term.SellerName) != "" {
			ui.termSellerPicker.SetSelected(CustomerProfile{Key: normalizeCustomerName(term.SellerName), Name: term.SellerName, CPF: term.SellerCPF, Phone: term.SellerPhone}, "")
		}
	}
	ui.suppressTermLookup = false
	ui.refreshTermPreview()
}
