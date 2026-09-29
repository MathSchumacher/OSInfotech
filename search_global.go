package main

import (
	"context"
	"fmt"
	"image/color"
	"strings"
	"sync/atomic"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
)

// globalSearchResult agrupa o que a busca geral encontrou.
type globalSearchResult struct {
	Customers []CustomerProfile
	Orders    []ServiceOrder
	Terms     []ServiceTerm
}

func (r globalSearchResult) empty() bool {
	return len(r.Customers) == 0 && len(r.Orders) == 0 && len(r.Terms) == 0
}

// GlobalSearch procura clientes, OS e termos com o mesmo texto. Qualquer dado
// serve: nome, CPF, telefone, numero da OS, IMEI, aparelho...
func (s *Store) GlobalSearch(ctx context.Context, query string) (globalSearchResult, error) {
	var result globalSearchResult
	query = strings.TrimSpace(query)
	if query == "" {
		return result, nil
	}
	customers, err := s.FindCustomerProfiles(ctx, query, 5)
	if err != nil {
		return result, err
	}
	result.Customers = customers

	orders, err := s.List(ctx, "", query)
	if err != nil {
		return result, err
	}
	if len(orders) > 6 {
		orders = orders[:6]
	}
	result.Orders = orders

	terms, err := s.ListTerms(ctx, query, termFilterAll)
	if err != nil {
		return result, err
	}
	if len(terms) > 4 {
		terms = terms[:4]
	}
	result.Terms = terms
	return result, nil
}

// globalSearchBox e o campo de busca da barra superior (desktop, com popup) e
// da pagina de busca (celular, resultados na propria pagina).
type globalSearchBox struct {
	ui      *mainUI
	entry   *dbsEntry
	results *fyne.Container
	popup   *widget.PopUp
	seq     atomic.Uint64
	inline  bool
	root    fyne.CanvasObject
}

func newGlobalSearchBox(ui *mainUI) *globalSearchBox {
	g := &globalSearchBox{ui: ui, inline: isMobileLayout()}
	g.entry = newFormEntry()
	g.entry.SetPlaceHolder("Buscar cliente, OS, CPF, telefone, IMEI…  (Ctrl+K)")
	if g.inline {
		g.entry.SetPlaceHolder("Buscar cliente, OS, CPF, telefone, IMEI…")
	}
	g.entry.ActionItem = widget.NewIcon(theme.SearchIcon())
	g.entry.OnChanged = func(text string) { g.lookup(text) }
	g.entry.OnSubmitted = func(string) { g.openFirst() }
	g.results = container.NewVBox()
	if g.inline {
		g.root = container.NewVBox(g.entry, g.results)
	} else {
		g.root = g.entry
	}
	return g
}

func (g *globalSearchBox) Object() fyne.CanvasObject { return g.root }

func (g *globalSearchBox) focus() {
	if g.ui.window != nil {
		g.ui.window.Canvas().Focus(g.entry)
	}
}

func (g *globalSearchBox) close() {
	g.seq.Add(1)
	if g.popup != nil {
		g.popup.Hide()
	}
	g.results.Objects = nil
	g.results.Refresh()
	g.entry.SetText("")
}

func (g *globalSearchBox) lookup(text string) {
	query := strings.TrimSpace(text)
	seq := g.seq.Add(1)
	if len([]rune(query)) < 2 {
		g.render(globalSearchResult{}, "")
		return
	}
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 4*time.Second)
		defer cancel()
		result, err := g.ui.store.GlobalSearch(ctx, query)
		if err != nil || seq != g.seq.Load() {
			return
		}
		fyne.Do(func() {
			if seq != g.seq.Load() || strings.TrimSpace(g.entry.Text) != query {
				return
			}
			g.render(result, query)
		})
	}()
}

// openFirst abre o primeiro resultado ao pressionar Enter.
func (g *globalSearchBox) openFirst() {
	if len(g.results.Objects) == 0 {
		return
	}
	for _, obj := range g.results.Objects {
		if row, ok := obj.(*tapCard); ok && row.OnTapped != nil {
			row.OnTapped()
			return
		}
	}
}

func (g *globalSearchBox) render(result globalSearchResult, query string) {
	g.results.Objects = nil
	if query == "" {
		if g.popup != nil {
			g.popup.Hide()
		}
		g.results.Refresh()
		return
	}
	if result.empty() {
		g.results.Add(paddedTRBL(newText("Nada encontrado para \""+query+"\"", sizeCaption, false, inkMuted), 8, 8, 8, 8))
	}
	if len(result.Customers) > 0 {
		g.results.Add(paddedTRBL(newEyebrow("Clientes"), 8, 8, 2, 8))
		for _, c := range result.Customers {
			customer := c
			g.results.Add(g.row(theme.AccountIcon(), customer.Name, customerMetaLine(customer), func() {
				g.close()
				g.ui.openCustomer(customer)
			}))
		}
	}
	if len(result.Orders) > 0 {
		g.results.Add(paddedTRBL(newEyebrow("Ordens de serviço"), 8, 8, 2, 8))
		for _, o := range result.Orders {
			order := o
			title := fmt.Sprintf("OS %06d · %s", order.ID, order.Customer)
			meta := strings.TrimSpace(order.Device + " " + order.BrandModel + " · " + order.Status)
			g.results.Add(g.row(theme.DocumentIcon(), title, meta, func() {
				g.close()
				g.ui.openOrder(order)
			}))
		}
	}
	if len(result.Terms) > 0 {
		g.results.Add(paddedTRBL(newEyebrow("Termos"), 8, 8, 2, 8))
		for _, t := range result.Terms {
			term := t
			title := fmt.Sprintf("Termo %06d · %s", term.ID, firstNonEmpty(term.SellerName, "sem vendedor"))
			meta := strings.TrimSpace(term.DeviceKind + " " + term.Brand + " " + term.Model + " · " + term.Status)
			g.results.Add(g.row(theme.DocumentCreateIcon(), title, meta, func() {
				g.close()
				g.ui.openTerm(term)
			}))
		}
	}
	g.results.Refresh()
	if g.inline {
		return
	}
	if g.popup == nil {
		bg := newThemedRect(radiusLg, 1, func(t designTokens) (color.Color, color.Color) {
			return t.Surface, t.BorderStrong
		})
		content := container.NewStack(bg, paddedTRBL(g.results, 6, 6, 6, 6))
		g.popup = widget.NewPopUp(content, g.ui.window.Canvas())
	}
	g.popup.Resize(fyne.NewSize(fyne.Max(g.entry.Size().Width, 420), g.results.MinSize().Height+12))
	g.popup.ShowAtRelativePosition(fyne.NewPos(0, g.entry.Size().Height+4), g.entry)
}

func (g *globalSearchBox) row(icon fyne.Resource, title string, meta string, onTap func()) fyne.CanvasObject {
	ic := newToneIcon(icon, toneNeutral, 18)
	name := newText(title, sizeBody, true, inkPrimary).SetTruncate(true)
	sub := newText(meta, sizeCaption, false, inkSecondary).SetTruncate(true)
	body := container.NewBorder(nil, nil, container.NewCenter(ic), nil, paddedTRBL(container.NewVBox(name, sub), 0, 0, 0, 10))
	row := newTapCard(body, radiusSm, 8, onTap)
	row.paint = func(t designTokens, hovered bool) (color.Color, color.Color) {
		if hovered {
			return t.Hover, color.Transparent
		}
		return color.Transparent, color.Transparent
	}
	return row
}

// buildSearchPage e a pagina de busca do celular.
func (ui *mainUI) buildSearchPage() fyne.CanvasObject {
	header, _, _ := ui.pageTitle("Busca", "Buscar", "Cliente, OS, termo, CPF, telefone ou IMEI", "", nil)
	return pageScaffold(header, ui.globalSearch.Object(), true)
}
