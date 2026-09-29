package main

import (
	"context"
	"strings"
)

func ctxBackground() context.Context { return context.Background() }

// ---------------------------------------------------------------------------
// Mascaras de digitacao. Todas recebem o texto atual e devolvem o texto
// formatado; sao idempotentes (formatar duas vezes nao muda nada), o que e o
// que permite usa-las dentro de OnChanged sem loop.
// ---------------------------------------------------------------------------

// maskCPF formata CPF (11 digitos) ou CNPJ (mais que 11) enquanto digita.
func maskCPF(text string) string {
	d := onlyDigits(text)
	if d == "" {
		return ""
	}
	if len(d) > 11 {
		return maskCNPJ(d)
	}
	var b strings.Builder
	for i, r := range d {
		switch i {
		case 3, 6:
			b.WriteRune('.')
		case 9:
			b.WriteRune('-')
		}
		b.WriteRune(r)
	}
	return b.String()
}

func maskCNPJ(text string) string {
	d := onlyDigits(text)
	if len(d) > 14 {
		d = d[:14]
	}
	var b strings.Builder
	for i, r := range d {
		switch i {
		case 2, 5:
			b.WriteRune('.')
		case 8:
			b.WriteRune('/')
		case 12:
			b.WriteRune('-')
		}
		b.WriteRune(r)
	}
	return b.String()
}

// phoneDigits extrai o telefone do que foi digitado ou colado.
//
// Cortar os digitos em 11 sem olhar onde eles estao nao recusa um texto errado:
// inventa um numero. "5198104717 ou 51997077640" virava "(51) 98104-7175", que
// junta o fim do primeiro telefone com o comeco do segundo e parece
// perfeitamente valido. Com o WhatsApp ligado, isso manda a OS do cliente, com
// aparelho e valor, para um estranho.
//
// A separacao entre os numeros esta no texto, entao ela e respeitada: os grupos
// de digitos sao juntados enquanto couberem num telefone, e o que passa disso
// fica de fora. "5198104717 ou 51997077640" devolve o primeiro telefone
// inteiro, e nao uma mistura dos dois.
func phoneDigits(text string) string {
	groups := digitGroups(text)
	if len(groups) == 0 {
		return ""
	}
	// Digitado de uma vez so, sem separador: corta no tamanho maximo, que e o
	// comportamento certo enquanto o operador ainda esta digitando.
	if len(groups) == 1 {
		return capPhone(stripCountry(groups[0]))
	}
	// Cabe tudo num telefone so? Entao era um numero so, escrito com espacos.
	if whole := stripCountry(strings.Join(groups, "")); len(whole) <= 11 {
		return whole
	}
	// Ha mais de um numero no texto. Fica o primeiro completo: juntar grupos
	// ate o limite misturaria o fim de um com o comeco do outro.
	kept := ""
	for _, group := range groups {
		if len(stripCountry(kept+group)) > 11 {
			break
		}
		kept += group
	}
	if kept == "" {
		return capPhone(stripCountry(groups[0]))
	}
	return stripCountry(kept)
}

// stripCountry tira o 55 de um numero colado com o codigo do pais.
func stripCountry(d string) string {
	if len(d) > 11 && strings.HasPrefix(d, "55") {
		return d[2:]
	}
	return d
}

// capPhone corta no tamanho maximo de um telefone brasileiro.
func capPhone(d string) string {
	if len(d) > 11 {
		return d[:11]
	}
	return d
}

// digitGroups separa as sequencias de digitos do texto.
func digitGroups(text string) []string {
	var groups []string
	current := strings.Builder{}
	for _, r := range text {
		if r >= '0' && r <= '9' {
			current.WriteRune(r)
			continue
		}
		if current.Len() > 0 {
			groups = append(groups, current.String())
			current.Reset()
		}
	}
	if current.Len() > 0 {
		groups = append(groups, current.String())
	}
	return groups
}

// phoneHasExtraNumber diz se o texto trazia mais telefone do que coube. E o que
// dispara o aviso: guardar o primeiro em silencio tambem esconde o problema.
func phoneHasExtraNumber(text string) bool {
	kept := phoneDigits(text)
	if kept == "" {
		return false
	}
	groups := digitGroups(text)
	if len(groups) < 2 {
		return false
	}
	total := 0
	for _, group := range groups {
		total += len(group)
	}
	return total > len(kept)+2 // folga para o "55" retirado
}

// maskPhone formata (11) 91234-5678 ou (11) 1234-5678.
func maskPhone(text string) string {
	d := phoneDigits(text)
	if d == "" {
		return ""
	}
	var b strings.Builder
	b.WriteRune('(')
	for i, r := range d {
		if i == 2 {
			b.WriteString(") ")
		}
		if len(d) > 10 && i == 7 {
			b.WriteRune('-')
		}
		if len(d) <= 10 && i == 6 {
			b.WriteRune('-')
		}
		b.WriteRune(r)
	}
	return b.String()
}

// maskMoney trata o campo como uma maquina de cartao: os digitos entram pela
// direita. "1500" vira "R$ 15,00", "150000" vira "R$ 1.500,00".
func maskMoney(text string) string {
	d := strings.TrimLeft(onlyDigits(text), "0")
	if strings.TrimSpace(text) == "" {
		return ""
	}
	if d == "" {
		return "R$ 0,00"
	}
	if len(d) > 15 {
		d = d[:15]
	}
	var cents int64
	for _, r := range d {
		cents = cents*10 + int64(r-'0')
	}
	return formatBRL(cents)
}

// attachMask instala a mascara no campo mantendo o handler original (next).
func attachMask(entry *dbsEntry, mask func(string) string, next func(string)) {
	if entry == nil {
		return
	}
	var applying bool
	entry.OnChanged = func(text string) {
		if applying {
			return
		}
		formatted := mask(text)
		if formatted != text {
			applying = true
			entry.SetText(formatted)
			applying = false
			text = formatted
		}
		if next != nil {
			next(text)
		}
	}
}

// newMaskedEntry cria um campo ja com mascara e placeholder. Toda mascara deste
// app e de numero, entao o campo ja pede o teclado numerico no celular.
func newMaskedEntry(placeholder string, mask func(string) string) *dbsEntry {
	e := newNumericEntry()
	e.SetPlaceHolder(placeholder)
	attachMask(e, mask, nil)
	return e
}

// whatsappURL monta o link wa.me para um telefone brasileiro.
func whatsappURL(phone string) string {
	d := onlyDigits(phone)
	if len(d) < 10 {
		return ""
	}
	if !strings.HasPrefix(d, "55") || len(d) <= 11 {
		d = "55" + d
	}
	return "https://wa.me/" + d
}
