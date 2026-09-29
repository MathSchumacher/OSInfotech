package main

import (
	"context"
	"fmt"
	"time"
)

// ---------------------------------------------------------------------------
// As duas filas escondidas dentro de "Aguardando aprovação"
//
// Medindo a base da loja: das 92 OS aguardando aprovacao, 46 tem orcamento
// lancado e 46 nao tem. Sao dois problemas opostos com o mesmo rotulo na tela.
//
//	sem orcamento  -> a bola esta com a LOJA: falta avaliar e dar o preco
//	com orcamento  -> a bola esta com o CLIENTE: falta ele responder
//
// A primeira fila e producao atrasada (46 aparelhos, o mais velho parado ha 56
// dias). A segunda e cobranca: R$ 12.469 orcados e parados, 88% de tudo que a
// loja faturou no periodo. Empilhadas no mesmo monte, nenhuma das duas tem o
// que fazer obvio.
//
// A distincao e derivada, nao e um status novo. O dado que separa as duas ja
// esta gravado desde sempre, entao nao ha migracao, nao ha coluna nova e nao
// depende de deploy do servidor. E quando o operador lanca o valor, a OS muda
// de fila sozinha, que e exatamente o que deveria acontecer.
// ---------------------------------------------------------------------------

// Filtros derivados da lista. Nao sao status gravados: sao perguntas sobre a
// OS, e por isso viajam pelo mesmo campo de filtro com um prefixo proprio.
const (
	filterNeedsQuote  = "fila:orcar"
	filterNeedsAnswer = "fila:responder"
	// filterNotePassword nao e uma fila de trabalho, e uma lista de limpeza: as
	// OS antigas que ainda tem a senha do aparelho escrita na anotacao. Usa o
	// mesmo caminho por ser a mesma natureza de pergunta sobre a OS.
	filterNotePassword = "fila:senha-na-anotacao"
)

// quoteOverdueDays e o ponto em que um orcamento nao dado vira atraso. Tres
// dias porque a bancada entrega em 2,5 dias na mediana: se o conserto inteiro
// sai nesse tempo, o preco nao pode demorar mais que isso.
const quoteOverdueDays = 3

type queueKind int

const (
	queueNone queueKind = iota
	queueNeedsQuote
	queueNeedsAnswer
)

// orderQueue diz de quem e a vez nesta OS.
func orderQueue(order ServiceOrder) queueKind {
	if order.Status != statusPending {
		return queueNone
	}
	if order.AmountCents > 0 {
		return queueNeedsAnswer
	}
	return queueNeedsQuote
}

// queueLine descreve a fila numa etiqueta curta, para a linha da lista.
func queueLabel(kind queueKind) string {
	switch kind {
	case queueNeedsQuote:
		return "Preciso orçar"
	case queueNeedsAnswer:
		return "Esperando o cliente"
	default:
		return ""
	}
}

// QueueStat resume uma das filas.
type QueueStat struct {
	Orders    int
	Overdue   int
	OldestDay int
	Cents     int64
}

// Empty diz se a fila esta vazia, para a tela nao inventar aviso.
func (q QueueStat) Empty() bool { return q.Orders == 0 }

// Summary e a linha de apoio do ladrilho. Carrega um fato so, porque o espaco
// embaixo do numero e estreito e texto cortado no meio nao informa nada: na
// fila de orcar o que doi e o tempo, na de resposta e o dinheiro parado.
func (q QueueStat) Summary(withMoney bool) string {
	if q.Orders == 0 {
		return "Nada esperando"
	}
	if withMoney && q.Cents > 0 {
		return formatBRL(q.Cents) + " parados"
	}
	if q.OldestDay <= 0 {
		return "abertas hoje"
	}
	return fmt.Sprintf("mais antiga há %d dias", q.OldestDay)
}

// QueueStats conta as duas filas numa consulta so.
func (s *Store) QueueStats(ctx context.Context) (needsQuote QueueStat, needsAnswer QueueStat, err error) {
	rows, err := s.db.QueryContext(ctx, `
SELECT
	CASE WHEN amount_cents > 0 THEN 1 ELSE 0 END AS orcada,
	COUNT(*),
	COALESCE(SUM(amount_cents), 0),
	COALESCE(SUM(CASE WHEN created_at < ? THEN 1 ELSE 0 END), 0),
	MIN(created_at)
FROM service_orders
WHERE status = ?
GROUP BY orcada`,
		encodeTime(time.Now().UTC().AddDate(0, 0, -quoteOverdueDays)), statusPending)
	if err != nil {
		return QueueStat{}, QueueStat{}, err
	}
	defer rows.Close()

	for rows.Next() {
		var (
			quoted  int
			stat    QueueStat
			oldest  string
			overdue int
		)
		if err := rows.Scan(&quoted, &stat.Orders, &stat.Cents, &overdue, &oldest); err != nil {
			return QueueStat{}, QueueStat{}, err
		}
		stat.Overdue = overdue
		if when := decodeTime(oldest); !when.IsZero() {
			stat.OldestDay = int(time.Since(when).Hours() / 24)
		}
		if quoted == 1 {
			needsAnswer = stat
		} else {
			needsQuote = stat
		}
	}
	return needsQuote, needsAnswer, rows.Err()
}

// queueFilterClause traduz um filtro derivado em condicao de SQL. Devolve
// false quando o filtro recebido nao e uma fila.
func queueFilterClause(filter string) (string, []any, bool) {
	switch filter {
	case filterNeedsQuote:
		return "status = ? AND amount_cents <= 0", []any{statusPending}, true
	case filterNeedsAnswer:
		return "status = ? AND amount_cents > 0", []any{statusPending}, true
	case filterNotePassword:
		// Filtro mais largo que a regra do Go (mentionsDevicePassword), que e a
		// autoridade: aqui basta chegar perto, a tarja mostra o numero exato.
		return notesMentionPasswordSQL + " AND osfold(notes) NOT LIKE '%sem senha%'" +
			" AND osfold(notes) NOT LIKE '%nao tem senha%'", nil, true
	default:
		return "", nil, false
	}
}

// filterValueFromLabel traduz o rotulo que aparece no Select para o valor
// interno do filtro, e filterLabelFromValue faz o caminho de volta. Os status
// passam direto: neles o rotulo e o valor sao a mesma coisa.
func filterValueFromLabel(label string) string {
	switch label {
	case "Todos os status":
		return ""
	case queueLabel(queueNeedsQuote):
		return filterNeedsQuote
	case queueLabel(queueNeedsAnswer):
		return filterNeedsAnswer
	default:
		return label
	}
}

func filterLabelFromValue(value string) string {
	switch value {
	case "":
		return "Todos os status"
	case filterNeedsQuote:
		return queueLabel(queueNeedsQuote)
	case filterNeedsAnswer:
		return queueLabel(queueNeedsAnswer)
	default:
		return value
	}
}
