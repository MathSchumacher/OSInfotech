//go:build !android

package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"sync"
	"syscall"
	"time"

	"go.mau.fi/whatsmeow"
	waE2E "go.mau.fi/whatsmeow/proto/waE2E"
	"go.mau.fi/whatsmeow/store/sqlstore"
	"go.mau.fi/whatsmeow/types"
	waLog "go.mau.fi/whatsmeow/util/log"
	"google.golang.org/protobuf/proto"
)

// ---------------------------------------------------------------------------
// Ponte de WhatsApp
//
// O app fala o protocolo do WhatsApp Web atraves da whatsmeow, mas nao dentro do
// proprio processo: e o mesmo executavel rodado de novo com --whatsapp-bridge,
// conversando por HTTP em loopback. O motivo e simples: uma falha na pilha de
// protocolo derruba o processo, e o processo que nao pode cair e o que registra
// as OS no balcao. Com a ponte separada, o pior caso e ficar sem enviar
// mensagem ate o proximo envio, e nao perder um atendimento pela metade.
//
// Nao ha servidor nem Docker: a sessao mora num SQLite ao lado do banco do app,
// e o pareamento e um QR lido na tela pelo celular da loja.
//
// A porta e local e exige um segredo sorteado a cada execucao. Uma porta local
// sem autenticacao que envia WhatsApp em nome da loja e um buraco, nao uma
// comodidade.
// ---------------------------------------------------------------------------

const bridgeFlag = "--whatsapp-bridge"

// bridgeArgs sao os parametros que o app passa ao filho.
type bridgeArgs struct {
	Store string
	Addr  string
	Token string
}

// parseBridgeArgs le os argumentos sem mexer no flag global, que o Fyne tambem
// usa.
func parseBridgeArgs(argv []string) (bridgeArgs, bool) {
	var args bridgeArgs
	found := false
	for i := 0; i < len(argv); i++ {
		switch argv[i] {
		case bridgeFlag:
			found = true
		case "--wa-store":
			if i+1 < len(argv) {
				args.Store = argv[i+1]
				i++
			}
		case "--wa-addr":
			if i+1 < len(argv) {
				args.Addr = argv[i+1]
				i++
			}
		case "--wa-token":
			if i+1 < len(argv) {
				args.Token = argv[i+1]
				i++
			}
		}
	}
	return args, found
}

// runWhatsAppBridge roda a ponte quando o executavel foi chamado no modo ponte.
// Devolve false quando e uma execucao normal do app.
func runWhatsAppBridge() bool {
	args, found := parseBridgeArgs(os.Args[1:])
	if !found {
		return false
	}
	if args.Token == "" || args.Store == "" || args.Addr == "" {
		return true // chamada invalida: sai sem abrir a interface
	}
	serveWhatsAppBridge(args)
	return true
}

type waBridge struct {
	client *whatsmeow.Client

	mu        sync.Mutex
	qr        string
	qrUntil   time.Time
	lastError string
}

func (b *waBridge) setQR(code string) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.qr = code
	b.qrUntil = time.Now().Add(55 * time.Second)
}

func (b *waBridge) clearQR() {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.qr = ""
	b.qrUntil = time.Time{}
}

func (b *waBridge) currentQR() (string, int64) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.qr == "" || time.Now().After(b.qrUntil) {
		return "", 0
	}
	return b.qr, int64(time.Until(b.qrUntil).Seconds())
}

func (b *waBridge) setError(err error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if err == nil {
		b.lastError = ""
		return
	}
	b.lastError = err.Error()
}

func (b *waBridge) errorText() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.lastError
}

func serveWhatsAppBridge(args bridgeArgs) {
	logger := waLog.Noop
	dsn := fmt.Sprintf("file:%s?_foreign_keys=on&_busy_timeout=5000", args.Store)
	container, err := sqlstore.New(context.Background(), "sqlite3", dsn, logger)
	if err != nil {
		writeBridgeFailure(args, "sessao: "+err.Error())
		return
	}
	device, err := container.GetFirstDevice(context.Background())
	if err != nil {
		writeBridgeFailure(args, "aparelho: "+err.Error())
		return
	}

	bridge := &waBridge{client: whatsmeow.NewClient(device, logger)}
	go bridge.connect()

	mux := http.NewServeMux()
	guard := func(next http.HandlerFunc) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) {
			if r.Header.Get("X-Bridge-Token") != args.Token {
				http.Error(w, "forbidden", http.StatusForbidden)
				return
			}
			next(w, r)
		}
	}

	mux.HandleFunc("/status", guard(func(w http.ResponseWriter, r *http.Request) {
		qr, expires := bridge.currentQR()
		number := ""
		if bridge.client.Store.ID != nil {
			number = bridge.client.Store.ID.User
		}
		writeBridgeJSON(w, map[string]any{
			"connected":  bridge.client.IsConnected() && bridge.client.IsLoggedIn(),
			"logged_in":  bridge.client.IsLoggedIn(),
			"number":     number,
			"qr":         qr,
			"qr_expires": expires,
			"error":      bridge.errorText(),
		})
	}))

	mux.HandleFunc("/send", guard(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "post only", http.StatusMethodNotAllowed)
			return
		}
		var req struct {
			Number string `json:"number"`
			Text   string `json:"text"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "json invalido", http.StatusBadRequest)
			return
		}
		number, text := strings.TrimSpace(req.Number), strings.TrimSpace(req.Text)
		if number == "" || text == "" {
			http.Error(w, "numero e texto sao obrigatorios", http.StatusBadRequest)
			return
		}
		if !bridge.client.IsLoggedIn() {
			http.Error(w, "whatsapp nao conectado", http.StatusConflict)
			return
		}

		ctx, cancel := context.WithTimeout(r.Context(), 25*time.Second)
		defer cancel()

		// Pergunta antes se o numero existe no WhatsApp. Mandar para numero que
		// nao existe e desperdicio e, em volume, e o padrao que marca a conta.
		found, err := bridge.client.IsOnWhatsApp(ctx, []string{"+" + number})
		if err != nil {
			writeBridgeJSON(w, map[string]any{"sent": false, "reason": "consulta falhou: " + err.Error()})
			return
		}
		if len(found) == 0 || !found[0].IsIn {
			writeBridgeJSON(w, map[string]any{"sent": false, "reason": "este numero nao tem WhatsApp"})
			return
		}

		// Envia para o endereco que o servidor devolveu, sem remontar. O
		// WhatsApp devolve alguns destinatarios como @lid, e forcar tudo para
		// @s.whatsapp.net gera um endereco invalido: e o erro que faz nenhuma
		// mensagem sair.
		jid := found[0].JID
		if jid.Server == "" {
			jid.Server = types.DefaultUserServer
		}
		message := &waE2E.Message{Conversation: proto.String(text)}
		sent, err := bridge.client.SendMessage(ctx, jid, message)
		if err != nil {
			writeBridgeJSON(w, map[string]any{"sent": false, "reason": err.Error()})
			return
		}
		writeBridgeJSON(w, map[string]any{"sent": true, "id": sent.ID})
	}))

	mux.HandleFunc("/logout", guard(func(w http.ResponseWriter, r *http.Request) {
		if err := bridge.client.Logout(r.Context()); err != nil {
			http.Error(w, err.Error(), http.StatusBadGateway)
			return
		}
		bridge.clearQR()
		go bridge.connect()
		writeBridgeJSON(w, map[string]any{"ok": true})
	}))

	server := &http.Server{Addr: args.Addr, Handler: mux, ReadHeaderTimeout: 5 * time.Second}
	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt, syscall.SIGTERM)
	go func() {
		<-stop
		shutdown, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		_ = server.Shutdown(shutdown)
	}()

	if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		writeBridgeFailure(args, "porta: "+err.Error())
	}
	bridge.client.Disconnect()
	_ = closeQuietly(container)
}

// connect liga a sessao. Sem sessao salva, abre o laco do QR; com sessao, so
// reconecta.
func (b *waBridge) connect() {
	defer func() {
		// A pilha de protocolo pode entrar em panico numa mudanca do WhatsApp.
		// A ponte cai sozinha e o app continua de pe.
		if r := recover(); r != nil {
			b.setError(fmt.Errorf("falha interna da ponte: %v", r))
		}
	}()
	if b.client.Store.ID == nil {
		qrChan, err := b.client.GetQRChannel(context.Background())
		if err != nil {
			b.setError(err)
			return
		}
		if err := b.client.Connect(); err != nil {
			b.setError(err)
			return
		}
		for event := range qrChan {
			switch event.Event {
			case "code":
				b.setQR(event.Code)
			case "success":
				b.clearQR()
				b.setError(nil)
			case "timeout":
				b.clearQR()
			}
		}
		return
	}
	if err := b.client.Connect(); err != nil {
		b.setError(err)
	}
}

func writeBridgeJSON(w http.ResponseWriter, payload map[string]any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(payload)
}

// writeBridgeFailure deixa o motivo num arquivo ao lado da sessao: a ponte roda
// sem console, entao sem isto a falha some.
func writeBridgeFailure(args bridgeArgs, reason string) {
	_ = os.WriteFile(args.Store+".erro", []byte(reason), 0o600)
}

func closeQuietly(container *sqlstore.Container) error {
	if container == nil {
		return nil
	}
	return container.Close()
}

var _ = sql.ErrNoRows
