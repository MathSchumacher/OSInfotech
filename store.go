package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"

	_ "github.com/mattn/go-sqlite3"
)

const (
	statusPending      = "Aguardando aprovação"
	statusApproved     = "Aprovado"
	statusUnauthorized = "Não autorizado"
	statusNoRepair     = "Sem conserto"
	// statusReady separa "consertado, esperando o cliente vir buscar" de
	// "entregue". Sem ele nao havia como avisar que o aparelho esta pronto: a OS
	// pulava de aprovada direto para entregue.
	statusReady     = "Pronto para retirada"
	statusDelivered = "Entregue"

	termStatusDraft     = "Rascunho"
	termStatusGenerated = "Gerado"
	termStatusSigned    = "Assinado"
)

var orderStatuses = []string{statusPending, statusApproved, statusReady, statusUnauthorized, statusNoRepair, statusDelivered}

var termStatuses = []string{termStatusDraft, termStatusGenerated, termStatusSigned}

// Estados possiveis de cada item da checklist de entrada do aparelho.
const (
	checkStateNone   = ""
	checkStateOK     = "ok"
	checkStateDefect = "defeito"
	checkStateNA     = "na"
)

// ChecklistItem representa uma condicao verificada na entrada do aparelho.
type ChecklistItem struct {
	Label string `json:"label"`
	State string `json:"state"`
}

// OrderChecklist e o preenchimento da checklist para uma OS. Guarda um snapshot
// do nome/itens do template no momento do preenchimento, para o historico nao
// mudar se o template for editado depois.
type OrderChecklist struct {
	TemplateID   int64           `json:"template_id,omitempty"`
	TemplateName string          `json:"template_name,omitempty"`
	DeviceType   string          `json:"device_type,omitempty"`
	Items        []ChecklistItem `json:"items,omitempty"`
}

// OrderPhoto e uma foto do estado de entrada do aparelho, hospedada no R2.
type OrderPhoto struct {
	URL string `json:"url"`
	Key string `json:"key"`
}

// ChecklistTemplate e uma lista de checklist reutilizavel, editavel pelo usuario
// e opcionalmente vinculada a um tipo de aparelho (Celular, Console, TV...).
type ChecklistTemplate struct {
	ID         int64
	Name       string
	DeviceType string
	Items      []string
	CreatedAt  time.Time
	UpdatedAt  time.Time
}

type ServiceOrder struct {
	ID             int64
	Status         string
	CPF            string
	Customer       string
	Phone          string
	Device         string
	BrandModel     string
	Serial         string
	DeviceKey      string // senha ou desenho de desbloqueio, nunca impresso
	ModelKey       string // modelo normalizado pelo catalogo (derivado)
	ServiceKey     string // tipo de servico reconhecido no defeito (derivado)
	Issue          string
	Accessories    string
	Notes          string
	AmountCents    int64
	CostCents      int64
	TechnicianID   int64
	TechnicianName string
	Checklist      OrderChecklist
	Photos         []OrderPhoto
	// DeliveredAt e carimbado pelo Save quando o status vira Entregue, e e a
	// data que o relatorio financeiro usa. PaidAt vazio numa OS entregue
	// significa que o cliente ficou devendo.
	DeliveredAt   time.Time
	PaidAt        time.Time
	PaymentMethod string
	// PaidCents e quanto o cliente ja entregou, e nao se pagou: sinal e
	// pagamento parcial sao praticas da loja.
	PaidCents int64
	// ContactName e ContactPhone sao o segundo caminho ate o cliente: a mae, a
	// esposa, o filho que atende.
	ContactName  string
	ContactPhone string
	// ContactedAt e so de leitura aqui: quem grava e o MarkOrderContacted. Ficar
	// fora do Save evita que um formulario que nao conhece o campo o apague.
	ContactedAt time.Time
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

// encodeChecklist/decodeChecklist e encodePhotos/decodePhotos convertem entre os
// tipos e o JSON guardado nas colunas TEXT.
func encodeChecklist(c OrderChecklist) string {
	if len(c.Items) == 0 && c.TemplateID == 0 && c.TemplateName == "" {
		return ""
	}
	data, err := json.Marshal(c)
	if err != nil {
		return ""
	}
	return string(data)
}

func remoteChecklistText(c *OrderChecklist) string {
	if c == nil {
		return ""
	}
	return encodeChecklist(*c)
}

func decodeChecklist(raw string) OrderChecklist {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return OrderChecklist{}
	}
	var c OrderChecklist
	if err := json.Unmarshal([]byte(raw), &c); err != nil {
		return OrderChecklist{}
	}
	return c
}

func encodePhotos(photos []OrderPhoto) string {
	if len(photos) == 0 {
		return ""
	}
	data, err := json.Marshal(photos)
	if err != nil {
		return ""
	}
	return string(data)
}

func decodePhotos(raw string) []OrderPhoto {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil
	}
	var photos []OrderPhoto
	if err := json.Unmarshal([]byte(raw), &photos); err != nil {
		return nil
	}
	return photos
}

func (order ServiceOrder) NetCents() int64 {
	return order.AmountCents - order.CostCents
}

type FinancialTotals struct {
	GrossCents int64
	CostCents  int64
	NetCents   int64
	Orders     int
	// MissingCostOrders e quantas OS entregues nao tem custo lancado. Sem esse
	// numero o app mostrava bruto menos quase nada e chamava de lucro.
	MissingCostOrders int
	UnpaidOrders      int
	UnpaidCents       int64
}

type FinancialReport struct {
	Title      string
	PeriodText string
	Start      time.Time
	End        time.Time
	Orders     []ServiceOrder
	GrossCents int64
	CostCents  int64
	NetCents   int64
	// Ver FinancialTotals: sem isto o resultado do periodo parece lucro.
	MissingCostOrders int
	UnpaidOrders      int
	UnpaidCents       int64
}

// orderOwedCents diz quanto falta o cliente pagar nesta OS.
//
// Duas regras que parecem detalhe e nao sao. Primeira: "sem pagamento
// registrado" nao e divida — as OS entregues antes da 0.12.0 nao tem forma de
// pagamento, e trata-las como divida inventaria cobranca que ninguem fez.
// Segunda: o que falta e a diferenca, e nao o valor cheio, porque sinal e
// pagamento parcial sao praticas da loja ("foi pago 100 dos 220, falta 150").
func orderOwedCents(order ServiceOrder) int64 {
	if order.Status != statusDelivered || order.PaymentMethod == "" {
		return 0
	}
	if owed := order.AmountCents - order.PaidCents; owed > 0 {
		return owed
	}
	return 0
}

// orderIsOwing diz se sobrou algo a receber nesta OS.
func orderIsOwing(order ServiceOrder) bool { return orderOwedCents(order) > 0 }

// CostIsPartial diz se o resultado do periodo foi calculado com custo faltando,
// e portanto nao pode ser chamado de lucro.
func (r FinancialReport) CostIsPartial() bool {
	return r.MissingCostOrders > 0
}

type CustomerProfile struct {
	Key        string
	Name       string
	CPF        string
	Phone      string
	AvatarURL  string
	AvatarKey  string
	Orders     int
	LastSeen   time.Time
	LastDevice string
	LastModel  string
}

type ServiceTerm struct {
	ID            int64
	TermType      string
	Status        string
	SellerName    string
	SellerCPF     string
	SellerRG      string
	SellerPhone   string
	SellerAddress string
	BuyerCompany  string
	BuyerDocument string
	BuyerAddress  string
	BuyerPhone    string
	DeviceKind    string
	Brand         string
	Model         string
	Color         string
	IMEI1         string
	IMEI2         string
	Accessories   string
	Declaration   string
	AmountCents   int64
	PaymentMethod string
	City          string
	TermDate      time.Time
	PDFKey        string
	PDFURL        string
	SignedKey     string
	SignedURL     string
	CreatedAt     time.Time
	UpdatedAt     time.Time
}

type Store struct {
	db *sql.DB
}

func defaultDatabasePath() (string, error) {
	// DBS_DATA_DIR permite rodar uma copia de teste com dados separados da
	// instalacao normal (ex.: build de previa apontando para a nuvem sem
	// misturar com o banco local do desenvolvedor).
	if dir := strings.TrimSpace(os.Getenv("DBS_DATA_DIR")); dir != "" {
		return filepath.Join(dir, "osinfotech.db"), nil
	}
	// Modo portatil: uma pasta "dados" ao lado do executavel vira a pasta de
	// dados deste app (o instalador normal nao cria essa pasta).
	if dir, ok := portableDataDir(); ok {
		return filepath.Join(dir, "osinfotech.db"), nil
	}
	base, err := databaseBaseDir(runtime.GOOS, os.Getenv("FILESDIR"), os.UserConfigDir)
	if err != nil {
		return "", err
	}
	return filepath.Join(base, "OSInfotech", "osinfotech.db"), nil
}

// portableDataDir devolve <pasta do exe>/dados quando ela existe.
func portableDataDir() (string, bool) {
	if runtime.GOOS == "android" {
		return "", false
	}
	exe, err := os.Executable()
	if err != nil {
		return "", false
	}
	dir := filepath.Join(filepath.Dir(exe), "dados")
	if info, err := os.Stat(dir); err == nil && info.IsDir() {
		return dir, true
	}
	return "", false
}

func databaseBaseDir(goos string, filesDir string, userConfigDir func() (string, error)) (string, error) {
	if goos == "android" {
		filesDir = strings.TrimSpace(filesDir)
		if filesDir == "" {
			return "", errors.New("diretorio privado do Android indisponivel")
		}
		return filesDir, nil
	}
	return userConfigDir()
}

func OpenStore(path string) (*Store, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, err
	}

	db, err := sql.Open(searchDriverName, path)
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1)

	s := &Store{db: db}
	if err := s.configure(context.Background()); err != nil {
		_ = db.Close()
		return nil, err
	}
	if err := s.migrate(context.Background()); err != nil {
		_ = db.Close()
		return nil, err
	}
	return s, nil
}

func (s *Store) Close() error {
	return s.db.Close()
}

func (s *Store) configure(ctx context.Context) error {
	pragmas := []string{
		"PRAGMA journal_mode=WAL;",
		"PRAGMA synchronous=NORMAL;",
		"PRAGMA busy_timeout=5000;",
		"PRAGMA foreign_keys=ON;",
	}
	for _, pragma := range pragmas {
		if _, err := s.db.ExecContext(ctx, pragma); err != nil {
			return err
		}
	}
	return nil
}

func (s *Store) migrate(ctx context.Context) error {
	const schema = `
CREATE TABLE IF NOT EXISTS service_orders (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	status TEXT NOT NULL,
	cpf TEXT NOT NULL DEFAULT '',
	customer TEXT NOT NULL,
	phone TEXT NOT NULL DEFAULT '',
	device TEXT NOT NULL,
	brand_model TEXT NOT NULL DEFAULT '',
	serial TEXT NOT NULL DEFAULT '',
	issue TEXT NOT NULL,
	accessories TEXT NOT NULL DEFAULT '',
	notes TEXT NOT NULL DEFAULT '',
	amount_cents INTEGER NOT NULL DEFAULT 0,
	cost_cents INTEGER NOT NULL DEFAULT 0,
	technician_id INTEGER NOT NULL DEFAULT 0,
	technician_name TEXT NOT NULL DEFAULT '',
	created_at TEXT NOT NULL,
	updated_at TEXT NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_service_orders_status_updated ON service_orders(status, updated_at DESC);
CREATE INDEX IF NOT EXISTS idx_service_orders_customer ON service_orders(customer);
CREATE INDEX IF NOT EXISTS idx_service_orders_cpf ON service_orders(cpf);
CREATE TABLE IF NOT EXISTS checklist_templates (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	name TEXT NOT NULL,
	device_type TEXT NOT NULL DEFAULT '',
	items TEXT NOT NULL DEFAULT '[]',
	created_at TEXT NOT NULL,
	updated_at TEXT NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_checklist_templates_device ON checklist_templates(device_type, name);
CREATE TABLE IF NOT EXISTS app_settings (
	key TEXT PRIMARY KEY,
	value TEXT NOT NULL
);
CREATE TABLE IF NOT EXISTS customer_profiles (
	normalized_name TEXT PRIMARY KEY,
	name TEXT NOT NULL,
	cpf TEXT NOT NULL DEFAULT '',
	phone TEXT NOT NULL DEFAULT '',
	avatar_url TEXT NOT NULL DEFAULT '',
	avatar_key TEXT NOT NULL DEFAULT '',
	last_device TEXT NOT NULL DEFAULT '',
	last_model TEXT NOT NULL DEFAULT '',
	last_seen TEXT NOT NULL DEFAULT '',
	created_at TEXT NOT NULL,
	updated_at TEXT NOT NULL
);
CREATE TABLE IF NOT EXISTS sales_terms (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	term_type TEXT NOT NULL DEFAULT '',
	status TEXT NOT NULL DEFAULT 'Rascunho',
	seller_name TEXT NOT NULL DEFAULT '',
	seller_cpf TEXT NOT NULL DEFAULT '',
	seller_rg TEXT NOT NULL DEFAULT '',
	seller_phone TEXT NOT NULL DEFAULT '',
	seller_address TEXT NOT NULL DEFAULT '',
	buyer_company TEXT NOT NULL DEFAULT '',
	buyer_document TEXT NOT NULL DEFAULT '',
	buyer_address TEXT NOT NULL DEFAULT '',
	buyer_phone TEXT NOT NULL DEFAULT '',
	device_kind TEXT NOT NULL DEFAULT '',
	brand TEXT NOT NULL DEFAULT '',
	model TEXT NOT NULL DEFAULT '',
	color TEXT NOT NULL DEFAULT '',
	imei1 TEXT NOT NULL DEFAULT '',
	imei2 TEXT NOT NULL DEFAULT '',
	accessories TEXT NOT NULL DEFAULT '',
	declaration TEXT NOT NULL DEFAULT '',
	amount_cents INTEGER NOT NULL DEFAULT 0,
	payment_method TEXT NOT NULL DEFAULT '',
	city TEXT NOT NULL DEFAULT '',
	term_date TEXT NOT NULL DEFAULT '',
	pdf_key TEXT NOT NULL DEFAULT '',
	pdf_url TEXT NOT NULL DEFAULT '',
	signed_key TEXT NOT NULL DEFAULT '',
	signed_url TEXT NOT NULL DEFAULT '',
	created_at TEXT NOT NULL,
	updated_at TEXT NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_sales_terms_status_updated ON sales_terms(status, updated_at DESC);
CREATE INDEX IF NOT EXISTS idx_sales_terms_seller ON sales_terms(seller_name);
CREATE INDEX IF NOT EXISTS idx_sales_terms_device ON sales_terms(device_kind, brand, model);
CREATE TABLE IF NOT EXISTS app_users (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	name TEXT NOT NULL,
	login TEXT NOT NULL UNIQUE,
	password_hash TEXT NOT NULL,
	role TEXT NOT NULL,
	active INTEGER NOT NULL DEFAULT 1,
	created_at TEXT NOT NULL,
	updated_at TEXT NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_app_users_active_login ON app_users(active, login);
CREATE TABLE IF NOT EXISTS audit_events (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	actor_user_id INTEGER NOT NULL,
	actor_name TEXT NOT NULL,
	action TEXT NOT NULL,
	subject_type TEXT NOT NULL,
	subject_id INTEGER NOT NULL DEFAULT 0,
	subject_name TEXT NOT NULL DEFAULT '',
	details TEXT NOT NULL DEFAULT '',
	created_at TEXT NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_audit_events_created_at ON audit_events(created_at DESC, id DESC);
CREATE TABLE IF NOT EXISTS sync_entity_map (
	entity_type TEXT NOT NULL,
	local_key TEXT NOT NULL,
	remote_id TEXT NOT NULL,
	revision INTEGER NOT NULL DEFAULT 0,
	updated_at TEXT NOT NULL,
	PRIMARY KEY(entity_type, local_key),
	UNIQUE(entity_type, remote_id)
);
CREATE TABLE IF NOT EXISTS sync_queue (
	entity_type TEXT NOT NULL,
	entity_remote_id TEXT NOT NULL,
	local_key TEXT NOT NULL,
	action TEXT NOT NULL CHECK(action IN ('upsert', 'delete')),
	base_revision INTEGER NOT NULL DEFAULT 0,
	payload_json TEXT NOT NULL DEFAULT '{}',
	queued_at TEXT NOT NULL,
	PRIMARY KEY(entity_type, entity_remote_id)
);
CREATE INDEX IF NOT EXISTS idx_sync_queue_queued_at ON sync_queue(queued_at, entity_type);
CREATE TABLE IF NOT EXISTS whatsapp_log (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	order_id INTEGER NOT NULL DEFAULT 0,
	number TEXT NOT NULL DEFAULT '',
	kind TEXT NOT NULL DEFAULT '',
	sent_at TEXT NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_whatsapp_log_sent_at ON whatsapp_log(sent_at DESC);
CREATE TABLE IF NOT EXISTS customer_distinct (
	left_key TEXT NOT NULL,
	right_key TEXT NOT NULL,
	created_at TEXT NOT NULL,
	PRIMARY KEY(left_key, right_key)
);
`
	if _, err := s.db.ExecContext(ctx, schema); err != nil {
		return err
	}
	for _, migration := range []struct {
		column     string
		definition string
	}{
		{column: "cost_cents", definition: "cost_cents INTEGER NOT NULL DEFAULT 0"},
		{column: "technician_id", definition: "technician_id INTEGER NOT NULL DEFAULT 0"},
		{column: "technician_name", definition: "technician_name TEXT NOT NULL DEFAULT ''"},
		{column: "checklist", definition: "checklist TEXT NOT NULL DEFAULT ''"},
		{column: "photos", definition: "photos TEXT NOT NULL DEFAULT ''"},
		// Data da ultima cobranca por WhatsApp. Fica so neste aparelho por
		// enquanto: o servidor ainda nao conhece o campo.
		{column: "contacted_at", definition: "contacted_at TEXT NOT NULL DEFAULT ''"},
		// Senha ou desenho de desbloqueio do aparelho. 89 das 219 OS ja
		// escreviam isso no meio do defeito, que e impresso no comprovante.
		{column: "device_key", definition: "device_key TEXT NOT NULL DEFAULT ''"},
		// Chaves derivadas do modelo e do defeito, usadas pela memoria de preco.
		// Nao sao digitadas: saem do catalogo e do texto ja gravado, entao cada
		// aparelho chega ao mesmo valor sem precisar sincronizar.
		{column: "model_key", definition: "model_key TEXT NOT NULL DEFAULT ''"},
		{column: "service_key", definition: "service_key TEXT NOT NULL DEFAULT ''"},
		// Fechamento da OS. Sem delivered_at o relatorio usava a data da ultima
		// edicao como data de entrega, entao corrigir uma OS antiga movia a
		// receita de mes. E sem pagamento, "Entregue" nao dizia se a loja
		// recebeu: 96 OS entregues e nenhuma forma de saber quem ficou devendo.
		{column: "delivered_at", definition: "delivered_at TEXT NOT NULL DEFAULT ''"},
		{column: "paid_at", definition: "paid_at TEXT NOT NULL DEFAULT ''"},
		{column: "payment_method", definition: "payment_method TEXT NOT NULL DEFAULT ''"},
		// Contato alternativo. O campo do telefone virou campo de recado: 11 OS
		// da base escrevem quem atende dentro dele ("51 99917-0194 - NANDO",
		// "51999373754 ESPOSA"), e hoje isso nem e mais possivel digitar, porque
		// a mascara apaga letras. A loja perdeu uma capacidade que usava.
		{column: "contact_name", definition: "contact_name TEXT NOT NULL DEFAULT ''"},
		{column: "contact_phone", definition: "contact_phone TEXT NOT NULL DEFAULT ''"},
		// Quanto o cliente ja pagou. O modelo binario da 0.12.0 nao tem onde
		// guardar sinal nem pagamento parcial, e a base escreve isso em texto
		// livre: "100 de entrada", "foi pago 100 dos 220, falta 150".
		{column: "paid_cents", definition: "paid_cents INTEGER NOT NULL DEFAULT 0"},
	} {
		if err := s.addServiceOrderColumn(ctx, migration.column, migration.definition); err != nil {
			return err
		}
	}
	if _, err := s.db.ExecContext(ctx, "CREATE INDEX IF NOT EXISTS idx_service_orders_technician_updated ON service_orders(technician_id, updated_at DESC)"); err != nil {
		return err
	}
	if _, err := s.db.ExecContext(ctx, "CREATE INDEX IF NOT EXISTS idx_service_orders_delivered ON service_orders(delivered_at)"); err != nil {
		return err
	}
	// As OS entregues antes desta versao nao tem data de entrega. A melhor
	// estimativa disponivel e a data da ultima edicao, que e exatamente o que o
	// relatorio ja usava. Carimbar uma vez congela essa atribuicao: daqui para
	// frente editar uma OS antiga nao move mais dinheiro de mes.
	// Quem ja tinha pagamento registrado pagou o valor cheio: era o unico
	// estado que o modelo binario sabia representar.
	if _, err := s.db.ExecContext(ctx,
		"UPDATE service_orders SET paid_cents = amount_cents "+
			"WHERE paid_at <> '' AND paid_cents = 0 AND amount_cents > 0"); err != nil {
		return err
	}
	if _, err := s.db.ExecContext(ctx,
		"UPDATE service_orders SET delivered_at = substr(updated_at, 1, 19) || 'Z' "+
			"WHERE status = ? AND delivered_at = '' AND length(updated_at) >= 19",
		statusDelivered); err != nil {
		return err
	}
	// Renomeia status legados para os novos rótulos de forma idempotente.
	for _, rename := range []struct{ from, to string }{
		{from: "Pendente", to: statusPending},
		{from: "Aprovada", to: statusApproved},
	} {
		if _, err := s.db.ExecContext(ctx, "UPDATE service_orders SET status = ? WHERE status = ?", rename.to, rename.from); err != nil {
			return err
		}
	}
	return nil
}

func (s *Store) addServiceOrderColumn(ctx context.Context, column string, definition string) error {
	rows, err := s.db.QueryContext(ctx, "PRAGMA table_info(service_orders)")
	if err != nil {
		return err
	}
	defer rows.Close()

	for rows.Next() {
		var (
			index      int
			name       string
			dataType   string
			notNull    int
			defaultVal any
			primaryKey int
		)
		if err := rows.Scan(&index, &name, &dataType, &notNull, &defaultVal, &primaryKey); err != nil {
			return err
		}
		if name == column {
			return nil
		}
	}
	if err := rows.Err(); err != nil {
		return err
	}
	_, err = s.db.ExecContext(ctx, "ALTER TABLE service_orders ADD COLUMN "+definition)
	return err
}

func (s *Store) Save(ctx context.Context, order *ServiceOrder) error {
	now := time.Now().UTC()
	applyOrderKeys(order)
	if order.Status == "" {
		order.Status = statusPending
	}
	order.UpdatedAt = now
	if err := s.stampDelivery(ctx, order, now); err != nil {
		return err
	}

	if order.ID == 0 {
		order.CreatedAt = now
		res, err := s.db.ExecContext(ctx, `
INSERT INTO service_orders (
	status, cpf, customer, phone, device, brand_model, serial, device_key, issue,
	accessories, notes, amount_cents, cost_cents, technician_id, technician_name,
	checklist, photos, model_key, service_key, delivered_at, paid_at, payment_method,
	paid_cents, contact_name, contact_phone, created_at, updated_at
) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			order.Status, order.CPF, order.Customer, order.Phone, order.Device,
			order.BrandModel, order.Serial, order.DeviceKey, order.Issue, order.Accessories,
			order.Notes, order.AmountCents, order.CostCents, order.TechnicianID,
			order.TechnicianName, encodeChecklist(order.Checklist), encodePhotos(order.Photos),
			order.ModelKey, order.ServiceKey,
			encodeSecond(order.DeliveredAt), encodeSecond(order.PaidAt), order.PaymentMethod,
			order.PaidCents, order.ContactName, order.ContactPhone,
			encodeTime(order.CreatedAt), encodeTime(order.UpdatedAt),
		)
		if err != nil {
			return err
		}
		id, err := res.LastInsertId()
		if err != nil {
			return err
		}
		order.ID = id
		if err := s.UpsertCustomerProfile(ctx, *order); err != nil {
			return err
		}
		return s.queueOrder(ctx, *order)
	}

	_, err := s.db.ExecContext(ctx, `
UPDATE service_orders SET
	status = ?, cpf = ?, customer = ?, phone = ?, device = ?, brand_model = ?,
	serial = ?, device_key = ?, issue = ?, accessories = ?, notes = ?, amount_cents = ?,
	cost_cents = ?, technician_id = ?, technician_name = ?, checklist = ?,
	photos = ?, model_key = ?, service_key = ?, delivered_at = ?, paid_at = ?,
	payment_method = ?, paid_cents = ?, contact_name = ?, contact_phone = ?,
	updated_at = ?
WHERE id = ?`,
		order.Status, order.CPF, order.Customer, order.Phone, order.Device,
		order.BrandModel, order.Serial, order.DeviceKey, order.Issue, order.Accessories,
		order.Notes, order.AmountCents, order.CostCents, order.TechnicianID,
		order.TechnicianName, encodeChecklist(order.Checklist), encodePhotos(order.Photos),
		order.ModelKey, order.ServiceKey,
		encodeSecond(order.DeliveredAt), encodeSecond(order.PaidAt), order.PaymentMethod,
		order.PaidCents, order.ContactName, order.ContactPhone,
		encodeTime(order.UpdatedAt), order.ID,
	)
	if err != nil {
		return err
	}
	if err := s.UpsertCustomerProfile(ctx, *order); err != nil {
		return err
	}
	return s.queueOrder(ctx, *order)
}

// stampDelivery decide a data de entrega da OS. Fica aqui, e nao em quem chama,
// porque ha quatro caminhos que gravam OS (formulario, menu da lista, uniao de
// clientes e a sincronizacao) e todos passam pelo Save. Antes disso o relatorio
// usava a data da ultima edicao, e corrigir uma OS antiga movia a receita de
// mes.
func (s *Store) stampDelivery(ctx context.Context, order *ServiceOrder, now time.Time) error {
	if order.Status != statusDelivered {
		// Desfez a entrega: a OS voltou para a bancada e nao tem data de
		// entrega. O pagamento fica: dinheiro que entrou nao volta sozinho, e
		// so OS entregue conta no relatorio.
		order.DeliveredAt = time.Time{}
		return nil
	}
	if !order.DeliveredAt.IsZero() {
		return nil
	}
	if order.ID != 0 {
		// Quem chamou pode nao carregar o campo (o formulario nao tem essa
		// caixa), entao a data ja gravada manda.
		var previous string
		err := s.db.QueryRowContext(ctx,
			"SELECT delivered_at FROM service_orders WHERE id = ?", order.ID).Scan(&previous)
		if err != nil && !errors.Is(err, sql.ErrNoRows) {
			return err
		}
		order.DeliveredAt = decodeTime(previous)
	}
	if order.DeliveredAt.IsZero() {
		// Truncado como a coluna guarda, senao o valor em memoria fica com
		// nanossegundos que o banco corta, e os dois nunca comparam iguais.
		order.DeliveredAt = now.Truncate(time.Second)
	}
	return nil
}

func (s *Store) UpsertCustomerProfile(ctx context.Context, order ServiceOrder) error {
	key := normalizeCustomerName(order.Customer)
	if key == "" {
		return nil
	}
	// Se o CPF ja pertence a outro cadastro, atualiza aquele em vez de abrir um
	// cliente novo: dois cadastros com o mesmo documento sao a mesma pessoa, e
	// trocar o nome do cadastro e decisao do operador, nao da OS.
	if documentKey(order.CPF) != "" {
		if conflict, found, err := s.FindCustomerConflict(ctx, key, order.CPF, ""); err == nil && found {
			key = conflict.Profile.Key
			order.Customer = conflict.Profile.Name
		}
	}

	now := time.Now().UTC()
	lastSeen := order.UpdatedAt
	if lastSeen.IsZero() {
		lastSeen = now
	}

	_, err := s.db.ExecContext(ctx, `
INSERT INTO customer_profiles (
	normalized_name, name, cpf, phone, last_device, last_model, last_seen, created_at, updated_at
) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
ON CONFLICT(normalized_name) DO UPDATE SET
	name = excluded.name,
	cpf = CASE WHEN excluded.cpf <> '' THEN excluded.cpf ELSE customer_profiles.cpf END,
	phone = CASE WHEN excluded.phone <> '' THEN excluded.phone ELSE customer_profiles.phone END,
	last_device = CASE WHEN excluded.last_device <> '' THEN excluded.last_device ELSE customer_profiles.last_device END,
	last_model = CASE WHEN excluded.last_model <> '' THEN excluded.last_model ELSE customer_profiles.last_model END,
	last_seen = excluded.last_seen,
	updated_at = excluded.updated_at`,
		key, order.Customer, order.CPF, order.Phone, order.Device, order.BrandModel,
		encodeTime(lastSeen), encodeTime(now), encodeTime(now),
	)
	if err != nil {
		return err
	}
	return s.queueCustomerByKey(ctx, key)
}

func (s *Store) Get(ctx context.Context, id int64) (ServiceOrder, error) {
	row := s.db.QueryRowContext(ctx,
		"SELECT "+orderSelectColumns+" FROM service_orders WHERE id = ?", id)
	return scanOrder(row)
}

func encodeChecklistItems(items []string) string {
	clean := make([]string, 0, len(items))
	for _, item := range items {
		if strings.TrimSpace(item) != "" {
			clean = append(clean, strings.TrimSpace(item))
		}
	}
	data, err := json.Marshal(clean)
	if err != nil {
		return "[]"
	}
	return string(data)
}

func decodeChecklistItems(raw string) []string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil
	}
	var items []string
	if err := json.Unmarshal([]byte(raw), &items); err != nil {
		return nil
	}
	return items
}

func scanChecklistTemplate(scanner rowScanner) (ChecklistTemplate, error) {
	var tpl ChecklistTemplate
	var items, createdAt, updatedAt string
	if err := scanner.Scan(&tpl.ID, &tpl.Name, &tpl.DeviceType, &items, &createdAt, &updatedAt); err != nil {
		return ChecklistTemplate{}, err
	}
	tpl.Items = decodeChecklistItems(items)
	tpl.CreatedAt = decodeTime(createdAt)
	tpl.UpdatedAt = decodeTime(updatedAt)
	return tpl, nil
}

func (s *Store) ListChecklistTemplates(ctx context.Context) ([]ChecklistTemplate, error) {
	rows, err := s.db.QueryContext(ctx, `
SELECT id, name, device_type, items, created_at, updated_at
FROM checklist_templates
ORDER BY CASE WHEN device_type = '' THEN 1 ELSE 0 END, lower(device_type), lower(name), id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	templates := make([]ChecklistTemplate, 0, 8)
	for rows.Next() {
		tpl, err := scanChecklistTemplate(rows)
		if err != nil {
			return nil, err
		}
		templates = append(templates, tpl)
	}
	return templates, rows.Err()
}

func (s *Store) GetChecklistTemplate(ctx context.Context, id int64) (ChecklistTemplate, error) {
	row := s.db.QueryRowContext(ctx, `
SELECT id, name, device_type, items, created_at, updated_at
FROM checklist_templates WHERE id = ?`, id)
	return scanChecklistTemplate(row)
}

func (s *Store) ChecklistTemplateCount(ctx context.Context) (int, error) {
	var count int
	err := s.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM checklist_templates").Scan(&count)
	return count, err
}

func (s *Store) SaveChecklistTemplate(ctx context.Context, tpl *ChecklistTemplate) error {
	if tpl == nil {
		return errors.New("template invalido")
	}
	tpl.Name = strings.TrimSpace(tpl.Name)
	tpl.DeviceType = strings.TrimSpace(tpl.DeviceType)
	if tpl.Name == "" {
		return errors.New("informe o nome da lista")
	}
	if len(tpl.Items) == 0 {
		return errors.New("adicione ao menos um item na lista")
	}
	now := time.Now().UTC()
	tpl.UpdatedAt = now
	if tpl.ID == 0 {
		tpl.CreatedAt = now
		res, err := s.db.ExecContext(ctx, `
INSERT INTO checklist_templates(name, device_type, items, created_at, updated_at)
VALUES (?, ?, ?, ?, ?)`, tpl.Name, tpl.DeviceType, encodeChecklistItems(tpl.Items), encodeTime(now), encodeTime(now))
		if err != nil {
			return err
		}
		if tpl.ID, err = res.LastInsertId(); err != nil {
			return err
		}
	} else {
		if _, err := s.db.ExecContext(ctx, `
UPDATE checklist_templates SET name = ?, device_type = ?, items = ?, updated_at = ?
WHERE id = ?`, tpl.Name, tpl.DeviceType, encodeChecklistItems(tpl.Items), encodeTime(now), tpl.ID); err != nil {
			return err
		}
	}
	return s.queueChecklistTemplate(ctx, *tpl)
}

func (s *Store) DeleteChecklistTemplate(ctx context.Context, id int64) error {
	if _, err := s.db.ExecContext(ctx, "DELETE FROM checklist_templates WHERE id = ?", id); err != nil {
		return err
	}
	return s.queueDelete(ctx, syncEntityChecklistTemplate, strconv.FormatInt(id, 10))
}

// defaultChecklistTemplates sao as listas "de presente", que o usuario pode
// editar, excluir ou desvincular livremente.
func defaultChecklistTemplates() []ChecklistTemplate {
	return []ChecklistTemplate{
		{Name: "Celular", DeviceType: "Celular", Items: []string{
			"Aparelho liga", "Tela trincada", "Tela riscada", "Touch funcionando",
			"Face ID / biometria", "Camera frontal", "Camera traseira", "Botoes (power/volume)",
			"Alto-falante", "Microfone", "Carrega / conector", "Chip / bandeja",
			"Molhado / oxidado", "Carcaca (amassados)",
		}},
		{Name: "Console (PlayStation/Xbox)", DeviceType: "Console", Items: []string{
			"Liga", "Video / HDMI", "Leitor de disco", "Controle pareia", "Portas USB",
			"Superaquecimento", "Ventilacao / ruido", "HD / SSD", "Fonte de alimentacao", "Carcaca",
		}},
		{Name: "Televisao", DeviceType: "TV", Items: []string{
			"Liga", "Imagem", "Backlight / iluminacao", "Som", "Portas HDMI", "Controle remoto",
			"Tela trincada", "Manchas / pixels", "Wi-Fi / apps", "Base / suporte",
		}},
		{Name: "Notebook", DeviceType: "Notebook", Items: []string{
			"Liga", "Tela", "Teclado", "Touchpad", "Bateria", "Carrega", "Portas USB",
			"Ventilacao / aquecimento", "HD / SSD", "Audio", "Wi-Fi", "Carcaca",
		}},
	}
}

// RestoreDefaultChecklistTemplates insere as listas padrao (sempre), usado pelo
// botao "Restaurar listas padrao".
func (s *Store) RestoreDefaultChecklistTemplates(ctx context.Context) (int, error) {
	created := 0
	for _, tpl := range defaultChecklistTemplates() {
		tpl := tpl
		if err := s.SaveChecklistTemplate(ctx, &tpl); err != nil {
			return created, err
		}
		created++
	}
	_ = s.SetSetting(ctx, "checklist_defaults_seeded", "1")
	return created, nil
}

// SeedDefaultChecklistTemplates insere as listas padrao apenas uma vez por
// aparelho e somente se ainda nao houver nenhuma lista (evita duplicar quando o
// aparelho ja recebeu listas via sincronizacao).
func (s *Store) SeedDefaultChecklistTemplates(ctx context.Context) error {
	if seeded, ok, err := s.GetSetting(ctx, "checklist_defaults_seeded"); err != nil {
		return err
	} else if ok && seeded == "1" {
		return nil
	}
	count, err := s.ChecklistTemplateCount(ctx)
	if err != nil {
		return err
	}
	if count > 0 {
		return s.SetSetting(ctx, "checklist_defaults_seeded", "1")
	}
	// Aparelho novo ligado na nuvem espera a primeira leitura. Semeando antes,
	// cada aparelho cria a sua propria copia dos padroes e a lista fica
	// duplicada para todo mundo: foi assim que a base ficou com 8 modelos.
	if config, err := s.APIConfig(ctx); err == nil && strings.TrimSpace(config.Token) != "" {
		if _, found, err := s.GetSetting(ctx, syncCursorSetting); err == nil && !found {
			return nil
		}
	}
	_, err = s.RestoreDefaultChecklistTemplates(ctx)
	return err
}

// MergeDuplicateChecklistTemplates apaga modelos repetidos, mantendo um por
// nome e tipo de aparelho. O criterio e o identificador da nuvem em ordem
// alfabetica, para todos os aparelhos escolherem o mesmo sobrevivente.
func (s *Store) MergeDuplicateChecklistTemplates(ctx context.Context) (int, error) {
	rows, err := s.db.QueryContext(ctx, `
SELECT c.id, osfold(c.name) || '|' || osfold(c.device_type) AS chave, COALESCE(m.remote_id, '')
FROM checklist_templates c
LEFT JOIN sync_entity_map m ON m.entity_type = ? AND m.local_key = CAST(c.id AS TEXT)
ORDER BY chave, COALESCE(m.remote_id, ''), c.id`, syncEntityChecklistTemplate)
	if err != nil {
		return 0, err
	}
	type row struct {
		id  int64
		key string
	}
	var all []row
	for rows.Next() {
		var r row
		var remote string
		if err := rows.Scan(&r.id, &r.key, &remote); err != nil {
			rows.Close()
			return 0, err
		}
		all = append(all, r)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return 0, err
	}

	seen := map[string]bool{}
	removed := 0
	for _, r := range all {
		if !seen[r.key] {
			seen[r.key] = true
			continue
		}
		if err := s.DeleteChecklistTemplate(ctx, r.id); err != nil {
			return removed, err
		}
		removed++
	}
	return removed, nil
}

func (s *Store) List(ctx context.Context, status string, query string) ([]ServiceOrder, error) {
	var args []any
	where := []string{"1 = 1"}

	// Um filtro de fila nao e um status: e uma pergunta sobre a OS (ver
	// workqueue.go). Entra aqui para a lista, a busca e a contagem seguirem
	// usando o mesmo caminho.
	if clause, queueArgs, ok := queueFilterClause(status); ok {
		where = append(where, clause)
		args = append(args, queueArgs...)
	} else if status != "" && status != "Todos" {
		where = append(where, "status = ?")
		args = append(args, status)
	}

	if clause, searchArgs, ok := buildSearchFilter(query, orderSearchSpec); ok {
		where = append(where, "("+clause+")")
		args = append(args, searchArgs...)
	}

	stmt := fmt.Sprintf("SELECT %s FROM service_orders WHERE %s ORDER BY id DESC",
		orderSelectColumns, strings.Join(where, " AND "))

	rows, err := s.db.QueryContext(ctx, stmt, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var orders []ServiceOrder
	for rows.Next() {
		order, err := scanOrder(rows)
		if err != nil {
			return nil, err
		}
		orders = append(orders, order)
	}
	return orders, rows.Err()
}

func (s *Store) Counts(ctx context.Context) (map[string]int, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT status, COUNT(*) FROM service_orders GROUP BY status`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	counts := map[string]int{
		"Todos":            0,
		statusPending:      0,
		statusApproved:     0,
		statusReady:        0,
		statusUnauthorized: 0,
		statusNoRepair:     0,
		statusDelivered:    0,
		filterNeedsQuote:   0,
		filterNeedsAnswer:  0,
	}
	for rows.Next() {
		var status string
		var count int
		if err := rows.Scan(&status, &count); err != nil {
			return nil, err
		}
		counts[status] = count
		counts["Todos"] += count
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	// As duas filas derivadas entram no mesmo mapa, para os chips e o painel
	// contarem por um caminho so.
	var needsQuote, needsAnswer int
	if err := s.db.QueryRowContext(ctx, `
SELECT
	COALESCE(SUM(CASE WHEN amount_cents <= 0 THEN 1 ELSE 0 END), 0),
	COALESCE(SUM(CASE WHEN amount_cents > 0 THEN 1 ELSE 0 END), 0)
FROM service_orders WHERE status = ?`, statusPending).Scan(&needsQuote, &needsAnswer); err != nil {
		return nil, err
	}
	counts[filterNeedsQuote] = needsQuote
	counts[filterNeedsAnswer] = needsAnswer
	return counts, nil
}

func (s *Store) FinancialTotals(ctx context.Context) (FinancialTotals, error) {
	var totals FinancialTotals
	err := s.db.QueryRowContext(ctx, `
SELECT
	COALESCE(SUM(amount_cents), 0),
	COALESCE(SUM(cost_cents), 0),
	COUNT(*),
	COALESCE(SUM(CASE WHEN cost_cents <= 0 THEN 1 ELSE 0 END), 0),
	COALESCE(SUM(CASE WHEN payment_method <> '' AND amount_cents > paid_cents THEN 1 ELSE 0 END), 0),
	COALESCE(SUM(CASE WHEN payment_method <> '' AND amount_cents > paid_cents
		THEN amount_cents - paid_cents ELSE 0 END), 0)
FROM service_orders
WHERE status = ?`, statusDelivered).Scan(
		&totals.GrossCents, &totals.CostCents, &totals.Orders,
		&totals.MissingCostOrders, &totals.UnpaidOrders, &totals.UnpaidCents)
	if err != nil {
		return FinancialTotals{}, err
	}
	totals.NetCents = totals.GrossCents - totals.CostCents
	return totals, nil
}

func (s *Store) FinancialReport(ctx context.Context, start time.Time, end time.Time, title string) (FinancialReport, error) {
	if start.IsZero() || end.IsZero() || !end.After(start) {
		return FinancialReport{}, errors.New("periodo invalido")
	}
	report := FinancialReport{
		Title:      strings.TrimSpace(title),
		PeriodText: fmt.Sprintf("%s a %s", start.Format("02/01/2006"), end.Add(-time.Nanosecond).Format("02/01/2006")),
		Start:      start,
		End:        end,
	}
	if report.Title == "" {
		report.Title = "Relatorio financeiro"
	}

	// O periodo sai da data de entrega, nao da ultima edicao. Com updated_at,
	// corrigir um telefone numa OS de julho jogava aquela receita para o mes da
	// correcao, e dois relatorios do mesmo mes nao batiam.
	rows, err := s.db.QueryContext(ctx,
		"SELECT "+orderSelectColumns+` FROM service_orders
WHERE status = ? AND delivered_at >= ? AND delivered_at < ?
ORDER BY delivered_at ASC, id ASC`, statusDelivered, encodeTime(start.UTC()), encodeTime(end.UTC()))
	if err != nil {
		return FinancialReport{}, err
	}
	defer rows.Close()

	for rows.Next() {
		order, err := scanOrder(rows)
		if err != nil {
			return FinancialReport{}, err
		}
		report.Orders = append(report.Orders, order)
		report.GrossCents += order.AmountCents
		report.CostCents += order.CostCents
		if order.CostCents <= 0 {
			report.MissingCostOrders++
		}
		if owed := orderOwedCents(order); owed > 0 {
			report.UnpaidOrders++
			report.UnpaidCents += owed
		}
	}
	if err := rows.Err(); err != nil {
		return FinancialReport{}, err
	}
	report.NetCents = report.GrossCents - report.CostCents
	return report, nil
}

func (s *Store) Delete(ctx context.Context, id int64) error {
	if id <= 0 {
		return errors.New("OS invalida")
	}
	if err := s.queueDelete(ctx, syncEntityOrder, strconv.FormatInt(id, 10)); err != nil {
		return err
	}
	_, err := s.db.ExecContext(ctx, `DELETE FROM service_orders WHERE id = ?`, id)
	return err
}

// MarkOrderContacted anota que o cliente desta OS foi cobrado agora.
func (s *Store) MarkOrderContacted(ctx context.Context, id int64, when time.Time) error {
	if id <= 0 {
		return errors.New("OS invalida")
	}
	_, err := s.db.ExecContext(ctx, `UPDATE service_orders SET contacted_at = ? WHERE id = ?`, encodeTime(when.UTC()), id)
	return err
}

// OrderContactedAt devolve a data da ultima cobranca, se houver.
func (s *Store) OrderContactedAt(ctx context.Context, id int64) (time.Time, bool, error) {
	if id <= 0 {
		return time.Time{}, false, nil
	}
	var raw string
	if err := s.db.QueryRowContext(ctx, `SELECT contacted_at FROM service_orders WHERE id = ?`, id).Scan(&raw); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return time.Time{}, false, nil
		}
		return time.Time{}, false, err
	}
	if strings.TrimSpace(raw) == "" {
		return time.Time{}, false, nil
	}
	when := decodeTime(raw)
	return when, !when.IsZero(), nil
}

// StaleOrderCount conta as OS em aberto que ja passaram do prazo de cobranca.
// E o numero que o sino deve mostrar: um alerta que marca tudo que existe para
// de ser alerta.
func (s *Store) StaleOrderCount(ctx context.Context, days int) (int, error) {
	limit := encodeTime(time.Now().UTC().AddDate(0, 0, -days))
	var count int
	err := s.db.QueryRowContext(ctx, `
SELECT COUNT(*) FROM service_orders
WHERE status IN (?, ?, ?) AND created_at < ?`, statusPending, statusApproved, statusReady, limit).Scan(&count)
	return count, err
}

// LogWhatsApp registra o envio: serve de trilha e alimenta o teto diario.
func (s *Store) LogWhatsApp(ctx context.Context, orderID int64, number, kind string) error {
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO whatsapp_log (order_id, number, kind, sent_at) VALUES (?, ?, ?, ?)`,
		orderID, number, kind, encodeTime(time.Now().UTC()))
	return err
}

// WhatsAppSentToday conta os envios de hoje. Disparo em volume e o que faz o
// WhatsApp bloquear um numero, entao o teto existe mesmo com a loja sendo
// pequena.
func (s *Store) WhatsAppSentToday(ctx context.Context) (int, error) {
	start := time.Now().Local()
	start = time.Date(start.Year(), start.Month(), start.Day(), 0, 0, 0, 0, time.Local)
	var count int
	err := s.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM whatsapp_log WHERE sent_at >= ?`, encodeTime(start.UTC())).Scan(&count)
	return count, err
}

// WhatsAppLastSent devolve quando esta OS foi avisada pela ultima vez.
func (s *Store) WhatsAppLastSent(ctx context.Context, orderID int64) (time.Time, bool, error) {
	var raw string
	err := s.db.QueryRowContext(ctx,
		`SELECT sent_at FROM whatsapp_log WHERE order_id = ? ORDER BY id DESC LIMIT 1`, orderID).Scan(&raw)
	if errors.Is(err, sql.ErrNoRows) {
		return time.Time{}, false, nil
	}
	if err != nil {
		return time.Time{}, false, err
	}
	when := decodeTime(raw)
	return when, !when.IsZero(), nil
}

func (s *Store) GetSetting(ctx context.Context, key string) (string, bool, error) {
	var value string
	err := s.db.QueryRowContext(ctx, `SELECT value FROM app_settings WHERE key = ?`, key).Scan(&value)
	if err == nil {
		return value, true, nil
	}
	if errors.Is(err, sql.ErrNoRows) {
		return "", false, nil
	}
	return "", false, err
}

func (s *Store) SetSetting(ctx context.Context, key string, value string) error {
	_, err := s.db.ExecContext(ctx, `
INSERT INTO app_settings(key, value) VALUES(?, ?)
ON CONFLICT(key) DO UPDATE SET value = excluded.value`, key, strings.TrimSpace(value))
	return err
}

func (s *Store) FindCustomerProfiles(ctx context.Context, query string, limit int) ([]CustomerProfile, error) {
	profileWhere, profileArgs, ok := buildSearchFilter(query, customerProfileSearchSpec)
	if !ok {
		return nil, nil
	}
	where, args, ok := buildSearchFilter(query, orderCustomerSearchSpec)
	if !ok {
		return nil, nil
	}
	if limit <= 0 {
		limit = 8
	}

	profiles := make([]CustomerProfile, 0, limit)
	index := make(map[string]int)
	byDocument := make(map[string]int)

	profileStmt := fmt.Sprintf(`
SELECT normalized_name, name, cpf, phone, avatar_url, avatar_key, last_device, last_model, last_seen
FROM customer_profiles
WHERE %s
ORDER BY %s,
	updated_at DESC
LIMIT ?`, profileWhere, searchRankExpr("name"))
	profileArgs = append(profileArgs, searchRankArgs(query)...)
	profileArgs = append(profileArgs, limit)

	profileRows, err := s.db.QueryContext(ctx, profileStmt, profileArgs...)
	if err != nil {
		return nil, err
	}
	for profileRows.Next() {
		var profile CustomerProfile
		var lastSeen string
		if err := profileRows.Scan(
			&profile.Key,
			&profile.Name,
			&profile.CPF,
			&profile.Phone,
			&profile.AvatarURL,
			&profile.AvatarKey,
			&profile.LastDevice,
			&profile.LastModel,
			&lastSeen,
		); err != nil {
			_ = profileRows.Close()
			return nil, err
		}
		profile.LastSeen = decodeTime(lastSeen)
		index[profile.Key] = len(profiles)
		if doc := documentKey(profile.CPF); doc != "" {
			byDocument[doc] = len(profiles)
		}
		profiles = append(profiles, profile)
	}
	if err := profileRows.Close(); err != nil {
		return nil, err
	}

	stmt := fmt.Sprintf(`
SELECT id, customer, cpf, phone, device, brand_model, updated_at
FROM service_orders
WHERE customer <> '' AND %s
ORDER BY %s,
	updated_at DESC,
	id DESC
LIMIT ?`, where, searchRankExpr("customer"))
	args = append(args, searchRankArgs(query)...)
	args = append(args, limit*5)

	rows, err := s.db.QueryContext(ctx, stmt, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	for rows.Next() {
		var (
			id        int64
			name      string
			cpf       string
			phone     string
			device    string
			model     string
			updatedAt string
		)
		if err := rows.Scan(&id, &name, &cpf, &phone, &device, &model, &updatedAt); err != nil {
			return nil, err
		}

		key := normalizeCustomerName(name)
		if key == "" {
			continue
		}
		pos, ok := index[key]
		if !ok {
			if doc := documentKey(cpf); doc != "" {
				pos, ok = byDocument[doc]
			}
		}
		if ok {
			profiles[pos].Orders++
			if profiles[pos].CPF == "" {
				profiles[pos].CPF = strings.TrimSpace(cpf)
			}
			if profiles[pos].Phone == "" {
				profiles[pos].Phone = strings.TrimSpace(phone)
			}
			continue
		}

		index[key] = len(profiles)
		if doc := documentKey(cpf); doc != "" {
			byDocument[doc] = len(profiles)
		}
		profiles = append(profiles, CustomerProfile{
			Key:        key,
			Name:       strings.TrimSpace(name),
			CPF:        strings.TrimSpace(cpf),
			Phone:      strings.TrimSpace(phone),
			Orders:     1,
			LastSeen:   decodeTime(updatedAt),
			LastDevice: strings.TrimSpace(device),
			LastModel:  strings.TrimSpace(model),
		})
		if len(profiles) >= limit {
			break
		}
	}
	return profiles, rows.Err()
}

func (s *Store) SaveCustomerAvatar(ctx context.Context, profile CustomerProfile) error {
	key := normalizeCustomerName(profile.Name)
	if key == "" {
		return errors.New("informe o nome do cliente antes de salvar a foto")
	}

	now := time.Now().UTC()
	_, err := s.db.ExecContext(ctx, `
INSERT INTO customer_profiles (
	normalized_name, name, cpf, phone, avatar_url, avatar_key, created_at, updated_at
) VALUES (?, ?, ?, ?, ?, ?, ?, ?)
ON CONFLICT(normalized_name) DO UPDATE SET
	name = excluded.name,
	cpf = CASE WHEN excluded.cpf <> '' THEN excluded.cpf ELSE customer_profiles.cpf END,
	phone = CASE WHEN excluded.phone <> '' THEN excluded.phone ELSE customer_profiles.phone END,
	avatar_url = excluded.avatar_url,
	avatar_key = excluded.avatar_key,
	updated_at = excluded.updated_at`,
		key, profile.Name, profile.CPF, profile.Phone, profile.AvatarURL, profile.AvatarKey,
		encodeTime(now), encodeTime(now),
	)
	if err != nil {
		return err
	}
	return s.queueCustomerByKey(ctx, key)
}

func (s *Store) SaveCustomerProfile(ctx context.Context, profile CustomerProfile) error {
	key := normalizeCustomerName(profile.Name)
	if key == "" {
		return errors.New("informe o nome do cliente")
	}

	now := time.Now().UTC()
	lastSeen := profile.LastSeen
	if lastSeen.IsZero() {
		lastSeen = now
	}

	_, err := s.db.ExecContext(ctx, `
INSERT INTO customer_profiles (
	normalized_name, name, cpf, phone, avatar_url, avatar_key, last_device,
	last_model, last_seen, created_at, updated_at
) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
ON CONFLICT(normalized_name) DO UPDATE SET
	name = excluded.name,
	cpf = excluded.cpf,
	phone = excluded.phone,
	avatar_url = CASE WHEN excluded.avatar_url <> '' THEN excluded.avatar_url ELSE customer_profiles.avatar_url END,
	avatar_key = CASE WHEN excluded.avatar_key <> '' THEN excluded.avatar_key ELSE customer_profiles.avatar_key END,
	last_device = CASE WHEN excluded.last_device <> '' THEN excluded.last_device ELSE customer_profiles.last_device END,
	last_model = CASE WHEN excluded.last_model <> '' THEN excluded.last_model ELSE customer_profiles.last_model END,
	last_seen = CASE WHEN excluded.last_seen <> '' THEN excluded.last_seen ELSE customer_profiles.last_seen END,
	updated_at = excluded.updated_at`,
		key, profile.Name, profile.CPF, profile.Phone, profile.AvatarURL, profile.AvatarKey,
		profile.LastDevice, profile.LastModel, encodeTime(lastSeen), encodeTime(now), encodeTime(now),
	)
	if err != nil {
		return err
	}
	return s.queueCustomerByKey(ctx, key)
}

func (s *Store) DeleteCustomerProfile(ctx context.Context, key string) error {
	key = normalizeCustomerName(key)
	if key == "" {
		return errors.New("cliente invalido")
	}
	if err := s.queueDelete(ctx, syncEntityCustomer, key); err != nil {
		return err
	}
	_, err := s.db.ExecContext(ctx, `DELETE FROM customer_profiles WHERE normalized_name = ?`, key)
	return err
}

func (s *Store) ListCustomerProfiles(ctx context.Context, query string) ([]CustomerProfile, error) {
	if strings.TrimSpace(query) != "" {
		return s.FindCustomerProfiles(ctx, query, 500)
	}

	rows, err := s.db.QueryContext(ctx, `
SELECT normalized_name, name, cpf, phone, avatar_url, avatar_key, last_device, last_model, last_seen
FROM customer_profiles
ORDER BY osfold(name) ASC
LIMIT 500`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	index := make(map[string]int)
	// byDocument agrupa pelo CPF/CNPJ: uma OS escrita com o nome abreviado do
	// cliente nao pode virar um segundo "cliente" na lista.
	byDocument := make(map[string]int)
	merged := make([]CustomerProfile, 0, 128)
	for rows.Next() {
		var profile CustomerProfile
		var lastSeen string
		if err := rows.Scan(
			&profile.Key,
			&profile.Name,
			&profile.CPF,
			&profile.Phone,
			&profile.AvatarURL,
			&profile.AvatarKey,
			&profile.LastDevice,
			&profile.LastModel,
			&lastSeen,
		); err != nil {
			return nil, err
		}
		profile.LastSeen = decodeTime(lastSeen)
		index[profile.Key] = len(merged)
		if doc := documentKey(profile.CPF); doc != "" {
			byDocument[doc] = len(merged)
		}
		merged = append(merged, profile)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	orderRows, err := s.db.QueryContext(ctx, `
SELECT customer, cpf, phone, device, brand_model, updated_at
FROM service_orders
WHERE customer <> ''
ORDER BY updated_at DESC, id DESC
LIMIT 2000`)
	if err != nil {
		return nil, err
	}
	defer orderRows.Close()

	for orderRows.Next() {
		var (
			name      string
			cpf       string
			phone     string
			device    string
			model     string
			updatedAt string
		)
		if err := orderRows.Scan(&name, &cpf, &phone, &device, &model, &updatedAt); err != nil {
			return nil, err
		}

		key := normalizeCustomerName(name)
		if key == "" {
			continue
		}
		pos, ok := index[key]
		if !ok {
			if doc := documentKey(cpf); doc != "" {
				pos, ok = byDocument[doc]
			}
		}
		if ok {
			merged[pos].Orders++
			if merged[pos].CPF == "" {
				merged[pos].CPF = strings.TrimSpace(cpf)
			}
			if merged[pos].Phone == "" {
				merged[pos].Phone = strings.TrimSpace(phone)
			}
			if merged[pos].LastDevice == "" {
				merged[pos].LastDevice = strings.TrimSpace(device)
			}
			if merged[pos].LastModel == "" {
				merged[pos].LastModel = strings.TrimSpace(model)
			}
			if merged[pos].LastSeen.IsZero() {
				merged[pos].LastSeen = decodeTime(updatedAt)
			}
			continue
		}

		index[key] = len(merged)
		if doc := documentKey(cpf); doc != "" {
			byDocument[doc] = len(merged)
		}
		merged = append(merged, CustomerProfile{
			Key:        key,
			Name:       strings.TrimSpace(name),
			CPF:        strings.TrimSpace(cpf),
			Phone:      strings.TrimSpace(phone),
			Orders:     1,
			LastSeen:   decodeTime(updatedAt),
			LastDevice: strings.TrimSpace(device),
			LastModel:  strings.TrimSpace(model),
		})
	}
	if err := orderRows.Err(); err != nil {
		return nil, err
	}
	return merged, nil
}

func (s *Store) SaveTerm(ctx context.Context, term *ServiceTerm) error {
	now := time.Now().UTC()
	if term.Status == "" {
		term.Status = termStatusDraft
	}
	if term.TermType == "" {
		term.TermType = "Compra e venda de equipamento usado"
	}
	if term.TermDate.IsZero() {
		term.TermDate = now
	}
	term.UpdatedAt = now

	if term.ID == 0 {
		term.CreatedAt = now
		res, err := s.db.ExecContext(ctx, `
INSERT INTO sales_terms (
	term_type, status, seller_name, seller_cpf, seller_rg, seller_phone,
	seller_address, buyer_company, buyer_document, buyer_address, buyer_phone,
	device_kind, brand, model, color, imei1, imei2, accessories, declaration,
	amount_cents, payment_method, city, term_date, pdf_key, pdf_url, signed_key,
	signed_url, created_at, updated_at
) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			term.TermType, term.Status, term.SellerName, term.SellerCPF, term.SellerRG, term.SellerPhone,
			term.SellerAddress, term.BuyerCompany, term.BuyerDocument, term.BuyerAddress, term.BuyerPhone,
			term.DeviceKind, term.Brand, term.Model, term.Color, term.IMEI1, term.IMEI2, term.Accessories,
			term.Declaration, term.AmountCents, term.PaymentMethod, term.City, encodeTime(term.TermDate),
			term.PDFKey, term.PDFURL, term.SignedKey, term.SignedURL, encodeTime(term.CreatedAt),
			encodeTime(term.UpdatedAt),
		)
		if err != nil {
			return err
		}
		id, err := res.LastInsertId()
		if err != nil {
			return err
		}
		term.ID = id
		return s.queueTerm(ctx, *term)
	}

	_, err := s.db.ExecContext(ctx, `
UPDATE sales_terms SET
	term_type = ?, status = ?, seller_name = ?, seller_cpf = ?, seller_rg = ?,
	seller_phone = ?, seller_address = ?, buyer_company = ?, buyer_document = ?,
	buyer_address = ?, buyer_phone = ?, device_kind = ?, brand = ?, model = ?,
	color = ?, imei1 = ?, imei2 = ?, accessories = ?, declaration = ?,
	amount_cents = ?, payment_method = ?, city = ?, term_date = ?, pdf_key = ?,
	pdf_url = ?, signed_key = ?, signed_url = ?, updated_at = ?
WHERE id = ?`,
		term.TermType, term.Status, term.SellerName, term.SellerCPF, term.SellerRG,
		term.SellerPhone, term.SellerAddress, term.BuyerCompany, term.BuyerDocument,
		term.BuyerAddress, term.BuyerPhone, term.DeviceKind, term.Brand, term.Model,
		term.Color, term.IMEI1, term.IMEI2, term.Accessories, term.Declaration,
		term.AmountCents, term.PaymentMethod, term.City, encodeTime(term.TermDate),
		term.PDFKey, term.PDFURL, term.SignedKey, term.SignedURL, encodeTime(term.UpdatedAt),
		term.ID,
	)
	if err != nil {
		return err
	}
	return s.queueTerm(ctx, *term)
}

func (s *Store) GetTerm(ctx context.Context, id int64) (ServiceTerm, error) {
	row := s.db.QueryRowContext(ctx, `
SELECT id, term_type, status, seller_name, seller_cpf, seller_rg, seller_phone,
	seller_address, buyer_company, buyer_document, buyer_address, buyer_phone,
	device_kind, brand, model, color, imei1, imei2, accessories, declaration,
	amount_cents, payment_method, city, term_date, pdf_key, pdf_url, signed_key,
	signed_url, created_at, updated_at
FROM sales_terms
WHERE id = ?`, id)
	return scanTerm(row)
}

func (s *Store) ListTerms(ctx context.Context, query string, documentFilter string) ([]ServiceTerm, error) {
	var args []any
	where := []string{"1 = 1"}

	if clause, searchArgs, ok := buildSearchFilter(query, termSearchSpec); ok {
		where = append(where, "("+clause+")")
		args = append(args, searchArgs...)
	}

	switch strings.TrimSpace(documentFilter) {
	case termFilterDrafts:
		where = append(where, "status = ?")
		args = append(args, termStatusDraft)
	case termFilterAwaitingSignature:
		where = append(where, "(pdf_key <> '' OR pdf_url <> '')")
		where = append(where, "signed_key = '' AND signed_url = ''")
	case termFilterSigned:
		where = append(where, "(signed_key <> '' OR signed_url <> '' OR status = ?)")
		args = append(args, termStatusSigned)
	case termFilterWithoutPDF:
		where = append(where, "pdf_key = '' AND pdf_url = ''")
	}

	stmt := fmt.Sprintf(`
SELECT id, term_type, status, seller_name, seller_cpf, seller_rg, seller_phone,
	seller_address, buyer_company, buyer_document, buyer_address, buyer_phone,
	device_kind, brand, model, color, imei1, imei2, accessories, declaration,
	amount_cents, payment_method, city, term_date, pdf_key, pdf_url, signed_key,
	signed_url, created_at, updated_at
FROM sales_terms
WHERE %s
ORDER BY updated_at DESC, id DESC
LIMIT 500`, strings.Join(where, " AND "))

	rows, err := s.db.QueryContext(ctx, stmt, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	terms := make([]ServiceTerm, 0, 64)
	for rows.Next() {
		term, err := scanTerm(rows)
		if err != nil {
			return nil, err
		}
		terms = append(terms, term)
	}
	return terms, rows.Err()
}

func (s *Store) DeleteTerm(ctx context.Context, id int64) error {
	if id <= 0 {
		return errors.New("termo invalido")
	}
	if err := s.queueDelete(ctx, syncEntityTerm, strconv.FormatInt(id, 10)); err != nil {
		return err
	}
	_, err := s.db.ExecContext(ctx, `DELETE FROM sales_terms WHERE id = ?`, id)
	return err
}

func (s *Store) AttachTermPDF(ctx context.Context, id int64, key string, url string) error {
	_, err := s.db.ExecContext(ctx, `
UPDATE sales_terms SET status = ?, pdf_key = ?, pdf_url = ?, updated_at = ?
WHERE id = ?`, termStatusGenerated, key, url, encodeTime(time.Now().UTC()), id)
	if err != nil {
		return err
	}
	term, err := s.GetTerm(ctx, id)
	if err != nil {
		return err
	}
	return s.queueTerm(ctx, term)
}

func (s *Store) AttachSignedTerm(ctx context.Context, id int64, key string, url string) error {
	_, err := s.db.ExecContext(ctx, `
UPDATE sales_terms SET status = ?, signed_key = ?, signed_url = ?, updated_at = ?
WHERE id = ?`, termStatusSigned, key, url, encodeTime(time.Now().UTC()), id)
	if err != nil {
		return err
	}
	term, err := s.GetTerm(ctx, id)
	if err != nil {
		return err
	}
	return s.queueTerm(ctx, term)
}

type rowScanner interface {
	Scan(dest ...any) error
}

// orderSelectColumns e a lista de colunas de toda consulta que alimenta scanOrder.
// Ficou centralizada porque eram quatro consultas identicas espalhadas em tres
// arquivos: acrescentar coluna em tres delas e esquecer a quarta nao da erro de
// compilacao, so devolve dado faltando em uma tela so.
const orderSelectColumns = `id, status, cpf, customer, phone, device, brand_model, serial, device_key, issue,
	accessories, notes, amount_cents, cost_cents, technician_id, technician_name,
	checklist, photos, delivered_at, paid_at, payment_method, paid_cents,
	contact_name, contact_phone, contacted_at, created_at, updated_at`

func scanOrder(scanner rowScanner) (ServiceOrder, error) {
	var order ServiceOrder
	var createdAt, updatedAt string
	var deliveredAt, paidAt, contactedAt string
	var checklist, photos string
	err := scanner.Scan(
		&order.ID,
		&order.Status,
		&order.CPF,
		&order.Customer,
		&order.Phone,
		&order.Device,
		&order.BrandModel,
		&order.Serial,
		&order.DeviceKey,
		&order.Issue,
		&order.Accessories,
		&order.Notes,
		&order.AmountCents,
		&order.CostCents,
		&order.TechnicianID,
		&order.TechnicianName,
		&checklist,
		&photos,
		&deliveredAt,
		&paidAt,
		&order.PaymentMethod,
		&order.PaidCents,
		&order.ContactName,
		&order.ContactPhone,
		&contactedAt,
		&createdAt,
		&updatedAt,
	)
	if err != nil {
		return ServiceOrder{}, err
	}

	order.Checklist = decodeChecklist(checklist)
	order.Photos = decodePhotos(photos)
	order.DeliveredAt = decodeTime(deliveredAt)
	order.PaidAt = decodeTime(paidAt)
	order.ContactedAt = decodeTime(contactedAt)
	order.CreatedAt = decodeTime(createdAt)
	order.UpdatedAt = decodeTime(updatedAt)
	return order, nil
}

func scanTerm(scanner rowScanner) (ServiceTerm, error) {
	var term ServiceTerm
	var termDate, createdAt, updatedAt string
	err := scanner.Scan(
		&term.ID,
		&term.TermType,
		&term.Status,
		&term.SellerName,
		&term.SellerCPF,
		&term.SellerRG,
		&term.SellerPhone,
		&term.SellerAddress,
		&term.BuyerCompany,
		&term.BuyerDocument,
		&term.BuyerAddress,
		&term.BuyerPhone,
		&term.DeviceKind,
		&term.Brand,
		&term.Model,
		&term.Color,
		&term.IMEI1,
		&term.IMEI2,
		&term.Accessories,
		&term.Declaration,
		&term.AmountCents,
		&term.PaymentMethod,
		&term.City,
		&termDate,
		&term.PDFKey,
		&term.PDFURL,
		&term.SignedKey,
		&term.SignedURL,
		&createdAt,
		&updatedAt,
	)
	if err != nil {
		return ServiceTerm{}, err
	}

	term.TermDate = decodeTime(termDate)
	term.CreatedAt = decodeTime(createdAt)
	term.UpdatedAt = decodeTime(updatedAt)
	return term, nil
}

func encodeTime(t time.Time) string {
	return t.UTC().Format(time.RFC3339Nano)
}

// encodeSecond grava a data com precisao de segundo e largura fixa, e vazio
// quando nao ha data. Dois motivos: RFC3339Nano corta zeros a direita, entao
// ".4372Z" e ".5Z" nao comparam como texto na ordem certa; e a hora zero
// viraria "0001-01-01T00:00:00Z", que passaria pelas comparacoes de periodo
// como se fosse uma entrega de verdade. As colunas antigas ficam como estao:
// updated_at e o cursor da sincronizacao, e reescrever todas reenviaria a base
// inteira.
func encodeSecond(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return t.UTC().Truncate(time.Second).Format(time.RFC3339)
}

func decodeTime(s string) time.Time {
	t, err := time.Parse(time.RFC3339Nano, s)
	if err != nil {
		return time.Time{}
	}
	return t.Local()
}

func normalizeQuery(s string) string {
	return strings.ToLower(strings.TrimSpace(s))
}

func normalizeCustomerName(s string) string {
	return strings.Join(strings.Fields(normalizeQuery(s)), " ")
}

func onlyDigits(s string) string {
	var b strings.Builder
	for _, r := range s {
		if r >= '0' && r <= '9' {
			b.WriteRune(r)
		}
	}
	return b.String()
}
