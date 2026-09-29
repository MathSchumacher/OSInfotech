package main

import (
	"database/sql"
	"strconv"
	"strings"
	"unicode"

	sqlite3 "github.com/mattn/go-sqlite3"
)

// searchDriverName e o driver sqlite registrado com as funcoes auxiliares de
// busca (osfold / osdigits). O sqlite embutido so conhece lower() ASCII, entao
// "JOSE" e "Jose" ate combinam, mas "JOSÉ" nunca vira "josé" e quem digita
// "jose" jamais encontra "José". As funcoes abaixo resolvem isso em Go.
const searchDriverName = "sqlite3_osinfotech"

func init() {
	sql.Register(searchDriverName, &sqlite3.SQLiteDriver{
		ConnectHook: func(conn *sqlite3.SQLiteConn) error {
			if err := conn.RegisterFunc("osfold", foldSearchText, true); err != nil {
				return err
			}
			return conn.RegisterFunc("osdigits", onlyDigits, true)
		},
	})
}

// foldSearchText normaliza texto para comparacao: minusculas, sem acentos e com
// espacos colapsados. "  José  DA Silva " -> "jose da silva".
func foldSearchText(s string) string {
	var b strings.Builder
	b.Grow(len(s))
	for _, r := range strings.ToLower(s) {
		if folded, ok := foldRune(r); ok {
			if folded != 0 {
				b.WriteRune(folded)
			}
			continue
		}
		b.WriteRune(r)
	}
	return strings.Join(strings.Fields(b.String()), " ")
}

// foldRune devolve o equivalente sem acento de r. O segundo retorno indica que a
// rune foi tratada aqui; folded == 0 significa "descartar" (marcas combinantes
// de textos em forma NFD).
func foldRune(r rune) (rune, bool) {
	switch r {
	case 'á', 'à', 'â', 'ã', 'ä', 'å', 'ā', 'ă', 'ą':
		return 'a', true
	case 'é', 'è', 'ê', 'ë', 'ē', 'ĕ', 'ė', 'ę', 'ě':
		return 'e', true
	case 'í', 'ì', 'î', 'ï', 'ī', 'į':
		return 'i', true
	case 'ó', 'ò', 'ô', 'õ', 'ö', 'ø', 'ō':
		return 'o', true
	case 'ú', 'ù', 'û', 'ü', 'ū', 'ů':
		return 'u', true
	case 'ç', 'ć', 'č':
		return 'c', true
	case 'ñ', 'ń':
		return 'n', true
	case 'ý', 'ÿ':
		return 'y', true
	case 'š', 'ś':
		return 's', true
	case 'ž', 'ź', 'ż':
		return 'z', true
	}
	if unicode.In(r, unicode.Mn) {
		return 0, true
	}
	return r, false
}

// searchToken e um termo digitado pelo usuario ja preparado para a consulta.
type searchToken struct {
	text    string // normalizado (sem acento, minusculo)
	digits  string // apenas digitos do termo
	numeric bool   // termo sem nenhuma letra (ex.: "123", "#42", "11 98765-4321")
}

// searchNoiseTokens sao prefixos que o usuario digita antes do numero da OS.
// So sao descartados quando sobra outro termo na busca.
var searchNoiseTokens = map[string]bool{
	"os": true, "o.s": true, "o.s.": true, "n": true, "n.": true,
	"no": true, "num": true, "num.": true, "numero": true, "ordem": true,
}

// parseSearchTokens quebra a busca em termos independentes. Cada termo precisa
// casar com algum campo (OR entre campos, AND entre termos), de modo que
// "joao iphone" encontre a OS do Joao com iPhone mesmo que nome e aparelho
// estejam em colunas diferentes - o que a busca antiga, com um unico LIKE da
// frase inteira, nunca conseguia.
func parseSearchTokens(query string) []searchToken {
	fields := strings.Fields(query)
	tokens := make([]searchToken, 0, len(fields))
	for _, field := range fields {
		text := foldSearchText(field)
		trimmed := strings.Trim(text, "#º°ª.,;:/-()")
		if trimmed != "" {
			text = trimmed
		}
		if text == "" {
			continue
		}
		token := searchToken{
			text:    text,
			digits:  onlyDigits(text),
			numeric: strings.IndexFunc(text, unicode.IsLetter) < 0,
		}
		tokens = append(tokens, token)
	}

	filtered := tokens[:0]
	for _, token := range tokens {
		if searchNoiseTokens[token.text] && len(tokens) > 1 {
			continue
		}
		filtered = append(filtered, token)
	}
	if len(filtered) == 0 {
		return nil
	}
	return filtered
}

// searchSpec descreve onde procurar em uma tabela.
type searchSpec struct {
	TextCols  []string // colunas textuais (nome, aparelho, defeito, ...)
	DigitCols []string // colunas com pontuacao variavel (CPF, telefone, CNPJ)
	IDCol     string   // coluna de numero da OS/termo, quando existir
}

// Especificacoes de busca por tabela. Manter estas listas junto do restante da
// simetria de campos: coluna nova pesquisavel entra aqui tambem.
var (
	orderSearchSpec = searchSpec{
		TextCols: []string{
			"customer", "device", "brand_model", "serial", "issue",
			"accessories", "notes", "technician_name", "status",
		},
		DigitCols: []string{"cpf", "phone"},
		IDCol:     "id",
	}

	termSearchSpec = searchSpec{
		TextCols: []string{
			"term_type", "status", "seller_name", "seller_phone", "buyer_company",
			"buyer_phone", "device_kind", "brand", "model", "color", "imei1",
			"imei2", "city",
		},
		DigitCols: []string{"seller_cpf", "buyer_document", "seller_phone", "buyer_phone"},
		IDCol:     "id",
	}

	customerProfileSearchSpec = searchSpec{
		TextCols:  []string{"name", "phone", "last_device", "last_model"},
		DigitCols: []string{"cpf", "phone"},
	}

	orderCustomerSearchSpec = searchSpec{
		TextCols:  []string{"customer", "phone", "device", "brand_model"},
		DigitCols: []string{"cpf", "phone"},
	}
)

// minDigitSearch evita que um "1" solto case com quase todo telefone da base.
const minDigitSearch = 3

// buildSearchFilter monta a clausula WHERE da busca e seus argumentos. Devolve
// ok == false quando nao ha nada a filtrar.
func buildSearchFilter(query string, spec searchSpec) (string, []any, bool) {
	tokens := parseSearchTokens(query)
	if len(tokens) == 0 {
		return "", nil, false
	}

	var (
		clauses []string
		args    []any
	)
	for _, token := range tokens {
		var conds []string
		like := "%" + token.text + "%"
		for _, col := range spec.TextCols {
			conds = append(conds, "osfold(IFNULL("+col+", '')) LIKE ?")
			args = append(args, like)
		}
		// So consulta CPF/telefone quando o termo realmente tem digitos. A
		// versao antiga montava "LIKE '%%'" para buscas sem numero, o que era
		// verdadeiro em toda linha e fazia a busca devolver a base inteira.
		if len(token.digits) >= minDigitSearch {
			digitsLike := "%" + token.digits + "%"
			for _, col := range spec.DigitCols {
				conds = append(conds, "osdigits(IFNULL("+col+", '')) LIKE ?")
				args = append(args, digitsLike)
			}
		}
		if spec.IDCol != "" && token.numeric && len(token.digits) > 0 && len(token.digits) <= 9 {
			if id, err := strconv.ParseInt(token.digits, 10, 64); err == nil {
				conds = append(conds, spec.IDCol+" = ?")
				args = append(args, id)
			}
		}
		if len(conds) == 0 {
			continue
		}
		clauses = append(clauses, "("+strings.Join(conds, " OR ")+")")
	}
	if len(clauses) == 0 {
		return "", nil, false
	}
	return strings.Join(clauses, " AND "), args, true
}

// searchRankExpr ordena por relevancia: casamento exato primeiro, depois quem
// comeca com o texto buscado, por fim o resto.
func searchRankExpr(col string) string {
	return `CASE
		WHEN osfold(IFNULL(` + col + `, '')) = ? THEN 0
		WHEN osfold(IFNULL(` + col + `, '')) LIKE ? THEN 1
		ELSE 2
	END`
}

// searchRankArgs devolve os argumentos de searchRankExpr para a busca informada.
func searchRankArgs(query string) []any {
	folded := foldSearchText(query)
	return []any{folded, folded + "%"}
}
