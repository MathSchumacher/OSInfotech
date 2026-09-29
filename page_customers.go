package main

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/layout"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
)

var customerColumns = []float32{3, 2.2, 0.8, 1.4}

func (ui *mainUI) buildCustomerWidgets() {
	ui.clientSearchEntry = newFormEntry()
	if isMobileLayout() {
		ui.clientSearchEntry.SetPlaceHolder("Buscar nome, CPF, telefone…")
	} else {
		ui.clientSearchEntry.SetPlaceHolder("Buscar por nome, CPF, telefone ou aparelho")
	}
	ui.clientSearchEntry.ActionItem = widget.NewIcon(theme.SearchIcon())
	ui.clientSearchEntry.OnChanged = func(string) { ui.refreshClients() }
	ui.clientSummary = newText("", sizeCaption, false, inkMuted)

	ui.clientList = widget.NewList(
		func() int { return len(ui.clients) },
		func() fyne.CanvasObject { return newCustomerRow() },
		func(id widget.ListItemID, obj fyne.CanvasObject) {
			if id < 0 || id >= len(ui.clients) {
				return
			}
			obj.(*customerRow).update(ui.clients[id])
		},
	)
	ui.clientList.OnSelected = func(id widget.ListItemID) {
		if ui.suppressSelect || id < 0 || id >= len(ui.clients) {
			return
		}
		ui.openCustomer(ui.clients[id])
	}

	ui.clientNameEntry = newFormEntry()
	ui.clientNameEntry.SetPlaceHolder("Nome completo")
	ui.clientCPFEntry = newMaskedEntry("000.000.000-00", maskCPF)
	ui.clientPhoneEntry = newMaskedEntry("(00) 00000-0000", maskPhone)

	ui.clientAvatarImage = canvas.NewImageFromResource(theme.AccountIcon())
	ui.clientAvatarImage.SetMinSize(fyne.NewSize(72, 72))
	ui.clientAvatarImage.FillMode = canvas.ImageFillContain
	ui.clientAvatarImage.CornerRadius = 36
	ui.clientAvatarStatus = widget.NewLabel("Sem foto")
	ui.clientAvatarStatus.Importance = widget.LowImportance
	clientAvatarButton := ghostButton("Adicionar foto", theme.FileImageIcon(), func() { ui.chooseClientAvatar() })
	ui.clientAvatarBox = container.NewHBox(ui.clientAvatarImage, gap(10, 1), container.NewVBox(clientAvatarButton, ui.clientAvatarStatus))
	ui.clientHistoryBox = container.NewVBox()
	ui.clientSummaryBox = container.NewVBox()
	ui.deleteClientBtn = dangerButton("Excluir", theme.DeleteIcon(), func() { ui.deleteCustomer() })
}

// ---------------------------------------------------------------------------
// Pagina: lista de clientes.
// ---------------------------------------------------------------------------

func (ui *mainUI) buildCustomersPage() fyne.CanvasObject {
	header, _, sub := ui.pageTitle("Cadastro", "Clientes", "Carregando…", "", nil,
		primaryButton("Novo cliente", theme.ContentAddIcon(), func() { ui.newCustomer() }))
	ui.clientsTitleSub = sub

	filters := cardPadded(container.NewBorder(nil, nil, nil, ui.clientSummary, ui.clientSearchEntry), 12)
	ui.clientsDuplicateBox = container.NewVBox()
	ui.clientsDuplicateBox.Hide()
	ui.clientsEmpty = cardPadded(emptyState(theme.AccountIcon(), "Nenhum cliente encontrado",
		"Digite qualquer dado do cliente: nome, CPF, telefone ou o aparelho que ele já trouxe.",
		ghostButton("Limpar busca", theme.ContentClearIcon(), func() { ui.clearClientFilters() })), 32)
	ui.clientsEmpty.Hide()

	var table fyne.CanvasObject
	if isMobileLayout() {
		table = ui.clientList
	} else {
		table = container.NewBorder(tableHeader(12, customerColumns, "Cliente", "Último aparelho", "OS", "Último atendimento"), nil, nil, nil, ui.clientList)
	}
	ui.clientsTable = cardPadded(table, 8)
	body := container.NewBorder(container.NewVBox(ui.clientsDuplicateBox, filters, gap(1, 10)), nil, nil, nil, container.NewStack(ui.clientsTable, ui.clientsEmpty))
	return pageScaffold(header, body, false)
}

func (ui *mainUI) clearClientFilters() {
	if ui.clientSearchEntry != nil {
		ui.clientSearchEntry.SetText("")
	}
	ui.refreshClients()
}

func (ui *mainUI) refreshClients() {
	if ui.clientList == nil {
		return
	}
	clients, err := ui.store.ListCustomerProfiles(context.Background(), ui.clientSearchEntry.Text)
	if err != nil {
		dialog.ShowError(err, ui.window)
		return
	}
	ui.clients = clients
	ui.clientList.Refresh()
	filtered := strings.TrimSpace(ui.clientSearchEntry.Text) != ""
	if ui.clientsEmpty != nil {
		if len(clients) == 0 {
			ui.clientsEmpty.Show()
			ui.clientsTable.Hide()
		} else {
			ui.clientsEmpty.Hide()
			ui.clientsTable.Show()
		}
	}
	switch {
	case filtered && len(clients) == 0:
		ui.clientSummary.SetText("Nenhum resultado")
	case filtered:
		ui.clientSummary.SetText(fmt.Sprintf("%d resultado(s)", len(clients)))
	default:
		ui.clientSummary.SetText(fmt.Sprintf("%d clientes", len(clients)))
	}
	if ui.clientsTitleSub != nil && !filtered {
		ui.clientsTitleSub.SetText(fmt.Sprintf("%d clientes cadastrados", len(clients)))
	}
	ui.refreshDuplicateWarning()
	if ui.currentClientKey != "" {
		for i, client := range ui.clients {
			if client.Key == ui.currentClientKey {
				ui.selectSilently(ui.clientList, i)
				return
			}
		}
	}
	ui.selectSilently(ui.clientList, -1)
}

type customerRow struct {
	widget.BaseWidget
	av      *avatar
	name    *themedText
	contact *themedText
	device  *themedText
	count   *themedText
	seen    *themedText
	root    fyne.CanvasObject
}

func newCustomerRow() *customerRow {
	r := &customerRow{}
	r.av = newAvatar(34)
	r.name = cellText("Cliente", true, inkPrimary)
	r.contact = cellText("", false, inkMuted)
	r.device = cellText("", false, inkSecondary)
	r.count = cellText("", false, inkSecondary)
	r.seen = cellText("", false, inkMuted)
	texts := container.NewVBox(layout.NewSpacer(), r.name, r.contact, layout.NewSpacer())
	identity := container.NewBorder(nil, nil, container.NewCenter(r.av), nil, paddedTRBL(texts, 0, 0, 0, 10))
	if isMobileLayout() {
		r.root = paddedTRBL(container.NewBorder(nil, nil, nil, container.NewCenter(r.count), identity), 6, 4, 6, 4)
	} else {
		r.root = paddedTRBL(container.New(newColumnsLayout(12, customerColumns...), identity,
			vcenter(r.device), vcenter(r.count), vcenter(r.seen)), 6, 12, 6, 12)
	}
	r.ExtendBaseWidget(r)
	return r
}

func (r *customerRow) update(profile CustomerProfile) {
	r.av.SetName(profile.Name)
	if profile.AvatarURL != "" {
		r.av.SetPhotoURL(profile.AvatarURL)
	}
	r.name.SetText(profile.Name)
	r.contact.SetText(customerMetaLine(CustomerProfile{CPF: profile.CPF, Phone: profile.Phone}))
	r.device.SetText(firstNonEmpty(strings.TrimSpace(profile.LastDevice+" "+profile.LastModel), "—"))
	if profile.Orders > 0 {
		r.count.SetText(fmt.Sprintf("%d OS", profile.Orders))
	} else {
		r.count.SetText("sem OS")
	}
	if profile.LastSeen.IsZero() {
		r.seen.SetText("—")
	} else {
		r.seen.SetText(relativeDate(profile.LastSeen))
	}
}

func (r *customerRow) CreateRenderer() fyne.WidgetRenderer { return widget.NewSimpleRenderer(r.root) }

// ---------------------------------------------------------------------------
// Pagina: cliente (dados + historico).
// ---------------------------------------------------------------------------

func (ui *mainUI) buildCustomerEditorPage() fyne.CanvasObject {
	whatsapp := ghostButton("WhatsApp", theme.MailSendIcon(), func() { ui.openWhatsApp(ui.clientPhoneEntry.Text) })
	newOrder := secondaryButton("Nova OS para o cliente", theme.ContentAddIcon(), func() { ui.newOrderFromCustomer() })
	save := primaryButton("Salvar cliente", theme.DocumentSaveIcon(), func() { ui.saveCustomer() })
	var actions []fyne.CanvasObject
	if isMobileLayout() {
		actions = []fyne.CanvasObject{whatsapp, newOrder, ui.deleteClientBtn}
	} else {
		actions = []fyne.CanvasObject{whatsapp, newOrder, ui.deleteClientBtn, gap(6, 1), save}
	}
	header, title, meta := ui.pageTitle("Cliente", "Novo cliente", "", "Clientes", func() { ui.showPage(pageCustomers) }, actions...)
	ui.clientDetailTitle = title
	ui.clientDetailMeta = meta

	dataCard := card(container.NewVBox(
		sectionHeader("Dados do cliente"),
		ui.clientAvatarBox,
		gap(1, 4),
		field("Nome", ui.clientNameEntry, ""),
		fieldGrid(2,
			field("CPF", ui.clientCPFEntry, ""),
			field("Telefone", ui.clientPhoneEntry, "WhatsApp de contato"),
		),
	))
	historyCard := card(container.NewVBox(
		sectionHeader("Histórico de OS"),
		ui.clientHistoryBox,
	))
	summaryCard := card(container.NewVBox(
		sectionHeader("Resumo"),
		ui.clientSummaryBox,
	))

	if isMobileLayout() {
		body := container.NewVBox(dataCard, summaryCard, historyCard, gap(1, 60))
		return container.NewBorder(nil, ui.mobileActionBar(save), nil, nil, pageScaffold(header, body, true))
	}
	main := container.NewVBox(dataCard, historyCard)
	rail := container.NewStack(gap(300, 1), container.NewVBox(summaryCard))
	return pageScaffold(header, container.NewBorder(nil, nil, nil, rail, main), true)
}

func (ui *mainUI) openWhatsApp(phone string) {
	link := whatsappURL(phone)
	if link == "" {
		ui.notifyTone("Informe um telefone com DDD para abrir o WhatsApp.", toneWarning)
		return
	}
	if err := openExternalURL(link); err != nil {
		dialog.ShowError(err, ui.window)
	}
}

func (ui *mainUI) newCustomer() {
	ui.showPage(pageCustomerEditor)
	ui.clearCustomerForm()
	ui.window.Canvas().Focus(ui.clientNameEntry)
}

func (ui *mainUI) openCustomer(profile CustomerProfile) {
	ui.showPage(pageCustomerEditor)
	ui.loadCustomerProfile(profile)
}

func (ui *mainUI) clearCustomerForm() {
	ui.currentClientKey = ""
	ui.clientNameEntry.SetText("")
	ui.clientCPFEntry.SetText("")
	ui.clientPhoneEntry.SetText("")
	ui.clientAvatarURL = ""
	ui.clientAvatarKey = ""
	ui.clearClientAvatarPreview()
	if ui.clientDetailTitle != nil {
		ui.clientDetailTitle.SetText("Novo cliente")
		ui.clientDetailMeta.SetText("Cadastre o cliente sem precisar abrir uma OS.")
	}
	if ui.clientList != nil {
		ui.clientList.UnselectAll()
	}
	ui.renderCustomerHistory(nil)
}

func (ui *mainUI) loadCustomerProfile(profile CustomerProfile) {
	ui.currentClientKey = profile.Key
	ui.clientNameEntry.SetText(profile.Name)
	ui.clientCPFEntry.SetText(profile.CPF)
	ui.clientPhoneEntry.SetText(profile.Phone)
	ui.clientAvatarURL = profile.AvatarURL
	ui.clientAvatarKey = profile.AvatarKey
	if ui.clientDetailTitle != nil {
		ui.clientDetailTitle.SetText(profile.Name)
		meta := "Cliente cadastrado"
		if profile.Orders > 0 {
			meta = fmt.Sprintf("%d OS no histórico", profile.Orders)
		}
		if !profile.LastSeen.IsZero() {
			meta += " · último atendimento " + relativeDate(profile.LastSeen)
		}
		ui.clientDetailMeta.SetText(meta)
	}
	if profile.AvatarURL != "" {
		ui.loadClientAvatarPreviewFromURL(profile.AvatarURL)
	} else {
		ui.clearClientAvatarPreview()
	}
	ui.loadCustomerHistory(profile.Key)
}

// loadCustomerHistory busca as OS do cliente e monta historico + resumo.
func (ui *mainUI) loadCustomerHistory(key string) {
	if key == "" {
		ui.renderCustomerHistory(nil)
		return
	}
	go func() {
		orders, err := ui.store.List(context.Background(), "", "")
		if err != nil {
			return
		}
		mine := make([]ServiceOrder, 0)
		for _, o := range orders {
			if normalizeCustomerName(o.Customer) == key {
				mine = append(mine, o)
			}
		}
		fyne.Do(func() {
			if ui.currentClientKey != key {
				return
			}
			ui.renderCustomerHistory(mine)
		})
	}()
}

func (ui *mainUI) renderCustomerHistory(orders []ServiceOrder) {
	if ui.clientHistoryBox == nil {
		return
	}
	ui.clientHistoryBox.Objects = nil
	if len(orders) == 0 {
		ui.clientHistoryBox.Add(paddedTRBL(newText("Nenhuma OS registrada para este cliente.", sizeCaption, false, inkMuted), 6, 0, 4, 0))
	}
	for _, o := range orders {
		order := o
		number := newText(fmt.Sprintf("%06d", order.ID), sizeBody, true, inkPrimary)
		device := newText(firstNonEmpty(strings.TrimSpace(order.Device+" "+order.BrandModel), "Aparelho"), sizeBody, true, inkPrimary).SetTruncate(true)
		issue := newText(firstNonEmpty(firstLine(order.Issue), "—")+" · "+formatDate(order.CreatedAt), sizeCaption, false, inkSecondary).SetTruncate(true)
		amount := newText(formatBRL(order.AmountCents), sizeBody, true, inkPrimary)
		right := container.NewVBox(container.NewHBox(newStatusPill(order.Status, orderStatusTone(order.Status))), amount)
		body := container.NewBorder(nil, nil, container.NewCenter(number), container.NewCenter(right),
			paddedTRBL(container.NewVBox(device, issue), 0, 8, 0, 12))
		ui.clientHistoryBox.Add(newTapCard(body, radiusMd, 8, func() { ui.openOrder(order) }))
	}
	ui.clientHistoryBox.Refresh()

	if ui.clientSummaryBox == nil {
		return
	}
	var open, delivered int
	var paid int64
	var last string
	for i, o := range orders {
		switch o.Status {
		case statusDelivered:
			delivered++
			paid += o.AmountCents
		case statusPending, statusApproved, statusReady:
			open++
		}
		if i == 0 {
			last = relativeDate(o.UpdatedAt)
		}
	}
	total, _ := keyValue("Total de OS", fmt.Sprint(len(orders)))
	opened, _ := keyValue("Em aberto", fmt.Sprint(open))
	done, _ := keyValue("Entregues", fmt.Sprint(delivered))
	money, _ := keyValue("Total pago", formatBRL(paid))
	lastRow, _ := keyValue("Última OS", firstNonEmpty(last, "—"))
	ui.clientSummaryBox.Objects = []fyne.CanvasObject{total, opened, done, money, lastRow}
	ui.clientSummaryBox.Refresh()
}

func (ui *mainUI) deleteCustomer() {
	if !ui.requireAdmin("excluir clientes") {
		return
	}
	if ui.currentClientKey == "" {
		ui.notifyTone("Abra um cliente para excluir.", toneWarning)
		return
	}
	key := ui.currentClientKey
	dialog.ShowConfirm("Excluir cliente", "Excluir o cadastro deste cliente? As OS antigas continuam preservadas.", func(ok bool) {
		if !ok {
			return
		}
		if err := ui.store.DeleteCustomerProfile(context.Background(), key); err != nil {
			dialog.ShowError(err, ui.window)
			return
		}
		ui.clearCustomerForm()
		ui.showPage(pageCustomers)
		ui.syncD1Async(false)
		ui.notify("Cliente excluído")
	}, ui.window)
}

func (ui *mainUI) newOrderFromCustomer() {
	name := strings.TrimSpace(ui.clientNameEntry.Text)
	if name == "" {
		ui.notifyTone("Salve o cliente antes de abrir uma OS.", toneWarning)
		return
	}
	profile := CustomerProfile{
		Key:       normalizeCustomerName(name),
		Name:      name,
		CPF:       strings.TrimSpace(ui.clientCPFEntry.Text),
		Phone:     strings.TrimSpace(ui.clientPhoneEntry.Text),
		AvatarURL: ui.clientAvatarURL,
		AvatarKey: ui.clientAvatarKey,
	}
	ui.clearForm()
	ui.customerPicker.SetSelected(profile, "")
	ui.applyCustomerProfile(profile, true)
	ui.showPage(pageOrderEditor)
	ui.window.Canvas().Focus(ui.deviceEntry)
}

var errNoCustomer = errors.New("cliente nao informado")
