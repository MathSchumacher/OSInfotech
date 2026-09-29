package main

import (
	"context"
	"fmt"
	"image/color"
	"path/filepath"
	"strings"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
)

type settingsSection int

const (
	settingsAppearance settingsSection = iota
	settingsAccess
	settingsTeam
	settingsCloud
	settingsWhatsApp
	settingsPrinting
	settingsChecklists
	settingsTermDefaults
	settingsUpdates
	settingsAbout
)

type settingsEntry struct {
	section  settingsSection
	title    string
	subtitle string
	icon     fyne.Resource
	admin    bool
}

var settingsEntries = []settingsEntry{
	{settingsAppearance, "Aparência", "Tema escuro ou claro", theme.ColorPaletteIcon(), false},
	{settingsAccess, "Acesso deste aparelho", "Login lembrado e sessão mantida", theme.LoginIcon(), false},
	{settingsTeam, "Equipe", "Usuários, permissões e auditoria", theme.AccountIcon(), true},
	{settingsCloud, "Nuvem e sincronização", "Empresa, status e armazenamento", theme.StorageIcon(), false},
	{settingsWhatsApp, "WhatsApp da loja", "Conectar o número e avisar clientes", theme.MailSendIcon(), true},
	{settingsPrinting, "Impressão 80 mm", "Identificação no topo do recibo", theme.DocumentPrintIcon(), true},
	{settingsChecklists, "Checklists de entrada", "Listas por tipo de aparelho", theme.ListIcon(), false},
	{settingsTermDefaults, "Padrões de termo", "Dados fixos de vendedor e comprador", theme.DocumentCreateIcon(), true},
	{settingsUpdates, "Atualizações", "Versão instalada e novas versões", theme.DownloadIcon(), false},
	{settingsAbout, "Sobre", "Versão, pasta de dados e suporte", theme.InfoIcon(), false},
}

// settingsPage e a secao de configuracoes: menu de secoes a esquerda (ou lista
// no celular) e o conteudo da secao ativa.
type settingsPage struct {
	ui       *mainUI
	root     fyne.CanvasObject
	navBox   *fyne.Container
	navItems map[settingsSection]*tapCard
	content  *fyne.Container
	sections map[settingsSection]fyne.CanvasObject
	current  settingsSection
	title    *themedText
	subtitle *themedText
	back     *dbsButton
	listView fyne.CanvasObject

	// equipe
	teamBox  *fyne.Container
	auditBox *fyne.Container

	// nuvem
	cloudAPI, cloudStatus, cloudOrg, cloudUser, cloudSync *widget.Label
	cloudRefresh                                          *dbsButton

	// checklists
	checklistBox *fyne.Container

	// acesso
	accessStatus *themedText

	// atualizacoes
	updateStatus *themedText
}

func newSettingsPage(ui *mainUI) *settingsPage {
	p := &settingsPage{ui: ui, navItems: map[settingsSection]*tapCard{}, sections: map[settingsSection]fyne.CanvasObject{}}
	p.navBox = container.NewVBox()
	p.content = container.NewStack()
	p.title = newText("Configurações", sizeTitle, true, inkPrimary)
	p.subtitle = newText("Preferências do sistema", sizeBody, false, inkSecondary)
	p.back = iconButton(theme.NavigateBackIcon(), func() { p.showList() })
	p.back.Hide()

	for _, e := range settingsEntries {
		entry := e
		p.navBox.Add(p.navRow(entry))
	}

	rule := newFill(2, func(t designTokens) color.Color { return t.Brand })
	rule.minSize = fyne.NewSize(3, 1)
	titles := container.NewVBox(newEyebrowBrand("Sistema"), gap(1, 2), p.title, p.subtitle)
	block := container.NewBorder(nil, nil, container.NewHBox(rule, gap(12, 1)), nil, titles)
	if isMobileLayout() {
		p.title.SetTruncate(true)
		p.subtitle.SetTruncate(true)
		header := container.NewVBox(container.NewBorder(nil, nil, container.NewCenter(p.back), nil, block), gap(1, 8))
		p.listView = card(p.navBox)
		p.root = pageScaffold(header, container.NewStack(p.listView, p.content), true)
		p.content.Hide()
	} else {
		header := container.NewVBox(block, gap(1, 10))
		nav := container.NewStack(gap(250, 1), cardPadded(p.navBox, 8))
		body := container.NewBorder(nil, nil, container.NewVBox(nav), nil, paddedTRBL(container.NewVBox(p.content), 0, 0, 0, 16))
		p.root = pageScaffold(header, body, true)
		p.show(settingsAppearance)
	}
	return p
}

func (p *settingsPage) Object() fyne.CanvasObject { return p.root }

func (p *settingsPage) navRow(entry settingsEntry) *tapCard {
	icon := newToneIcon(entry.icon, toneNeutral, 18)
	title := newText(entry.title, sizeBody, true, inkPrimary)
	sub := newText(entry.subtitle, sizeCaption, false, inkMuted)
	body := container.NewBorder(nil, nil, container.NewCenter(icon), nil, paddedTRBL(container.NewVBox(title, sub), 0, 0, 0, 10))
	row := newTapCard(body, radiusMd, 8, func() { p.show(entry.section) })
	row.paint = func(t designTokens, hovered bool) (color.Color, color.Color) {
		if p.current == entry.section && !isMobileLayout() {
			return t.BrandSoft, rgbaOf(t.Brand, 0.35)
		}
		if hovered {
			return t.Hover, color.Transparent
		}
		return color.Transparent, color.Transparent
	}
	p.navItems[entry.section] = row
	return row
}

// refresh aplica permissoes e recarrega a secao ativa.
func (p *settingsPage) refresh() {
	admin := p.ui.currentUser.IsAdmin()
	for _, e := range settingsEntries {
		if row, ok := p.navItems[e.section]; ok {
			if e.admin && !admin {
				row.Hide()
			} else {
				row.Show()
			}
		}
	}
	if isMobileLayout() && p.listView != nil && p.content.Hidden {
		return
	}
	p.show(p.current)
}

func (p *settingsPage) showList() {
	if !isMobileLayout() {
		return
	}
	p.content.Hide()
	p.listView.Show()
	p.back.Hide()
	p.title.SetText("Configurações")
	p.subtitle.SetText("Preferências do sistema")
}

func (p *settingsPage) show(section settingsSection) {
	p.current = section
	for _, row := range p.navItems {
		row.Refresh()
	}
	for _, e := range settingsEntries {
		if e.section == section {
			if e.admin && !p.ui.currentUser.IsAdmin() {
				p.content.Objects = []fyne.CanvasObject{cardPadded(emptyState(theme.WarningIcon(), "Acesso restrito", "Somente o administrador altera esta configuração.", nil), 32)}
				p.content.Refresh()
				return
			}
			if isMobileLayout() {
				p.title.SetText(e.title)
				p.subtitle.SetText(e.subtitle)
			}
		}
	}
	// Secoes sao sempre reconstruidas: sao baratas e mostram dados atuais.
	var obj fyne.CanvasObject
	switch section {
	case settingsAccess:
		obj = p.buildAccess()
	case settingsTeam:
		obj = p.buildTeam()
	case settingsCloud:
		obj = p.buildCloud()
	case settingsWhatsApp:
		obj = p.buildWhatsApp()
	case settingsPrinting:
		obj = p.buildPrinting()
	case settingsChecklists:
		obj = p.buildChecklists()
	case settingsTermDefaults:
		obj = p.buildTermDefaults()
	case settingsUpdates:
		obj = p.buildUpdates()
	case settingsAbout:
		obj = p.buildAbout()
	default:
		obj = p.buildAppearance()
	}
	p.content.Objects = []fyne.CanvasObject{obj}
	p.content.Refresh()
	if isMobileLayout() {
		p.listView.Hide()
		p.content.Show()
		p.back.Show()
	}
}

// sectionCard padroniza o cabecalho das secoes.
func sectionCard(title string, description string, body ...fyne.CanvasObject) fyne.CanvasObject {
	items := []fyne.CanvasObject{newText(title, sizeSection, true, inkPrimary)}
	if description != "" {
		items = append(items, mutedLabel(description))
	}
	items = append(items, gap(1, 6))
	items = append(items, body...)
	return card(container.NewVBox(items...))
}

// ---------------------------------------------------------------------------
// Aparencia
// ---------------------------------------------------------------------------

func (p *settingsPage) buildAppearance() fyne.CanvasObject {
	option := func(mode themeMode, title string, desc string) fyne.CanvasObject {
		preview := newThemedRect(radiusMd, 1, func(t designTokens) (color.Color, color.Color) {
			if mode == themeDark {
				return darkTokens.Canvas, darkTokens.BorderStrong
			}
			return lightTokens.Canvas, lightTokens.BorderStrong
		})
		preview.minSize = fyne.NewSize(72, 48)
		accent := newFill(3, func(t designTokens) color.Color {
			if mode == themeDark {
				return darkTokens.Brand
			}
			return lightTokens.Brand
		})
		accent.minSize = fyne.NewSize(28, 6)
		surface := newFill(4, func(t designTokens) color.Color {
			if mode == themeDark {
				return darkTokens.Surface
			}
			return lightTokens.Surface
		})
		surface.minSize = fyne.NewSize(48, 20)
		swatch := container.NewStack(preview, container.NewCenter(container.NewVBox(accent, gap(1, 4), surface)))
		check := newToneIcon(theme.ConfirmIcon(), toneBrand, 20)
		if activeThemeMode() != mode {
			check.Hide()
		}
		body := container.NewBorder(nil, nil, swatch, check, paddedTRBL(container.NewVBox(
			newText(title, sizeBody, true, inkPrimary), newText(desc, sizeCaption, false, inkMuted)), 0, 8, 0, 12))
		row := newTapCard(body, radiusMd, 10, func() {
			applyThemeMode(p.ui.app, mode)
			p.show(settingsAppearance)
		})
		row.paint = func(t designTokens, hovered bool) (color.Color, color.Color) {
			if activeThemeMode() == mode {
				return t.BrandSoft, rgbaOf(t.Brand, 0.4)
			}
			if hovered {
				return t.Hover, t.BorderStrong
			}
			return t.SurfaceRaised, t.Border
		}
		return row
	}
	return sectionCard("Aparência", "O tema vale para este computador ou celular.",
		fieldGrid(2,
			option(themeDark, "Escuro", "Padrão. Segue a identidade da DBS."),
			option(themeLight, "Claro", "Fundo claro com a versão clara da logo."),
		),
	)
}

// ---------------------------------------------------------------------------
// Acesso deste aparelho
// ---------------------------------------------------------------------------

func (p *settingsPage) buildAccess() fyne.CanvasObject {
	ui := p.ui
	remember := widget.NewCheck("Lembrar meu login neste aparelho", nil)
	remember.SetChecked(ui.rememberLoginEnabled())
	keep := widget.NewCheck("Manter conectado (entrar direto ao abrir)", nil)
	keep.SetChecked(ui.keepSessionEnabled())

	apply := func() {
		ui.rememberAccess(ui.currentUser.Login, remember.Checked, keep.Checked, ui.currentUser)
		p.accessStatus.SetText(ui.accessSummary())
	}
	remember.OnChanged = func(on bool) {
		if !on && keep.Checked {
			keep.SetChecked(false)
			return // o OnChanged do keep chama apply
		}
		apply()
	}
	keep.OnChanged = func(on bool) {
		if on && !remember.Checked {
			remember.SetChecked(true)
			return
		}
		apply()
	}

	p.accessStatus = newText(ui.accessSummary(), sizeCaption, false, inkMuted)
	forget := ghostButton("Esquecer este aparelho", theme.DeleteIcon(), func() {
		dialog.ShowConfirm("Esquecer este aparelho", "Apagar o login lembrado e a sessão mantida? Na próxima vez será preciso digitar login e senha.", func(ok bool) {
			if !ok {
				return
			}
			ui.forgetDevice()
			remember.SetChecked(false)
			keep.SetChecked(false)
			p.accessStatus.SetText(ui.accessSummary())
			ui.notify("Aparelho esquecido")
		}, ui.window)
	})
	logout := secondaryButton("Sair agora", theme.LogoutIcon(), func() { ui.lockSession() })

	return sectionCard("Acesso deste aparelho",
		"Vale só para este computador ou celular: nada disso é enviado para a nuvem. A senha nunca é guardada.",
		remember,
		keep,
		gap(1, 4),
		p.accessStatus,
		gap(1, 8),
		container.NewHBox(logout, forget),
	)
}

// ---------------------------------------------------------------------------
// Equipe
// ---------------------------------------------------------------------------

func (p *settingsPage) buildTeam() fyne.CanvasObject {
	p.teamBox = container.NewVBox()
	p.auditBox = container.NewVBox()
	newUser := primaryButton("Novo técnico", theme.ContentAddIcon(), func() {
		p.ui.showUserEditor(AppUser{Role: roleTechnician, Active: true})
	})
	audit := ghostButton("Ver auditoria", theme.HistoryIcon(), func() { p.loadAudit() })
	p.loadTeam()
	return container.NewVBox(
		sectionCard("Usuários e permissões", "Técnicos veem e editam as próprias OS; administradores veem tudo, custos e relatórios.",
			container.NewHBox(newUser, audit),
			gap(1, 6),
			p.teamBox,
		),
		card(container.NewVBox(sectionHeader("Auditoria"), p.auditBox)),
	)
}

func (p *settingsPage) loadTeam() {
	go func() {
		_ = p.ui.refreshUsersFromCloud(context.Background())
		users, err := p.ui.store.ListUsers(context.Background())
		fyne.Do(func() {
			p.teamBox.Objects = nil
			if err != nil {
				p.teamBox.Add(mutedLabel(err.Error()))
			}
			for _, u := range users {
				user := u
				av := newAvatar(34)
				av.SetName(user.Name)
				name := newText(user.Name, sizeBody, true, inkPrimary)
				meta := newText("@"+user.Login+" · "+user.DisplayRole(), sizeCaption, false, inkSecondary)
				state := newStatusPill("Ativo", toneSuccess)
				if !user.Active {
					state.Set("Inativo", toneNeutral)
				}
				edit := ghostButton("Editar", theme.SettingsIcon(), func() { p.ui.showUserEditor(user) })
				row := container.NewBorder(nil, nil, av, container.NewHBox(container.NewCenter(state), edit),
					paddedTRBL(container.NewVBox(name, meta), 0, 0, 0, 10))
				p.teamBox.Add(paddedTRBL(row, 4, 0, 4, 0))
				p.teamBox.Add(hairline())
			}
			p.teamBox.Refresh()
		})
	}()
}

func (p *settingsPage) loadAudit() {
	events, err := p.ui.store.ListAuditEvents(context.Background(), 150)
	if err != nil {
		dialog.ShowError(err, p.ui.window)
		return
	}
	p.auditBox.Objects = nil
	if len(events) == 0 {
		p.auditBox.Add(mutedLabel("Ainda não há eventos registrados."))
	}
	for _, event := range events {
		title := newText(event.Action, sizeBody, true, inkPrimary)
		detail := fmt.Sprintf("%s · %s · %s", event.ActorName, event.SubjectName, formatDate(event.CreatedAt))
		if event.Details != "" {
			detail += " · " + event.Details
		}
		p.auditBox.Add(container.NewVBox(title, newText(detail, sizeCaption, false, inkSecondary), gap(1, 4)))
	}
	p.auditBox.Refresh()
}

// ---------------------------------------------------------------------------
// Nuvem
// ---------------------------------------------------------------------------

func (p *settingsPage) buildCloud() fyne.CanvasObject {
	ui := p.ui
	p.cloudAPI = mutedLabel("")
	p.cloudStatus = bodyLabel("Verificando…")
	p.cloudOrg = bodyLabel("Empresa: verificando…")
	p.cloudUser = bodyLabel("Usuário: " + firstNonEmpty(ui.currentUser.Login, "sem login"))
	p.cloudSync = bodyLabel("Sincronização: verificando…")

	p.cloudRefresh = ghostButton("Verificar status", theme.ViewRefreshIcon(), func() { p.loadCloudStatus() })
	syncNow := primaryButton("Sincronizar agora", theme.ViewRefreshIcon(), func() { ui.syncD1Async(true) })
	editOrg := ghostButton("Editar empresa", theme.DocumentCreateIcon(), func() {
		ui.showOrganizationNameDialog(strings.TrimPrefix(p.cloudOrg.Text, "Empresa: "), func() { p.loadCloudStatus() })
	})
	activate := secondaryButton("Ativar empresa", theme.ConfirmIcon(), func() { ui.showCloudActivationDialog() })
	r2 := ghostButton("Storage R2", theme.StorageIcon(), func() { ui.showR2SettingsDialog() })
	if !ui.currentUser.IsAdmin() {
		editOrg.Disable()
		activate.Disable()
		r2.Disable()
	}
	p.loadCloudStatus()
	return container.NewVBox(
		sectionCard("Empresa e sincronização", "Os dados ficam neste aparelho e são sincronizados com a nuvem da DBS.",
			container.NewHBox(p.cloudRefresh, syncNow),
			gap(1, 6),
			infoRow("Status", p.cloudStatus),
			infoRow("Empresa ativa", p.cloudOrg),
			infoRow("Sessão", p.cloudUser),
			infoRow("Dados locais", p.cloudSync),
			infoRow("API", p.cloudAPI),
		),
		sectionCard("Administração", "Ativação da empresa e armazenamento de fotos e documentos.",
			container.NewHBox(editOrg, activate, r2),
		),
	)
}

func infoRow(label string, body fyne.CanvasObject) fyne.CanvasObject {
	return container.NewVBox(newEyebrow(label), body, gap(1, 2))
}

func (p *settingsPage) loadCloudStatus() {
	ui := p.ui
	p.cloudRefresh.Disable()
	p.cloudStatus.SetText("Verificando conexão com a nuvem…")
	go func() {
		cfg, cfgErr := ui.store.APIConfig(context.Background())
		var initialized bool
		var status remoteOrganizationStatus
		var statusErr error
		if cfgErr == nil {
			var client *WorkerClient
			client, cfgErr = NewWorkerClient(cfg)
			if cfgErr == nil {
				ctx, cancel := context.WithTimeout(context.Background(), 12*time.Second)
				initialized, statusErr = client.SetupStatus(ctx)
				if statusErr == nil && initialized && strings.TrimSpace(cfg.Token) != "" {
					status, statusErr = client.OrganizationStatus(ctx)
				}
				cancel()
			}
		}
		queueSize, _ := ui.store.SyncQueueSize(context.Background())
		lastSuccess, hasLastSuccess, _ := ui.store.GetSetting(context.Background(), syncLastSuccessKey)
		lastFailure, hasLastFailure, _ := ui.store.GetSetting(context.Background(), syncLastFailureKey)

		fyne.Do(func() {
			p.cloudRefresh.Enable()
			if cfgErr != nil {
				p.cloudStatus.SetText("Erro de configuração da API: " + cfgErr.Error())
				return
			}
			p.cloudAPI.SetText(cfg.BaseURL)
			switch {
			case statusErr != nil:
				p.cloudStatus.SetText("Não foi possível confirmar a nuvem: " + statusErr.Error())
				p.cloudOrg.SetText("Empresa: indisponível")
			case !initialized:
				p.cloudStatus.SetText("Empresa ainda não ativada na nuvem.")
				p.cloudOrg.SetText("Empresa: nenhuma")
			case strings.TrimSpace(cfg.Token) == "":
				p.cloudStatus.SetText("Empresa ativa na nuvem. Faça login para ver detalhes e sincronizar.")
				p.cloudOrg.SetText("Empresa: ativa no servidor")
			default:
				p.cloudStatus.SetText("Conectado à nuvem.")
				p.cloudOrg.SetText("Empresa: " + firstNonEmpty(status.Organization.Name, "sem nome"))
				p.cloudUser.SetText(fmt.Sprintf("Usuário: %s · %s", status.User.Login, status.User.Role))
			}
			syncText := fmt.Sprintf("Fila local: %d alterações pendentes.", queueSize)
			if hasLastSuccess && strings.TrimSpace(lastSuccess) != "" {
				syncText += " Última sincronização: " + formatDate(decodeTime(lastSuccess)) + "."
			} else {
				syncText += " Ainda sem sincronização concluída."
			}
			if hasLastFailure && strings.TrimSpace(lastFailure) != "" {
				syncText += " Última falha: " + lastFailure
			}
			p.cloudSync.SetText(syncText)
			ui.refreshSyncIndicator()
		})
	}()
}

// ---------------------------------------------------------------------------
// Impressao 80 mm
// ---------------------------------------------------------------------------

func (p *settingsPage) buildPrinting() fyne.CanvasObject {
	ui := p.ui
	settings, err := ui.thermalPrintSettings(context.Background())
	if err != nil {
		settings = defaultThermalPrintSettings()
	}
	title := newFormEntry()
	title.SetText(settings.Title)
	title.SetPlaceHolder("Ex.: DBS Assistência Técnica")
	subtitle := newFormEntry()
	subtitle.SetText(settings.Subtitle)
	subtitle.SetPlaceHolder("Ex.: Assistência técnica")
	footer := newFormMultiLine()
	footer.SetText(settings.Footer)
	footer.SetPlaceHolder("Ex.: Garantia de 90 dias sobre o serviço executado.")
	save := primaryButton("Salvar identificação", theme.DocumentSaveIcon(), func() {
		updated := ThermalPrintSettings{Title: title.Text, Subtitle: subtitle.Text, Footer: footer.Text}.normalized()
		if err := ui.saveThermalPrintSettings(context.Background(), updated); err != nil {
			dialog.ShowError(err, ui.window)
			return
		}
		ui.notify("Identificação salva para os próximos recibos")
	})
	return sectionCard("Recibo térmico de 80 mm", "Texto impresso no recibo de OS entregue ao cliente.",
		field("Título no topo", title, ""),
		field("Subtítulo", subtitle, ""),
		field("Rodapé", footer, "Prazo de garantia e de retirada, impressos no pé do recibo"),
		container.NewHBox(save),
	)
}

// ---------------------------------------------------------------------------
// Checklists
// ---------------------------------------------------------------------------

func (p *settingsPage) buildChecklists() fyne.CanvasObject {
	ui := p.ui
	p.checklistBox = container.NewVBox()
	var rebuild func()
	rebuild = func() {
		ui.refreshChecklistTemplates()
		p.checklistBox.Objects = nil
		if len(ui.checklistTemplates) == 0 {
			p.checklistBox.Add(mutedLabel("Nenhuma lista cadastrada."))
		}
		for _, tpl := range ui.checklistTemplates {
			current := tpl
			name := newText(templateDisplayName(current), sizeBody, true, inkPrimary)
			meta := newText(fmt.Sprintf("%d itens · %s", len(current.Items), strings.Join(current.Items, ", ")), sizeCaption, false, inkMuted)
			edit := ghostButton("Editar", theme.DocumentCreateIcon(), func() { ui.showChecklistTemplateEditor(&current, rebuild) })
			del := ghostButton("Excluir", theme.DeleteIcon(), func() {
				dialog.ShowConfirm("Excluir lista", "Excluir a lista \""+current.Name+"\"?", func(ok bool) {
					if !ok {
						return
					}
					if err := ui.store.DeleteChecklistTemplate(context.Background(), current.ID); err != nil {
						dialog.ShowError(err, ui.window)
						return
					}
					rebuild()
				}, ui.window)
			})
			row := container.NewBorder(nil, nil, nil, container.NewHBox(edit, del), container.NewVBox(name, meta))
			p.checklistBox.Add(paddedTRBL(row, 4, 0, 4, 0))
			p.checklistBox.Add(hairline())
		}
		p.checklistBox.Refresh()
	}
	rebuild()
	newBtn := primaryButton("Nova lista", theme.ContentAddIcon(), func() { ui.showChecklistTemplateEditor(nil, rebuild) })
	restoreBtn := ghostButton("Restaurar listas padrão", theme.ViewRefreshIcon(), func() {
		dialog.ShowConfirm("Restaurar listas", "Adicionar as listas padrão (celular, console, TV, notebook)?", func(ok bool) {
			if !ok {
				return
			}
			if _, err := ui.store.RestoreDefaultChecklistTemplates(context.Background()); err != nil {
				dialog.ShowError(err, ui.window)
				return
			}
			rebuild()
		}, ui.window)
	})
	return sectionCard("Checklists de entrada", "Cada lista é escolhida na OS e cada item recebe OK, defeito ou não se aplica.",
		container.NewHBox(newBtn, restoreBtn),
		gap(1, 6),
		p.checklistBox,
	)
}

// ---------------------------------------------------------------------------
// Padroes de termo
// ---------------------------------------------------------------------------

func (p *settingsPage) buildTermDefaults() fyne.CanvasObject {
	ui := p.ui
	defaults, err := ui.loadTermDefaults(context.Background())
	if err != nil {
		defaults = defaultTermDefaults()
	}
	termTypeSelect := widget.NewSelect(salesTermTypes, nil)
	termTypeSelect.SetSelected(firstNonEmpty(defaults.TermType, defaultTermType))
	sellerName := newFormEntry()
	sellerName.SetText(defaults.SellerName)
	sellerDoc := newMaskedEntry("CPF ou CNPJ", maskCPF)
	sellerDoc.SetText(defaults.SellerDocument)
	sellerRG := newFormEntry()
	sellerRG.SetText(defaults.SellerRG)
	sellerPhone := newMaskedEntry("(00) 00000-0000", maskPhone)
	sellerPhone.SetText(defaults.SellerPhone)
	sellerAddress := newFormEntry()
	sellerAddress.SetText(defaults.SellerAddress)
	buyerCompany := newFormEntry()
	buyerCompany.SetText(defaults.BuyerCompany)
	buyerDoc := newMaskedEntry("CNPJ ou CPF", maskCPF)
	buyerDoc.SetText(defaults.BuyerDocument)
	buyerAddress := newFormEntry()
	buyerAddress.SetText(defaults.BuyerAddress)
	buyerPhone := newMaskedEntry("(00) 00000-0000", maskPhone)
	buyerPhone.SetText(defaults.BuyerPhone)
	amount := newFormEntry()
	attachMask(amount, maskMoney, nil)
	amount.SetText(formatBRL(defaults.AmountCents))
	payment := newFormEntry()
	payment.SetText(defaults.PaymentMethod)
	city := newFormEntry()
	city.SetText(defaults.City)
	declaration := newFormMultiLine()
	declaration.SetMinRowsVisible(4)
	declaration.SetText(firstNonEmpty(defaults.Declaration, defaultTermDeclaration()))

	collect := func() (TermDefaults, error) {
		value, err := parseBRL(amount.Text)
		if err != nil {
			return TermDefaults{}, err
		}
		return TermDefaults{
			TermType:       firstNonEmpty(termTypeSelect.Selected, defaultTermType),
			SellerName:     strings.TrimSpace(sellerName.Text),
			SellerDocument: strings.TrimSpace(sellerDoc.Text),
			SellerRG:       strings.TrimSpace(sellerRG.Text),
			SellerPhone:    strings.TrimSpace(sellerPhone.Text),
			SellerAddress:  strings.TrimSpace(sellerAddress.Text),
			BuyerCompany:   strings.TrimSpace(buyerCompany.Text),
			BuyerDocument:  strings.TrimSpace(buyerDoc.Text),
			BuyerAddress:   strings.TrimSpace(buyerAddress.Text),
			BuyerPhone:     strings.TrimSpace(buyerPhone.Text),
			AmountCents:    value,
			PaymentMethod:  strings.TrimSpace(payment.Text),
			City:           strings.TrimSpace(city.Text),
			Declaration:    strings.TrimSpace(declaration.Text),
		}, nil
	}
	fill := func(d TermDefaults) {
		termTypeSelect.SetSelected(d.TermType)
		sellerName.SetText(d.SellerName)
		sellerDoc.SetText(d.SellerDocument)
		sellerRG.SetText(d.SellerRG)
		sellerPhone.SetText(d.SellerPhone)
		sellerAddress.SetText(d.SellerAddress)
		buyerCompany.SetText(d.BuyerCompany)
		buyerDoc.SetText(d.BuyerDocument)
		buyerAddress.SetText(d.BuyerAddress)
		buyerPhone.SetText(d.BuyerPhone)
		amount.SetText(formatBRL(d.AmountCents))
		payment.SetText(d.PaymentMethod)
		city.SetText(d.City)
		declaration.SetText(d.Declaration)
	}
	save := primaryButton("Salvar padrões", theme.DocumentSaveIcon(), func() {
		d, err := collect()
		if err != nil {
			dialog.ShowError(err, ui.window)
			return
		}
		if err := ui.saveTermDefaults(context.Background(), d); err != nil {
			dialog.ShowError(err, ui.window)
			return
		}
		ui.notify("Padrões de termo salvos")
	})
	apply := ghostButton("Aplicar no termo aberto", theme.ConfirmIcon(), func() {
		d, err := collect()
		if err != nil {
			dialog.ShowError(err, ui.window)
			return
		}
		ui.suppressTermLookup = true
		ui.applyTermDefaultsToForm(d)
		ui.suppressTermLookup = false
		ui.refreshTermPreview()
		ui.notify("Padrões aplicados no termo atual")
	})
	reset := ghostButton("Usar exemplo DBS", theme.ViewRefreshIcon(), func() {
		fill(defaultTermDefaults())
		ui.notifyTone("Exemplo carregado. Clique em Salvar padrões para gravar.", toneInfo)
	})

	return container.NewVBox(
		sectionCard("Padrões de termo", "Dados usados ao criar um novo termo de compra e venda.",
			container.NewHBox(save, apply, reset),
		),
		card(container.NewVBox(
			sectionHeader("Vendedor fixo"),
			field("Tipo padrão", termTypeSelect, ""),
			field("Nome", sellerName, ""),
			fieldGrid(2,
				field("CPF / CNPJ", ui.cnpjLookupField(sellerDoc, func() { ui.lookupCNPJInto(sellerDoc, sellerName, sellerPhone, sellerAddress, nil, nil) }), ""),
				field("RG", sellerRG, ""),
			),
			fieldGrid(2, field("Telefone", sellerPhone, ""), field("Endereço", sellerAddress, "")),
		)),
		card(container.NewVBox(
			sectionHeader("Comprador fixo"),
			field("Empresa", buyerCompany, ""),
			fieldGrid(2,
				field("CNPJ / CPF", ui.cnpjLookupField(buyerDoc, func() { ui.lookupCNPJInto(buyerDoc, buyerCompany, buyerPhone, buyerAddress, nil, nil) }), ""),
				field("Telefone", buyerPhone, ""),
			),
			field("Endereço", buyerAddress, ""),
		)),
		card(container.NewVBox(
			sectionHeader("Valores e texto"),
			fieldGrid(3, field("Valor padrão", amount, ""), field("Pagamento padrão", payment, ""), field("Cidade padrão", city, "")),
			field("Declaração padrão", declaration, ""),
		)),
	)
}

// ---------------------------------------------------------------------------
// Atualizacoes e Sobre
// ---------------------------------------------------------------------------

func (p *settingsPage) buildUpdates() fyne.CanvasObject {
	p.updateStatus = newText("", sizeCaption, false, inkMuted)
	check := primaryButton("Verificar atualizações", theme.DownloadIcon(), func() {
		p.updateStatus.SetText("Verificando…")
		p.ui.checkUpdates(true)
	})
	version, _ := keyValue("Versão instalada", "v"+appVersion)
	channel := "Windows (instalador automático)"
	if isMobileLayout() {
		channel = "Android (APK pelo R2)"
	}
	chRow, _ := keyValue("Canal", channel)
	return sectionCard("Atualizações", "O app avisa quando houver uma versão nova ao entrar.",
		version, chRow, gap(1, 6), container.NewHBox(check), p.updateStatus,
	)
}

func (p *settingsPage) buildAbout() fyne.CanvasObject {
	name, _ := keyValue("Sistema", appDisplayName)
	version, _ := keyValue("Versão", "v"+appVersion)
	data, _ := keyValue("Pasta de dados", filepath.Dir(p.ui.dbPath))
	logo := newBrandImage(brandWide, fyne.NewSize(260, 126))
	return sectionCard("Sobre", "",
		container.NewCenter(logo),
		gap(1, 8),
		name, version, data,
	)
}
