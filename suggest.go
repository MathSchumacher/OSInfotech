package main

import (
	"context"
	"errors"
	"fmt"
	"image/color"
	"strings"
	"sync/atomic"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/layout"
)

// ---------------------------------------------------------------------------
// Sugestoes a partir do historico
//
// A loja digita as mesmas coisas o tempo todo: "troca de tela" aparece 25 vezes
// na base, "tela" 10, "virus" 7. Um terco das OS repete um defeito que ja tinha
// sido escrito igual antes, e outro tanto repete a combinacao de aparelho e
// modelo. Tudo isso era digitado de novo, com grafias diferentes, o que ainda
// por cima estraga a busca.
//
// Aqui o campo oferece o que ja existe na base, em fichas clicaveis embaixo
// dele, ordenadas por quantas vezes foram usadas.
// ---------------------------------------------------------------------------

// suggestColumns e a lista fechada de colunas que aceitam sugestao. O nome da
// coluna entra na consulta por concatenacao, entao nada de fora desta lista.
var suggestColumns = map[string]string{
	"issue":       "issue",
	"brand_model": "brand_model",
	"device":      "device",
	"accessories": "accessories",
}

// SuggestOrderValues devolve os valores mais usados naquela coluna que combinam
// com o que ja foi digitado.
func (s *Store) SuggestOrderValues(ctx context.Context, column string, typed string, limit int) ([]string, error) {
	col, ok := suggestColumns[column]
	if !ok {
		return nil, errors.New("coluna sem sugestao: " + column)
	}
	if limit <= 0 || limit > 12 {
		limit = 6
	}
	like := "%" + foldSearchText(typed) + "%"
	query := fmt.Sprintf(`
SELECT TRIM(%s) AS valor, COUNT(*) AS usos
FROM service_orders
WHERE TRIM(%s) <> '' AND osfold(%s) LIKE ?
GROUP BY osfold(TRIM(%s))
ORDER BY usos DESC, valor
LIMIT ?`, col, col, col, col)
	rows, err := s.db.QueryContext(ctx, query, like, limit*3)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	typedFold := foldSearchText(typed)
	seen := map[string]bool{}
	values := make([]string, 0, limit)
	for rows.Next() {
		var value string
		var uses int
		if err := rows.Scan(&value, &uses); err != nil {
			return nil, err
		}
		// So a primeira linha: a equipe escreve a senha do aparelho embaixo do
		// defeito, e sugerir isso levaria a senha de um cliente para a OS de
		// outro.
		value = strings.TrimSpace(firstLine(value))
		if value == "" {
			continue
		}
		key := foldSearchText(value)
		if key == typedFold || seen[key] {
			continue
		}
		seen[key] = true
		values = append(values, value)
		if len(values) >= limit {
			break
		}
	}
	return values, rows.Err()
}

// ---------------------------------------------------------------------------
// Fichas de sugestao embaixo do campo.
// ---------------------------------------------------------------------------

type suggestBar struct {
	ui     *mainUI
	entry  *dbsEntry
	column string
	box    *fyne.Container
	seq    atomic.Uint64
	// whenEmpty oferece os valores mais usados antes de o operador digitar
	// qualquer coisa. Serve para o tipo de aparelho, em que 75% da base e
	// "celular" e a lista inteira cabe em cinco fichas.
	whenEmpty bool
	// catalog completa as sugestoes com o catalogo de modelos embutido. O
	// historico da loja vem primeiro: o que ela ja consertou vale mais que um
	// aparelho qualquer da lista.
	catalog bool
}

// newSuggestBar liga um campo as sugestoes do historico e devolve o objeto que
// deve ser colocado logo abaixo dele.
func (ui *mainUI) newSuggestBar(entry *dbsEntry, column string, whenEmpty bool, catalog bool) fyne.CanvasObject {
	bar := &suggestBar{ui: ui, entry: entry, column: column, box: container.NewVBox(), whenEmpty: whenEmpty, catalog: catalog}
	bar.box.Hide()
	previous := entry.OnChanged
	entry.OnChanged = func(text string) {
		if previous != nil {
			previous(text)
		}
		bar.lookup(text)
	}
	if whenEmpty {
		bar.lookup("")
	}
	return bar.box
}

// lookup busca em segundo plano e so pinta se o texto ainda for o mesmo.
func (b *suggestBar) lookup(text string) {
	typed := strings.TrimSpace(text)
	seq := b.seq.Add(1)
	if len([]rune(typed)) < 2 && !(b.whenEmpty && typed == "") {
		b.clearNow()
		return
	}
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		limit := 4
		if b.catalog {
			limit = 2
		}
		values, err := b.ui.store.SuggestOrderValues(ctx, b.column, typed, limit)
		if err != nil || seq != b.seq.Load() {
			return
		}
		if b.catalog {
			values = appendCatalogModels(values, typed, 4)
		}
		fyne.Do(func() {
			if seq != b.seq.Load() || strings.TrimSpace(b.entry.Text) != typed {
				return
			}
			b.render(values)
		})
	}()
}

// clearNow e chamado da propria thread da interface (digitacao e clique).
func (b *suggestBar) clearNow() {
	b.seq.Add(1)
	b.box.Objects = nil
	b.box.Hide()
	b.box.Refresh()
}

// appendCatalogModels completa a lista com modelos do catalogo que ainda nao
// apareceram no historico.
func appendCatalogModels(values []string, typed string, limit int) []string {
	seen := map[string]bool{}
	for _, value := range values {
		seen[foldSearchText(value)] = true
	}
	for _, model := range searchModels(typed, limit) {
		if len(values) >= limit {
			break
		}
		key := foldSearchText(model.Display)
		if seen[key] {
			continue
		}
		seen[key] = true
		values = append(values, model.Display)
	}
	return values
}

func (b *suggestBar) render(values []string) {
	b.box.Objects = nil
	if len(values) == 0 {
		b.box.Hide()
		b.box.Refresh()
		return
	}
	chips := make([]fyne.CanvasObject, 0, len(values))
	for _, value := range values {
		pick := value
		// Sem truncar: o texto truncado reporta largura minima de 24 pontos e a
		// ficha colapsava para "Sa...".
		label := newText(pick, sizeCaption, false, inkSecondary)
		card := newTapCard(paddedTRBL(label, 3, 9, 3, 9), radiusSm, 0, func() {
			b.entry.SetText(pick)
			b.clearNow()
		})
		card.paint = func(t designTokens, hovered bool) (color.Color, color.Color) {
			if hovered {
				return nrgbaAlpha(t.Brand, 0.16), nrgbaAlpha(t.Brand, 0.5)
			}
			return t.GlassSunken, t.GlassBorder
		}
		chips = append(chips, card)
	}
	b.box.Add(gap(1, 4))
	b.box.Add(container.New(layout.NewRowWrapLayout(), chips...))
	b.box.Show()
	b.box.Refresh()
}
