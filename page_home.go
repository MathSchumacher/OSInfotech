package main

import (
	"context"
	"fmt"
	"image/color"
	"sort"
	"strings"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/layout"
	"fyne.io/fyne/v2/theme"
)

var homeOrderColumns = []float32{0.9, 2.2, 2.4, 1.7, 1.2, 0.7}

// ---------------------------------------------------------------------------
// Pagina inicial: painel de controle.
// ---------------------------------------------------------------------------

func (ui *mainUI) buildHomePage() fyne.CanvasObject {
	hero := ui.buildHomeHero()

	// "Aguardando aprovação" era um monte so, e escondia duas filas com donos
	// diferentes: metade espera a loja dar o preco, metade espera o cliente
	// responder. Separadas, cada uma tem o que fazer obvio.
	quoteLabel, answerLabel := "Preciso orçar", "Esperando o cliente"
	if isMobileLayout() {
		quoteLabel, answerLabel = "Orçar", "Esperando"
	}
	needsQuote := newStatTile(quoteLabel, theme.DocumentCreateIcon(), toneWarning)
	needsQuote.OnTap = func() { ui.openOrdersWithStatus(filterNeedsQuote) }
	needsAnswer := newStatTile(answerLabel, theme.MailSendIcon(), toneDanger)
	needsAnswer.OnTap = func() { ui.openOrdersWithStatus(filterNeedsAnswer) }
	approved := newStatTile("Em bancada", theme.ComputerIcon(), toneInfo)
	approved.OnTap = func() { ui.openOrdersWithStatus(statusApproved) }
	approved.SetSub("Aprovadas, em conserto")
	delivered := newStatTile("Entregues", theme.ConfirmIcon(), toneSuccess)
	delivered.OnTap = func() { ui.openOrdersWithStatus(statusDelivered) }
	delivered.SetSub("Finalizadas")
	money := newStatTile("Faturamento do mês", theme.DocumentIcon(), toneBrand)
	money.OnTap = func() { ui.showPage(pageReports) }
	total := newStatTile("Total de OS", theme.ListIcon(), toneNeutral)
	total.OnTap = func() { ui.openOrdersWithStatus("") }
	ui.homeTiles["needsQuote"] = needsQuote
	ui.homeTiles["needsAnswer"] = needsAnswer
	ui.homeTiles["approved"] = approved
	ui.homeTiles["delivered"] = delivered
	ui.homeTiles["money"] = money
	ui.homeTiles["total"] = total

	var tiles fyne.CanvasObject
	if isMobileLayout() {
		tiles = container.NewVBox(
			container.NewGridWithColumns(2, needsQuote, needsAnswer),
			container.NewGridWithColumns(2, approved, delivered),
			money,
		)
	} else {
		// Quatro: as duas filas que pedem acao, o que esta em conserto e o
		// dinheiro. "Entregues" e "Total" sairam da linha porque sao leitura,
		// nao trabalho, e continuam a um toque nos filtros da lista.
		tiles = container.NewGridWithColumns(4, needsQuote, needsAnswer, approved, money)
	}

	ui.homeAttention = container.NewVBox()
	ui.homeRecent = container.NewVBox()
	ui.homeAttentionTitle = newText("", sizeSection, true, inkPrimary).SetTruncate(true)
	ui.homeRecentTitle = newText("", sizeSection, true, inkPrimary).SetTruncate(true)
	attention := ui.homeInfoCard(theme.HistoryIcon(), "Esperando orçamento há mais tempo", ui.homeAttentionTitle, ui.homeAttention, func() { ui.openOrdersWithStatus(filterNeedsQuote) })
	recent := ui.homeInfoCard(theme.HistoryIcon(), "Atividade recente", ui.homeRecentTitle, ui.homeRecent, func() { ui.openOrdersWithStatus("") })

	ordersCard := ui.buildHomeOrdersCard()
	ui.homeStatus = newHomeStatusCard(ui)

	var body fyne.CanvasObject
	if isMobileLayout() {
		body = container.NewVBox(hero, tiles, attention, recent, ordersCard, ui.homeStatus.Object())
	} else {
		bottom := container.NewBorder(nil, nil, nil, container.NewStack(gap(300, 1), ui.homeStatus.Object()), ordersCard)
		body = container.NewVBox(hero, gap(1, 2), tiles, gap(1, 2), container.NewGridWithColumns(2, attention, recent), gap(1, 2), bottom)
	}
	inset := pageInset()
	scroll := container.NewVScroll(paddedTRBL(body, 6, inset, inset, inset))
	return scroll
}

// buildHomeHero e o banner de boas-vindas com a arte da marca.
func (ui *mainUI) buildHomeHero() fyne.CanvasObject {
	ui.homeGreeting = newText("Boa noite", 30, true, inkPrimary)
	bang := newText("!", 30, true, inkBrand)
	greetingRow := container.NewHBox(ui.homeGreeting, bang)
	ui.homeDateText = newText("", sizeCaption, false, inkMuted)
	intro := mutedLabel("Aqui você acompanha o andamento das ordens de serviço e o desempenho da sua assistência técnica.")
	rule := newFill(2, func(t designTokens) color.Color { return t.Brand })
	rule.minSize = fyne.NewSize(3, 1)
	texts := container.NewVBox(newEyebrowBrand("Painel de controle"), gap(1, 4), greetingRow, ui.homeDateText, gap(1, 4), intro)
	textBlock := container.NewHBox(rule, gap(14, 1), texts)

	if isMobileLayout() {
		return newGlassPanel(paddedTRBL(textBlock, 10, 8, 10, 4), 12, heroGlass)
	}
	art := newBrandImage(brandArt, fyne.NewSize(300, 176))
	body := container.NewBorder(nil, nil, container.NewCenter(paddedTRBL(textBlock, 10, 0, 10, 8)), paddedTRBL(art, 0, 12, 0, 0), nil)
	return newGlassPanel(body, 14, heroGlass)
}

// homeInfoCard e o cartao com chip de icone, eyebrow, titulo e lista curta.
func (ui *mainUI) homeInfoCard(icon fyne.Resource, eyebrow string, title *themedText, list *fyne.Container, seeAll func()) fyne.CanvasObject {
	chip := container.NewStack(
		newThemedRect(radiusMd, 1, func(t designTokens) (color.Color, color.Color) { return t.BrandSoft, rgbaOf(t.Brand, 0.4) }),
		container.NewCenter(newToneIcon(icon, toneBrand, 24)),
	)
	chipBox := container.NewGridWrap(fyne.NewSize(48, 48), chip)
	var link fyne.CanvasObject
	if isMobileLayout() {
		link = container.NewCenter(newArrowButton(toneBrand, seeAll))
	} else {
		link = container.NewCenter(ghostButton("Ver todas", theme.NavigateNextIcon(), seeAll))
	}
	head := container.NewBorder(nil, nil, container.NewCenter(chipBox), link,
		paddedTRBL(container.NewVBox(newEyebrow(eyebrow), gap(1, 2), title), 0, 8, 0, 12))
	return card(container.NewVBox(head, list))
}

// buildHomeOrdersCard e a tabela de OS recentes do painel.
func (ui *mainUI) buildHomeOrdersCard() fyne.CanvasObject {
	chip := container.NewStack(
		newThemedRect(radiusMd, 1, func(t designTokens) (color.Color, color.Color) { return t.BrandSoft, rgbaOf(t.Brand, 0.4) }),
		container.NewCenter(newToneIcon(theme.DocumentIcon(), toneBrand, 24)),
	)
	chipBox := container.NewGridWrap(fyne.NewSize(48, 48), chip)
	title := newText("Ordens de serviço", sizeSection, true, inkPrimary)
	sub := newText("Acompanhe o status das ordens e mantenha o controle total.", sizeCaption, false, inkSecondary).SetTruncate(true)
	newBtn := primaryButton("Nova OS", theme.ContentAddIcon(), func() { ui.newOrder() })
	head := container.NewBorder(nil, nil, container.NewCenter(chipBox), container.NewCenter(newBtn),
		paddedTRBL(container.NewVBox(title, sub), 0, 8, 0, 12))

	ui.homeOrdersBox = container.NewVBox()
	ui.homeOrdersEmpty = emptyState(theme.DocumentIcon(), "Nenhuma ordem de serviço encontrada",
		"As ordens de serviço criadas aparecerão aqui.",
		secondaryButton("Criar nova OS", theme.ContentAddIcon(), func() { ui.newOrder() }))
	var table fyne.CanvasObject
	if isMobileLayout() {
		table = ui.homeOrdersBox
	} else {
		table = container.NewVBox(tableHeader(12, homeOrderColumns, "Nº OS", "Cliente", "Equipamento", "Status", "Data", "Ações"), gap(1, 4), ui.homeOrdersBox)
	}
	return card(container.NewVBox(head, gap(1, 10), table, paddedTRBL(ui.homeOrdersEmpty, 18, 0, 18, 0)))
}

// homeStatusCard e o painel escuro "sistema conectado" (sempre escuro, como
// no material da marca), com os checks de status e a acao de sincronizar.
type homeStatusCard struct {
	root    fyne.CanvasObject
	cloud   *themedText
	sync    *themedText
	version *themedText
	user    *themedText
	online  *canvas.Circle
	state   *themedText
}

func newHomeStatusCard(ui *mainUI) *homeStatusCard {
	c := &homeStatusCard{}
	title := newText("Sua assistência", 18, true, inkOnBrand)
	title2 := newText("técnica, conectada.", 18, true, inkOnBrand)
	c.cloud = newText("Nuvem: verificando…", sizeCaption, false, inkOnBrand).SetTruncate(true)
	c.sync = newText("Sincronização: —", sizeCaption, false, inkOnBrand).SetTruncate(true)
	c.version = newText("Versão v"+appVersion, sizeCaption, false, inkOnBrand).SetTruncate(true)
	c.user = newText("Usuário: —", sizeCaption, false, inkOnBrand).SetTruncate(true)
	row := func(text *themedText) fyne.CanvasObject {
		check := container.NewStack(
			newThemedRect(6, 1, func(t designTokens) (color.Color, color.Color) { return t.BrandSoft, rgbaOf(t.Brand, 0.5) }),
			container.NewCenter(newToneIcon(theme.ConfirmIcon(), toneBrand, 14)),
		)
		return container.NewBorder(nil, nil, container.NewCenter(container.NewGridWrap(fyne.NewSize(24, 24), check)), nil, paddedTRBL(text, 0, 0, 0, 10))
	}
	c.online = canvas.NewCircle(color.Transparent)
	c.online.Resize(fyne.NewSize(8, 8))
	c.state = newText("Sistema online", sizeCaption, false, inkOnBrand)
	syncNow := primaryButton("Sincronizar agora", theme.ViewRefreshIcon(), func() { ui.syncD1Async(true) })
	body := container.NewVBox(
		title, title2, gap(1, 10),
		row(c.cloud), gap(1, 6), row(c.sync), gap(1, 6), row(c.version), gap(1, 6), row(c.user),
		gap(1, 14),
		syncNow,
		gap(1, 8),
		container.NewHBox(container.NewCenter(container.NewGridWrap(fyne.NewSize(8, 8), c.online)), gap(6, 1), c.state),
	)
	c.root = newGlassPanel(body, 18, func(t designTokens) glassStyle {
		return glassStyle{
			Fill:    color.NRGBA{R: 12, G: 10, B: 12, A: 240},
			FillEnd: color.NRGBA{R: 40, G: 8, B: 12, A: 240},
			Border:  nrgbaAlpha(t.Brand, 0.55),
			BorderW: 1,
			Radius:  radiusLg,
			Glow:    nrgbaAlpha(t.Brand, 0.35),
			GlowR:   26,
		}
	})
	return c
}

func (c *homeStatusCard) Object() fyne.CanvasObject { return c.root }

// update le o estado da nuvem e do usuario.
func (c *homeStatusCard) update(ui *mainUI) {
	t := tk()
	c.user.SetText("Usuário: " + firstNonEmpty(ui.currentUser.Name, "—"))
	c.version.SetText("Versão v" + appVersion)
	if ui.syncPill != nil {
		switch ui.syncPill.state {
		case syncOK:
			c.cloud.SetText("Nuvem: conectada")
			c.sync.SetText(ui.syncPill.label)
			c.online.FillColor = t.Success
			c.state.SetText("Sistema online")
		case syncWorking:
			c.cloud.SetText("Nuvem: conectada")
			c.sync.SetText(ui.syncPill.label)
			c.online.FillColor = t.Info
			c.state.SetText("Sincronizando")
		case syncFailed:
			c.cloud.SetText("Nuvem: conectada")
			c.sync.SetText("Última tentativa falhou")
			c.online.FillColor = t.Warning
			c.state.SetText("Verifique a conexão")
		case syncOffline:
			c.cloud.SetText("Nuvem: não ativada neste aparelho")
			c.sync.SetText("Sincronização desligada")
			c.online.FillColor = t.Neutral
			c.state.SetText("Modo local")
		default:
			c.cloud.SetText("Nuvem: verificando…")
			c.sync.SetText("Sincronização: —")
			c.online.FillColor = t.Neutral
			c.state.SetText("Sistema")
		}
	}
	c.online.Refresh()
}

func (ui *mainUI) openOrdersWithStatus(status string) {
	ui.orderStatusFilter = status
	if ui.statusChips != nil {
		ui.statusChips.Select(status)
	}
	if ui.statusFilter != nil {
		ui.statusFilter.Selected = filterLabelFromValue(status)
		ui.statusFilter.Refresh()
	}
	ui.showPage(pageOrders)
}

func (ui *mainUI) refreshHome() {
	if ui.homeGreeting == nil {
		return
	}
	name := strings.Fields(ui.currentUser.Name)
	first := ""
	if len(name) > 0 {
		first = ", " + name[0]
	}
	ui.homeGreeting.SetText(greeting() + first)
	ui.homeDateText.SetText(longDatePT(time.Now()))

	if tile, ok := ui.homeTiles["money"]; ok {
		if ui.currentUser.IsAdmin() {
			tile.Show()
		} else {
			tile.Hide()
		}
	}
	if tile, ok := ui.homeTiles["total"]; ok && !isMobileLayout() {
		tile.Hide()
	}

	ui.updateCounts()
	if ui.homeStatus != nil {
		ui.homeStatus.update(ui)
	}

	orders, err := ui.store.List(context.Background(), "", "")
	if err != nil {
		return
	}
	// O cartao mostra a fila que so a loja resolve: aparelho parado esperando
	// um preco. A outra fila, de quem ja foi orcado, se resolve com mensagem,
	// e sai pelo ladrilho ao lado.
	pending := make([]ServiceOrder, 0)
	for _, o := range orders {
		if orderQueue(o) == queueNeedsQuote {
			pending = append(pending, o)
		}
	}
	sort.SliceStable(pending, func(i, j int) bool { return pending[i].CreatedAt.Before(pending[j].CreatedAt) })
	recent := append([]ServiceOrder(nil), orders...)
	sort.SliceStable(recent, func(i, j int) bool { return recent[i].UpdatedAt.After(recent[j].UpdatedAt) })

	// cartao "aguardando ha mais tempo"
	ui.homeAttention.Objects = nil
	if len(pending) == 0 {
		ui.homeAttentionTitle.SetText("Nenhum aparelho esperando orçamento")
		ui.homeAttention.Add(paddedTRBL(newText("Toda OS aberta já tem preço lançado.", sizeCaption, false, inkMuted), 6, 0, 0, 0))
	} else {
		ui.homeAttentionTitle.SetText(fmt.Sprintf("%d aparelhos esperando orçamento", len(pending)))
		ui.homeAttention.Add(gap(1, 8))
		for i, o := range pending {
			if i >= 3 {
				break
			}
			ui.homeAttention.Add(ui.homeOrderRow(o, true))
		}
	}
	ui.homeAttention.Refresh()

	// cartao "atividade recente"
	ui.homeRecent.Objects = nil
	if len(recent) == 0 {
		ui.homeRecentTitle.SetText("Nenhuma atividade recente")
		ui.homeRecent.Add(paddedTRBL(newText("As últimas movimentações aparecerão aqui.", sizeCaption, false, inkMuted), 6, 0, 0, 0))
	} else {
		last := recent[0]
		ui.homeRecentTitle.SetText(fmt.Sprintf("OS %06d · %s · %s", last.ID, last.Customer, relativeDate(last.UpdatedAt)))
		ui.homeRecent.Add(gap(1, 8))
		for i, o := range recent {
			if i >= 3 {
				break
			}
			ui.homeRecent.Add(ui.homeOrderRow(o, false))
		}
	}
	ui.homeRecent.Refresh()

	// tabela de OS recentes
	ui.homeOrdersBox.Objects = nil
	if len(recent) == 0 {
		ui.homeOrdersEmpty.Show()
	} else {
		ui.homeOrdersEmpty.Hide()
		for i, o := range recent {
			if i >= 8 {
				break
			}
			ui.homeOrdersBox.Add(ui.homeTableRow(o))
		}
	}
	ui.homeOrdersBox.Refresh()

	if ui.currentUser.IsAdmin() {
		go func() {
			report, err := buildFinancialReport(context.Background(), ui.store, "Mensal", time.Now())
			if err != nil {
				return
			}
			fyne.Do(func() {
				if tile, ok := ui.homeTiles["money"]; ok {
					tile.SetValue(formatBRL(report.GrossCents))
					tile.SetSub(homeMoneySub(report))
				}
			})
		}()
	}
}

// homeMoneySub resume o mes sem chamar de lucro o que ainda nao e. Com custo
// faltando, o que a loja precisa ver primeiro e o que falta lancar.
func homeMoneySub(report FinancialReport) string {
	// Um fato so, como nos ladrilhos das filas: embaixo do numero cabe pouco, e
	// duas metades cortadas no meio nao dizem nenhuma das duas coisas.
	if report.UnpaidOrders > 0 {
		return formatBRL(report.UnpaidCents) + " a receber"
	}
	if report.CostIsPartial() {
		return fmt.Sprintf("%d sem custo lançado", report.MissingCostOrders)
	}
	if len(report.Orders) == 0 {
		return "Nenhuma entrega no mês"
	}
	return "lucro " + formatBRL(report.NetCents)
}

func (ui *mainUI) updateHomeTiles(counts map[string]int) {
	if len(ui.homeTiles) == 0 {
		return
	}
	if t, ok := ui.homeTiles["needsQuote"]; ok {
		t.SetValue(fmt.Sprint(counts[filterNeedsQuote]))
	}
	if t, ok := ui.homeTiles["needsAnswer"]; ok {
		t.SetValue(fmt.Sprint(counts[filterNeedsAnswer]))
	}
	// As linhas de apoio contam a historia que o numero sozinho nao conta: ha
	// quanto tempo a mais velha espera, e quanto dinheiro esta parado.
	if needsQuote, needsAnswer, err := ui.store.QueueStats(context.Background()); err == nil {
		if t, ok := ui.homeTiles["needsQuote"]; ok {
			t.SetSub(needsQuote.Summary(false))
		}
		if t, ok := ui.homeTiles["needsAnswer"]; ok {
			t.SetSub(needsAnswer.Summary(true))
		}
	}
	if t, ok := ui.homeTiles["approved"]; ok {
		t.SetValue(fmt.Sprint(counts[statusApproved]))
	}
	if t, ok := ui.homeTiles["delivered"]; ok {
		t.SetValue(fmt.Sprint(counts[statusDelivered]))
	}
	if t, ok := ui.homeTiles["total"]; ok {
		t.SetValue(fmt.Sprint(counts["Todos"]))
	}
	if ui.alertBell != nil {
		// O sino conta o que passou do prazo de cobranca. Marcando tudo que
		// existe ele ficava em 91 por semanas e deixava de ser aviso.
		stale, err := ui.store.StaleOrderCount(context.Background(), staleOrderDays)
		if err != nil {
			stale = counts[statusPending]
		}
		ui.alertBell.SetCount(stale)
	}
}

// homeOrderRow e a linha compacta usada nos cartoes do inicio.
func (ui *mainUI) homeOrderRow(order ServiceOrder, showAge bool) fyne.CanvasObject {
	number := newText(fmt.Sprintf("%06d", order.ID), sizeBody, true, inkPrimary)
	name := newText(order.Customer, sizeBody, true, inkPrimary).SetTruncate(true)
	device := newText(strings.TrimSpace(order.Device+" "+order.BrandModel), sizeCaption, false, inkSecondary).SetTruncate(true)
	var right fyne.CanvasObject
	if showAge {
		days := int(time.Since(order.CreatedAt).Hours() / 24)
		tone := toneNeutral
		if days >= 3 {
			tone = toneWarning
		}
		if days >= 7 {
			tone = toneDanger
		}
		right = newStatusPill(ageLabel(days), tone)
	} else {
		right = newStatusPill(order.Status, orderStatusTone(order.Status))
	}
	body := container.NewBorder(nil, nil, container.NewCenter(number), container.NewCenter(right),
		paddedTRBL(container.NewVBox(name, device), 0, 8, 0, 12))
	return newTapCard(body, radiusMd, 8, func() { ui.openOrder(order) })
}

// homeTableRow e a linha da tabela de OS recentes do painel.
func (ui *mainUI) homeTableRow(order ServiceOrder) fyne.CanvasObject {
	open := func() { ui.openOrder(order) }
	if isMobileLayout() {
		return ui.homeOrderRow(order, false)
	}
	row := container.New(newColumnsLayout(12, homeOrderColumns...),
		vcenter(newText(fmt.Sprintf("%06d", order.ID), sizeBody, true, inkPrimary)),
		vcenter(cellText(order.Customer, true, inkPrimary)),
		vcenter(cellText(strings.TrimSpace(order.Device+" "+order.BrandModel), false, inkSecondary)),
		container.NewCenter(container.NewHBox(newStatusPill(order.Status, orderStatusTone(order.Status)), layout.NewSpacer())),
		vcenter(cellText(relativeDate(order.UpdatedAt), false, inkMuted)),
		container.NewCenter(newArrowButton(toneNeutral, open)),
	)
	return newTapCard(paddedTRBL(row, 4, 8, 4, 8), radiusMd, 4, open)
}

func ageLabel(days int) string {
	switch {
	case days <= 0:
		return "hoje"
	case days == 1:
		return "1 dia"
	default:
		return fmt.Sprintf("%d dias", days)
	}
}

func greeting() string {
	h := time.Now().Hour()
	switch {
	case h < 12:
		return "Bom dia"
	case h < 18:
		return "Boa tarde"
	default:
		return "Boa noite"
	}
}

var (
	weekdaysPT = []string{"domingo", "segunda-feira", "terça-feira", "quarta-feira", "quinta-feira", "sexta-feira", "sábado"}
	monthsPT   = []string{"", "janeiro", "fevereiro", "março", "abril", "maio", "junho", "julho", "agosto", "setembro", "outubro", "novembro", "dezembro"}
)

func longDatePT(t time.Time) string {
	t = t.Local()
	return fmt.Sprintf("%s, %d de %s de %d", weekdaysPT[t.Weekday()], t.Day(), monthsPT[t.Month()], t.Year())
}

// relativeDate resume uma data recente ("hoje 14:02", "ontem", "há 3 dias", "12/08").
func relativeDate(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	t = t.Local()
	now := time.Now()
	y, m, d := t.Date()
	ny, nm, nd := now.Date()
	if y == ny && m == nm && d == nd {
		return "hoje " + t.Format("15:04")
	}
	yesterday := now.AddDate(0, 0, -1)
	if yy, ym, yd := yesterday.Date(); y == yy && m == ym && d == yd {
		return "ontem " + t.Format("15:04")
	}
	days := int(now.Sub(t).Hours() / 24)
	if days < 7 {
		return fmt.Sprintf("há %d dias", days)
	}
	if y == ny {
		return t.Format("02/01")
	}
	return t.Format("02/01/2006")
}
