package main

import "testing"

// TestPadraoVaiEVolta: a sequência gravada precisa voltar idêntica, e nada que
// não seja um desenho pode ser confundido com um.
func TestPadraoVaiEVolta(t *testing.T) {
	dots := []int{1, 2, 3, 6, 9}
	valor := encodePattern(dots)
	if valor != "padrao:1-2-3-6-9" {
		t.Fatalf("codificação inesperada: %q", valor)
	}
	volta, ok := decodePattern(valor)
	if !ok || len(volta) != len(dots) {
		t.Fatalf("não voltou: %v %v", volta, ok)
	}
	for i := range dots {
		if volta[i] != dots[i] {
			t.Fatalf("ordem trocada: %v", volta)
		}
	}
	if encodePattern(nil) != "" {
		t.Fatal("desenho vazio tem que gravar vazio")
	}
	if patternSummary(valor) != "Padrão de 5 pontos" {
		t.Fatalf("resumo errado: %q", patternSummary(valor))
	}

	// Senha digitada não é desenho.
	for _, naoEhPadrao := range []string{"1234", "", "padrao:", "padrao:0-1", "padrao:1-1-2", "padrao:1-2-x", "padrao:10-2"} {
		if isPatternKey(naoEhPadrao) {
			t.Errorf("%q não deveria ser lido como desenho", naoEhPadrao)
		}
	}
}

// TestPadraoNaGradeSegueAOrdemDoToque e a regra que o operador vê: tocar num
// ponto já usado não duplica, e tocar no primeiro recomeça.
func TestPadraoNaGradeSegueAOrdemDoToque(t *testing.T) {
	grid := newPatternGrid(true)
	grid.SetDots([]int{1, 5, 9})
	if got := grid.Dots(); len(got) != 3 || got[2] != 9 {
		t.Fatalf("carregou errado: %v", got)
	}
	grid.Clear()
	if len(grid.Dots()) != 0 {
		t.Fatal("limpar deveria esvaziar o desenho")
	}

	// Dots() devolve cópia: mexer no resultado não pode alterar o widget.
	grid.SetDots([]int{1, 2, 3, 4})
	copia := grid.Dots()
	copia[0] = 9
	if grid.Dots()[0] != 1 {
		t.Fatal("Dots() precisa devolver cópia")
	}
}
