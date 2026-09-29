package main

import (
	"archive/zip"
	"context"
	"io"
	"os"
	"strings"
	"testing"
	"time"
)

func TestFinancialReportAndExports(t *testing.T) {
	ctx := context.Background()
	store := openTestStore(t)
	insertReportOrder(t, store, "Cliente A", statusDelivered, 10000, 3000, time.Date(2026, 7, 6, 12, 0, 0, 0, time.UTC))
	insertReportOrder(t, store, "Cliente B", statusDelivered, 20000, 5000, time.Date(2026, 7, 7, 12, 0, 0, 0, time.UTC))
	insertReportOrder(t, store, "Cliente fora", statusPending, 50000, 1000, time.Date(2026, 7, 7, 12, 0, 0, 0, time.UTC))

	report, err := store.FinancialReport(ctx,
		time.Date(2026, 7, 1, 0, 0, 0, 0, time.UTC),
		time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC),
		"Relatorio financeiro mensal",
	)
	if err != nil {
		t.Fatal(err)
	}
	if len(report.Orders) != 2 || report.GrossCents != 30000 || report.CostCents != 8000 || report.NetCents != 22000 {
		t.Fatalf("report = %#v", report)
	}

	dir := t.TempDir()
	pdfPath, err := writeFinancialReportPDF(report, dir)
	if err != nil {
		t.Fatalf("writeFinancialReportPDF() error = %v", err)
	}
	pdf, err := os.ReadFile(pdfPath)
	if err != nil || len(pdf) < 4 || string(pdf[:4]) != "%PDF" {
		t.Fatalf("invalid PDF export: len=%d err=%v", len(pdf), err)
	}

	xlsxPath, err := writeFinancialReportXLSX(report, dir)
	if err != nil {
		t.Fatalf("writeFinancialReportXLSX() error = %v", err)
	}
	xlsxSheet := readZipEntry(t, xlsxPath, "xl/worksheets/sheet1.xml")
	if !strings.Contains(xlsxSheet, "SUM(F5:F6)") || !strings.Contains(xlsxSheet, "SUM(G5:G6)") || !strings.Contains(xlsxSheet, "SUM(H5:H6)") {
		t.Fatalf("xlsx formulas missing: %s", xlsxSheet)
	}

	docxPath, err := writeFinancialReportDOCX(report, dir)
	if err != nil {
		t.Fatalf("writeFinancialReportDOCX() error = %v", err)
	}
	doc := readZipEntry(t, docxPath, "word/document.xml")
	if !strings.Contains(doc, "Relatorio financeiro mensal") || !strings.Contains(doc, "Lucro liquido") {
		t.Fatalf("docx content missing: %s", doc)
	}
}

func insertReportOrder(t *testing.T, store *Store, customer string, status string, amount int64, cost int64, updatedAt time.Time) {
	t.Helper()
	createdAt := updatedAt.Add(-2 * time.Hour)
	// O relatorio filtra por data de entrega, entao a OS entregue precisa tê-la.
	deliveredAt := ""
	if status == statusDelivered {
		deliveredAt = encodeSecond(updatedAt)
	}
	_, err := store.db.Exec(`
INSERT INTO service_orders(status, cpf, customer, phone, device, brand_model, serial, issue,
	accessories, notes, amount_cents, cost_cents, technician_id, technician_name,
	delivered_at, created_at, updated_at)
VALUES (?, '', ?, '', 'Celular', 'Modelo', '', 'Servico', '', '', ?, ?, 0, '', ?, ?, ?)`,
		status, customer, amount, cost, deliveredAt, encodeTime(createdAt), encodeTime(updatedAt),
	)
	if err != nil {
		t.Fatal(err)
	}
}

func readZipEntry(t *testing.T, path string, name string) string {
	t.Helper()
	reader, err := zip.OpenReader(path)
	if err != nil {
		t.Fatal(err)
	}
	defer reader.Close()
	for _, file := range reader.File {
		if file.Name != name {
			continue
		}
		rc, err := file.Open()
		if err != nil {
			t.Fatal(err)
		}
		content, err := io.ReadAll(rc)
		_ = rc.Close()
		if err != nil {
			t.Fatal(err)
		}
		return string(content)
	}
	t.Fatalf("zip entry %s not found in %s", name, path)
	return ""
}
