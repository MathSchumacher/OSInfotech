package main

import "testing"

func TestFormatCNPJ(t *testing.T) {
	got := formatCNPJ("44886268000148")
	want := "44.886.268/0001-48"
	if got != want {
		t.Fatalf("formatCNPJ() = %q, want %q", got, want)
	}
}

func TestFormatBrazilPhone(t *testing.T) {
	got := formatBrazilPhone("5199312526")
	want := "(51) 9931-2526"
	if got != want {
		t.Fatalf("formatBrazilPhone() = %q, want %q", got, want)
	}
}

func TestFormatCNPJAddress(t *testing.T) {
	got := formatCNPJAddress(brasilAPICNPJResponse{
		DescricaoTipoLogradouro: "RUA",
		Logradouro:              "MORRETINHOS",
		Numero:                  "741",
		Bairro:                  "PORTAO VELHO",
		Municipio:               "PORTAO",
		UF:                      "RS",
	})
	want := "RUA MORRETINHOS 741 PORTAO VELHO PORTAO RS"
	if got != want {
		t.Fatalf("formatCNPJAddress() = %q, want %q", got, want)
	}
}
