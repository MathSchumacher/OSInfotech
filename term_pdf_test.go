package main

import (
	"context"
	"os"
	"testing"
	"time"
)

func TestWriteTermPDF(t *testing.T) {
	path, err := writeTermPDF(ServiceTerm{
		ID:            1,
		TermType:      defaultTermType,
		Status:        termStatusDraft,
		SellerName:    "Maria Silva",
		SellerCPF:     "000.000.000-00",
		BuyerCompany:  "OSInfotech Assistencia Tecnica",
		DeviceKind:    "Celular",
		Brand:         "LG",
		Model:         "K10",
		Declaration:   defaultTermDeclaration(),
		AmountCents:   10000,
		PaymentMethod: "Pix",
		City:          "Portao/RS",
		TermDate:      time.Date(2026, 6, 17, 0, 0, 0, 0, time.Local),
	}, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(data) < 4 || string(data[:4]) != "%PDF" {
		t.Fatalf("arquivo gerado nao parece PDF: %q", data[:min(len(data), 8)])
	}
}

func TestParseTermDate(t *testing.T) {
	date, err := parseTermDate("17/06/2026")
	if err != nil {
		t.Fatal(err)
	}
	if date.Day() != 17 || date.Month() != 6 || date.Year() != 2026 {
		t.Fatalf("data inesperada: %v", date)
	}
}

func TestListTermsDocumentFilters(t *testing.T) {
	ctx := context.Background()
	store := openTestStore(t)

	draft := ServiceTerm{
		TermType:      defaultTermType,
		Status:        termStatusDraft,
		SellerName:    "Maria Silva",
		BuyerCompany:  "DBS Assistencia",
		BuyerDocument: "44.886.268/0001-48",
		DeviceKind:    "Celular",
		IMEI1:         "123456789",
		AmountCents:   10000,
		TermDate:      time.Now(),
	}
	if err := store.SaveTerm(ctx, &draft); err != nil {
		t.Fatalf("SaveTerm draft error = %v", err)
	}

	generated := ServiceTerm{
		TermType:     defaultTermType,
		Status:       termStatusGenerated,
		SellerName:   "Joao Santos",
		BuyerCompany: "Comprador fixo",
		DeviceKind:   "Notebook",
		PDFKey:       "terms/generated/TERMO-000002.pdf",
		AmountCents:  20000,
		TermDate:     time.Now(),
	}
	if err := store.SaveTerm(ctx, &generated); err != nil {
		t.Fatalf("SaveTerm generated error = %v", err)
	}

	signed := ServiceTerm{
		TermType:    defaultTermType,
		Status:      termStatusSigned,
		SellerName:  "Ana Pereira",
		DeviceKind:  "TV",
		PDFKey:      "terms/generated/TERMO-000003.pdf",
		SignedKey:   "terms/signed/TERMO-000003-assinado.pdf",
		AmountCents: 30000,
		TermDate:    time.Now(),
	}
	if err := store.SaveTerm(ctx, &signed); err != nil {
		t.Fatalf("SaveTerm signed error = %v", err)
	}

	assertCount := func(filter, query string, want int) {
		t.Helper()
		terms, err := store.ListTerms(ctx, query, filter)
		if err != nil {
			t.Fatalf("ListTerms(%q, %q) error = %v", filter, query, err)
		}
		if len(terms) != want {
			t.Fatalf("ListTerms(%q, %q) = %d, want %d", filter, query, len(terms), want)
		}
	}

	assertCount(termFilterAll, "", 3)
	assertCount(termFilterWithoutPDF, "", 1)
	assertCount(termFilterAwaitingSignature, "", 1)
	assertCount(termFilterSigned, "", 1)
	assertCount(termFilterAll, "44886268000148", 1)
	assertCount(termFilterAll, "123456789", 1)
}
