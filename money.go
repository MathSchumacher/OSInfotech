package main

import (
	"fmt"
	"strconv"
	"strings"
	"unicode"
)

func parseBRL(input string) (int64, error) {
	s := strings.TrimSpace(input)
	s = strings.ReplaceAll(s, "R$", "")
	s = strings.ReplaceAll(s, "r$", "")
	s = strings.TrimSpace(s)
	if s == "" {
		return 0, nil
	}

	lastSep := -1
	for i, r := range s {
		if r == ',' || r == '.' {
			lastSep = i
		}
	}

	var whole, frac strings.Builder
	for i, r := range s {
		switch {
		case unicode.IsDigit(r):
			if lastSep >= 0 && i > lastSep {
				frac.WriteRune(r)
			} else {
				whole.WriteRune(r)
			}
		case r == ',' || r == '.' || unicode.IsSpace(r):
			continue
		default:
			return 0, fmt.Errorf("valor invalido: use formato R$ 123,45")
		}
	}

	wholeText := whole.String()
	if wholeText == "" {
		wholeText = "0"
	}

	fracText := frac.String()
	if len(fracText) > 2 {
		return 0, fmt.Errorf("valor invalido: use no maximo duas casas decimais")
	}
	for len(fracText) < 2 {
		fracText += "0"
	}

	reais, err := strconv.ParseInt(wholeText, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("valor invalido: %w", err)
	}
	centavos, err := strconv.ParseInt(fracText, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("centavos invalidos: %w", err)
	}

	return reais*100 + centavos, nil
}

func formatBRL(cents int64) string {
	sign := ""
	if cents < 0 {
		sign = "-"
		cents = -cents
	}

	reais := cents / 100
	centavos := cents % 100
	grouped := formatThousands(reais)

	return fmt.Sprintf("%sR$ %s,%02d", sign, grouped, centavos)
}

func formatThousands(value int64) string {
	raw := strconv.FormatInt(value, 10)
	if len(raw) <= 3 {
		return raw
	}

	var out []byte
	firstGroup := len(raw) % 3
	if firstGroup == 0 {
		firstGroup = 3
	}
	out = append(out, raw[:firstGroup]...)
	for i := firstGroup; i < len(raw); i += 3 {
		out = append(out, '.')
		out = append(out, raw[i:i+3]...)
	}
	return string(out)
}
