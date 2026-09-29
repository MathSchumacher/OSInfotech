package main

import (
	"context"
	"errors"
	"fmt"
	"image/color"
	"path/filepath"
	"strings"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/layout"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
)

// Pesos das colunas da tabela de OS (desktop).
var orderColumns = []float32{0.9, 2.4, 2.8, 1.8, 1.1, 1.2}

// ---------------------------------------------------------------------------
// Widgets do formulario de OS (compartilhados entre desktop e celular).
// ---------------------------------------------------------------------------

func (ui *mainUI) buildOrderWidgets() {
	ui.orderStatusFilter = ""

	ui.searchEntry = newFormEntry()
	if isMobileLayout() {
		ui.searchEntry.SetPlaceHolder("Buscar OS, cliente, CPF, telefone…")
	} else {
		ui.searchEntry.SetPlaceHolder("Buscar por nº da OS, cliente, CPF, telefone, aparelho ou defeito")
	}
	ui.searchEntry.ActionItem = widget.NewIcon(theme.SearchIcon())
	// A busca roda a cada tecla e reconstroi a lista inteira. Com uma espera
	// curta ela roda uma vez por pausa, e a digitacao nao engasga quando a base
	// crescer.
	ui.searchEntry.OnChanged = func(string) { ui.scheduleOrderSearch() }

	// No celular o filtro e um Select; as duas filas entram como as primeiras
	// opcoes, traduzidas de e para o valor interno logo abaixo.
	filterOptions := append([]string{"Todos os status", queueLabel(queueNeedsQuote), queueLabel(queueNeedsAnswer)}, orderStatuses...)
	ui.statusFilter = widget.NewSelect(filterOptions, func(label string) {
		ui.orderStatusFilter = filterValueFromLabel(label)
		ui.refreshList()
	})
	ui.statusFilter.Selected = "Todos os status"

	ui.statusChips = newChipGroup(func(value string) {
		ui.orderStatusFilter = value
		ui.refreshList()
	})
	ui.statusChips.Add("Todas", "")
	// As duas filas primeiro: sao as perguntas que o balcao faz de manha
	// ("o que eu preciso orçar?", "quem não me respondeu?"), e nenhuma delas
	// tinha resposta com os status sozinhos.
	ui.statusChips.Add(queueLabel(queueNeedsQuote), filterNeedsQuote)
	ui.statusChips.Add(queueLabel(queueNeedsAnswer), filterNeedsAnswer)
	for _, status := range orderStatuses {
		ui.statusChips.Add(status, status)
	}

	ui.onlyMineCheck = widget.NewCheck("Só minhas OS", func(bool) { ui.refreshList() })
	ui.listSummary = newText("", sizeCaption, false, inkMuted)

	ui.orderList = widget.NewList(
		func() int { return len(ui.orders) },
		func() fyne.CanvasObject {
			row := newOrderRow()
			row.onStatus = ui.showStatusMenu
			return row
		},
		func(id widget.ListItemID, obj fyne.CanvasObject) {
			if id < 0 || id >= len(ui.orders) {
				return
			}
			obj.(*orderRow).update(ui.orders[id])
		},
	)
	ui.orderList.HideSeparators = false
	ui.orderList.OnSelected = func(id widget.ListItemID) {
		if ui.suppressSelect || id < 0 || id >= len(ui.orders) {
			return
		}
		order := ui.orders[id]
		// O Fyne ignora um clique numa linha que ja esta selecionada, entao
		// voltar da OS e clicar nela de novo nao fazia nada. Soltando a selecao
		// aqui, a mesma linha pode ser reaberta quantas vezes quiser.
		ui.orderList.Unselect(id)
		ui.openOrder(order)
	}

	// ---- editor ----
	ui.statusSelect = widget.NewSelect(orderStatuses, func(status string) {
		if ui.detailStatus != nil {
			ui.detailStatus.Set(status, orderStatusTone(status))
		}
	})
	ui.statusSelect.SetSelected(statusPending)
	ui.detailStatus = newStatusPill(statusPending, orderStatusTone(statusPending))

	ui.customerPicker = newCustomerPicker(ui, "Buscar cliente por nome, CPF, telefone ou aparelho",
		func(profile CustomerProfile) { ui.applyCustomerProfile(profile, true) },
		func() { ui.clearCustomerFields() })

	ui.nameEntry = newFormEntry()
	ui.nameEntry.SetPlaceHolder("Nome completo")
	ui.cpfEntry = newNumericEntry()
	ui.cpfEntry.SetPlaceHolder("000.000.000-00")
	attachMask(ui.cpfEntry, maskCPF, func(text string) {
		if ui.suppressCustomerLookup || ui.customerPicker.selected != nil {
			return
		}
		ui.customerPicker.recognizeByField(text, "cpf", func(p CustomerProfile) { ui.applyCustomerProfile(p, false) })
	})
	ui.phoneEntry = newNumericEntry()
	ui.phoneEntry.SetPlaceHolder("(00) 00000-0000")
	attachMask(ui.phoneEntry, maskPhone, func(text string) {
		ui.refreshPhoneWarning()
		if ui.suppressCustomerLookup || ui.customerPicker.selected != nil {
			return
		}
		ui.customerPicker.recognizeByField(text, "phone", func(p CustomerProfile) { ui.applyCustomerProfile(p, false) })
	})

	// Contato alternativo: a mae, a esposa, o filho que atende. Onze OS da base
	// escreviam isso dentro do campo do telefone porque nao havia onde, e a
	// mascara de hoje nem deixa mais.
	ui.contactNameEntry = newFormEntry()
	ui.contactNameEntry.SetPlaceHolder("Mãe, esposa, filho…")
	ui.contactPhoneEntry = newNumericEntry()
	ui.contactPhoneEntry.SetPlaceHolder("(00) 00000-0000")
	attachMask(ui.contactPhoneEntry, maskPhone, nil)
	ui.phoneWarning = container.NewVBox()
	ui.phoneWarning.Hide()

	ui.avatarImage = canvas.NewImageFromResource(theme.AccountIcon())
	ui.avatarImage.SetMinSize(fyne.NewSize(64, 64))
	ui.avatarImage.FillMode = canvas.ImageFillContain
	ui.avatarImage.CornerRadius = 32
	ui.avatarStatus = widget.NewLabel("Sem foto")
	ui.avatarStatus.Importance = widget.LowImportance
	avatarButton := ghostButton("Foto do cliente", theme.FileImageIcon(), func() { ui.chooseAvatar() })
	ui.avatarBox = container.NewHBox(ui.avatarImage, gap(8, 1), container.NewVBox(avatarButton, ui.avatarStatus))

	ui.deviceEntry = newFormEntry()
	ui.deviceEntry.SetPlaceHolder("Celular, notebook, PC, TV, console…")
	ui.modelEntry = newFormEntry()
	ui.modelEntry.SetPlaceHolder("Ex.: Samsung Galaxy S21")
	ui.serialEntry = newFormEntry()
	ui.deviceKeyEntry = newFormEntry()
	ui.deviceKeyEntry.SetPlaceHolder("1234, 0000, sem senha…")
	ui.deviceKeyEntry.OnChanged = func(text string) {
		// Digitar no campo desfaz o desenho: sao duas formas da mesma coisa, e
		// a ultima que o operador mexeu e a que vale.
		if !isPatternKey(ui.currentDeviceKey) {
			ui.currentDeviceKey = text
		}
		ui.refreshNotePasswordHint()
	}
	// A linha troca de forma conforme o que esta guardado: senha digitada usa o
	// campo de texto; desenho mostra a miniatura e o resumo, porque um campo
	// desabilitado com "Padrão de 5 pontos" dentro parece campo morto, e nao
	// informacao.
	ui.deviceKeyRow = container.NewStack()
	ui.patternBtn = secondaryButton("Desenhar padrão", theme.GridIcon(), func() {
		ui.showPatternDialog(ui.currentDeviceKey, func(value string) {
			ui.setDeviceKey(value)
			if value == "" {
				ui.notify("Desenho apagado")
				return
			}
			ui.notify(patternSummary(value) + " salvo")
		})
	})
	ui.serialEntry.SetPlaceHolder("Série, IMEI ou patrimônio")
	ui.issueEntry = newFormMultiLine()
	ui.issueEntry.SetPlaceHolder("O que o cliente relatou? Ex.: não liga, tela trincada…")
	ui.issueEntry.SetMinRowsVisible(3)
	ui.accessEntry = newFormMultiLine()
	ui.accessEntry.SetPlaceHolder("Carregador, capa, cabo, controle…")
	ui.accessEntry.SetMinRowsVisible(2)
	ui.notesEntry = newFormMultiLine()
	ui.notesEntry.SetPlaceHolder("Diagnóstico, combinados, peças, prazos…")
	ui.notesEntry.SetMinRowsVisible(4)
	ui.notePasswordBox = container.NewVBox()
	ui.notePasswordBox.Hide()
	ui.notesEntry.OnChanged = func(string) { ui.refreshNotePasswordHint() }

	ui.amountEntry = newNumericEntry()
	ui.amountEntry.SetPlaceHolder("R$ 0,00")
	attachMask(ui.amountEntry, maskMoney, func(string) { ui.updateNetAmountPreview() })
	ui.costEntry = newNumericEntry()
	ui.costEntry.SetPlaceHolder("R$ 0,00")
	attachMask(ui.costEntry, maskMoney, func(string) { ui.updateNetAmountPreview() })
	// Entrada e pagamento parcial. A loja ja faz isso e escreve na anotacao:
	// "100 de entrada", "foi pago 100 dos 220, falta 150".
	ui.paidEntry = newNumericEntry()
	ui.paidEntry.SetPlaceHolder("R$ 0,00")
	attachMask(ui.paidEntry, maskMoney, nil)

	ui.netAmountText = newText("R$ 0,00", sizeDisplay, true, inkPrimary)
	ui.netAmountLabel = widget.NewLabel("")
	ui.netAmountLabel.Wrapping = fyne.TextWrapWord
	ui.techLabel = widget.NewLabel("Aguardando login")
	ui.techLabel.Importance = widget.LowImportance
	ui.techLabel.Wrapping = fyne.TextWrapWord

	ui.deleteOSButton = dangerButton("Excluir OS", theme.DeleteIcon(), func() { ui.deleteCurrent() })
	ui.orderSaveBtn = primaryButton("Salvar OS", theme.DocumentSaveIcon(), func() { ui.saveCurrent() })
}

// ---------------------------------------------------------------------------
// Pagina: lista de OS.
// ---------------------------------------------------------------------------

func (ui *mainUI) buildOrdersPage() fyne.CanvasObject {
	header, _, sub := ui.pageTitle("Operação", "Ordens de serviço", "Carregando…", "", nil,
		primaryButton("Nova OS", theme.ContentAddIcon(), func() { ui.newOrder() }))
	ui.ordersTitleSub = sub

	var filters fyne.CanvasObject
	if isMobileLayout() {
		filters = container.NewVBox(ui.searchEntry, ui.statusFilter, ui.listSummary)
	} else {
		top := container.NewBorder(nil, nil, nil, ui.onlyMineCheck, ui.searchEntry)
		bottom := container.NewBorder(nil, nil, nil, ui.listSummary, container.NewHScroll(ui.statusChips.Object()))
		filters = container.NewVBox(top, gap(1, 4), bottom)
	}
	filterCard := cardPadded(filters, 12)

	ui.ordersEmpty = cardPadded(emptyState(theme.SearchIcon(), "Nenhuma OS encontrada",
		"Ajuste a busca ou limpe os filtros para ver todas as ordens de serviço.",
		ghostButton("Limpar filtros", theme.ContentClearIcon(), func() { ui.clearOrderFilters() })), 32)
	ui.ordersEmpty.Hide()

	var table fyne.CanvasObject
	if isMobileLayout() {
		table = ui.orderList
	} else {
		header := tableHeader(12, orderColumns, "Nº", "Cliente", "Aparelho", "Status", "Valor", "Tempo")
		table = container.NewBorder(header, nil, nil, nil, ui.orderList)
	}
	ui.ordersTable = cardPadded(table, 8)
	ui.ordersPasswordBox = container.NewVBox()
	ui.ordersPasswordBox.Hide()
	ui.refreshOrdersPasswordAlert()
	body := container.NewBorder(container.NewVBox(ui.ordersPasswordBox, filterCard, gap(1, 10)), nil, nil, nil,
		container.NewStack(ui.ordersTable, ui.ordersEmpty))
	return pageScaffold(header, body, false)
}

func (ui *mainUI) orderFiltersActive() bool {
	if ui.searchEntry != nil && strings.TrimSpace(ui.searchEntry.Text) != "" {
		return true
	}
	if ui.onlyMineCheck != nil && ui.onlyMineCheck.Checked {
		return true
	}
	return ui.orderStatusFilter != ""
}

func (ui *mainUI) clearOrderFilters() {
	if ui.searchEntry != nil {
		ui.searchEntry.SetText("")
	}
	if ui.onlyMineCheck != nil {
		ui.onlyMineCheck.SetChecked(false)
	}
	ui.orderStatusFilter = ""
	if ui.statusChips != nil {
		ui.statusChips.Select("")
	}
	if ui.statusFilter != nil {
		ui.statusFilter.Selected = "Todos os status"
		ui.statusFilter.Refresh()
	}
	ui.refreshList()
}

func (ui *mainUI) refreshList() {
	if ui.orderList == nil {
		return
	}
	ctx := context.Background()
	query := ""
	if ui.searchEntry != nil {
		query = ui.searchEntry.Text
	}
	orders, err := ui.store.List(ctx, ui.orderStatusFilter, query)
	if err != nil {
		dialog.ShowError(err, ui.window)
		return
	}
	if ui.onlyMineCheck != nil && ui.onlyMineCheck.Checked && ui.currentUser.ID != 0 {
		filtered := orders[:0]
		for _, o := range orders {
			if o.TechnicianID == ui.currentUser.ID {
				filtered = append(filtered, o)
			}
		}
		orders = filtered
	}
	ui.orders = orders
	ui.orderList.Refresh()
	ui.updateOrderSummary()
	ui.updateCounts()
	if ui.currentID != 0 {
		ui.selectCurrentInList()
	}
}

func (ui *mainUI) updateOrderSummary() {
	count := len(ui.orders)
	if ui.ordersEmpty != nil && ui.ordersTable != nil {
		if count == 0 {
			ui.ordersEmpty.Show()
			ui.ordersTable.Hide()
		} else {
			ui.ordersEmpty.Hide()
			ui.ordersTable.Show()
		}
	}
	if ui.listSummary == nil {
		return
	}
	switch {
	case ui.orderFiltersActive() && count == 0:
		ui.listSummary.SetText("Nenhum resultado para os filtros atuais")
	case ui.orderFiltersActive():
		ui.listSummary.SetText(fmt.Sprintf("%d resultado(s)", count))
	default:
		ui.listSummary.SetText(fmt.Sprintf("%d OS no total", count))
	}
}

// updateCounts atualiza contadores usados nos chips, no menu e no inicio.
func (ui *mainUI) updateCounts() {
	counts, err := ui.store.Counts(context.Background())
	if err != nil {
		return
	}
	if ui.statusChips != nil {
		ui.statusChips.SetCount("", counts["Todos"])
		ui.statusChips.SetCount(filterNeedsQuote, counts[filterNeedsQuote])
		ui.statusChips.SetCount(filterNeedsAnswer, counts[filterNeedsAnswer])
		for _, status := range orderStatuses {
			ui.statusChips.SetCount(status, counts[status])
		}
	}
	if ui.ordersBadge != nil {
		ui.ordersBadge.SetCount(counts[statusPending])
	}
	if ui.ordersTitleSub != nil {
		// O resumo passou a separar as duas esperas: "aguardando aprovação"
		// juntava quem espera preco com quem espera resposta do cliente.
		ui.ordersTitleSub.SetText(fmt.Sprintf("%d OS · %d a orçar · %d esperando o cliente · %d em bancada · %d entregues",
			counts["Todos"], counts[filterNeedsQuote], counts[filterNeedsAnswer],
			counts[statusApproved], counts[statusDelivered]))
	}
	ui.updateHomeTiles(counts)
	ui.refreshOrdersPasswordAlert()
}

func (ui *mainUI) selectCurrentInList() {
	for i, order := range ui.orders {
		if order.ID == ui.currentID {
			ui.selectSilently(ui.orderList, i)
			return
		}
	}
	ui.selectSilently(ui.orderList, -1)
}

// orderRow e a linha da tabela de OS (desktop: colunas; celular: empilhada).
type orderRow struct {
	widget.BaseWidget
	orderID  int64
	onStatus func(int64, fyne.CanvasObject)
	number   *themedText
	customer *themedText
	phone    *themedText
	device   *themedText
	issue    *themedText
	status   *statusPill
	amount   *themedText
	updated  *themedText
	root     fyne.CanvasObject
}

func newOrderRow() *orderRow {
	r := &orderRow{}
	r.number = newText("000000", sizeBody, true, inkPrimary)
	r.customer = cellText("Cliente", true, inkPrimary)
	r.phone = cellText("", false, inkMuted)
	r.device = cellText("Aparelho", false, inkPrimary)
	r.issue = cellText("", false, inkMuted)
	r.status = newStatusPill(statusPending, toneWarning)
	// O status e o que mais muda no dia a dia: da para trocar direto na lista,
	// sem abrir a OS e voltar.
	statusTap := newTapCard(r.status, radiusMd, 2, nil)
	statusTap.paint = func(t designTokens, hovered bool) (color.Color, color.Color) {
		if hovered {
			return t.Hover, t.Border
		}
		return color.Transparent, color.Transparent
	}
	statusTap.OnTapped = func() {
		if r.onStatus != nil && r.orderID != 0 {
			r.onStatus(r.orderID, statusTap)
		}
	}
	r.amount = cellText("R$ 0,00", true, inkPrimary)
	r.amount.SetAlignment(fyne.TextAlignTrailing)
	r.updated = cellText("", false, inkMuted)
	if isMobileLayout() {
		top := container.NewBorder(nil, nil, r.number, statusTap, nil)
		r.root = paddedTRBL(container.NewVBox(top, gap(1, 2), r.customer, r.device, r.updated), 8, 4, 8, 4)
	} else {
		r.root = paddedTRBL(container.New(newColumnsLayout(12, orderColumns...),
			container.NewCenter(r.number),
			container.NewVBox(layout.NewSpacer(), r.customer, r.phone, layout.NewSpacer()),
			container.NewVBox(layout.NewSpacer(), r.device, r.issue, layout.NewSpacer()),
			container.NewCenter(container.NewHBox(statusTap, layout.NewSpacer())),
			vcenter(r.amount),
			vcenter(r.updated),
		), 6, 12, 6, 12)
	}
	r.ExtendBaseWidget(r)
	return r
}

func (r *orderRow) update(order ServiceOrder) {
	r.orderID = order.ID
	r.number.SetText(fmt.Sprintf("%06d", order.ID))
	r.customer.SetText(firstNonEmpty(order.Customer, "Sem cliente"))
	r.phone.SetText(order.Phone)
	device := strings.TrimSpace(order.Device)
	if order.BrandModel != "" {
		device = strings.TrimSpace(device + " · " + order.BrandModel)
	}
	if isMobileLayout() && order.Issue != "" {
		device += " — " + firstLine(order.Issue)
	}
	r.device.SetText(firstNonEmpty(device, "Aparelho não informado"))
	r.issue.SetText(firstLine(order.Issue))
	r.status.Set(order.Status, orderStatusTone(order.Status))
	r.amount.SetText(formatBRL(order.AmountCents))
	// A idade da OS em aberto vale mais que a data da ultima edicao: e ela que
	// diz quem precisa ser cobrado.
	note, tone := orderListNote(order)
	r.updated.SetText(note)
	r.updated.SetInk(inkForTone(tone))
}

func (r *orderRow) CreateRenderer() fyne.WidgetRenderer {
	return widget.NewSimpleRenderer(r.root)
}

// ---------------------------------------------------------------------------
// Pagina: editor de OS.
// ---------------------------------------------------------------------------

func (ui *mainUI) buildOrderEditorPage() fyne.CanvasObject {
	print80 := secondaryButton("Imprimir 80 mm", theme.DocumentPrintIcon(), func() { ui.generateThermalReceipt() })
	receipt := ghostButton("Comprovante", theme.DocumentIcon(), func() { ui.generateReceipt() })
	var actions []fyne.CanvasObject
	if isMobileLayout() {
		actions = []fyne.CanvasObject{print80, receipt, ui.deleteOSButton}
	} else {
		saveNew := secondaryButton("Salvar e nova", theme.ContentAddIcon(), func() { ui.saveAndNew() })
		actions = []fyne.CanvasObject{receipt, print80, ui.deleteOSButton, gap(6, 1), saveNew, ui.orderSaveBtn}
	}
	header, title, meta := ui.pageTitle("Ordem de serviço", "Nova ordem de serviço", "", "Ordens de serviço", func() { ui.showPage(pageOrders) }, actions...)
	ui.detailTitle = title
	ui.detailMeta = meta

	// --- cartoes principais ---
	customerCard := card(container.NewVBox(
		sectionHeader("Cliente"),
		ui.customerPicker.Object(),
		gap(1, 4),
		fieldGrid(2,
			field("Nome", ui.nameEntry, ""),
			field("Telefone", ui.phoneEntry, "WhatsApp de contato"),
		),
		ui.phoneWarning,
		fieldGrid(2,
			field("Quem mais atende", ui.contactNameEntry, "Opcional; quando o cliente não atende"),
			field("Telefone do contato", ui.contactPhoneEntry, ""),
		),
		fieldGrid(2,
			field("CPF", ui.cpfEntry, "Opcional; usado para localizar o cliente"),
			field("Foto", ui.avatarBox, ""),
		),
	))

	// As fichas de sugestao vem do proprio historico da loja: um terco das OS
	// repete um defeito ja escrito antes, e 75% da base e celular.
	deviceSuggest := ui.newSuggestBar(ui.deviceEntry, "device", true, false)
	modelSuggest := ui.newSuggestBar(ui.modelEntry, "brand_model", false, true)
	issueSuggest := ui.newSuggestBar(ui.issueEntry, "issue", false, false)
	// Modelo e defeito juntos definem o servico: sempre que um deles muda, a
	// dica de preco e refeita.
	ui.watchForPriceHint(ui.modelEntry)
	ui.watchForPriceHint(ui.issueEntry)
	deviceCard := card(container.NewVBox(
		sectionHeader("Aparelho"),
		// Cada barra de sugestao fica dentro da propria coluna: em grade
		// separada, a barra escondida some da linha e a outra pula de lugar.
		fieldGrid(2,
			container.NewVBox(field("Tipo de aparelho", ui.deviceEntry, ""), deviceSuggest),
			container.NewVBox(field("Marca e modelo", ui.modelEntry, ""), modelSuggest),
		),
		field("Defeito relatado", ui.issueEntry, "Aparece no comprovante entregue ao cliente"),
		issueSuggest,
		fieldGrid(2,
			field("Senha ou desenho", ui.deviceKeyRow,
				"Fica só no sistema, não sai no comprovante"),
			field("Série / IMEI", ui.serialEntry, ""),
		),
		field("Acessórios deixados", ui.accessEntry, ""),
	))

	intakeCard := card(container.NewVBox(
		sectionHeaderWithAction("Checklist de entrada", ghostButton("Gerenciar listas", theme.SettingsIcon(), func() { ui.openSettingsSection(settingsChecklists) })),
		ui.buildChecklistSection(),
		gap(1, 10),
		sectionHeader("Fotos do aparelho"),
		ui.buildPhotosSection(),
	))

	notesCard := card(container.NewVBox(
		sectionHeader("Observações internas"),
		field("Anotações da bancada", ui.notesEntry, "Fica só no sistema, não sai no comprovante"),
		ui.notePasswordBox,
	))

	// --- trilho lateral ---
	// Secundario de proposito: o primario da tela e o Salvar. Este botao ja e o
	// unico do cartao, entao nao precisa gritar.
	ui.followUpBtn = secondaryButton("Cobrar no WhatsApp", theme.MailSendIcon(), func() { ui.followUpCurrent() })
	ui.contactedLabel = newText("", sizeCaption, false, inkMuted).SetTruncate(true)
	ui.followUpBox = container.NewVBox()
	statusCard := card(container.NewVBox(
		sectionHeader("Andamento"),
		field("Status", ui.statusSelect, ""),
		field("Responsável", ui.techLabel, ""),
		ui.followUpBox,
	))
	ui.costField = field("Custo interno", ui.costEntry, "Peças e mão de obra (só o administrador vê)")
	if !ui.currentUser.IsAdmin() {
		// A pagina e montada na primeira vez que a OS abre, depois do login: sem
		// isto o campo de custo nascia visivel para o tecnico.
		ui.costField.Hide()
	}
	ui.netAmountTitle = newEyebrow("Lucro líquido")
	// Label, e nao themedText: a linha do fechamento passa de quarenta letras e
	// precisa quebrar dentro do trilho de 320px.
	ui.deliveryLabel = widget.NewLabel("")
	ui.deliveryLabel.Wrapping = fyne.TextWrapWord
	deliveryFix := ghostButton("Corrigir fechamento", theme.ContentUndoIcon(), func() {
		order, err := ui.store.Get(context.Background(), ui.currentID)
		if err != nil {
			ui.notifyTone("Não consegui abrir esta OS: "+err.Error(), toneDanger)
			return
		}
		ui.offerDeliveryClose(order)
	})
	ui.deliveryBox = container.NewVBox(gap(1, 10), ui.deliveryLabel, gap(1, 4), deliveryFix)
	ui.deliveryBox.Hide()
	netBlock := container.NewVBox(ui.netAmountTitle, ui.netAmountText, ui.netAmountLabel, ui.deliveryBox)
	ui.priceHintBox = container.NewVBox()
	ui.priceHintBox.Hide()
	valuesCard := card(container.NewVBox(
		sectionHeader("Valores"),
		fieldGrid(2,
			field("Valor cobrado", ui.amountEntry, ""),
			ui.costField,
		),
		fieldGrid(2,
			field("Já pago", ui.paidEntry, "Entrada ou pagamento parcial"),
			gap(1, 1),
		),
		ui.priceHintBox,
	))
	// O resumo fica no trilho: e leitura, nao digitacao.
	summaryCard := card(container.NewVBox(
		sectionHeader("Resumo"),
		sunkenCard(netBlock, 12),
	))

	if isMobileLayout() {
		body := container.NewVBox(statusCard, customerCard, deviceCard, valuesCard, summaryCard, intakeCard, notesCard, gap(1, 60))
		bar := ui.mobileActionBar(ui.orderSaveBtn)
		return container.NewBorder(nil, bar, nil, nil, pageScaffold(header, body, true))
	}

	// O valor vem logo depois do aparelho, e nao no fim do trilho lateral: pela
	// ordem do Tab o painel da direita vinha depois do checklist inteiro, e o
	// campo de valor, que e preenchido em quase toda OS, ficava inalcancavel
	// pelo teclado. No celular a ordem ja era esta.
	main := container.NewVBox(customerCard, deviceCard, valuesCard, intakeCard, notesCard)
	rail := container.NewVBox(statusCard, summaryCard)
	columns := container.NewBorder(nil, nil, nil, container.NewStack(gap(320, 1), rail), main)
	return pageScaffold(header, columns, true)
}

// mobileActionBar fixa o botao principal no rodape (celular).
func (ui *mainUI) mobileActionBar(primary fyne.CanvasObject) fyne.CanvasObject {
	bg := newFill(0, func(t designTokens) color.Color { return t.Surface })
	return container.NewStack(bg, container.NewBorder(hairline(), nil, nil, nil, paddedTRBL(primary, 10, 14, 10, 14)))
}

// openOrder carrega a OS no editor e mostra a pagina.
func (ui *mainUI) openOrder(order ServiceOrder) {
	ui.showPage(pageOrderEditor)
	ui.loadOrder(order)
}

// clearCustomerFields limpa so os dados de cliente (botao "Trocar").
func (ui *mainUI) clearCustomerFields() {
	ui.suppressCustomerLookup = true
	ui.nameEntry.SetText("")
	ui.cpfEntry.SetText("")
	ui.phoneEntry.SetText("")
	ui.lastAutoCPF = ""
	ui.lastAutoPhone = ""
	ui.clearAvatarPreview()
	ui.suppressCustomerLookup = false
}

// applyCustomerProfile preenche o formulario com um cliente conhecido.
// forceName sobrescreve o nome; caso contrario so completa campos vazios.
func (ui *mainUI) applyCustomerProfile(profile CustomerProfile, forceName bool) {
	ui.suppressCustomerLookup = true
	defer func() { ui.suppressCustomerLookup = false }()

	if forceName || strings.TrimSpace(ui.nameEntry.Text) == "" {
		ui.nameEntry.SetText(profile.Name)
	}
	if profile.CPF != "" && canReplaceProfileField(ui.cpfEntry.Text, ui.lastAutoCPF) {
		ui.cpfEntry.SetText(profile.CPF)
		ui.lastAutoCPF = profile.CPF
	}
	if profile.Phone != "" && canReplaceProfileField(ui.phoneEntry.Text, ui.lastAutoPhone) {
		ui.phoneEntry.SetText(profile.Phone)
		ui.lastAutoPhone = profile.Phone
	}
	if profile.LastDevice != "" && strings.TrimSpace(ui.deviceEntry.Text) == "" && ui.currentID == 0 {
		// Sugestao discreta: o ultimo aparelho vai como placeholder, nao como valor.
		ui.deviceEntry.SetPlaceHolder("Último: " + strings.TrimSpace(profile.LastDevice+" "+profile.LastModel))
	}
	ui.currentAvatarURL = profile.AvatarURL
	ui.currentAvatarKey = profile.AvatarKey
	if profile.AvatarURL != "" {
		ui.loadAvatarPreviewFromURL(profile.AvatarURL)
	} else {
		ui.clearAvatarPreview()
	}
}

func canReplaceProfileField(current string, lastAuto string) bool {
	current = strings.TrimSpace(current)
	return current == "" || onlyDigits(current) == onlyDigits(lastAuto)
}

func (ui *mainUI) clearForm() {
	ui.orderFormSeq++
	ui.currentID = 0
	ui.refreshFollowUp(ServiceOrder{})
	ui.suppressCustomerLookup = true
	ui.statusSelect.SetSelected(statusPending)
	ui.cpfEntry.SetText("")
	ui.nameEntry.SetText("")
	ui.phoneEntry.SetText("")
	ui.deviceEntry.SetText("")
	ui.deviceEntry.SetPlaceHolder("Celular, notebook, PC, TV, console…")
	ui.modelEntry.SetText("")
	ui.serialEntry.SetText("")
	ui.setDeviceKey("")
	ui.issueEntry.SetText("")
	ui.accessEntry.SetText("")
	ui.notesEntry.SetText("")
	ui.amountEntry.SetText("")
	ui.costEntry.SetText("")
	ui.currentCostCents = 0
	ui.currentTechnicianID = ui.currentUser.ID
	ui.currentTechnicianName = ui.currentUser.Name
	ui.currentDeliveredAt = time.Time{}
	ui.currentPaidAt = time.Time{}
	ui.currentPaymentMethod = ""
	ui.contactNameEntry.SetText("")
	ui.contactPhoneEntry.SetText("")
	ui.paidEntry.SetText("")
	ui.refreshPhoneWarning()
	ui.refreshDelivery(ServiceOrder{})
	if ui.currentUser.ID == 0 {
		ui.techLabel.SetText("Aguardando login")
	} else {
		ui.techLabel.SetText(ui.currentUser.Name + " (" + ui.currentUser.DisplayRole() + ")")
	}
	if ui.detailTitle != nil {
		ui.detailTitle.SetText("Nova ordem de serviço")
		ui.detailMeta.SetText("Preencha os dados do cliente e do aparelho. Campos essenciais: nome, telefone, aparelho e defeito.")
	}
	if ui.orderList != nil {
		ui.orderList.UnselectAll()
	}
	ui.lastAutoCPF = ""
	ui.lastAutoPhone = ""
	ui.clearAvatarPreview()
	if ui.customerPicker != nil {
		ui.customerPicker.Reset()
	}
	ui.resetChecklistAndPhotos()
	ui.suppressCustomerLookup = false
	ui.updateNetAmountPreview()
	ui.markOrderFormSaved()
}

func (ui *mainUI) loadOrder(order ServiceOrder) {
	ui.orderFormSeq++
	ui.currentID = order.ID
	ui.suppressCustomerLookup = true
	// Um status fora da lista (base antiga, outro aparelho) faria o Select ficar
	// com o valor da OS anterior e gravar esse valor errado no proximo save.
	ensureSelectOption(ui.statusSelect, order.Status)
	ui.statusSelect.SetSelected(order.Status)
	ui.cpfEntry.SetText(order.CPF)
	ui.nameEntry.SetText(order.Customer)
	ui.phoneEntry.SetText(order.Phone)
	ui.deviceEntry.SetText(order.Device)
	ui.deviceEntry.SetPlaceHolder("Celular, notebook, PC, TV, console…")
	ui.modelEntry.SetText(order.BrandModel)
	ui.serialEntry.SetText(order.Serial)
	ui.setDeviceKey(order.DeviceKey)
	ui.issueEntry.SetText(order.Issue)
	ui.accessEntry.SetText(order.Accessories)
	ui.notesEntry.SetText(order.Notes)
	ui.amountEntry.SetText(formatBRL(order.AmountCents))
	ui.currentCostCents = order.CostCents
	ui.currentTechnicianID = order.TechnicianID
	ui.currentTechnicianName = order.TechnicianName
	ui.currentDeliveredAt = order.DeliveredAt
	ui.currentPaidAt = order.PaidAt
	ui.currentPaymentMethod = order.PaymentMethod
	ui.contactNameEntry.SetText(order.ContactName)
	ui.contactPhoneEntry.SetText(order.ContactPhone)
	ui.paidEntry.SetText(formatBRL(order.PaidCents))
	ui.refreshPhoneWarning()
	ui.refreshDelivery(order)
	if ui.currentUser.IsAdmin() {
		ui.costEntry.SetText(formatBRL(order.CostCents))
	} else {
		ui.costEntry.SetText("")
	}
	if strings.TrimSpace(order.TechnicianName) == "" {
		ui.techLabel.SetText("Sem responsável definido")
	} else {
		ui.techLabel.SetText(order.TechnicianName)
	}
	if ui.detailTitle != nil {
		ui.detailTitle.SetText(fmt.Sprintf("OS %06d", order.ID))
		ui.detailMeta.SetText(orderMetaLine(order))
	}
	ui.lastAutoCPF = ""
	ui.lastAutoPhone = ""
	ui.clearAvatarPreview()
	if ui.customerPicker != nil {
		ui.customerPicker.Reset()
		if strings.TrimSpace(order.Customer) != "" {
			ui.customerPicker.SetSelected(CustomerProfile{
				Key: normalizeCustomerName(order.Customer), Name: order.Customer, CPF: order.CPF, Phone: order.Phone,
			}, "")
			ui.loadCustomerAvatarForOrder(order.Customer)
		}
	}
	ui.applyChecklistToForm(order.Checklist)
	ui.currentPhotos = order.Photos
	ui.refreshPhotoThumbs()
	ui.refreshFollowUp(order)
	ui.refreshPriceHint()
	ui.suppressCustomerLookup = false
	ui.markOrderFormSaved()
	ui.updateNetAmountPreview()
}

// loadCustomerAvatarForOrder busca a foto do perfil do cliente da OS.
func (ui *mainUI) loadCustomerAvatarForOrder(name string) {
	go func() {
		profiles, err := ui.store.FindCustomerProfiles(context.Background(), name, 3)
		if err != nil {
			return
		}
		key := normalizeCustomerName(name)
		for _, p := range profiles {
			if p.Key == key {
				profile := p
				fyne.Do(func() {
					if ui.customerPicker.selected != nil && ui.customerPicker.selected.Key == key {
						ui.customerPicker.SetSelected(profile, "")
					}
					ui.currentAvatarURL = profile.AvatarURL
					ui.currentAvatarKey = profile.AvatarKey
					if profile.AvatarURL != "" {
						ui.loadAvatarPreviewFromURL(profile.AvatarURL)
					}
				})
				return
			}
		}
	}()
}

func orderMetaLine(order ServiceOrder) string {
	parts := []string{"Criada em " + formatDate(order.CreatedAt), "atualizada " + relativeDate(order.UpdatedAt)}
	if strings.TrimSpace(order.TechnicianName) != "" {
		parts = append(parts, "responsável: "+order.TechnicianName)
	}
	return strings.Join(parts, " · ")
}

func (ui *mainUI) saveCurrent() {
	order, err := ui.collectOrder()
	if err != nil {
		ui.notifyTone(err.Error(), toneDanger)
		return
	}
	// O CPF pode pertencer a outro cliente: quem decide e o operador.
	if ui.warnOrderCustomerConflict(order, func() { ui.persistOrder() }) {
		return
	}
	ui.persistOrder()
}

// ensureSavedForPrint salva a OS antes de imprimir, em vez de mandar o operador
// salvar primeiro e voltar.
func (ui *mainUI) ensureSavedForPrint() bool {
	if ui.currentID != 0 {
		return true
	}
	return ui.persistOrder()
}

// persistOrder grava a OS ja validada e diz se conseguiu.
func (ui *mainUI) persistOrder() bool {
	order, err := ui.collectOrder()
	if err != nil {
		ui.notifyTone(err.Error(), toneDanger)
		return false
	}
	// Guarda o status anterior para saber se a OS acabou de ficar pronta.
	previousStatus := ""
	if ui.currentID != 0 {
		if before, getErr := ui.store.Get(context.Background(), ui.currentID); getErr == nil {
			previousStatus = before.Status
		}
	}
	if err := ui.store.Save(context.Background(), &order); err != nil {
		dialog.ShowError(err, ui.window)
		return false
	}
	ui.currentID = order.ID
	ui.currentCostCents = order.CostCents
	ui.currentTechnicianID = order.TechnicianID
	ui.currentTechnicianName = order.TechnicianName
	ui.currentDeliveredAt = order.DeliveredAt
	ui.currentPaidAt = order.PaidAt
	ui.currentPaymentMethod = order.PaymentMethod
	ui.paidEntry.SetText(formatBRL(order.PaidCents))
	ui.refreshDelivery(order)
	ui.suppressCustomerLookup = true
	ui.amountEntry.SetText(formatBRL(order.AmountCents))
	if ui.currentUser.IsAdmin() {
		ui.costEntry.SetText(formatBRL(order.CostCents))
	}
	ui.suppressCustomerLookup = false
	ui.updateNetAmountPreview()
	ui.detailTitle.SetText(fmt.Sprintf("OS %06d", order.ID))
	ui.detailMeta.SetText(orderMetaLine(order))
	ui.refreshFollowUp(order)
	ui.markOrderFormSaved()
	ui.refreshList()
	ui.syncD1Async(false)
	ui.notify(fmt.Sprintf("OS %06d salva", order.ID))
	// Ficou pronta agora: e a hora de avisar o cliente, sem ele precisar ligar
	// para saber.
	if order.Status == statusReady && previousStatus != statusReady {
		ui.offerReadyNotice(order)
	}
	// Acabou de ser entregue: e a unica hora em que o custo da peca e o
	// pagamento sao conhecidos.
	if order.Status == statusDelivered && previousStatus != statusDelivered {
		ui.offerDeliveryClose(order)
	}
	return true
}

// saveAndNew salva e ja abre a proxima OS. No balcao uma entrada vem atras da
// outra, e voltar pela navegacao custava dois cliques a cada aparelho.
func (ui *mainUI) saveAndNew() {
	order, err := ui.collectOrder()
	if err != nil {
		ui.notifyTone(err.Error(), toneDanger)
		return
	}
	finish := func() {
		if ui.persistOrder() {
			ui.newOrder()
		}
	}
	if ui.warnOrderCustomerConflict(order, finish) {
		return
	}
	finish()
}

func (ui *mainUI) collectOrder() (ServiceOrder, error) {
	if ui.currentUser.ID == 0 {
		return ServiceOrder{}, errors.New("entre com um usuário para salvar a OS")
	}
	customer := strings.TrimSpace(ui.nameEntry.Text)
	phone := strings.TrimSpace(ui.phoneEntry.Text)
	device := strings.TrimSpace(ui.deviceEntry.Text)
	issue := strings.TrimSpace(ui.issueEntry.Text)

	if customer == "" {
		return ServiceOrder{}, errors.New("informe o nome do cliente")
	}
	if phone == "" {
		return ServiceOrder{}, errors.New("informe o telefone do cliente")
	}
	if device == "" {
		return ServiceOrder{}, errors.New("informe o aparelho")
	}
	if issue == "" {
		return ServiceOrder{}, errors.New("informe o defeito relatado")
	}

	amount, err := parseBRL(ui.amountEntry.Text)
	if err != nil {
		return ServiceOrder{}, err
	}
	cost := ui.currentCostCents
	if ui.currentUser.IsAdmin() {
		cost, err = parseBRL(ui.costEntry.Text)
		if err != nil {
			return ServiceOrder{}, err
		}
	}

	paid, err := parseBRL(ui.paidEntry.Text)
	if err != nil {
		return ServiceOrder{}, err
	}

	technicianID := ui.currentTechnicianID
	technicianName := ui.currentTechnicianName
	if ui.currentID == 0 || technicianID == 0 {
		technicianID = ui.currentUser.ID
		technicianName = ui.currentUser.Name
	}
	if !ui.currentUser.IsAdmin() && technicianID != ui.currentUser.ID {
		return ServiceOrder{}, errors.New("somente o técnico responsável ou um administrador pode alterar esta OS")
	}

	return ServiceOrder{
		ID:             ui.currentID,
		Status:         ui.statusSelect.Selected,
		CPF:            strings.TrimSpace(ui.cpfEntry.Text),
		Customer:       customer,
		Phone:          phone,
		Device:         device,
		BrandModel:     strings.TrimSpace(ui.modelEntry.Text),
		Serial:         strings.TrimSpace(ui.serialEntry.Text),
		DeviceKey:      ui.deviceKeyValue(),
		Issue:          issue,
		Accessories:    strings.TrimSpace(ui.accessEntry.Text),
		Notes:          strings.TrimSpace(ui.notesEntry.Text),
		AmountCents:    amount,
		CostCents:      cost,
		TechnicianID:   technicianID,
		TechnicianName: technicianName,
		Checklist:      ui.collectChecklist(),
		Photos:         ui.currentPhotos,
		DeliveredAt:    ui.currentDeliveredAt,
		PaidAt:         ui.currentPaidAt,
		PaymentMethod:  ui.currentPaymentMethod,
		PaidCents:      paid,
		ContactName:    strings.TrimSpace(ui.contactNameEntry.Text),
		ContactPhone:   strings.TrimSpace(ui.contactPhoneEntry.Text),
	}, nil
}

func (ui *mainUI) updateNetAmountPreview() {
	if ui.netAmountText == nil {
		return
	}
	if !ui.currentUser.IsAdmin() {
		gross, err := parseBRL(ui.amountEntry.Text)
		if err != nil {
			ui.netAmountText.SetText("—")
		} else {
			ui.netAmountText.SetText(formatBRL(gross))
		}
		// O tecnico nao ve custo, entao o numero grande e o valor cobrado: chamar
		// isso de "lucro liquido" mostrava o bruto como se fosse lucro.
		if ui.netAmountTitle != nil {
			ui.netAmountTitle.SetText("VALOR COBRADO")
		}
		ui.netAmountLabel.SetText("O custo interno e o lucro aparecem só para o administrador")
		return
	}
	gross, grossErr := parseBRL(ui.amountEntry.Text)
	cost, costErr := parseBRL(ui.costEntry.Text)
	if grossErr != nil || costErr != nil {
		ui.netAmountText.SetText("Confira os valores")
		ui.netAmountLabel.SetText("")
		return
	}
	// Sem custo lancado nao ha lucro para mostrar: bruto menos zero e o bruto, e
	// chamar isso de lucro e o erro que fazia o painel inteiro mentir.
	if cost <= 0 {
		if ui.netAmountTitle != nil {
			ui.netAmountTitle.SetText("Valor cobrado")
		}
		ui.netAmountText.SetText(formatBRL(gross))
		ui.netAmountLabel.SetText("Sem custo lançado, o lucro desta OS é desconhecido")
		return
	}
	if ui.netAmountTitle != nil {
		ui.netAmountTitle.SetText("Lucro líquido")
	}
	ui.netAmountText.SetText(formatBRL(gross - cost))
	ui.netAmountLabel.SetText(fmt.Sprintf("Cobrado %s − custo %s", formatBRL(gross), formatBRL(cost)))
}

func (ui *mainUI) deleteCurrent() {
	if !ui.requireAdmin("excluir OS") {
		return
	}
	if ui.currentID == 0 {
		ui.notifyTone("Salve ou abra uma OS antes de excluir.", toneWarning)
		return
	}
	id := ui.currentID
	dialog.ShowConfirm("Excluir OS", fmt.Sprintf("Excluir a OS %06d? Esta ação não pode ser desfeita.", id), func(ok bool) {
		if !ok {
			return
		}
		if err := ui.store.Delete(context.Background(), id); err != nil {
			dialog.ShowError(err, ui.window)
			return
		}
		ui.clearForm()
		ui.showPage(pageOrders)
		ui.syncD1Async(false)
		ui.notify(fmt.Sprintf("OS %06d excluída", id))
	}, ui.window)
}

func (ui *mainUI) generateReceipt() {
	if !ui.ensureSavedForPrint() {
		return
	}
	order, err := ui.store.Get(context.Background(), ui.currentID)
	if err != nil {
		dialog.ShowError(err, ui.window)
		return
	}
	outDir := filepath.Join(filepath.Dir(ui.dbPath), "recibos")
	path, err := writeReceiptHTML(order, outDir)
	if err != nil {
		dialog.ShowError(err, ui.window)
		return
	}
	ui.showGeneratedFileDialog("Comprovante gerado", "O comprovante em HTML está pronto para abrir ou imprimir.", path, "")
}

// showGeneratedFileDialog e o dialogo padrao para arquivos gerados.
func (ui *mainUI) showGeneratedFileDialog(title string, message string, path string, publicURL string) {
	pathLabel := widget.NewLabel(path)
	pathLabel.Wrapping = fyne.TextWrapBreak
	openLocal := primaryButton("Abrir arquivo", theme.FolderOpenIcon(), func() {
		uri, err := fileURL(path)
		if err != nil {
			dialog.ShowError(err, ui.window)
			return
		}
		if err := fyne.CurrentApp().OpenURL(uri); err != nil {
			dialog.ShowError(err, ui.window)
		}
	})
	items := []fyne.CanvasObject{mutedLabel(message), pathLabel, openLocal}
	if publicURL != "" {
		items = append(items, ghostButton("Abrir na nuvem", theme.MailAttachmentIcon(), func() {
			if err := openExternalURL(publicURL); err != nil {
				dialog.ShowError(err, ui.window)
			}
		}))
	}
	d := dialog.NewCustom(title, "Fechar", container.NewVBox(items...), ui.window)
	d.Resize(fyne.NewSize(520, 220))
	d.Show()
}
