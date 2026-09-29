package main

import (
	"context"
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// TestPonteSobeDeVerdade liga a ponte com uma sessao nova e confere que ela
// responde e chega a produzir um QR. Roda so com DBS_WA_SMOKE=1: abre conexao
// real com o WhatsApp.
func TestPonteSobeDeVerdade(t *testing.T) {
	if os.Getenv("DBS_WA_SMOKE") != "1" {
		t.Skip("smoke da ponte: defina DBS_WA_SMOKE=1")
	}
	// Pasta propria: a ponte segura o arquivo da sessao enquanto vive, e o
	// TempDir do teste falharia ao tentar apagar.
	dir, err := os.MkdirTemp("", "dbs-wa-smoke")
	if err != nil {
		t.Fatal(err)
	}
	args := bridgeArgs{
		Store: filepath.Join(dir, "whatsapp.db"),
		Addr:  "127.0.0.1:18787",
		Token: "segredo-de-teste",
	}
	go serveWhatsAppBridge(args)

	client := &http.Client{Timeout: 5 * time.Second}
	ask := func() (whatsappStatus, int, error) {
		req, _ := http.NewRequestWithContext(context.Background(), http.MethodGet, "http://"+args.Addr+"/status", nil)
		req.Header.Set("X-Bridge-Token", args.Token)
		resp, err := client.Do(req)
		if err != nil {
			return whatsappStatus{}, 0, err
		}
		defer resp.Body.Close()
		var status whatsappStatus
		_ = json.NewDecoder(resp.Body).Decode(&status)
		return status, resp.StatusCode, nil
	}

	var status whatsappStatus
	deadline := time.Now().Add(40 * time.Second)
	_ = dir
	for time.Now().Before(deadline) {
		s, code, err := ask()
		if err == nil && code == http.StatusOK {
			status = s
			if status.QR != "" || status.Error != "" {
				break
			}
		}
		time.Sleep(time.Second)
	}
	t.Logf("status: conectado=%v pareado=%v qr=%d caracteres erro=%q",
		status.Connected, status.LoggedIn, len(status.QR), status.Error)
	if status.QR == "" && status.Error == "" {
		t.Fatal("a ponte não chegou a produzir QR nem reportar erro")
	}
	if status.QR != "" {
		if img := renderQRCode(status.QR, 220); img == nil {
			t.Fatal("o QR real não pôde ser desenhado")
		}
		t.Log("QR real gerado e desenhado com sucesso")
	}

	// A porta não pode aceitar quem não tem o segredo.
	req, _ := http.NewRequest(http.MethodGet, "http://"+args.Addr+"/status", nil)
	resp, reqErr := client.Do(req)
	if reqErr != nil {
		t.Fatal(reqErr)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusForbidden {
		t.Fatalf("sem o segredo a ponte deveria recusar, veio %d", resp.StatusCode)
	}
}
