package main

import (
	"strings"
	"testing"
	"time"
)

// TestBuscaDeModeloForaDeOrdem: digitar "13 iph" tem que achar o iPhone 13 tão
// bem quanto "iph 13". Ninguém digita o nome inteiro na ordem certa.
func TestBuscaDeModeloForaDeOrdem(t *testing.T) {
	casos := []struct {
		digitado string
		esperado string
	}{
		{"13 iph", "iPhone 13"},
		{"iph 13", "iPhone 13"},
		{"a14", "Samsung Galaxy A14"},
		{"a14 sam", "Samsung Galaxy A14"},
		{"g54", "Motorola moto g54 5G"},
		{"a03 core", "Samsung Galaxy A03 Core"},
		{"redmi 9", "Redmi 9"},
	}
	for _, caso := range casos {
		found := searchModels(caso.digitado, 5)
		if len(found) == 0 {
			t.Fatalf("%q não achou nada", caso.digitado)
		}
		if found[0].Display != caso.esperado {
			nomes := make([]string, 0, len(found))
			for _, m := range found {
				nomes = append(nomes, m.Display)
			}
			t.Fatalf("%q trouxe %v, esperava %q primeiro", caso.digitado, nomes, caso.esperado)
		}
	}
}

// TestCatalogoNaoInventa: texto que não é aparelho conhecido não vira modelo.
func TestCatalogoNaoInventa(t *testing.T) {
	for _, texto := range []string{"ps4", "xbox 360", "oster", "aoc", ""} {
		if canonical := canonicalModel(texto); canonical != "" {
			t.Fatalf("%q não deveria virar modelo, virou %q", texto, canonical)
		}
	}
	// E o que não está no catálogo ainda agrupa pelo texto normalizado.
	if key := modelKeyFor("PS4 Slim"); key != "ps4 slim" {
		t.Fatalf("chave de agrupamento errada: %q", key)
	}
}

// TestChaveDoModeloUneGrafias: é isso que faz a memória de preço existir.
func TestChaveDoModeloUneGrafias(t *testing.T) {
	base := modelKeyFor("Samsung Galaxy A14")
	for _, escrita := range []string{"a14", "samsung a14", "Galaxy A14", "SAMSUNG GALAXY A14"} {
		if got := modelKeyFor(escrita); got != base {
			t.Fatalf("%q deu chave %q, esperava %q", escrita, got, base)
		}
	}
}

// TestCatalogoCarregaRapido: o índice é montado uma vez e a busca precisa ser
// imperceptível enquanto se digita.
func TestCatalogoCarregaRapido(t *testing.T) {
	loadModelCatalog()
	if len(modelList) < 3000 {
		t.Fatalf("catálogo pequeno demais: %d modelos", len(modelList))
	}
	start := time.Now()
	for i := 0; i < 200; i++ {
		searchModels("sam a1", 5)
	}
	perCall := time.Since(start) / 200
	t.Logf("%d modelos, %v por busca", len(modelList), perCall.Round(time.Microsecond))
	if perCall > 5*time.Millisecond {
		t.Fatalf("busca lenta demais: %v", perCall)
	}
	if !strings.Contains(modelCatalogRaw, "iPhone 13") {
		t.Fatal("o catálogo embutido não veio junto")
	}
}
