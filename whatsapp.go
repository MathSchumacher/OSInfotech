package main

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"
)

// ---------------------------------------------------------------------------
// WhatsApp da loja
//
// O app abre uma ponte com o WhatsApp Web da propria loja: o atendente le um QR
// uma vez e, a partir dai, avisar o cliente que o aparelho esta pronto vira um
// toque, na mesma tela da OS. Antes disso o unico caminho era abrir o wa.me,
// trocar de janela e digitar.
//
// A ponte roda no mesmo executavel, num processo separado (ver
// whatsapp_bridge.go). Este arquivo e o lado do app: sobe a ponte quando
// precisa, adota a que ja estiver rodando, e fala com ela por loopback.
//
// Sobre risco: e um cliente nao oficial. O que faz o WhatsApp bloquear um numero
// e disparar para quem nao pediu contato. Aqui o envio e sempre para um cliente
// que deixou um aparelho na loja, sobre a OS dele, com limite diario e so no
// horario comercial. Ainda assim, o numero usado deve ser o da loja, nunca o
// pessoal do dono.
// ---------------------------------------------------------------------------

const (
	whatsappPairedSetting = "whatsapp_paired"
	whatsappDailyCap      = 60 // envios por dia, com folga sobre o uso real da loja
)

// whatsappStatus e o que a ponte diz sobre a ligacao.
type whatsappStatus struct {
	Connected bool   `json:"connected"`
	LoggedIn  bool   `json:"logged_in"`
	Number    string `json:"number"`
	QR        string `json:"qr"`
	QRExpires int64  `json:"qr_expires"`
	Error     string `json:"error"`
}

// Headline e a linha que a tela mostra acima do QR.
func (s whatsappStatus) Headline() string {
	switch {
	case s.Connected:
		return "Conectado como +" + s.Number
	case s.QR != "":
		return fmt.Sprintf("Leia o código no WhatsApp do celular da loja (%ds)", s.QRExpires)
	case s.Error != "":
		return "Falha: " + s.Error
	case s.LoggedIn:
		return "Pareado, reconectando…"
	default:
		return "Iniciando a conexão…"
	}
}

// bridgeHandle guarda como falar com a ponte que esta rodando.
type bridgeHandle struct {
	Addr  string `json:"addr"`
	Token string `json:"token"`
	PID   int    `json:"pid"`
}

type whatsappService struct {
	dataDir string
	http    *http.Client

	mu      sync.Mutex
	handle  bridgeHandle
	process *os.Process
}

func newWhatsAppService(dataDir string) *whatsappService {
	return &whatsappService{
		dataDir: dataDir,
		http:    &http.Client{Timeout: 30 * time.Second},
	}
}

func (w *whatsappService) storePath() string  { return filepath.Join(w.dataDir, "whatsapp.db") }
func (w *whatsappService) handlePath() string { return filepath.Join(w.dataDir, "whatsapp-ponte.json") }

// Available diz se este aparelho pode abrir a ponte. No celular nao pode: o
// WhatsApp da loja fica no computador do balcao.
func (w *whatsappService) Available() bool { return !isAndroid() }

// ensureRunning garante uma ponte no ar, adotando a que existir.
func (w *whatsappService) ensureRunning() (bridgeHandle, error) {
	if !w.Available() {
		return bridgeHandle{}, errors.New("o WhatsApp da loja é configurado no computador")
	}
	w.mu.Lock()
	defer w.mu.Unlock()

	if w.handle.Addr != "" && w.ping(w.handle) {
		return w.handle, nil
	}
	if saved, err := w.readHandle(); err == nil && w.ping(saved) {
		w.handle = saved
		return saved, nil
	}
	return w.spawn()
}

// spawn sobe a ponte. Precisa do lock.
func (w *whatsappService) spawn() (bridgeHandle, error) {
	exe, err := os.Executable()
	if err != nil {
		return bridgeHandle{}, err
	}
	port, err := freeLoopbackPort()
	if err != nil {
		return bridgeHandle{}, err
	}
	secret := make([]byte, 24)
	if _, err := rand.Read(secret); err != nil {
		return bridgeHandle{}, err
	}
	handle := bridgeHandle{
		Addr:  fmt.Sprintf("127.0.0.1:%d", port),
		Token: hex.EncodeToString(secret),
	}
	if err := os.MkdirAll(w.dataDir, 0o755); err != nil {
		return bridgeHandle{}, err
	}
	_ = os.Remove(w.storePath() + ".erro")

	cmd := exec.Command(exe, bridgeFlag,
		"--wa-store", w.storePath(),
		"--wa-addr", handle.Addr,
		"--wa-token", handle.Token)
	cmd.Dir = w.dataDir
	if err := cmd.Start(); err != nil {
		return bridgeHandle{}, err
	}
	handle.PID = cmd.Process.Pid
	w.process = cmd.Process
	go func() { _ = cmd.Wait() }() // nao deixa processo zumbi

	// A ponte leva um instante para abrir a porta.
	deadline := time.Now().Add(12 * time.Second)
	for time.Now().Before(deadline) {
		if w.ping(handle) {
			w.handle = handle
			_ = w.writeHandle(handle)
			return handle, nil
		}
		if reason, err := os.ReadFile(w.storePath() + ".erro"); err == nil && len(reason) > 0 {
			return bridgeHandle{}, errors.New(strings.TrimSpace(string(reason)))
		}
		time.Sleep(250 * time.Millisecond)
	}
	return bridgeHandle{}, errors.New("a ponte do WhatsApp não respondeu a tempo")
}

func (w *whatsappService) ping(handle bridgeHandle) bool {
	if handle.Addr == "" || handle.Token == "" {
		return false
	}
	client := &http.Client{Timeout: 2 * time.Second}
	req, err := http.NewRequest(http.MethodGet, "http://"+handle.Addr+"/status", nil)
	if err != nil {
		return false
	}
	req.Header.Set("X-Bridge-Token", handle.Token)
	resp, err := client.Do(req)
	if err != nil {
		return false
	}
	defer resp.Body.Close()
	return resp.StatusCode == http.StatusOK
}

func (w *whatsappService) readHandle() (bridgeHandle, error) {
	raw, err := os.ReadFile(w.handlePath())
	if err != nil {
		return bridgeHandle{}, err
	}
	var handle bridgeHandle
	if err := json.Unmarshal(raw, &handle); err != nil {
		return bridgeHandle{}, err
	}
	return handle, nil
}

func (w *whatsappService) writeHandle(handle bridgeHandle) error {
	raw, err := json.Marshal(handle)
	if err != nil {
		return err
	}
	return os.WriteFile(w.handlePath(), raw, 0o600)
}

// Status pergunta a ponte como esta a ligacao, subindo a ponte se precisar.
func (w *whatsappService) Status(ctx context.Context) (whatsappStatus, error) {
	handle, err := w.ensureRunning()
	if err != nil {
		return whatsappStatus{}, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "http://"+handle.Addr+"/status", nil)
	if err != nil {
		return whatsappStatus{}, err
	}
	req.Header.Set("X-Bridge-Token", handle.Token)
	resp, err := w.http.Do(req)
	if err != nil {
		return whatsappStatus{}, err
	}
	defer resp.Body.Close()
	var status whatsappStatus
	if err := json.NewDecoder(resp.Body).Decode(&status); err != nil {
		return whatsappStatus{}, err
	}
	return status, nil
}

// Send manda a mensagem. Devolve o motivo quando o WhatsApp recusa, para a tela
// poder explicar em vez de so falhar.
func (w *whatsappService) Send(ctx context.Context, number, text string) error {
	handle, err := w.ensureRunning()
	if err != nil {
		return err
	}
	body, err := json.Marshal(map[string]string{"number": number, "text": text})
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, "http://"+handle.Addr+"/send", bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("X-Bridge-Token", handle.Token)
	req.Header.Set("Content-Type", "application/json")
	resp, err := w.http.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusConflict {
		return errors.New("o WhatsApp da loja não está conectado")
	}
	var out struct {
		Sent   bool   `json:"sent"`
		Reason string `json:"reason"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return fmt.Errorf("resposta inesperada da ponte (%s)", resp.Status)
	}
	if !out.Sent {
		return errors.New(firstNonEmpty(out.Reason, "o WhatsApp não aceitou a mensagem"))
	}
	return nil
}

// Disconnect desfaz o pareamento, para trocar o numero da loja.
func (w *whatsappService) Disconnect(ctx context.Context) error {
	handle, err := w.ensureRunning()
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, "http://"+handle.Addr+"/logout", nil)
	if err != nil {
		return err
	}
	req.Header.Set("X-Bridge-Token", handle.Token)
	resp, err := w.http.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("a ponte recusou desconectar (%s)", resp.Status)
	}
	return nil
}

// Stop encerra a ponte junto com o app.
func (w *whatsappService) Stop() {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.process != nil {
		_ = w.process.Kill()
		w.process = nil
	}
	w.handle = bridgeHandle{}
	_ = os.Remove(w.handlePath())
}

// freeLoopbackPort pega uma porta livre deixando o sistema escolher.
func freeLoopbackPort() (int, error) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return 0, err
	}
	defer listener.Close()
	return listener.Addr().(*net.TCPAddr).Port, nil
}

// whatsappNumber devolve o telefone em digitos com o codigo do pais, que e o
// formato que o WhatsApp espera. Vazio quando o numero nao serve.
func whatsappNumber(phone string) string {
	digits := onlyDigits(phone)
	if len(digits) < 10 {
		return ""
	}
	if !strings.HasPrefix(digits, "55") || len(digits) <= 11 {
		digits = "55" + digits
	}
	if len(digits) < 12 || len(digits) > 13 {
		return ""
	}
	return digits
}

// isAndroid centraliza a checagem, que aparece em mais de um lugar.
func isAndroid() bool { return runtime.GOOS == "android" }
