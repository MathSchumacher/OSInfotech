package main

import (
	"context"
	"sort"
	"strings"
	"unicode"
)

// Uniao automatica de cadastros repetidos.
//
// Unir clientes muda dados do negocio, entao a regra e conservadora: so entra
// no automatico quem tem o mesmo CPF (o documento identifica a pessoa) ou
// nomes claramente equivalentes (um e o outro com sobrenome, ou com um erro de
// digitacao). Telefone repetido com nomes diferentes fica para o operador:
// marido e esposa costumam dividir o mesmo numero.

type mergePlanItem struct {
	Keep   CustomerProfile
	Merge  []CustomerProfile
	Reason string
}

type mergePlan struct {
	Auto   []mergePlanItem
	Manual []duplicateGroup
}

// PlanDuplicateMerge separa o que da para unir sozinho do que precisa de gente.
func (s *Store) PlanDuplicateMerge(ctx context.Context) (mergePlan, error) {
	groups, err := s.DuplicateCustomerGroups(ctx)
	if err != nil {
		return mergePlan{}, err
	}
	var plan mergePlan
	for _, group := range groups {
		if len(group.Profiles) < 2 {
			continue
		}
		reason := ""
		switch {
		case group.Field == "CPF":
			reason = "mesmo CPF"
		case equivalentNames(group.Profiles):
			reason = "mesmo telefone e nomes equivalentes"
		case oneNameAmongBlanks(group.Profiles):
			reason = "mesmo telefone e o outro cadastro esta sem nome"
		case sameNameWithDeviceNotes(group.Profiles):
			reason = "mesmo telefone e mesmo nome, com o aparelho anotado junto"
		default:
			plan.Manual = append(plan.Manual, group)
			continue
		}
		keep := pickSurvivor(group.Profiles)
		merge := make([]CustomerProfile, 0, len(group.Profiles)-1)
		for _, profile := range group.Profiles {
			if profile.Key != keep.Key {
				merge = append(merge, profile)
			}
		}
		plan.Auto = append(plan.Auto, mergePlanItem{Keep: keep, Merge: merge, Reason: reason})
	}
	return plan, nil
}

// ApplyMergePlan executa as unioes automaticas e devolve quantos cadastros
// foram absorvidos.
func (s *Store) ApplyMergePlan(ctx context.Context, plan mergePlan) (int, error) {
	merged := 0
	for _, item := range plan.Auto {
		for _, profile := range item.Merge {
			if err := s.MergeCustomerProfiles(ctx, item.Keep.Key, profile.Key); err != nil {
				return merged, err
			}
			merged++
		}
	}
	return merged, nil
}

// pickSurvivor escolhe o cadastro que fica: o nome mais completo (as OS dos
// outros migram de qualquer forma); em caso de empate, quem tem mais OS.
func pickSurvivor(profiles []CustomerProfile) CustomerProfile {
	best := profiles[0]
	for _, profile := range profiles[1:] {
		if betterSurvivor(profile, best) {
			best = profile
		}
	}
	return best
}

// betterSurvivor decide qual cadastro representa melhor o cliente: um nome de
// verdade ganha de um sem letras ("."), um nome limpo ganha de um com o
// aparelho anotado ("josé a06"), depois vale o nome mais completo e, por fim,
// quem tem mais OS.
func betterSurvivor(candidate CustomerProfile, best CustomerProfile) bool {
	if has, had := countLetters(candidate.Name) > 0, countLetters(best.Name) > 0; has != had {
		return has
	}
	if clean, wasClean := !hasDigit(candidate.Name), !hasDigit(best.Name); clean != wasClean {
		return clean
	}
	letters, bestLetters := countLetters(candidate.Name), countLetters(best.Name)
	if letters != bestLetters {
		return letters > bestLetters
	}
	return candidate.Orders > best.Orders
}

func countLetters(name string) int {
	count := 0
	for _, r := range name {
		if unicode.IsLetter(r) {
			count++
		}
	}
	return count
}

// equivalentNames diz se todos os nomes do grupo sao a mesma pessoa escrita de
// formas diferentes.
func equivalentNames(profiles []CustomerProfile) bool {
	for i := 0; i < len(profiles); i++ {
		for j := i + 1; j < len(profiles); j++ {
			if !sameNameWritten(profiles[i].Name, profiles[j].Name) {
				return false
			}
		}
	}
	return true
}

// sameNameWritten compara dois nomes ignorando acento e caixa. Aceita:
//   - um ser o outro com sobrenomes a mais ("Juliana" e "Juliana de Meneses");
//   - um erro de digitacao pequeno ("Botelho" e "Botego").
//
// Nao aceita complementos com numero ("José" e "José a06"), que sao anotacoes
// de aparelho e nao sobrenome.
func sameNameWritten(a string, b string) bool {
	left, right := strings.Fields(foldSearchText(a)), strings.Fields(foldSearchText(b))
	if len(left) == 0 || len(right) == 0 {
		return false
	}
	if len(left) > len(right) {
		left, right = right, left
	}
	// O nome curto precisa ser o inicio do longo.
	for i, word := range left {
		if !closeEnough(word, right[i]) {
			return false
		}
	}
	// As palavras a mais precisam parecer sobrenome (so letras, com 3+ letras).
	for _, extra := range right[len(left):] {
		if !looksLikeNameWord(extra) {
			return false
		}
	}
	return true
}

func looksLikeNameWord(word string) bool {
	if len([]rune(word)) < 2 {
		return false
	}
	for _, r := range word {
		if !unicode.IsLetter(r) {
			return false
		}
	}
	return true
}

// closeEnough aceita palavras iguais ou com um erro de digitacao.
func closeEnough(a string, b string) bool {
	if a == b {
		return true
	}
	if len(a) < 5 || len(b) < 5 {
		return false
	}
	return levenshtein(a, b) <= 2
}

func levenshtein(a string, b string) int {
	ar, br := []rune(a), []rune(b)
	previous := make([]int, len(br)+1)
	current := make([]int, len(br)+1)
	for j := range previous {
		previous[j] = j
	}
	for i := 1; i <= len(ar); i++ {
		current[0] = i
		for j := 1; j <= len(br); j++ {
			cost := 1
			if ar[i-1] == br[j-1] {
				cost = 0
			}
			current[j] = minInt(minInt(current[j-1]+1, previous[j]+1), previous[j-1]+cost)
		}
		copy(previous, current)
	}
	return previous[len(br)]
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// sortProfilesByName deixa a revisao previsivel.
func sortProfilesByName(profiles []CustomerProfile) {
	sort.SliceStable(profiles, func(i, j int) bool {
		return strings.ToLower(profiles[i].Name) < strings.ToLower(profiles[j].Name)
	})
}

// MergeObviousDuplicates faz sozinho as unioes que nao dependem de julgamento.
// Roda a cada sincronizacao: cadastros repetidos chegam de outros aparelhos e
// da base antiga, e nao adianta so avisar o operador.
func (s *Store) MergeObviousDuplicates(ctx context.Context) (int, error) {
	plan, err := s.PlanDuplicateMerge(ctx)
	if err != nil {
		return 0, err
	}
	return s.ApplyMergePlan(ctx, plan)
}

func hasDigit(text string) bool {
	for _, r := range text {
		if unicode.IsDigit(r) {
			return true
		}
	}
	return false
}

// foldedWords normaliza o nome para comparacao: sem acento, sem caixa, com um
// espaco entre as palavras.
func foldedWords(name string) string {
	return strings.Join(strings.Fields(foldSearchText(name)), " ")
}

// nameWithoutDeviceNotes tira do nome as anotacoes de aparelho que a equipe
// escreve junto ("josé a06" vira "jose").
func nameWithoutDeviceNotes(name string) string {
	words := strings.Fields(foldSearchText(name))
	kept := make([]string, 0, len(words))
	for _, word := range words {
		if !hasDigit(word) {
			kept = append(kept, word)
		}
	}
	return strings.Join(kept, " ")
}

// oneNameAmongBlanks: um cadastro tem nome e os outros nao ("." , "-"). O que
// nao tem nome nao identifica ninguem, entao vai para o que tem.
func oneNameAmongBlanks(profiles []CustomerProfile) bool {
	named := 0
	for _, profile := range profiles {
		if countLetters(profile.Name) > 0 {
			named++
		}
	}
	return named == 1
}

// sameNameWithDeviceNotes reconhece o mesmo cliente escrito com o aparelho no
// nome ("josé", "josé a06", "jose a13"). Exige um cadastro limpo no grupo como
// prova de que o nome e aquele; o telefone igual ja veio do agrupamento.
func sameNameWithDeviceNotes(profiles []CustomerProfile) bool {
	clean := false
	stripped := make([]string, 0, len(profiles))
	for _, profile := range profiles {
		name := nameWithoutDeviceNotes(profile.Name)
		if name == "" {
			return false
		}
		if name == foldedWords(profile.Name) {
			clean = true
		}
		stripped = append(stripped, name)
	}
	if !clean {
		return false
	}
	for i := 0; i < len(stripped); i++ {
		for j := i + 1; j < len(stripped); j++ {
			if !sameNameWritten(stripped[i], stripped[j]) {
				return false
			}
		}
	}
	return true
}
