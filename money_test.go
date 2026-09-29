package main

import "testing"

func TestParseBRL(t *testing.T) {
	tests := map[string]int64{
		"":            0,
		"0":           0,
		"10":          1000,
		"10,5":        1050,
		"10,50":       1050,
		"R$ 10,50":    1050,
		"1.234,56":    123456,
		"R$ 1.234,56": 123456,
		"1234.56":     123456,
	}

	for input, want := range tests {
		got, err := parseBRL(input)
		if err != nil {
			t.Fatalf("parseBRL(%q) returned error: %v", input, err)
		}
		if got != want {
			t.Fatalf("parseBRL(%q) = %d, want %d", input, got, want)
		}
	}
}

func TestFormatBRL(t *testing.T) {
	tests := map[int64]string{
		0:       "R$ 0,00",
		1050:    "R$ 10,50",
		123456:  "R$ 1.234,56",
		1234567: "R$ 12.345,67",
	}

	for input, want := range tests {
		if got := formatBRL(input); got != want {
			t.Fatalf("formatBRL(%d) = %q, want %q", input, got, want)
		}
	}
}
