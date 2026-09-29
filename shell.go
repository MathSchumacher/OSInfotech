package main

import (
	"context"
	"fmt"
	"image/color"
	"strings"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/driver/desktop"
	"fyne.io/fyne/v2/driver/mobile"
	"fyne.io/fyne/v2/layout"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
)

const (
	sidebarWidth  = 248
	topbarHeight  = 64
	pagePadding   = 28
	mobilePadding = 14
)

// brandTagline e a frase da marca mostrada na barra lateral.
var brandTagline = []string{"Tecnologia", "que mantém", "você conectado"}

// forceMobileLayout permite renderizar o layout de celular no desktop (usado
// pelos screenshots de revisao). nil = detectar pelo dispositivo.
var forceMobileLayout *bool

func isMobileLayout() bool {
	if forceMobileLayout != nil {
		return *forceMobileLayout
	}
	return fyne.CurrentDevice().IsMobile()
}

// ---------------------------------------------------------------------------
// Casca do app.
// ---------------------------------------------------------------------------

func (ui *mainUI) buildShell() fyne.CanvasObject {
	ui.pageHost = container.NewStack()
	ui.globalSearch = newGlobalSearchBox(ui)
	ui.syncPill = newSyncIndicator(func() { ui.openSettingsSection(settingsCloud) })
	ui.installBackHandler()
	if isMobileLayout() {
		return ui.buildMobileShell()
	}

	sidebar := ui.buildSidebar()
	topbar := ui.buildTopbar()
	body := container.NewBorder(topbar, nil, sidebar, nil, ui.pageHost)
	ui.installShortcuts()
	return container.NewStack(newBackgroundArt(), body, ui.toastLayer())
}

// toastLayer posiciona o toast no canto inferior direito sem bloquear cliques.
func (ui *mainUI) toastLayer() fyne.CanvasObject {
	pad := float32(pagePadding)
	bottom := pad
	if isMobileLayout() {
		pad = mobilePadding
		bottom = pad + 56
	}
	row := container.NewHBox(layout.NewSpacer(), ui.toast)
	col := container.NewVBox(layout.NewSpacer(), row)
	return paddedTRBL(col, 0, pad, bottom, pad)
}

// sidebarGlass e o painel da barra lateral: quase opaco, com borda fina.
func sidebarGlass(t designTokens) glassStyle {
	return glassStyle{Fill: t.GlassStrong, Border: t.GlassBorder, BorderW: 1, Radius: 0}
}

func (ui *mainUI) buildSidebar() fyne.CanvasObject {
	logo := newBrandImage(brandWide, fyne.NewSize(sidebarWidth-24, 104))
	brand := paddedTRBL(container.NewCenter(logo), 10, 12, 4, 12)

	ui.ordersBadge = newNavBadge()
	ui.navItems[navHome] = newNavItem("Início", theme.HomeIcon(), func() { ui.showPage(pageHome) })
	ui.navItems[navOrders] = newNavItem("Ordens de serviço", theme.DocumentIcon(), func() { ui.showPage(pageOrders) })
	ui.navItems[navOrders].badge = ui.ordersBadge
	ui.navItems[navCustomers] = newNavItem("Clientes", theme.AccountIcon(), func() { ui.showPage(pageCustomers) })
	ui.navItems[navTerms] = newNavItem("Termos", theme.DocumentCreateIcon(), func() { ui.showPage(pageTerms) })
	ui.navItems[navReports] = newNavItem("Relatórios", theme.ListIcon(), func() { ui.showPage(pageReports) })
	ui.navItems[navSettings] = newNavItem("Configurações", theme.SettingsIcon(), func() { ui.showPage(pageSettings) })

	nav := container.NewVBox(
		ui.navItems[navHome],
		gap(1, 4),
		ui.navItems[navOrders],
		gap(1, 4),
		ui.navItems[navCustomers],
		gap(1, 4),
		ui.navItems[navTerms],
		gap(1, 14),
		paddedTRBL(newEyebrow("Gestão"), 0, 0, 0, 12),
		gap(1, 6),
		ui.navItems[navReports],
		gap(1, 4),
		ui.navItems[navSettings],
	)

	footer := container.NewVBox(ui.buildTagline(), gap(1, 12), ui.buildUserFooter())
	body := container.NewBorder(brand, paddedTRBL(footer, 0, 14, 16, 14), nil, nil, paddedTRBL(nav, 8, 14, 12, 14))
	width := gap(sidebarWidth, 1)
	panel := newGlassPanel(nil, 0, sidebarGlass)
	return container.NewStack(panel, width, body)
}

// buildTagline e a assinatura da marca com o filete vermelho.
func (ui *mainUI) buildTagline() fyne.CanvasObject {
	rule := newFill(2, func(t designTokens) color.Color { return t.Brand })
	rule.minSize = fyne.NewSize(3, 1)
	lines := make([]fyne.CanvasObject, 0, len(brandTagline))
	for _, line := range brandTagline {
		lines = append(lines, newEyebrow(line))
	}
	text := container.NewVBox(lines...)
	return container.NewHBox(rule, gap(10, 1), text)
}

func (ui *mainUI) buildUserFooter() fyne.CanvasObject {
	ui.userAv = newAvatar(38)
	ui.userAv.SetName("?")
	ui.userName = newText("Aguardando login", sizeBody, true, inkPrimary).SetTruncate(true)
	ui.userRole = newText("v"+appVersion, sizeCaption, false, inkMuted).SetTruncate(true)
	chevron := newToneIcon(theme.NavigateNextIcon(), toneNeutral, 16)
	body := container.NewBorder(nil, nil, container.NewCenter(ui.userAv), container.NewCenter(chevron),
		paddedTRBL(container.NewVBox(ui.userName, ui.userRole), 0, 6, 0, 10))
	card := newTapCard(body, radiusMd, 10, func() { ui.showUserMenu(ui.userAv) })
	card.paint = func(t designTokens, hovered bool) (color.Color, color.Color) {
		if hovered {
			return t.Hover, t.GlassBorder
		}
		return t.GlassSunken, t.GlassBorder
	}
	return card
}

// showUserMenu abre o menu da conta (usado no rodape e na barra superior).
func (ui *mainUI) showUserMenu(anchor fyne.CanvasObject) {
	themeLabel, themeHint := "Tema claro", "Fundo claro para ambientes iluminados"
	if !isDarkMode() {
		themeLabel, themeHint = "Tema escuro", "Fundo escuro, padrão da DBS"
	}
	name, role := "Aguardando login", "v"+appVersion
	if ui.currentUser.ID != 0 {
		name = ui.currentUser.Name
		role = ui.currentUser.Role + " · DBS"
	}
	ui.showActionMenu(anchor, menuHeader(name, role, name, ""), []menuAction{
		{Label: "Configurações", Icon: theme.SettingsIcon(), Do: func() { ui.showPage(pageSettings) }},
		{Label: themeLabel, Hint: themeHint, Icon: theme.ColorPaletteIcon(), Do: func() { ui.toggleTheme() }},
		{Label: "Sincronizar agora", Hint: ui.syncHintText(), Icon: theme.ViewRefreshIcon(), Do: func() { ui.syncD1Async(true) }},
		{},
		{Label: "Trocar usuário", Icon: theme.LogoutIcon(), Tone: toneDanger, Do: func() { ui.lockSession() }},
	})
}

// syncHintText resume quando foi a ultima sincronizacao, para o menu.
func (ui *mainUI) syncHintText() string {
	value, found, err := ui.store.GetSetting(context.Background(), syncLastSuccessKey)
	if err != nil || !found || strings.TrimSpace(value) == "" {
		return "Ainda não sincronizado neste aparelho"
	}
	when := decodeTime(value)
	if when.IsZero() {
		return "Ainda não sincronizado neste aparelho"
	}
	elapsed := time.Since(when)
	switch {
	case elapsed < time.Minute:
		return "Atualizado agora mesmo"
	case elapsed < time.Hour:
		return fmt.Sprintf("Atualizado há %d min", int(elapsed.Minutes()))
	case elapsed < 24*time.Hour:
		return fmt.Sprintf("Atualizado há %d h", int(elapsed.Hours()))
	default:
		return "Atualizado em " + when.Local().Format("02/01 15:04")
	}
}

func (ui *mainUI) buildTopbar() fyne.CanvasObject {
	search := container.NewGridWrap(fyne.NewSize(560, 40), ui.globalSearch.Object())
	ui.alertBell = newAlertBell(func() { ui.openOrdersWithStatus(statusPending) })
	ui.userChip = ui.buildUserChip()
	right := container.NewHBox(container.NewCenter(ui.syncPill), gap(10, 1), container.NewCenter(ui.alertBell), gap(10, 1), ui.userChip)
	row := container.NewBorder(nil, nil, container.NewCenter(search), right, nil)
	content := paddedTRBL(row, 12, pagePadding, 8, pagePadding)
	height := gap(1, topbarHeight)
	return container.NewStack(height, content)
}

// buildUserChip e o cartao do usuario na barra superior.
func (ui *mainUI) buildUserChip() fyne.CanvasObject {
	ui.chipAv = newAvatar(32)
	ui.chipAv.SetName("?")
	ui.chipName = newText("Aguardando login", sizeBody, true, inkPrimary)
	ui.chipSub = newText(appShortName+" Assistência", sizeCaption, false, inkMuted)
	chevron := newToneIcon(theme.MenuDropDownIcon(), toneNeutral, 18)
	body := container.NewHBox(ui.chipAv, gap(8, 1), container.NewVBox(ui.chipName, ui.chipSub), gap(8, 1), container.NewCenter(chevron))
	card := newTapCard(body, radiusMd, 6, func() { ui.showUserMenu(ui.userChip) })
	card.paint = func(t designTokens, hovered bool) (color.Color, color.Color) {
		if hovered {
			return t.Hover, t.GlassBorder
		}
		return nrgbaAlpha(t.Glass, float64(t.Glass.A)/255*0.7), t.GlassBorder
	}
	return card
}

// ---------------------------------------------------------------------------
// Casca do celular: barra superior compacta + navegacao inferior.
// ---------------------------------------------------------------------------

func (ui *mainUI) buildMobileShell() fyne.CanvasObject {
	mark := newBrandImage(brandMark, fyne.NewSize(32, 32))
	title := newText(appShortName, sizeSection, true, inkPrimary)
	sub := newText(appTagline, sizeMicro, false, inkMuted)
	brand := container.NewHBox(mark, gap(8, 1), container.NewVBox(title, sub))
	search := iconButton(theme.SearchIcon(), func() { ui.showPage(pageSearch) })
	ui.alertBell = newAlertBell(func() { ui.openOrdersWithStatus(statusPending) })
	account := iconButton(theme.AccountIcon(), func() { ui.showUserMenu(ui.pageHost) })
	top := paddedTRBL(container.NewBorder(nil, nil, brand, container.NewHBox(container.NewCenter(ui.alertBell), search, account), nil), 6, 10, 6, 12)
	topbar := container.NewStack(newGlassPanel(nil, 0, sidebarGlass), top)

	ui.mobileNav[navHome] = newMobileNavItem("Início", theme.HomeIcon(), func() { ui.showPage(pageHome) })
	ui.mobileNav[navOrders] = newMobileNavItem("OS", theme.DocumentIcon(), func() { ui.showPage(pageOrders) })
	ui.mobileNav[navCustomers] = newMobileNavItem("Clientes", theme.AccountIcon(), func() { ui.showPage(pageCustomers) })
	ui.mobileNav[navTerms] = newMobileNavItem("Termos", theme.DocumentCreateIcon(), func() { ui.showPage(pageTerms) })
	ui.mobileNav[navSettings] = newMobileNavItem("Mais", theme.MenuIcon(), func() { ui.showPage(pageSettings) })
	items := container.NewGridWithColumns(5,
		ui.mobileNav[navHome], ui.mobileNav[navOrders], ui.mobileNav[navCustomers], ui.mobileNav[navTerms], ui.mobileNav[navSettings])
	bottom := container.NewStack(newGlassPanel(nil, 0, sidebarGlass), paddedTRBL(items, 4, 4, 6, 4))

	body := container.NewBorder(topbar, bottom, nil, nil, ui.pageHost)
	return container.NewStack(newBackgroundArt(), body, ui.toastLayer())
}

// ---------------------------------------------------------------------------
// Navegacao entre paginas.
// ---------------------------------------------------------------------------

func (ui *mainUI) buildPage(kind pageKind) fyne.CanvasObject {
	switch kind {
	case pageOrders:
		return ui.buildOrdersPage()
	case pageOrderEditor:
		return ui.buildOrderEditorPage()
	case pageCustomers:
		return ui.buildCustomersPage()
	case pageCustomerEditor:
		return ui.buildCustomerEditorPage()
	case pageTerms:
		return ui.buildTermsPage()
	case pageTermEditor:
		return ui.buildTermEditorPage()
	case pageReports:
		ui.reportPage = newReportPage(ui)
		return ui.reportPage.Object()
	case pageSettings:
		ui.settingsPage = newSettingsPage(ui)
		return ui.settingsPage.Object()
	case pageSearch:
		return ui.buildSearchPage()
	default:
		return ui.buildHomePage()
	}
}

func (ui *mainUI) showPage(kind pageKind) {
	// Sair do editor com texto digitado e a forma mais facil de perder um
	// atendimento inteiro: pergunta antes.
	if ui.current == pageOrderEditor && guardedPages[kind] && ui.orderFormDirty() {
		ui.confirmLeaveOrder(kind)
		return
	}
	page, ok := ui.pages[kind]
	if !ok {
		page = ui.buildPage(kind)
		ui.pages[kind] = page
	}
	ui.current = kind
	ui.pageHostSet(page)
	ui.setActiveNav(sectionOf(kind))
	switch kind {
	case pageHome:
		ui.refreshHome()
	case pageOrders:
		ui.refreshList()
	case pageCustomers:
		ui.refreshClients()
	case pageTerms:
		ui.refreshTerms()
	case pageReports:
		if ui.reportPage != nil {
			ui.reportPage.refresh()
		}
	case pageSettings:
		if ui.settingsPage != nil {
			ui.settingsPage.refresh()
		}
	case pageSearch:
		ui.globalSearch.focus()
	}
}

func (ui *mainUI) setActiveNav(section navSection) {
	for s, item := range ui.navItems {
		item.setActive(s == section)
	}
	for s, item := range ui.mobileNav {
		item.setActive(s == section)
	}
}

// openSettingsSection abre a pagina de configuracoes ja em uma secao.
func (ui *mainUI) openSettingsSection(section settingsSection) {
	ui.showPage(pageSettings)
	if ui.settingsPage != nil {
		ui.settingsPage.show(section)
	}
}

// Compatibilidade com o fluxo antigo (chamadas espalhadas nos handlers).
func (ui *mainUI) showOSDetail()     { ui.showPage(pageOrderEditor) }
func (ui *mainUI) showClientDetail() { ui.showPage(pageCustomerEditor) }
func (ui *mainUI) showTermDetail()   { ui.showPage(pageTermEditor) }

// newOrder abre o editor limpo para uma nova OS.
func (ui *mainUI) newOrder() {
	ui.showPage(pageOrderEditor)
	ui.clearForm()
	if ui.customerPicker != nil {
		ui.window.Canvas().Focus(ui.customerPicker.entry)
	}
}

// ---------------------------------------------------------------------------
// Sessao e permissoes.
// ---------------------------------------------------------------------------

func (ui *mainUI) updateSessionLabel() {
	name, role := "Aguardando login", "v"+appVersion
	avatarName := "?"
	if ui.currentUser.ID != 0 {
		name = ui.currentUser.Name
		role = ui.currentUser.DisplayRole() + " · v" + appVersion
		avatarName = ui.currentUser.Name
	}
	if ui.userName != nil {
		ui.userName.SetText(name)
		ui.userRole.SetText(role)
		ui.userAv.SetName(avatarName)
		ui.userAv.SetOnline(ui.currentUser.ID != 0)
	}
	if ui.chipName != nil {
		ui.chipName.SetText(name)
		ui.chipAv.SetName(avatarName)
		if ui.currentUser.ID != 0 {
			ui.chipSub.SetText(ui.currentUser.DisplayRole() + " · " + appShortName)
		}
	}
}

func (ui *mainUI) applyRoleAccess() {
	admin := ui.currentUser.IsAdmin()
	if item, ok := ui.navItems[navReports]; ok {
		if admin {
			item.Show()
		} else {
			item.Hide()
		}
	}
	for _, button := range []*dbsButton{ui.deleteOSButton, ui.deleteClientBtn, ui.deleteTermBtn} {
		if button == nil {
			continue
		}
		if admin {
			button.Enable()
		} else {
			button.Disable()
		}
	}
	if ui.costField != nil {
		if admin {
			ui.costField.Show()
		} else {
			ui.costField.Hide()
		}
	}
	if ui.costEntry != nil {
		if admin {
			ui.costEntry.Enable()
		} else {
			ui.costEntry.SetText("")
			ui.costEntry.Disable()
		}
	}
	ui.updateNetAmountPreview()
	if ui.settingsPage != nil {
		ui.settingsPage.refresh()
	}
}

func (ui *mainUI) toggleTheme() {
	mode := themeDark
	if isDarkMode() {
		mode = themeLight
	}
	applyThemeMode(ui.app, mode)
	if ui.content != nil {
		ui.content.Refresh()
	}
	if ui.settingsPage != nil {
		ui.settingsPage.refresh()
	}
}

// installShortcuts registra atalhos de teclado do desktop.
func (ui *mainUI) installShortcuts() {
	if ui.window == nil {
		return
	}
	bindings := []struct {
		shortcut fyne.Shortcut
		run      func()
	}{
		{&desktop.CustomShortcut{KeyName: fyne.KeyK, Modifier: fyne.KeyModifierControl}, func() { ui.globalSearch.focus() }},
		{&desktop.CustomShortcut{KeyName: fyne.KeyN, Modifier: fyne.KeyModifierControl}, ui.newOrder},
		{&desktop.CustomShortcut{KeyName: fyne.KeyS, Modifier: fyne.KeyModifierControl}, ui.saveCurrentPage},
	}

	c := ui.window.Canvas()
	byName := make(map[string]func(), len(bindings))
	for _, binding := range bindings {
		run := binding.run
		c.AddShortcut(binding.shortcut, func(fyne.Shortcut) { run() })
		byName[binding.shortcut.ShortcutName()] = run
	}

	// O mesmo atalho precisa valer com o cursor dentro de um campo de texto: o
	// Fyne entrega o atalho ao widget focado e nunca chega ate a janela.
	globalShortcutHandler = func(shortcut fyne.Shortcut) bool {
		run, ok := byName[shortcut.ShortcutName()]
		if !ok {
			return false
		}
		run()
		return true
	}
}

// installBackHandler trata o botao Voltar do Android. Sem tratamento o Fyne
// encerra a activity: apertar Voltar no meio de uma OS fechava o app e jogava
// fora o cadastro inteiro, fotos enviadas junto.
func (ui *mainUI) installBackHandler() {
	if ui.window == nil {
		return
	}
	canvas := ui.window.Canvas()
	if canvas == nil {
		return
	}
	canvas.SetOnTypedKey(func(event *fyne.KeyEvent) {
		if event == nil || event.Name != mobile.KeyBack {
			return
		}
		ui.goBackOnePage()
	})
}

// goBackOnePage volta um nivel na navegacao. Na tela inicial nao faz nada: sair
// do app e escolha do sistema, nao um efeito colateral de fechar uma OS.
func (ui *mainUI) goBackOnePage() {
	switch ui.current {
	case pageOrderEditor:
		ui.showPage(pageOrders)
	case pageCustomerEditor:
		ui.showPage(pageCustomers)
	case pageTermEditor:
		ui.showPage(pageTerms)
	case pageHome:
		// raiz: deixa como esta
	default:
		ui.showPage(pageHome)
	}
}

// saveCurrentPage salva o que estiver aberto (Ctrl+S).
func (ui *mainUI) saveCurrentPage() {
	switch ui.current {
	case pageOrderEditor:
		ui.saveCurrent()
	case pageCustomerEditor:
		ui.saveCustomer()
	case pageTermEditor:
		ui.saveTerm()
	}
}

// ---------------------------------------------------------------------------
// Estrutura de pagina: cabecalho + corpo rolavel.
// ---------------------------------------------------------------------------

func pageInset() float32 {
	if isMobileLayout() {
		return mobilePadding
	}
	return pagePadding
}

// pageScaffold empilha o cabecalho fixo e o corpo (rolavel) com o padding da
// pagina. Quando scrollBody e falso, o corpo nao rola (listas cuidam disso).
func pageScaffold(header fyne.CanvasObject, body fyne.CanvasObject, scrollBody bool) fyne.CanvasObject {
	inset := pageInset()
	top := inset
	if !isMobileLayout() {
		top = 6
	}
	head := paddedTRBL(header, top, inset, 0, inset)
	if scrollBody {
		scroll := container.NewVScroll(paddedTRBL(body, 14, inset, inset, inset))
		return container.NewBorder(head, nil, nil, nil, scroll)
	}
	return container.NewBorder(head, nil, nil, nil, paddedTRBL(body, 14, inset, inset, inset))
}

// pageTitle monta o bloco de titulo: filete vermelho, eyebrow, titulo,
// subtitulo e acoes a direita. backLabel/back opcionais criam a volta.
func (ui *mainUI) pageTitle(eyebrow string, title string, subtitle string, backLabel string, back func(), actions ...fyne.CanvasObject) (fyne.CanvasObject, *themedText, *themedText) {
	titleText := newText(title, sizeTitle, true, inkPrimary)
	subText := newText(subtitle, sizeBody, false, inkSecondary)
	texts := container.NewVBox()
	if eyebrow != "" {
		texts.Add(newEyebrowBrand(eyebrow))
		texts.Add(gap(1, 2))
	}
	texts.Add(titleText)
	texts.Add(subText)
	rule := newFill(2, func(t designTokens) color.Color { return t.Brand })
	rule.minSize = fyne.NewSize(3, 1)
	var block fyne.CanvasObject = container.NewBorder(nil, nil, container.NewHBox(rule, gap(12, 1)), nil, texts)

	var left fyne.CanvasObject
	if back != nil {
		if isMobileLayout() {
			left = container.NewCenter(iconButton(theme.NavigateBackIcon(), back))
		} else {
			crumb := ghostButton(backLabel, theme.NavigateBackIcon(), back)
			block = container.NewVBox(container.NewHBox(crumb), gap(1, 4), block)
		}
	}
	var right fyne.CanvasObject
	if len(actions) > 0 && !isMobileLayout() {
		right = container.NewCenter(container.NewHBox(actions...))
	}
	if isMobileLayout() {
		titleText.SetTruncate(true)
		subText.SetTruncate(true)
	}
	row := container.NewBorder(nil, nil, left, right, block)
	items := []fyne.CanvasObject{row}
	if isMobileLayout() && len(actions) > 0 {
		items = append(items, gap(1, 6), container.New(layout.NewRowWrapLayout(), actions...))
	}
	items = append(items, gap(1, 10))
	return container.NewVBox(items...), titleText, subText
}

// ---------------------------------------------------------------------------
// Item de navegacao (barra lateral).
// ---------------------------------------------------------------------------

type navItem struct {
	widget.BaseWidget
	label   string
	icon    fyne.Resource
	onTap   func()
	active  bool
	hovered bool
	badge   *navBadge

	rect    *canvas.Rectangle
	pill    *glassPanel
	iconImg *canvas.Image
	text    *canvas.Text
}

func newNavItem(label string, icon fyne.Resource, onTap func()) *navItem {
	n := &navItem{label: label, icon: icon, onTap: onTap}
	n.rect = canvas.NewRectangle(color.Transparent)
	n.rect.CornerRadius = radiusMd
	n.pill = newGlassPanel(nil, 0, brandPillGlass)
	n.pill.Hide()
	n.iconImg = canvas.NewImageFromResource(themedIcon(icon))
	n.iconImg.FillMode = canvas.ImageFillContain
	n.text = canvas.NewText(label, color.Black)
	n.text.TextSize = sizeBody
	n.text.TextStyle = fyne.TextStyle{Bold: true}
	n.ExtendBaseWidget(n)
	return n
}

func (n *navItem) setActive(active bool) {
	if n.active == active {
		return
	}
	n.active = active
	n.Refresh()
}

func (n *navItem) Tapped(*fyne.PointEvent) {
	if n.onTap != nil {
		n.onTap()
	}
}
func (n *navItem) MouseIn(*desktop.MouseEvent)    { n.hovered = true; n.Refresh() }
func (n *navItem) MouseMoved(*desktop.MouseEvent) {}
func (n *navItem) MouseOut()                      { n.hovered = false; n.Refresh() }
func (n *navItem) Cursor() desktop.Cursor         { return desktop.PointerCursor }

func (n *navItem) MinSize() fyne.Size {
	return fyne.NewSize(sidebarWidth-28, 44)
}

func (n *navItem) CreateRenderer() fyne.WidgetRenderer {
	r := &navItemRenderer{n: n}
	r.Refresh()
	return r
}

type navItemRenderer struct{ n *navItem }

func (r *navItemRenderer) Layout(size fyne.Size) {
	n := r.n
	n.rect.Resize(size)
	n.pill.Resize(size)
	n.pill.Move(fyne.NewPos(0, 0))
	n.iconImg.Resize(fyne.NewSize(20, 20))
	n.iconImg.Move(fyne.NewPos(14, (size.Height-20)/2))
	ts := n.text.MinSize()
	n.text.Resize(ts)
	n.text.Move(fyne.NewPos(46, (size.Height-ts.Height)/2))
	if n.badge != nil {
		bs := n.badge.MinSize()
		n.badge.Resize(bs)
		n.badge.Move(fyne.NewPos(size.Width-bs.Width-12, (size.Height-bs.Height)/2))
	}
}
func (r *navItemRenderer) MinSize() fyne.Size { return r.n.MinSize() }
func (r *navItemRenderer) Refresh() {
	t := tk()
	n := r.n
	switch {
	case n.active:
		n.pill.Show()
		n.rect.FillColor = color.Transparent
		n.text.Color = t.InkOnBrand
		n.iconImg.Resource = onBrandIcon(n.icon)
	case n.hovered:
		n.pill.Hide()
		n.rect.FillColor = t.Hover
		n.text.Color = t.Ink
		n.iconImg.Resource = themedIcon(n.icon)
	default:
		n.pill.Hide()
		n.rect.FillColor = color.Transparent
		n.text.Color = t.InkSecondary
		n.iconImg.Resource = themedIcon(n.icon)
	}
	n.pill.Refresh()
	n.rect.Refresh()
	n.iconImg.Refresh()
	n.text.Refresh()
	if n.badge != nil {
		n.badge.onBrand = n.active
		n.badge.Refresh()
	}
}
func (r *navItemRenderer) Objects() []fyne.CanvasObject {
	objs := []fyne.CanvasObject{r.n.pill, r.n.rect, r.n.iconImg, r.n.text}
	if r.n.badge != nil {
		objs = append(objs, r.n.badge)
	}
	return objs
}
func (r *navItemRenderer) Destroy() {}

// navBadge e o contador em pilula dentro do item de navegacao.
type navBadge struct {
	widget.BaseWidget
	count   int
	onBrand bool
	rect    *canvas.Rectangle
	text    *canvas.Text
}

func newNavBadge() *navBadge {
	b := &navBadge{}
	b.rect = canvas.NewRectangle(color.Transparent)
	b.rect.CornerRadius = 10
	b.text = canvas.NewText("", color.White)
	b.text.TextSize = sizeMicro
	b.text.TextStyle = fyne.TextStyle{Bold: true}
	b.text.Alignment = fyne.TextAlignCenter
	b.ExtendBaseWidget(b)
	b.Hide()
	return b
}

func (b *navBadge) SetCount(count int) {
	b.count = count
	b.text.Text = fmt.Sprint(count)
	if count > 0 {
		b.Show()
	} else {
		b.Hide()
	}
	b.Refresh()
}

func (b *navBadge) MinSize() fyne.Size {
	if b.count <= 0 {
		return fyne.NewSize(0, 0)
	}
	return fyne.NewSize(fyne.Max(22, b.text.MinSize().Width+12), 20)
}

func (b *navBadge) CreateRenderer() fyne.WidgetRenderer {
	r := &navBadgeRenderer{b: b}
	r.Refresh()
	return r
}

type navBadgeRenderer struct{ b *navBadge }

func (r *navBadgeRenderer) Layout(size fyne.Size) {
	r.b.rect.Resize(size)
	r.b.text.Resize(size)
}
func (r *navBadgeRenderer) MinSize() fyne.Size { return r.b.MinSize() }
func (r *navBadgeRenderer) Refresh() {
	t := tk()
	if r.b.onBrand {
		r.b.rect.FillColor = rgba(0xFFFFFF, 0.28)
	} else {
		r.b.rect.FillColor = t.Brand
	}
	r.b.text.Color = t.InkOnBrand
	r.b.rect.Refresh()
	r.b.text.Refresh()
}
func (r *navBadgeRenderer) Objects() []fyne.CanvasObject {
	return []fyne.CanvasObject{r.b.rect, r.b.text}
}
func (r *navBadgeRenderer) Destroy() {}

// ---------------------------------------------------------------------------
// Sino de alertas (OS aguardando aprovacao).
// ---------------------------------------------------------------------------

type alertBell struct {
	widget.BaseWidget
	count   int
	onTap   func()
	hovered bool
	bg      *canvas.Circle
	icon    *canvas.Image
	badge   *canvas.Circle
	text    *canvas.Text
}

func newAlertBell(onTap func()) *alertBell {
	b := &alertBell{onTap: onTap}
	b.bg = canvas.NewCircle(color.Transparent)
	b.bg.StrokeWidth = 1
	b.icon = canvas.NewImageFromResource(themedIcon(iconBell))
	b.icon.FillMode = canvas.ImageFillContain
	b.badge = canvas.NewCircle(color.Transparent)
	b.text = canvas.NewText("", color.White)
	b.text.TextSize = 9
	b.text.TextStyle = fyne.TextStyle{Bold: true}
	b.text.Alignment = fyne.TextAlignCenter
	b.ExtendBaseWidget(b)
	return b
}

func (b *alertBell) SetCount(count int) {
	b.count = count
	b.text.Text = fmt.Sprint(count)
	b.Refresh()
}

func (b *alertBell) Tapped(*fyne.PointEvent) {
	if b.onTap != nil {
		b.onTap()
	}
}
func (b *alertBell) MouseIn(*desktop.MouseEvent)    { b.hovered = true; b.Refresh() }
func (b *alertBell) MouseMoved(*desktop.MouseEvent) {}
func (b *alertBell) MouseOut()                      { b.hovered = false; b.Refresh() }
func (b *alertBell) Cursor() desktop.Cursor         { return desktop.PointerCursor }
func (b *alertBell) MinSize() fyne.Size             { return fyne.NewSize(40, 40) }

func (b *alertBell) CreateRenderer() fyne.WidgetRenderer {
	r := &alertBellRenderer{b: b}
	r.Refresh()
	return r
}

type alertBellRenderer struct{ b *alertBell }

func (r *alertBellRenderer) Layout(size fyne.Size) {
	b := r.b
	b.bg.Resize(fyne.NewSize(40, 40))
	b.bg.Move(fyne.NewPos((size.Width-40)/2, (size.Height-40)/2))
	b.icon.Resize(fyne.NewSize(20, 20))
	b.icon.Move(fyne.NewPos((size.Width-20)/2, (size.Height-20)/2))
	b.badge.Resize(fyne.NewSize(16, 16))
	b.badge.Move(fyne.NewPos(size.Width/2+6, size.Height/2-18))
	b.text.Resize(fyne.NewSize(16, 16))
	b.text.Move(fyne.NewPos(size.Width/2+6, size.Height/2-18))
}
func (r *alertBellRenderer) MinSize() fyne.Size { return r.b.MinSize() }
func (r *alertBellRenderer) Refresh() {
	t := tk()
	b := r.b
	if b.hovered {
		b.bg.FillColor = t.Hover
	} else {
		b.bg.FillColor = t.GlassSunken
	}
	b.bg.StrokeColor = t.GlassBorder
	b.icon.Resource = themedIcon(iconBell)
	if b.count > 0 {
		b.badge.FillColor = t.Brand
		b.badge.Show()
		b.text.Show()
	} else {
		b.badge.Hide()
		b.text.Hide()
	}
	b.text.Color = t.InkOnBrand
	b.bg.Refresh()
	b.icon.Refresh()
	b.badge.Refresh()
	b.text.Refresh()
}
func (r *alertBellRenderer) Objects() []fyne.CanvasObject {
	return []fyne.CanvasObject{r.b.bg, r.b.icon, r.b.badge, r.b.text}
}
func (r *alertBellRenderer) Destroy() {}

// ---------------------------------------------------------------------------
// Item da navegacao inferior (celular).
// ---------------------------------------------------------------------------

type mobileNavItem struct {
	widget.BaseWidget
	label  string
	icon   fyne.Resource
	onTap  func()
	active bool
	img    *canvas.Image
	text   *canvas.Text
	bar    *canvas.Rectangle
}

func newMobileNavItem(label string, icon fyne.Resource, onTap func()) *mobileNavItem {
	m := &mobileNavItem{label: label, icon: icon, onTap: onTap}
	m.img = canvas.NewImageFromResource(themedIcon(icon))
	m.img.FillMode = canvas.ImageFillContain
	m.text = canvas.NewText(label, color.Black)
	m.text.TextSize = sizeMicro
	m.text.TextStyle = fyne.TextStyle{Bold: true}
	m.text.Alignment = fyne.TextAlignCenter
	m.bar = canvas.NewRectangle(color.Transparent)
	m.bar.CornerRadius = 2
	m.ExtendBaseWidget(m)
	return m
}

func (m *mobileNavItem) setActive(active bool) {
	if m.active == active {
		return
	}
	m.active = active
	m.Refresh()
}

func (m *mobileNavItem) Tapped(*fyne.PointEvent) {
	if m.onTap != nil {
		m.onTap()
	}
}

func (m *mobileNavItem) MinSize() fyne.Size { return fyne.NewSize(56, 48) }

func (m *mobileNavItem) CreateRenderer() fyne.WidgetRenderer {
	r := &mobileNavItemRenderer{m: m}
	r.Refresh()
	return r
}

type mobileNavItemRenderer struct{ m *mobileNavItem }

func (r *mobileNavItemRenderer) Layout(size fyne.Size) {
	m := r.m
	m.bar.Resize(fyne.NewSize(24, 3))
	m.bar.Move(fyne.NewPos((size.Width-24)/2, 0))
	m.img.Resize(fyne.NewSize(22, 22))
	m.img.Move(fyne.NewPos((size.Width-22)/2, 7))
	ts := m.text.MinSize()
	m.text.Resize(fyne.NewSize(size.Width, ts.Height))
	m.text.Move(fyne.NewPos(0, 31))
}
func (r *mobileNavItemRenderer) MinSize() fyne.Size { return r.m.MinSize() }
func (r *mobileNavItemRenderer) Refresh() {
	t := tk()
	m := r.m
	if m.active {
		m.img.Resource = brandIcon(m.icon)
		m.text.Color = t.Brand
		m.bar.FillColor = t.Brand
	} else {
		m.img.Resource = themedIcon(m.icon)
		m.text.Color = t.InkMuted
		m.bar.FillColor = color.Transparent
	}
	m.img.Refresh()
	m.text.Refresh()
	m.bar.Refresh()
}
func (r *mobileNavItemRenderer) Objects() []fyne.CanvasObject {
	return []fyne.CanvasObject{r.m.bar, r.m.img, r.m.text}
}
func (r *mobileNavItemRenderer) Destroy() {}

// ---------------------------------------------------------------------------
// Grupo de chips (filtros de status).
// ---------------------------------------------------------------------------

type chipGroup struct {
	root      *fyne.Container
	chips     []*chip
	value     string
	OnChanged func(value string)
}

func newChipGroup(onChanged func(string)) *chipGroup {
	g := &chipGroup{OnChanged: onChanged}
	g.root = container.NewHBox()
	return g
}

// newChipGroupWrapped monta o grupo numa grade que quebra linha. A barra de
// status cabe numa linha so; cinco formas de pagamento na tela do celular, nao.
func newChipGroupWrapped(onChanged func(string), cell fyne.Size) *chipGroup {
	g := &chipGroup{OnChanged: onChanged}
	g.root = container.NewGridWrap(cell)
	return g
}

func (g *chipGroup) Object() fyne.CanvasObject { return g.root }

// Add inclui um chip; value "" representa "todos".
func (g *chipGroup) Add(label string, value string) *chip {
	c := newChip(label, value, g)
	g.chips = append(g.chips, c)
	g.root.Add(c)
	if len(g.chips) == 1 {
		c.selected = true
		g.value = value
	}
	return c
}

func (g *chipGroup) Select(value string) {
	g.value = value
	for _, c := range g.chips {
		c.selected = c.value == value
		c.Refresh()
	}
}

func (g *chipGroup) SetCount(value string, count int) {
	for _, c := range g.chips {
		if c.value == value {
			c.setCount(count)
		}
	}
}

func (g *chipGroup) choose(value string) {
	g.Select(value)
	if g.OnChanged != nil {
		g.OnChanged(value)
	}
}

type chip struct {
	widget.BaseWidget
	label    string
	value    string
	count    int
	hasCount bool
	selected bool
	hovered  bool
	group    *chipGroup
	rect     *canvas.Rectangle
	text     *canvas.Text
	countTxt *canvas.Text
}

func newChip(label string, value string, group *chipGroup) *chip {
	c := &chip{label: label, value: value, group: group}
	c.rect = canvas.NewRectangle(color.Transparent)
	c.rect.CornerRadius = 15
	c.rect.StrokeWidth = 1
	c.text = canvas.NewText(label, color.Black)
	c.text.TextSize = sizeCaption
	c.text.TextStyle = fyne.TextStyle{Bold: true}
	c.countTxt = canvas.NewText("", color.Black)
	c.countTxt.TextSize = sizeCaption
	c.ExtendBaseWidget(c)
	return c
}

func (c *chip) setCount(count int) {
	c.count = count
	c.hasCount = true
	c.countTxt.Text = fmt.Sprint(count)
	c.Refresh()
}

func (c *chip) Tapped(*fyne.PointEvent) {
	if c.group != nil {
		c.group.choose(c.value)
	}
}
func (c *chip) MouseIn(*desktop.MouseEvent)    { c.hovered = true; c.Refresh() }
func (c *chip) MouseMoved(*desktop.MouseEvent) {}
func (c *chip) MouseOut()                      { c.hovered = false; c.Refresh() }
func (c *chip) Cursor() desktop.Cursor         { return desktop.PointerCursor }

func (c *chip) MinSize() fyne.Size {
	w := c.text.MinSize().Width + 24
	if c.hasCount {
		w += c.countTxt.MinSize().Width + 6
	}
	return fyne.NewSize(w, 30)
}

func (c *chip) CreateRenderer() fyne.WidgetRenderer {
	r := &chipRenderer{c: c}
	r.Refresh()
	return r
}

type chipRenderer struct{ c *chip }

func (r *chipRenderer) Layout(size fyne.Size) {
	c := r.c
	c.rect.Resize(size)
	ts := c.text.MinSize()
	c.text.Resize(ts)
	c.text.Move(fyne.NewPos(12, (size.Height-ts.Height)/2))
	cs := c.countTxt.MinSize()
	c.countTxt.Resize(cs)
	c.countTxt.Move(fyne.NewPos(12+ts.Width+6, (size.Height-cs.Height)/2))
}
func (r *chipRenderer) MinSize() fyne.Size { return r.c.MinSize() }
func (r *chipRenderer) Refresh() {
	t := tk()
	c := r.c
	switch {
	case c.selected:
		c.rect.FillColor = t.BrandSoft
		c.rect.StrokeColor = rgbaOf(t.Brand, 0.5)
		c.text.Color = t.Brand
		c.countTxt.Color = t.Brand
	case c.hovered:
		c.rect.FillColor = t.Hover
		c.rect.StrokeColor = t.GlassBorder
		c.text.Color = t.Ink
		c.countTxt.Color = t.InkMuted
	default:
		c.rect.FillColor = t.GlassSunken
		c.rect.StrokeColor = t.GlassBorder
		c.text.Color = t.InkSecondary
		c.countTxt.Color = t.InkMuted
	}
	c.rect.Refresh()
	c.text.Refresh()
	c.countTxt.Refresh()
}
func (r *chipRenderer) Objects() []fyne.CanvasObject {
	return []fyne.CanvasObject{r.c.rect, r.c.text, r.c.countTxt}
}
func (r *chipRenderer) Destroy() {}

// ---------------------------------------------------------------------------
// Indicador de sincronizacao (barra superior).
// ---------------------------------------------------------------------------

type syncState int

const (
	syncUnknown syncState = iota
	syncWorking
	syncOK
	syncFailed
	syncOffline
)

type syncIndicator struct {
	widget.BaseWidget
	state syncState
	label string
	onTap func()
	rect  *canvas.Rectangle
	dot   *canvas.Circle
	text  *canvas.Text
}

func newSyncIndicator(onTap func()) *syncIndicator {
	s := &syncIndicator{onTap: onTap, state: syncUnknown, label: "Nuvem"}
	s.rect = canvas.NewRectangle(color.Transparent)
	s.rect.CornerRadius = 16
	s.rect.StrokeWidth = 1
	s.dot = canvas.NewCircle(color.Transparent)
	s.text = canvas.NewText(s.label, color.Black)
	s.text.TextSize = sizeCaption
	s.text.TextStyle = fyne.TextStyle{Bold: true}
	s.ExtendBaseWidget(s)
	return s
}

func (s *syncIndicator) Set(state syncState, label string) {
	s.state = state
	s.label = label
	s.text.Text = label
	s.Refresh()
}

func (s *syncIndicator) Tapped(*fyne.PointEvent) {
	if s.onTap != nil {
		s.onTap()
	}
}
func (s *syncIndicator) Cursor() desktop.Cursor { return desktop.PointerCursor }

func (s *syncIndicator) MinSize() fyne.Size {
	return fyne.NewSize(s.text.MinSize().Width+34, 32)
}

func (s *syncIndicator) CreateRenderer() fyne.WidgetRenderer {
	r := &syncIndicatorRenderer{s: s}
	r.Refresh()
	return r
}

type syncIndicatorRenderer struct{ s *syncIndicator }

func (r *syncIndicatorRenderer) Layout(size fyne.Size) {
	s := r.s
	s.rect.Resize(size)
	s.dot.Resize(fyne.NewSize(8, 8))
	s.dot.Move(fyne.NewPos(12, (size.Height-8)/2))
	ts := s.text.MinSize()
	s.text.Resize(ts)
	s.text.Move(fyne.NewPos(26, (size.Height-ts.Height)/2))
}
func (r *syncIndicatorRenderer) MinSize() fyne.Size { return r.s.MinSize() }
func (r *syncIndicatorRenderer) Refresh() {
	t := tk()
	var fg color.Color
	switch r.s.state {
	case syncOK:
		fg = t.Success
	case syncWorking:
		fg = t.Info
	case syncFailed:
		fg = t.Danger
	case syncOffline:
		fg = t.Warning
	default:
		fg = t.Neutral
	}
	r.s.rect.FillColor = t.GlassSunken
	r.s.rect.StrokeColor = t.GlassBorder
	r.s.dot.FillColor = fg
	r.s.text.Color = t.InkSecondary
	r.s.rect.Refresh()
	r.s.dot.Refresh()
	r.s.text.Refresh()
}
func (r *syncIndicatorRenderer) Objects() []fyne.CanvasObject {
	return []fyne.CanvasObject{r.s.rect, r.s.dot, r.s.text}
}
func (r *syncIndicatorRenderer) Destroy() {}

// refreshSyncIndicator resume o estado da fila para o indicador e o inicio.
func (ui *mainUI) refreshSyncIndicator() {
	if ui.syncPill == nil {
		return
	}
	if ui.syncRunning.Load() {
		// Uma sincronizacao em andamento ja esta escrevendo o proprio texto.
		return
	}
	go func() {
		queue, _ := ui.store.SyncQueueSize(ctxBackground())
		last, hasLast, _ := ui.store.GetSetting(ctxBackground(), syncLastSuccessKey)
		failure, hasFailure, _ := ui.store.GetSetting(ctxBackground(), syncLastFailureKey)
		config, err := ui.store.APIConfig(ctxBackground())
		fyne.Do(func() {
			if ui.syncRunning.Load() {
				return
			}
			switch {
			case err != nil || strings.TrimSpace(config.Token) == "":
				ui.syncPill.Set(syncOffline, "Nuvem não ativada")
			case hasFailure && strings.TrimSpace(failure) != "":
				ui.syncPill.Set(syncFailed, "Falha ao sincronizar")
			case queue > 0:
				ui.syncPill.Set(syncWorking, fmt.Sprintf("%d alterações pendentes", queue))
			case hasLast && strings.TrimSpace(last) != "":
				ui.syncPill.Set(syncOK, "Sincronizado "+decodeTime(last).Local().Format("15:04"))
			default:
				ui.syncPill.Set(syncUnknown, "Nuvem")
			}
			if ui.homeStatus != nil {
				ui.homeStatus.update(ui)
			}
		})
	}()
}
