package main

import (
	_ "embed"
	"sort"
	"strings"
	"sync"
)

// ---------------------------------------------------------------------------
// Catalogo de modelos
//
// O campo "Marca e modelo" era texto livre, e o banco mostrava o estrago: 62 OS
// de Samsung escritas de 34 formas, 31 de Motorola em 23, 25 OS sem modelo
// nenhum. Isso nao e so feio: sem um nome estavel para o aparelho a loja nunca
// consegue aprender o proprio preco, porque nada se repete.
//
// O catalogo vem embutido (assets/models.tsv, gerado por tools/make_models.py a
// partir da lista publica de dispositivos do Google mais os iPhones). Nao
// depende de internet.
//
// A busca aceita os termos em qualquer ordem, que e como as pessoas realmente
// digitam: "13 iph" acha o iPhone 13 tao bem quanto "iph 13", e "a14" acha o
// Galaxy A14 sem escrever "Samsung" antes.
// ---------------------------------------------------------------------------

//go:embed assets/models.tsv
var modelCatalogRaw string

type deviceModel struct {
	Brand   string
	Display string
	tier    int // 0 = marca que a loja mais atende
}

var (
	modelsOnce  sync.Once
	modelList   []deviceModel
	modelTokens map[string][]int32
	modelKeys   []string // tokens ordenados, para varrer por prefixo
)

// brandTier repete a prioridade usada na geracao do arquivo: sem isso um
// Infinix aparece na frente de um Redmi numa busca por "note 11".
var brandTier = map[string]int{
	"Samsung": 0, "Apple": 0, "Motorola": 0,
	"Xiaomi": 1, "Redmi": 1, "POCO": 1,
	"LG": 2, "Positivo": 2, "Multilaser": 2,
}

func loadModelCatalog() {
	modelsOnce.Do(func() {
		lines := strings.Split(modelCatalogRaw, "\n")
		modelList = make([]deviceModel, 0, len(lines))
		modelTokens = make(map[string][]int32, len(lines)*2)
		for _, line := range lines {
			line = strings.TrimSpace(line)
			if line == "" || strings.HasPrefix(line, "#") {
				continue
			}
			parts := strings.SplitN(line, "\t", 2)
			if len(parts) != 2 {
				continue
			}
			model := deviceModel{Brand: strings.TrimSpace(parts[0]), Display: strings.TrimSpace(parts[1])}
			if model.Display == "" {
				continue
			}
			tier, ok := brandTier[model.Brand]
			if !ok {
				tier = 5
			}
			model.tier = tier
			index := int32(len(modelList))
			modelList = append(modelList, model)
			for _, token := range modelSearchTokens(model) {
				modelTokens[token] = append(modelTokens[token], index)
			}
		}
		modelKeys = make([]string, 0, len(modelTokens))
		for token := range modelTokens {
			modelKeys = append(modelKeys, token)
		}
		sort.Strings(modelKeys)
	})
}

// modelSearchTokens quebra marca e nome em termos buscaveis, sem repetir.
func modelSearchTokens(model deviceModel) []string {
	seen := map[string]bool{}
	tokens := make([]string, 0, 6)
	for _, field := range []string{model.Brand, model.Display} {
		for _, token := range strings.Fields(foldSearchText(field)) {
			if len(token) == 0 || seen[token] {
				continue
			}
			seen[token] = true
			tokens = append(tokens, token)
		}
	}
	return tokens
}

// searchModels devolve os modelos que casam com todos os termos digitados, em
// qualquer ordem. Cada termo casa por inicio de palavra, entao "sam a1" chega no
// Galaxy A10 sem precisar escrever tudo.
func searchModels(typed string, limit int) []deviceModel {
	loadModelCatalog()
	terms := strings.Fields(foldSearchText(typed))
	if len(terms) == 0 || limit <= 0 {
		return nil
	}

	var matches map[int32]bool
	for _, term := range terms {
		current := map[int32]bool{}
		for _, index := range modelsWithPrefix(term) {
			if matches == nil || matches[index] {
				current[index] = true
			}
		}
		if len(current) == 0 {
			return nil
		}
		matches = current
	}

	found := make([]deviceModel, 0, len(matches))
	for index := range matches {
		found = append(found, modelList[index])
	}
	sort.Slice(found, func(i, j int) bool {
		if found[i].tier != found[j].tier {
			return found[i].tier < found[j].tier
		}
		if len(found[i].Display) != len(found[j].Display) {
			return len(found[i].Display) < len(found[j].Display)
		}
		return found[i].Display < found[j].Display
	})
	if len(found) > limit {
		found = found[:limit]
	}
	return found
}

// modelsWithPrefix devolve os modelos com algum termo comecando pelo prefixo.
func modelsWithPrefix(prefix string) []int32 {
	start := sort.SearchStrings(modelKeys, prefix)
	var out []int32
	for i := start; i < len(modelKeys); i++ {
		if !strings.HasPrefix(modelKeys[i], prefix) {
			break
		}
		out = append(out, modelTokens[modelKeys[i]]...)
	}
	return out
}

// canonicalModel devolve o nome oficial do aparelho para o texto digitado, ou
// vazio quando nao da para ter certeza. E a chave que faz a memoria de preco
// funcionar: sem ela "a14", "samsung a14" e "Galaxy A14" sao tres aparelhos.
func canonicalModel(typed string) string {
	typed = strings.TrimSpace(typed)
	if typed == "" {
		return ""
	}
	found := searchModels(typed, 2)
	if len(found) == 0 {
		return ""
	}
	// Texto que ja e o nome oficial resolve sozinho.
	if foldSearchText(found[0].Display) == foldSearchText(typed) {
		return found[0].Display
	}
	// Com mais de um candidato igualmente bom, nao adivinha: o operador escolhe.
	if len(found) > 1 && len(found[0].Display) == len(found[1].Display) {
		return ""
	}
	return found[0].Display
}

// modelKeyFor devolve a chave de agrupamento do aparelho. Cai no proprio texto
// normalizado quando o modelo nao esta no catalogo, que e o caso de console, TV
// e das marcas de eletrodomestico que tambem passam pelo balcao.
func modelKeyFor(brandModel string) string {
	if canonical := canonicalModel(brandModel); canonical != "" {
		return foldSearchText(canonical)
	}
	return foldSearchText(brandModel)
}
