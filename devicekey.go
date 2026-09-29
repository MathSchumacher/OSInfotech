package main

import (
	"context"
	"fmt"
	"regexp"
	"strings"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/theme"
)

// ---------------------------------------------------------------------------
// A senha do aparelho que estava sendo impressa
//
// O campo de observacoes sai no comprovante do cliente, no termico
// (escpos.go:55) e no PDF (receipt.go:133). E e ali que a loja guardava a senha
// do aparelho: de 94 OS com observacao preenchida, 51 falam em senha e 44 tem
// uma senha escrita de verdade. Coisas como "SENHA: 200508", "TA COM CAPINHA
// SENHA: 155128", "ORCAR, SENHA 1307". Reimprimir qualquer uma dessas OS poe o
// codigo de desbloqueio no papel que o cliente leva embora.
//
// O campo proprio existe desde a 0.9.0 e nunca e impresso. Falta mudar o que ja
// esta gravado no lugar errado, e avisar quem digitar ali de novo.
//
// A extracao e de proposito conservadora: so sai sozinho o que nao deixa duvida
// (a palavra "senha" seguida de 4 a 10 digitos). Padrao de desenho, senha com
// letra e texto ambiguo ficam para uma pessoa olhar, porque errar aqui
// significa mutilar a anotacao de trabalho de uma OS real.
// ---------------------------------------------------------------------------

// A busca roda no texto original, e nao no dobrado: dobrar troca "ç" por "c" e
// tira acentos, o que muda o tamanho em bytes e invalidaria as posicoes do
// recorte. Nem "senha" nem "desbloqueio" tem acento, entao (?i) basta.
// A fronteira de palavra no fim evita mutilar senha alfanumerica: em
// "senha 140516a" a senha e "140516a", e recortar so os digitos deixaria
// um "a" solto na anotacao. Sem a fronteira a regra simplesmente nao casa,
// e a OS cai na lista de revisao, que e o desfecho certo para o duvidoso.
var devicePasswordPattern = regexp.MustCompile(`(?i)(?:senha|desbloqueio)\s*(?:de\s+desbloqueio\s*)?[:=;\-]?\s*([0-9]{4,10})\b`)

// deviceNoPasswordPattern reconhece quem escreveu que NAO ha senha. Sem isto,
// uma nota como "sem senha 1234 tentativas" viraria uma senha.
var deviceNoPasswordPattern = regexp.MustCompile(`(?:sem|nao tem|n tem|nao possui|nao ha)\s+senha`)

// mentionsDevicePassword diz se a observacao ainda fala de senha, mesmo quando
// nao da para extrair. E o que alimenta a lista do que precisa de olho humano.
func mentionsDevicePassword(notes string) bool {
	folded := foldSearchText(notes)
	if folded == "" {
		return false
	}
	if !strings.Contains(folded, "senha") && !strings.Contains(folded, "desbloqueio") {
		return false
	}
	// "sem senha" e informacao legitima e nao pede revisao, a nao ser que haja
	// tambem uma senha de verdade escrita na mesma observacao.
	if deviceNoPasswordPattern.MatchString(folded) && !devicePasswordPattern.MatchString(notes) {
		return false
	}
	return true
}

// extractDevicePassword separa a senha da observacao. Devolve a senha, a
// observacao sem ela, e se havia algo seguro a mover.
func extractDevicePassword(notes string) (password string, cleaned string, ok bool) {
	if strings.TrimSpace(notes) == "" {
		return "", notes, false
	}
	if deviceNoPasswordPattern.MatchString(foldSearchText(notes)) {
		// "sem senha" e informacao da OS e fica onde esta.
		return "", notes, false
	}
	spans := devicePasswordPattern.FindAllStringSubmatchIndex(notes, -1)
	if len(spans) == 0 {
		return "", notes, false
	}

	found := make([]string, 0, len(spans))
	for _, span := range spans {
		found = append(found, notes[span[2]:span[3]])
	}
	// O recorte vai de tras para frente para as posicoes anteriores
	// continuarem valendo.
	cut := notes
	for i := len(spans) - 1; i >= 0; i-- {
		cut = cut[:spans[i][0]] + cut[spans[i][1]:]
	}
	return strings.Join(dedupeStrings(found), " / "), tidyNoteRemains(cut), true
}

// dedupeStrings tira repetidos mantendo a ordem: ha OS com a mesma senha
// escrita duas vezes.
func dedupeStrings(values []string) []string {
	seen := make(map[string]bool, len(values))
	out := make([]string, 0, len(values))
	for _, v := range values {
		if seen[v] {
			continue
		}
		seen[v] = true
		out = append(out, v)
	}
	return out
}

var (
	extraSpaces  = regexp.MustCompile(`[ \t]{2,}`)
	doublePunct  = regexp.MustCompile(`\s*([,;])\s*([,;])+`)
	leadingJunk  = regexp.MustCompile(`(?m)^[\s,;:.\-]+`)
	trailingJunk = regexp.MustCompile(`(?m)[\s,;:\-]+$`)
)

// tidyNoteRemains limpa a pontuacao solta que sobra depois de recortar a senha
// do meio da frase, sem reescrever o resto do texto.
func tidyNoteRemains(s string) string {
	s = extraSpaces.ReplaceAllString(s, " ")
	s = doublePunct.ReplaceAllString(s, "$1")
	s = leadingJunk.ReplaceAllString(s, "")
	s = trailingJunk.ReplaceAllString(s, "")
	return strings.TrimSpace(s)
}

// notesMentionPasswordSQL e a condicao usada pelas duas consultas abaixo.
const notesMentionPasswordSQL = `TRIM(COALESCE(notes, '')) <> ''
  AND (osfold(notes) LIKE '%senha%' OR osfold(notes) LIKE '%desbloqueio%')`

// MoveDevicePasswordsFromNotes tira a senha da observacao e grava no campo que
// nao e impresso. Devolve quantas OS foram corrigidas.
//
// So mexe em OS que ainda nao tem senha no campo proprio: se alguem ja digitou
// ali, o que esta na observacao pode ser outra coisa, e nao e hora de adivinhar.
func (s *Store) MoveDevicePasswordsFromNotes(ctx context.Context) (int, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id FROM service_orders WHERE `+notesMentionPasswordSQL+
			` AND TRIM(COALESCE(device_key, '')) = ''`)
	if err != nil {
		return 0, err
	}
	var ids []int64
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			rows.Close()
			return 0, err
		}
		ids = append(ids, id)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return 0, err
	}

	moved := 0
	for _, id := range ids {
		order, err := s.Get(ctx, id)
		if err != nil {
			return moved, err
		}
		password, cleaned, ok := extractDevicePassword(order.Notes)
		if !ok || password == "" {
			continue
		}
		order.DeviceKey = password
		order.Notes = cleaned
		if err := s.Save(ctx, &order); err != nil {
			return moved, err
		}
		moved++
	}
	return moved, nil
}

// DevicePasswordsNeedingReview lista as OS que ainda falam de senha na
// observacao depois da passagem automatica: padrao de desenho, senha com letra
// ou texto que so uma pessoa entende.
func (s *Store) DevicePasswordsNeedingReview(ctx context.Context) ([]ServiceOrder, error) {
	rows, err := s.db.QueryContext(ctx,
		"SELECT "+orderSelectColumns+" FROM service_orders WHERE "+notesMentionPasswordSQL+" ORDER BY id DESC")
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var pending []ServiceOrder
	for rows.Next() {
		order, err := scanOrder(rows)
		if err != nil {
			return nil, err
		}
		if mentionsDevicePassword(order.Notes) {
			pending = append(pending, order)
		}
	}
	return pending, rows.Err()
}

// ---------------------------------------------------------------------------
// Avisos na interface
// ---------------------------------------------------------------------------

// refreshNotePasswordHint mostra o aviso embaixo das anotacoes quando o
// operador digita uma senha ali. O campo certo fica a dois campos de distancia
// e nunca e impresso; o habito de escrever na anotacao vem de antes de ele
// existir.
func (ui *mainUI) refreshNotePasswordHint() {
	if ui.notePasswordBox == nil {
		return
	}
	password, cleaned, ok := extractDevicePassword(ui.notesEntry.Text)
	jaTemCampo := strings.TrimSpace(ui.deviceKeyEntry.Text) != ""
	if !ok || password == "" || jaTemCampo {
		ui.notePasswordBox.Hide()
		ui.notePasswordBox.Refresh()
		return
	}

	label := newText("Isto parece a senha do aparelho.", sizeCaption, true, inkPrimary)
	detail := newText("O campo \"Senha ou desenho\" guarda melhor: fica no sistema e não sai no comprovante.",
		sizeCaption, false, inkMuted)
	move := secondaryButton("Mover para o campo certo", theme.ContentCutIcon(), func() {
		ui.deviceKeyEntry.SetText(password)
		ui.notesEntry.SetText(cleaned)
		ui.notify("Senha movida para o campo do aparelho")
	})
	body := container.NewBorder(nil, nil, nil, container.NewCenter(move),
		container.NewVBox(label, detail))
	ui.notePasswordBox.Objects = []fyne.CanvasObject{gap(1, 6), newGlassPanel(paddedTRBL(body, 8, 10, 8, 10), 10, tintGlass(toneWarning))}
	ui.notePasswordBox.Show()
	ui.notePasswordBox.Refresh()
}

// refreshOrdersPasswordAlert mostra, na lista, quantas OS antigas ainda tem
// senha escrita na anotacao. Sao as que a passagem automatica nao conseguiu
// resolver sozinha: padrao de desenho, senha com letra, texto ambiguo.
func (ui *mainUI) refreshOrdersPasswordAlert() {
	if ui.ordersPasswordBox == nil {
		return
	}
	go func() {
		pending, err := ui.store.DevicePasswordsNeedingReview(context.Background())
		if err != nil {
			return
		}
		fyne.Do(func() {
			if ui.ordersPasswordBox == nil {
				return
			}
			ui.ordersPasswordBox.Objects = nil
			if len(pending) == 0 {
				ui.ordersPasswordBox.Hide()
				ui.ordersPasswordBox.Refresh()
				return
			}
			texto := fmt.Sprintf("%d OS ainda têm a senha do aparelho escrita nas anotações.", len(pending))
			if len(pending) == 1 {
				texto = fmt.Sprintf("A OS %06d ainda tem a senha do aparelho escrita nas anotações.", pending[0].ID)
			}
			label := newText(texto, sizeBody, false, inkPrimary).SetTruncate(true)
			review := secondaryButton("Revisar", theme.SearchIcon(), func() {
				ui.openOrdersWithStatus(filterNotePassword)
			})
			body := container.NewBorder(nil, nil,
				container.NewCenter(newToneIcon(theme.WarningIcon(), toneWarning, 20)),
				container.NewCenter(review), paddedTRBL(label, 0, 8, 0, 10))
			ui.ordersPasswordBox.Add(newGlassPanel(body, 12, tintGlass(toneWarning)))
			ui.ordersPasswordBox.Add(gap(1, 10))
			ui.ordersPasswordBox.Show()
			ui.ordersPasswordBox.Refresh()
		})
	}()
}
