package main

import (
	"context"
	"image/color"
	"strings"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/layout"
)

// ---------------------------------------------------------------------------
// Memoria de preco
//
// A troca de tela aparece em 67 OS da loja com preco entre R$ 80 e R$ 999. A
// diferenca e o aparelho, mas como o modelo era texto livre, so 2 combinacoes de
// modelo mais defeito se repetiam em toda a base: nao dava para aprender nada.
//
// Com o modelo normalizado pelo catalogo e o servico reconhecido a partir do
// defeito, a mesma base passa a ter 9 combinacoes repetidas. Isso vira duas
// coisas praticas: o orcamento sai na hora, o que ataca as 45 OS que nunca foram
// orcadas, e o custo entra junto, que hoje falta em 63 das 96 OS entregues.
//
// Nada disso precisa de tabela nova: o preco sai do proprio historico.
// ---------------------------------------------------------------------------

// serviceRule reconhece o tipo de servico pelo texto do defeito. A ordem
// importa: a primeira regra que casar vence, entao o especifico vem antes.
type serviceRule struct {
	Service string
	Words   []string
}

var serviceRules = []serviceRule{
	{"Não liga", []string{"nao liga", "não liga", "nao da imagem", "sem imagem", "nao acende"}},
	{"Troca de tela", []string{"troca de tela", "troca tela", "tela quebrada", "tela preta", "tela", "display", "touch", "lcd"}},
	{"Troca de bateria", []string{"bateria", "nao segura carga", "descarrega"}},
	{"Conector de carga", []string{"nao carrega", "conector de carga", "dock de carga", "conector", "carregando", "carga"}},
	{"Limpeza de oxidação", []string{"caiu na agua", "molhou", "molhado", "oxidacao", "agua"}},
	{"Formatação e vírus", []string{"virus", "formatar", "formatacao", "lento", "travando", "sistema"}},
	{"Áudio", []string{"microfone", "alto falante", "auricular", "nao sai som", "sem som", "som"}},
	{"Câmera", []string{"camera", "flash"}},
	{"Botões", []string{"botao", "botoes", "power", "volume"}},
	{"Placa", []string{"placa", "curto", "reballing"}},
}

// serviceFor reconhece o servico pelo defeito relatado. Devolve vazio quando
// nao da para dizer, e nesse caso a memoria de preco simplesmente nao aparece.
func serviceFor(issue string) string {
	text := foldSearchText(issue)
	if text == "" {
		return ""
	}
	for _, rule := range serviceRules {
		for _, word := range rule.Words {
			if strings.Contains(text, foldSearchText(word)) {
				return rule.Service
			}
		}
	}
	return ""
}

// serviceKeyFor e a chave de agrupamento do servico.
func serviceKeyFor(issue string) string {
	return foldSearchText(serviceFor(issue))
}

// pricePoint e uma cobranca anterior do mesmo servico no mesmo aparelho.
type pricePoint struct {
	OrderID     int64
	AmountCents int64
	CostCents   int64
	When        time.Time
}

// RecentPrices devolve o que a loja cobrou das ultimas vezes que fez este
// servico neste modelo, da mais recente para a mais antiga.
func (s *Store) RecentPrices(ctx context.Context, modelKey, serviceKey string, exceptID int64, limit int) ([]pricePoint, error) {
	modelKey = strings.TrimSpace(modelKey)
	serviceKey = strings.TrimSpace(serviceKey)
	if modelKey == "" || serviceKey == "" {
		return nil, nil
	}
	if limit <= 0 || limit > 10 {
		limit = 3
	}
	rows, err := s.db.QueryContext(ctx, `
SELECT id, amount_cents, cost_cents, updated_at
FROM service_orders
WHERE model_key = ? AND service_key = ? AND amount_cents > 0 AND id <> ?
ORDER BY id DESC
LIMIT ?`, modelKey, serviceKey, exceptID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var points []pricePoint
	for rows.Next() {
		var point pricePoint
		var when string
		if err := rows.Scan(&point.OrderID, &point.AmountCents, &point.CostCents, &when); err != nil {
			return nil, err
		}
		point.When = decodeTime(when)
		points = append(points, point)
	}
	return points, rows.Err()
}

// BackfillOrderKeys preenche modelo e servico nas OS que ainda nao tem. Roda nas
// que vieram da nuvem e nas antigas, uma vez cada. Os dois campos sao derivados
// do que ja esta gravado, entao cada aparelho chega ao mesmo resultado sozinho.
func (s *Store) BackfillOrderKeys(ctx context.Context, limit int) (int, error) {
	if limit <= 0 {
		limit = 500
	}
	rows, err := s.db.QueryContext(ctx, `
SELECT id, brand_model, issue FROM service_orders
WHERE model_key = '' AND (TRIM(brand_model) <> '' OR TRIM(issue) <> '')
LIMIT ?`, limit)
	if err != nil {
		return 0, err
	}
	type pending struct {
		id    int64
		model string
		issue string
	}
	var list []pending
	for rows.Next() {
		var item pending
		if err := rows.Scan(&item.id, &item.model, &item.issue); err != nil {
			rows.Close()
			return 0, err
		}
		list = append(list, item)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return 0, err
	}

	filled := 0
	for _, item := range list {
		modelKey := modelKeyFor(item.model)
		if modelKey == "" {
			// Marca a linha como processada para nao varrer de novo toda vez.
			modelKey = "-"
		}
		if _, err := s.db.ExecContext(ctx,
			`UPDATE service_orders SET model_key = ?, service_key = ? WHERE id = ?`,
			modelKey, serviceKeyFor(item.issue), item.id); err != nil {
			return filled, err
		}
		filled++
	}
	return filled, nil
}

// applyOrderKeys preenche as chaves derivadas antes de gravar a OS.
func applyOrderKeys(order *ServiceOrder) {
	if order == nil {
		return
	}
	key := modelKeyFor(order.BrandModel)
	if key == "" {
		key = "-"
	}
	order.ModelKey = key
	order.ServiceKey = serviceKeyFor(order.Issue)
}

// ---------------------------------------------------------------------------
// Dica de preco no formulario da OS.
// ---------------------------------------------------------------------------

// refreshPriceHint mostra o que a loja cobrou das ultimas vezes que fez este
// servico neste aparelho. E o que transforma o orcamento de "consultar depois"
// em uma decisao no balcao.
func (ui *mainUI) refreshPriceHint() {
	if ui.priceHintBox == nil || ui.modelEntry == nil || ui.issueEntry == nil {
		return
	}
	model, issue := ui.modelEntry.Text, ui.issueEntry.Text
	current := ui.currentID
	seq := ui.priceHintSeq.Add(1)
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		modelKey, serviceKey := modelKeyFor(model), serviceKeyFor(issue)
		points, err := ui.store.RecentPrices(ctx, modelKey, serviceKey, current, 3)
		if err != nil || seq != ui.priceHintSeq.Load() {
			return
		}
		service := serviceFor(issue)
		canonical := canonicalModel(model)
		if canonical == "" {
			canonical = strings.TrimSpace(model)
		}
		fyne.Do(func() {
			if seq != ui.priceHintSeq.Load() {
				return
			}
			ui.renderPriceHint(canonical, service, points)
		})
	}()
}

func (ui *mainUI) renderPriceHint(model, service string, points []pricePoint) {
	box := ui.priceHintBox
	box.Objects = nil
	if len(points) == 0 {
		box.Hide()
		box.Refresh()
		return
	}
	title := newText(service+" em "+model, sizeCaption, false, inkMuted)
	title.SetTruncate(true)

	chips := make([]fyne.CanvasObject, 0, len(points))
	seen := map[int64]bool{}
	for _, point := range points {
		if seen[point.AmountCents] {
			continue
		}
		seen[point.AmountCents] = true
		amount, cost := point.AmountCents, point.CostCents
		label := formatBRL(amount)
		if cost > 0 {
			label += " · custo " + formatBRL(cost)
		}
		// Sem truncar: o valor e curto e truncado ele reporta 24 pontos de
		// largura minima, virando "R$...".
		text := newText(label, sizeCaption, true, inkSecondary)
		chip := newTapCard(paddedTRBL(text, 3, 9, 3, 9), radiusSm, 0, func() {
			ui.applyRememberedPrice(amount, cost)
		})
		chip.paint = func(t designTokens, hovered bool) (color.Color, color.Color) {
			if hovered {
				return nrgbaAlpha(t.Brand, 0.16), nrgbaAlpha(t.Brand, 0.5)
			}
			return t.GlassSunken, t.GlassBorder
		}
		chips = append(chips, chip)
	}
	box.Add(gap(1, 4))
	box.Add(title)
	box.Add(container.New(layout.NewRowWrapLayout(), chips...))
	box.Show()
	box.Refresh()
}

// applyRememberedPrice preenche valor e custo com o que ja foi cobrado antes.
func (ui *mainUI) applyRememberedPrice(amountCents, costCents int64) {
	ui.amountEntry.SetText(formatBRL(amountCents))
	if costCents > 0 && ui.currentUser.IsAdmin() {
		ui.costEntry.SetText(formatBRL(costCents))
		ui.currentCostCents = costCents
	}
	ui.updateNetAmountPreview()
	ui.notify("Valor da última vez aplicado")
}

// watchForPriceHint refaz a dica de preco quando o campo muda, sem atropelar o
// que ja estava ligado no campo.
func (ui *mainUI) watchForPriceHint(entry *dbsEntry) {
	if entry == nil {
		return
	}
	previous := entry.OnChanged
	entry.OnChanged = func(text string) {
		if previous != nil {
			previous(text)
		}
		ui.refreshPriceHint()
	}
}
