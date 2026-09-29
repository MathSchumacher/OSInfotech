package main

import (
	"bytes"
	"os"
	"testing"
	"time"
)

func TestWriteThermalReceiptPDF(t *testing.T) {
	path, err := writeThermalReceiptPDF(ServiceOrder{
		ID:             42,
		Status:         statusDelivered,
		Customer:       "Cliente de teste",
		CPF:            "123.456.789-00",
		Phone:          "(51) 99999-9999",
		Device:         "Celular",
		BrandModel:     "Modelo X",
		Serial:         "123456",
		Issue:          "Troca de tela",
		Accessories:    "Capa e carregador",
		Notes:          "Retirar com documento",
		AmountCents:    12500,
		TechnicianName: "Tecnico de teste",
		CreatedAt:      time.Now(),
	}, t.TempDir())
	if err != nil {
		t.Fatalf("writeThermalReceiptPDF() error = %v", err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}
	if !bytes.HasPrefix(data, []byte("%PDF")) {
		t.Fatalf("thermal receipt does not look like a PDF: %q", data[:min(len(data), 12)])
	}
}
