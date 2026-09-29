package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// fakeCloud simula o worker: responde o pull em paginas e conta as chamadas.
type fakeCloud struct {
	pages   int
	perPage int
	calls   int
}

func (f *fakeCloud) handler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasPrefix(r.URL.Path, "/v1/sync/pull") {
			http.Error(w, "rota nao usada no teste", http.StatusNotFound)
			return
		}
		f.calls++
		page := f.calls
		changes := make([]map[string]any, 0, f.perPage)
		for i := 0; i < f.perPage; i++ {
			seq := (page-1)*f.perPage + i + 1
			changes = append(changes, map[string]any{
				"sequence":    seq,
				"entity_type": syncEntityCustomer,
				"entity_id":   fmt.Sprintf("cli-%03d", seq),
				"action":      "upsert",
				"revision":    1,
				"changed_at":  time.Now().UTC().Format(time.RFC3339),
				"entity": map[string]any{
					"id":         fmt.Sprintf("cli-%03d", seq),
					"name":       fmt.Sprintf("Cliente %03d", seq),
					"document":   "",
					"phone":      "",
					"revision":   1,
					"created_at": time.Now().UTC().Format(time.RFC3339),
					"updated_at": time.Now().UTC().Format(time.RFC3339),
				},
			})
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"cursor":   page * f.perPage,
			"changes":  changes,
			"has_more": page < f.pages,
		})
	})
}

func newSyncTestStore(t *testing.T, baseURL string) *Store {
	t.Helper()
	store := openTestStore(t)
	ctx := context.Background()
	if err := store.SaveAPIConfig(ctx, APIConfig{BaseURL: baseURL, Token: "token-de-teste"}); err != nil {
		t.Fatal(err)
	}
	if err := store.SetSetting(ctx, "d1_sync_seeded", "1"); err != nil {
		t.Fatal(err)
	}
	return store
}

// TestSyncClearsStaleFailure: uma nova tentativa apaga o erro anterior logo no
// comeco. Sem isso a interface mostrava "Falha ao sincronizar" para sempre.
func TestSyncClearsStaleFailure(t *testing.T) {
	cloud := &fakeCloud{pages: 3, perPage: 2}
	server := httptest.NewServer(cloud.handler())
	defer server.Close()

	store := newSyncTestStore(t, server.URL)
	ctx := context.Background()
	if err := store.SetSetting(ctx, syncLastFailureKey, "erro antigo de ontem"); err != nil {
		t.Fatal(err)
	}
	client, err := NewWorkerClient(APIConfig{BaseURL: server.URL, Token: "token-de-teste"})
	if err != nil {
		t.Fatal(err)
	}

	var progress []SyncSummary
	summary, err := store.SyncD1WithProgress(ctx, client, func(p SyncSummary) {
		progress = append(progress, p)
	})
	if err != nil {
		t.Fatal(err)
	}
	if summary.Pulled != cloud.pages*cloud.perPage {
		t.Fatalf("recebidos = %d, queria %d", summary.Pulled, cloud.pages*cloud.perPage)
	}
	if len(progress) != cloud.pages {
		t.Fatalf("progresso reportado %d vezes, queria %d", len(progress), cloud.pages)
	}
	if progress[0].Pulled != cloud.perPage {
		t.Fatalf("primeiro progresso = %+v", progress[0])
	}
	if failure, found, _ := store.GetSetting(ctx, syncLastFailureKey); found && strings.TrimSpace(failure) != "" {
		t.Fatalf("erro antigo continuou gravado: %q", failure)
	}
	if last, found, _ := store.GetSetting(ctx, syncLastSuccessKey); !found || strings.TrimSpace(last) == "" {
		t.Fatal("sucesso nao foi registrado")
	}
	clients, err := store.ListCustomerProfiles(ctx, "")
	if err != nil {
		t.Fatal(err)
	}
	if len(clients) != cloud.pages*cloud.perPage {
		t.Fatalf("clientes aplicados = %d", len(clients))
	}
}

// TestSyncKeepsCursorAfterFailure: se uma pagina falha no meio, o que ja veio
// continua valendo e a proxima tentativa recomeca de onde parou.
func TestSyncKeepsCursorAfterFailure(t *testing.T) {
	var calls int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if calls == 2 {
			http.Error(w, "instabilidade", http.StatusBadGateway)
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"cursor":   10 * calls,
			"changes":  []map[string]any{},
			"has_more": true,
		})
	}))
	defer server.Close()

	store := newSyncTestStore(t, server.URL)
	ctx := context.Background()
	client, err := NewWorkerClient(APIConfig{BaseURL: server.URL, Token: "token-de-teste"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.SyncD1(ctx, client); err == nil {
		t.Fatal("a falha da segunda pagina deveria ser reportada")
	}
	cursor, found, _ := store.GetSetting(ctx, syncCursorSetting)
	if !found || cursor != "10" {
		t.Fatalf("cursor apos a falha = %q (queria 10)", cursor)
	}
}

// TestRecusaNaoTravaSincronizacao: uma alteração que o servidor recusa não pode
// parar as outras nem ficar na fila para falhar de novo. Como o envio vem antes
// da leitura, uma linha ruim travava também o recebimento — para sempre.
func TestRecusaNaoTravaSincronizacao(t *testing.T) {
	var pushes int
	servidor := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasPrefix(r.URL.Path, "/v1/sync/push"):
			pushes++
			var body struct {
				Changes []SyncQueueEntry `json:"changes"`
			}
			_ = json.NewDecoder(r.Body).Decode(&body)
			results := make([]map[string]any, 0, len(body.Changes))
			for i, change := range body.Changes {
				// A primeira alteração do lote é sempre recusada; as outras passam.
				if i == 0 {
					results = append(results, map[string]any{
						"entity_id":   change.EntityRemote,
						"entity_type": change.EntityType,
						"status":      "rejected",
						"message":     "Cliente, equipamento e motivo sao obrigatorios.",
					})
					continue
				}
				results = append(results, map[string]any{
					"entity_id":   change.EntityRemote,
					"entity_type": change.EntityType,
					"status":      "accepted",
					"revision":    1,
				})
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"results": results})
		case strings.HasPrefix(r.URL.Path, "/v1/sync/pull"):
			_ = json.NewEncoder(w).Encode(map[string]any{"cursor": 0, "changes": []any{}, "has_more": false})
		default:
			http.Error(w, "rota nao usada no teste", http.StatusNotFound)
		}
	}))
	defer servidor.Close()

	store := newSyncTestStore(t, servidor.URL)
	ctx := context.Background()
	for i := 0; i < 3; i++ {
		order := ServiceOrder{
			Status: statusPending, Customer: fmt.Sprintf("Cliente %d", i), Phone: "51999998888",
			Device: "Celular", Issue: "tela", AmountCents: 10000,
		}
		if err := store.Save(ctx, &order); err != nil {
			t.Fatal(err)
		}
	}

	client, err := NewWorkerClient(APIConfig{BaseURL: servidor.URL, Token: "token-de-teste"})
	if err != nil {
		t.Fatal(err)
	}
	summary, err := store.SyncD1(ctx, client)
	if err != nil {
		t.Fatalf("uma recusa não pode devolver erro: %v", err)
	}
	if summary.Rejected == 0 {
		t.Fatal("a recusa deveria ser contada")
	}
	if summary.Pushed == 0 {
		t.Fatal("as alterações boas do mesmo lote deveriam ter subido")
	}
	if summary.RejectedReason == "" {
		t.Fatal("o motivo da recusa precisa chegar ao operador")
	}

	// A recusada saiu da fila: a próxima sincronização não tenta de novo.
	restante, err := store.SyncQueueSize(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if restante != 0 {
		t.Fatalf("a fila deveria ter esvaziado, sobraram %d", restante)
	}
}
