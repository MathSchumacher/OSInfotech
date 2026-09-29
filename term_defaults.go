package main

import (
	"context"
	"encoding/json"
)

const termDefaultsSetting = "term_defaults"

type TermDefaults struct {
	TermType       string `json:"term_type"`
	SellerName     string `json:"seller_name"`
	SellerDocument string `json:"seller_document"`
	SellerRG       string `json:"seller_rg"`
	SellerPhone    string `json:"seller_phone"`
	SellerAddress  string `json:"seller_address"`
	BuyerCompany   string `json:"buyer_company"`
	BuyerDocument  string `json:"buyer_document"`
	BuyerAddress   string `json:"buyer_address"`
	BuyerPhone     string `json:"buyer_phone"`
	AmountCents    int64  `json:"amount_cents"`
	PaymentMethod  string `json:"payment_method"`
	City           string `json:"city"`
	Declaration    string `json:"declaration"`
}

func defaultTermDefaults() TermDefaults {
	return TermDefaults{
		TermType:      defaultTermType,
		BuyerCompany:  "DBS Assistencia tecnica",
		BuyerDocument: "44.886.268/0001-48",
		BuyerAddress:  "Rua Morretinhos n741 Sao Jorge Portao RS",
		Declaration:   defaultTermDeclaration(),
	}
}

func (ui *mainUI) loadTermDefaults(ctx context.Context) (TermDefaults, error) {
	value, ok, err := ui.store.GetSetting(ctx, termDefaultsSetting)
	if err != nil {
		return TermDefaults{}, err
	}
	if !ok || value == "" {
		return defaultTermDefaults(), nil
	}

	var defaults TermDefaults
	if err := json.Unmarshal([]byte(value), &defaults); err != nil {
		return TermDefaults{}, err
	}
	return defaults, nil
}

func (ui *mainUI) saveTermDefaults(ctx context.Context, defaults TermDefaults) error {
	data, err := json.Marshal(defaults)
	if err != nil {
		return err
	}
	return ui.store.SetSetting(ctx, termDefaultsSetting, string(data))
}
