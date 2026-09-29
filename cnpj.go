package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
)

const brasilAPICNPJURL = "https://brasilapi.com.br/api/cnpj/v1/"

type CNPJCompany struct {
	Name     string
	Document string
	Phone    string
	Address  string
}

type brasilAPICNPJResponse struct {
	CNPJ                      string `json:"cnpj"`
	RazaoSocial               string `json:"razao_social"`
	NomeFantasia              string `json:"nome_fantasia"`
	DescricaoTipoLogradouro   string `json:"descricao_tipo_de_logradouro"`
	Logradouro                string `json:"logradouro"`
	Numero                    string `json:"numero"`
	Complemento               string `json:"complemento"`
	Bairro                    string `json:"bairro"`
	Municipio                 string `json:"municipio"`
	UF                        string `json:"uf"`
	CEP                       string `json:"cep"`
	DDDTelefone1              string `json:"ddd_telefone_1"`
	DDDTelefone2              string `json:"ddd_telefone_2"`
	DescricaoSituacao         string `json:"descricao_situacao_cadastral"`
	DescricaoMotivoSituacao   string `json:"descricao_motivo_situacao_cadastral"`
	DescricaoMatrizFilial     string `json:"descricao_identificador_matriz_filial"`
	CNAEFiscalDescricao       string `json:"cnae_fiscal_descricao"`
	NaturezaJuridica          string `json:"natureza_juridica"`
	NomeCidadeNoExterior      string `json:"nome_cidade_no_exterior"`
	EnteFederativoResponsavel string `json:"ente_federativo_responsavel"`
}

func lookupCNPJ(ctx context.Context, document string) (CNPJCompany, error) {
	digits := onlyDigits(document)
	if len(digits) != 14 {
		return CNPJCompany{}, errors.New("informe um CNPJ com 14 digitos")
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, brasilAPICNPJURL+digits, nil)
	if err != nil {
		return CNPJCompany{}, err
	}
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		return CNPJCompany{}, err
	}
	defer res.Body.Close()

	if res.StatusCode == http.StatusNotFound {
		return CNPJCompany{}, errors.New("CNPJ nao encontrado")
	}
	if res.StatusCode < 200 || res.StatusCode > 299 {
		return CNPJCompany{}, fmt.Errorf("consulta CNPJ falhou: HTTP %d", res.StatusCode)
	}

	var data brasilAPICNPJResponse
	if err := json.NewDecoder(res.Body).Decode(&data); err != nil {
		return CNPJCompany{}, err
	}

	name := strings.TrimSpace(data.RazaoSocial)
	if name == "" {
		name = strings.TrimSpace(data.NomeFantasia)
	}
	return CNPJCompany{
		Name:     name,
		Document: formatCNPJ(firstNonEmpty(data.CNPJ, digits)),
		Phone:    formatBrazilPhone(firstNonEmpty(data.DDDTelefone1, data.DDDTelefone2)),
		Address:  formatCNPJAddress(data),
	}, nil
}

func formatCNPJAddress(data brasilAPICNPJResponse) string {
	streetParts := []string{
		strings.TrimSpace(data.DescricaoTipoLogradouro),
		strings.TrimSpace(data.Logradouro),
	}
	street := strings.TrimSpace(strings.Join(nonEmptyParts(streetParts...), " "))
	parts := []string{street, strings.TrimSpace(data.Numero), strings.TrimSpace(data.Complemento), strings.TrimSpace(data.Bairro), strings.TrimSpace(data.Municipio), strings.TrimSpace(data.UF)}
	return strings.Join(nonEmptyParts(parts...), " ")
}

func formatCNPJ(digits string) string {
	digits = onlyDigits(digits)
	if len(digits) != 14 {
		return digits
	}
	return fmt.Sprintf("%s.%s.%s/%s-%s", digits[0:2], digits[2:5], digits[5:8], digits[8:12], digits[12:14])
}

func formatBrazilPhone(digits string) string {
	digits = onlyDigits(digits)
	switch len(digits) {
	case 10:
		return fmt.Sprintf("(%s) %s-%s", digits[0:2], digits[2:6], digits[6:10])
	case 11:
		return fmt.Sprintf("(%s) %s-%s", digits[0:2], digits[2:7], digits[7:11])
	default:
		return digits
	}
}

func nonEmptyParts(parts ...string) []string {
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part != "" {
			out = append(out, part)
		}
	}
	return out
}
