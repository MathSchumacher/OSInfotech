package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"
)

type SyncSummary struct {
	Pushed    int
	Pulled    int
	Conflicts int
	// Rejected conta as alteracoes que o servidor recusou por serem invalidas.
	// Elas saem da fila para nao travar o resto: a OS continua gravada aqui, e
	// volta para a fila assim que for corrigida e salva de novo.
	Rejected       int
	RejectedReason string
}

type remoteCustomer struct {
	ID        string  `json:"id"`
	Name      string  `json:"name"`
	Document  string  `json:"document"`
	Phone     string  `json:"phone"`
	AvatarKey string  `json:"avatar_key"`
	AvatarURL string  `json:"avatar_url"`
	Revision  int64   `json:"revision"`
	CreatedAt string  `json:"created_at"`
	UpdatedAt string  `json:"updated_at"`
	DeletedAt *string `json:"deleted_at"`
}

type remoteServiceOrder struct {
	ID               string          `json:"id"`
	Status           string          `json:"status"`
	CustomerID       string          `json:"customer_id"`
	CustomerName     string          `json:"customer_name"`
	CustomerDocument string          `json:"customer_document"`
	CustomerPhone    string          `json:"customer_phone"`
	Device           string          `json:"device"`
	BrandModel       string          `json:"brand_model"`
	Serial           string          `json:"serial"`
	Issue            string          `json:"issue"`
	Accessories      string          `json:"accessories"`
	Notes            string          `json:"notes"`
	GrossCents       int64           `json:"gross_amount_cents"`
	CostCents        int64           `json:"cost_amount_cents"`
	TechnicianID     string          `json:"technician_id"`
	Checklist        *OrderChecklist `json:"checklist"`
	Photos           []OrderPhoto    `json:"photos"`
	DeviceKey        string          `json:"device_key"`
	DeliveredAt      string          `json:"delivered_at"`
	PaidAt           string          `json:"paid_at"`
	PaymentMethod    string          `json:"payment_method"`
	ContactedAt      string          `json:"contacted_at"`
	ContactName      string          `json:"contact_name"`
	ContactPhone     string          `json:"contact_phone"`
	PaidCents        int64           `json:"paid_cents"`
	Revision         int64           `json:"revision"`
	CreatedAt        string          `json:"created_at"`
	UpdatedAt        string          `json:"updated_at"`
	DeletedAt        *string         `json:"deleted_at"`
}

type remoteChecklistTemplate struct {
	ID         string   `json:"id"`
	Name       string   `json:"name"`
	DeviceType string   `json:"device_type"`
	Items      []string `json:"items"`
	Revision   int64    `json:"revision"`
	CreatedAt  string   `json:"created_at"`
	UpdatedAt  string   `json:"updated_at"`
	DeletedAt  *string  `json:"deleted_at"`
}

type remoteSalesTerm struct {
	ID          string          `json:"id"`
	TermType    string          `json:"term_type"`
	Status      string          `json:"status"`
	Payload     json.RawMessage `json:"payload"`
	AmountCents int64           `json:"amount_cents"`
	PDFKey      string          `json:"pdf_key"`
	SignedKey   string          `json:"signed_key"`
	Revision    int64           `json:"revision"`
	CreatedAt   string          `json:"created_at"`
	UpdatedAt   string          `json:"updated_at"`
	DeletedAt   *string         `json:"deleted_at"`
}

// SyncProgress e chamado a cada lote enviado ou pagina recebida, para a
// interface mostrar o andamento (a primeira carga de um aparelho novo demora).
type SyncProgress func(summary SyncSummary)

func (s *Store) SyncD1(ctx context.Context, client *WorkerClient) (SyncSummary, error) {
	return s.SyncD1WithProgress(ctx, client, nil)
}

func (s *Store) SyncD1WithProgress(ctx context.Context, client *WorkerClient, onProgress SyncProgress) (SyncSummary, error) {
	if client == nil {
		return SyncSummary{}, errors.New("cliente de sincronizacao indisponivel")
	}
	if err := s.SeedSyncQueue(ctx); err != nil {
		return SyncSummary{}, err
	}
	// A tentativa comeca agora: o erro da tentativa anterior nao vale mais.
	// Sem isso um erro antigo ficava no banco e a interface mostrava "falha"
	// para sempre, mesmo com a sincronizacao andando normalmente.
	_ = s.SetSetting(ctx, syncLastFailureKey, "")

	summary := SyncSummary{}
	report := func() {
		if onProgress != nil {
			onProgress(summary)
		}
	}
	for {
		pending, err := s.PendingSyncChanges(ctx, 100)
		if err != nil {
			return summary, err
		}
		if len(pending) == 0 {
			break
		}
		results, err := client.Push(ctx, pending)
		if err != nil {
			return summary, err
		}
		resultByKey := make(map[string]syncPushResult, len(results))
		for _, result := range results {
			resultByKey[result.EntityType+":"+result.EntityID] = result
		}
		for _, change := range pending {
			result, found := resultByKey[change.EntityType+":"+change.EntityRemote]
			if !found {
				return summary, errors.New("a API nao retornou o resultado de uma alteracao")
			}
			switch result.Status {
			case "accepted":
				if len(result.Entity) > 0 && string(result.Entity) != "null" {
					if err := s.applyRemoteEntity(ctx, change.EntityType, result.Entity); err != nil {
						return summary, err
					}
				}
				if err := s.completeSyncChange(ctx, change, result.Revision); err != nil {
					return summary, err
				}
				summary.Pushed++
			case "conflict":
				if err := s.applyRemoteEntity(ctx, change.EntityType, result.Entity); err != nil {
					return summary, err
				}
				if err := s.discardSyncChange(ctx, change); err != nil {
					return summary, err
				}
				summary.Conflicts++
			default:
				// Recusa nao pode parar a sincronizacao. Antes isto devolvia
				// erro: a alteracao continuava na fila, era reenviada na
				// tentativa seguinte, recusada de novo, e como o envio vem
				// antes da leitura o aparelho tambem parava de receber. Uma
				// linha invalida travava tudo, para sempre.
				message := strings.TrimSpace(result.Message)
				if message == "" {
					message = "alteração recusada pela nuvem"
				}
				if err := s.discardSyncChange(ctx, change); err != nil {
					return summary, err
				}
				summary.Rejected++
				if summary.RejectedReason == "" {
					summary.RejectedReason = message
				}
			}
		}
		report()
		if len(pending) < 100 {
			break
		}
	}

	cursor := int64(0)
	if value, found, err := s.GetSetting(ctx, syncCursorSetting); err != nil {
		return summary, err
	} else if found {
		cursor, _ = strconv.ParseInt(value, 10, 64)
	}
	for {
		nextCursor, changes, hasMore, err := client.Pull(ctx, cursor)
		if err != nil {
			return summary, err
		}
		for _, change := range changes {
			if len(change.Entity) == 0 || string(change.Entity) == "null" {
				continue
			}
			if err := s.applyRemoteEntity(ctx, change.EntityType, change.Entity); err != nil {
				return summary, err
			}
			summary.Pulled++
		}
		// Se o servidor pede outra pagina mas nao avanca o cursor, parar: sem
		// isto o laco gira para sempre e a sincronizacao fica travada na sessao
		// inteira.
		stuck := hasMore && nextCursor <= cursor
		cursor = nextCursor
		if err := s.SetSetting(ctx, syncCursorSetting, strconv.FormatInt(cursor, 10)); err != nil {
			return summary, err
		}
		report()
		if stuck {
			return summary, errors.New("a nuvem nao avancou a leitura; tente sincronizar de novo")
		}
		if !hasMore {
			break
		}
	}
	_ = s.SetSetting(ctx, syncLastSuccessKey, encodeTime(time.Now().UTC()))
	_ = s.SetSetting(ctx, syncLastFailureKey, "")
	return summary, nil
}

func (s *Store) applyRemoteEntity(ctx context.Context, entityType string, data json.RawMessage) error {
	switch entityType {
	case syncEntityCustomer:
		var remote remoteCustomer
		if err := json.Unmarshal(data, &remote); err != nil {
			return err
		}
		return s.applyRemoteCustomer(ctx, remote)
	case syncEntityOrder:
		var remote remoteServiceOrder
		if err := json.Unmarshal(data, &remote); err != nil {
			return err
		}
		return s.applyRemoteOrder(ctx, remote)
	case syncEntityTerm:
		var remote remoteSalesTerm
		if err := json.Unmarshal(data, &remote); err != nil {
			return err
		}
		return s.applyRemoteTerm(ctx, remote)
	case syncEntityChecklistTemplate:
		var remote remoteChecklistTemplate
		if err := json.Unmarshal(data, &remote); err != nil {
			return err
		}
		return s.applyRemoteChecklistTemplate(ctx, remote)
	default:
		return fmt.Errorf("tipo remoto desconhecido: %s", entityType)
	}
}

func (s *Store) applyRemoteChecklistTemplate(ctx context.Context, remote remoteChecklistTemplate) error {
	if remote.ID == "" {
		return errors.New("template remoto sem identificador")
	}
	mapping, found, err := s.syncMappingByRemote(ctx, syncEntityChecklistTemplate, remote.ID)
	if err != nil {
		return err
	}
	if remote.DeletedAt != nil {
		if found {
			id, err := parseSyncLocalID(mapping.LocalKey)
			if err != nil {
				return err
			}
			if _, err := s.db.ExecContext(ctx, "DELETE FROM checklist_templates WHERE id = ?", id); err != nil {
				return err
			}
			return s.removeSyncMapping(ctx, syncEntityChecklistTemplate, remote.ID)
		}
		return nil
	}
	createdAt := firstNonEmpty(remote.CreatedAt, encodeTime(time.Now().UTC()))
	updatedAt := firstNonEmpty(remote.UpdatedAt, createdAt)
	items := encodeChecklistItems(remote.Items)
	if found {
		id, err := parseSyncLocalID(mapping.LocalKey)
		if err != nil {
			return err
		}
		if _, err := s.db.ExecContext(ctx, `
UPDATE checklist_templates SET name = ?, device_type = ?, items = ?, updated_at = ? WHERE id = ?`,
			remote.Name, remote.DeviceType, items, updatedAt, id); err != nil {
			return err
		}
		return s.saveSyncMapping(ctx, syncEntityChecklistTemplate, mapping.LocalKey, remote.ID, remote.Revision)
	}
	result, err := s.db.ExecContext(ctx, `
INSERT INTO checklist_templates(name, device_type, items, created_at, updated_at)
VALUES (?, ?, ?, ?, ?)`, remote.Name, remote.DeviceType, items, createdAt, updatedAt)
	if err != nil {
		return err
	}
	id, err := result.LastInsertId()
	if err != nil {
		return err
	}
	return s.saveSyncMapping(ctx, syncEntityChecklistTemplate, syncLocalKey(id), remote.ID, remote.Revision)
}

func (s *Store) applyRemoteCustomer(ctx context.Context, remote remoteCustomer) error {
	if remote.ID == "" {
		return errors.New("cliente remoto sem identificador")
	}
	mapping, found, err := s.syncMappingByRemote(ctx, syncEntityCustomer, remote.ID)
	if err != nil {
		return err
	}
	if remote.DeletedAt != nil {
		if found {
			if _, err := s.db.ExecContext(ctx, "DELETE FROM customer_profiles WHERE normalized_name = ?", mapping.LocalKey); err != nil {
				return err
			}
			return s.removeSyncMapping(ctx, syncEntityCustomer, remote.ID)
		}
		return nil
	}
	key := normalizeCustomerName(remote.Name)
	if key == "" {
		return errors.New("cliente remoto sem nome")
	}
	if found && mapping.LocalKey != key {
		if _, err := s.db.ExecContext(ctx, "DELETE FROM customer_profiles WHERE normalized_name = ?", mapping.LocalKey); err != nil {
			return err
		}
	}
	now := encodeTime(time.Now().UTC())
	updatedAt := firstNonEmpty(remote.UpdatedAt, now)
	createdAt := firstNonEmpty(remote.CreatedAt, updatedAt)
	_, err = s.db.ExecContext(ctx, `
INSERT INTO customer_profiles(normalized_name, name, cpf, phone, avatar_url, avatar_key, last_seen, created_at, updated_at)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
ON CONFLICT(normalized_name) DO UPDATE SET
	name = excluded.name, cpf = excluded.cpf, phone = excluded.phone, avatar_url = excluded.avatar_url,
	avatar_key = excluded.avatar_key, last_seen = excluded.last_seen, updated_at = excluded.updated_at`,
		key, remote.Name, remote.Document, remote.Phone, remote.AvatarURL, remote.AvatarKey, updatedAt, createdAt, updatedAt)
	if err != nil {
		return err
	}
	return s.saveSyncMapping(ctx, syncEntityCustomer, key, remote.ID, remote.Revision)
}

func (s *Store) applyRemoteOrder(ctx context.Context, remote remoteServiceOrder) error {
	if remote.ID == "" {
		return errors.New("OS remota sem identificador")
	}
	mapping, found, err := s.syncMappingByRemote(ctx, syncEntityOrder, remote.ID)
	if err != nil {
		return err
	}
	if remote.DeletedAt != nil {
		if found {
			id, err := parseSyncLocalID(mapping.LocalKey)
			if err != nil {
				return err
			}
			if _, err := s.db.ExecContext(ctx, "DELETE FROM service_orders WHERE id = ?", id); err != nil {
				return err
			}
			return s.removeSyncMapping(ctx, syncEntityOrder, remote.ID)
		}
		return nil
	}
	createdAt := firstNonEmpty(remote.CreatedAt, encodeTime(time.Now().UTC()))
	updatedAt := firstNonEmpty(remote.UpdatedAt, createdAt)
	technicianID, technicianName, _ := s.resolveRemoteTechnician(ctx, remote.TechnicianID)
	if found {
		id, err := parseSyncLocalID(mapping.LocalKey)
		if err != nil {
			return err
		}
		// Quando o tecnico da nuvem nao existe neste aparelho, mantem o que ja
		// estava: zerar o responsavel fazia a OS trocar de dono no proximo save.
		result, err := s.db.ExecContext(ctx, `
UPDATE service_orders SET status = ?, cpf = ?, customer = ?, phone = ?, device = ?, brand_model = ?,
	serial = ?, issue = ?, accessories = ?, notes = ?, amount_cents = ?, cost_cents = ?,
	technician_id = CASE WHEN ? <> 0 THEN ? ELSE technician_id END,
	technician_name = CASE WHEN ? <> 0 THEN ? ELSE technician_name END,
	checklist = ?, photos = ?,
	-- Campo vazio vindo da nuvem nao apaga o que existe aqui. Ate o worker
	-- passar a devolver estes campos ele responde "" para todos, e sem esta
	-- guarda a primeira leitura apagaria a senha do aparelho e a data da
	-- cobranca de toda a base. E o mesmo cuidado que ja existe com o tecnico.
	device_key = CASE WHEN ? <> '' THEN ? ELSE device_key END,
	delivered_at = CASE WHEN ? <> '' THEN ? ELSE delivered_at END,
	paid_at = CASE WHEN ? <> '' THEN ? ELSE paid_at END,
	payment_method = CASE WHEN ? <> '' THEN ? ELSE payment_method END,
	contacted_at = CASE WHEN ? <> '' THEN ? ELSE contacted_at END,
	contact_name = CASE WHEN ? <> '' THEN ? ELSE contact_name END,
	contact_phone = CASE WHEN ? <> '' THEN ? ELSE contact_phone END,
	paid_cents = CASE WHEN ? <> 0 THEN ? ELSE paid_cents END,
	updated_at = ?
WHERE id = ?`,
			remote.Status, remote.CustomerDocument, remote.CustomerName, remote.CustomerPhone, remote.Device, remote.BrandModel,
			remote.Serial, remote.Issue, remote.Accessories, remote.Notes, remote.GrossCents, remote.CostCents,
			technicianID, technicianID, technicianID, technicianName,
			remoteChecklistText(remote.Checklist), encodePhotos(remote.Photos),
			remote.DeviceKey, remote.DeviceKey,
			remote.DeliveredAt, remote.DeliveredAt,
			remote.PaidAt, remote.PaidAt,
			remote.PaymentMethod, remote.PaymentMethod,
			remote.ContactedAt, remote.ContactedAt,
			remote.ContactName, remote.ContactName,
			remote.ContactPhone, remote.ContactPhone,
			remote.PaidCents, remote.PaidCents,
			updatedAt, id)
		if err != nil {
			return err
		}
		// A linha local pode ter sido apagada aqui (exclusao que a nuvem recusou).
		// Nesse caso o vinculo antigo nao serve mais e a OS e recriada abaixo.
		if affected, err := result.RowsAffected(); err == nil && affected == 0 {
			if err := s.removeSyncMapping(ctx, syncEntityOrder, remote.ID); err != nil {
				return err
			}
		} else {
			return s.saveSyncMapping(ctx, syncEntityOrder, mapping.LocalKey, remote.ID, remote.Revision)
		}
	}
	result, err := s.db.ExecContext(ctx, `
INSERT INTO service_orders(status, cpf, customer, phone, device, brand_model, serial, issue, accessories, notes,
	amount_cents, cost_cents, technician_id, technician_name, checklist, photos, device_key,
	delivered_at, paid_at, payment_method, contacted_at, contact_name, contact_phone, paid_cents,
	created_at, updated_at)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		remote.Status, remote.CustomerDocument, remote.CustomerName, remote.CustomerPhone, remote.Device, remote.BrandModel,
		remote.Serial, remote.Issue, remote.Accessories, remote.Notes, remote.GrossCents, remote.CostCents,
		technicianID, technicianName,
		remoteChecklistText(remote.Checklist), encodePhotos(remote.Photos), remote.DeviceKey,
		remote.DeliveredAt, remote.PaidAt, remote.PaymentMethod, remote.ContactedAt,
		remote.ContactName, remote.ContactPhone, remote.PaidCents, createdAt, updatedAt)
	if err != nil {
		return err
	}
	id, err := result.LastInsertId()
	if err != nil {
		return err
	}
	return s.saveSyncMapping(ctx, syncEntityOrder, syncLocalKey(id), remote.ID, remote.Revision)
}

func (s *Store) applyRemoteTerm(ctx context.Context, remote remoteSalesTerm) error {
	if remote.ID == "" {
		return errors.New("termo remoto sem identificador")
	}
	mapping, found, err := s.syncMappingByRemote(ctx, syncEntityTerm, remote.ID)
	if err != nil {
		return err
	}
	if remote.DeletedAt != nil {
		if found {
			id, err := parseSyncLocalID(mapping.LocalKey)
			if err != nil {
				return err
			}
			if _, err := s.db.ExecContext(ctx, "DELETE FROM sales_terms WHERE id = ?", id); err != nil {
				return err
			}
			return s.removeSyncMapping(ctx, syncEntityTerm, remote.ID)
		}
		return nil
	}
	var term ServiceTerm
	if len(remote.Payload) > 0 {
		if err := json.Unmarshal(remote.Payload, &term); err != nil {
			return err
		}
	}
	term.TermType = firstNonEmpty(remote.TermType, term.TermType)
	term.Status = firstNonEmpty(remote.Status, term.Status)
	term.AmountCents = remote.AmountCents
	term.PDFKey = firstNonEmpty(remote.PDFKey, term.PDFKey)
	term.SignedKey = firstNonEmpty(remote.SignedKey, term.SignedKey)
	createdAt := decodeTime(firstNonEmpty(remote.CreatedAt, encodeTime(time.Now().UTC())))
	updatedAt := decodeTime(firstNonEmpty(remote.UpdatedAt, encodeTime(createdAt)))
	if term.TermDate.IsZero() {
		term.TermDate = createdAt
	}
	term.CreatedAt = createdAt
	term.UpdatedAt = updatedAt

	if found {
		id, err := parseSyncLocalID(mapping.LocalKey)
		if err != nil {
			return err
		}
		term.ID = id
		if _, err := s.db.ExecContext(ctx, `
UPDATE sales_terms SET term_type = ?, status = ?, seller_name = ?, seller_cpf = ?, seller_rg = ?,
seller_phone = ?, seller_address = ?, buyer_company = ?, buyer_document = ?, buyer_address = ?, buyer_phone = ?,
device_kind = ?, brand = ?, model = ?, color = ?, imei1 = ?, imei2 = ?, accessories = ?, declaration = ?,
amount_cents = ?, payment_method = ?, city = ?, term_date = ?, pdf_key = ?, pdf_url = ?, signed_key = ?, signed_url = ?, updated_at = ?
WHERE id = ?`,
			term.TermType, term.Status, term.SellerName, term.SellerCPF, term.SellerRG, term.SellerPhone, term.SellerAddress,
			term.BuyerCompany, term.BuyerDocument, term.BuyerAddress, term.BuyerPhone, term.DeviceKind, term.Brand, term.Model,
			term.Color, term.IMEI1, term.IMEI2, term.Accessories, term.Declaration, term.AmountCents, term.PaymentMethod,
			term.City, encodeTime(term.TermDate), term.PDFKey, term.PDFURL, term.SignedKey, term.SignedURL, encodeTime(term.UpdatedAt), id); err != nil {
			return err
		}
		return s.saveSyncMapping(ctx, syncEntityTerm, mapping.LocalKey, remote.ID, remote.Revision)
	}
	result, err := s.db.ExecContext(ctx, `
INSERT INTO sales_terms(term_type, status, seller_name, seller_cpf, seller_rg, seller_phone, seller_address,
buyer_company, buyer_document, buyer_address, buyer_phone, device_kind, brand, model, color, imei1, imei2,
accessories, declaration, amount_cents, payment_method, city, term_date, pdf_key, pdf_url, signed_key, signed_url, created_at, updated_at)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		term.TermType, term.Status, term.SellerName, term.SellerCPF, term.SellerRG, term.SellerPhone, term.SellerAddress,
		term.BuyerCompany, term.BuyerDocument, term.BuyerAddress, term.BuyerPhone, term.DeviceKind, term.Brand, term.Model,
		term.Color, term.IMEI1, term.IMEI2, term.Accessories, term.Declaration, term.AmountCents, term.PaymentMethod,
		term.City, encodeTime(term.TermDate), term.PDFKey, term.PDFURL, term.SignedKey, term.SignedURL, encodeTime(term.CreatedAt), encodeTime(term.UpdatedAt))
	if err != nil {
		return err
	}
	id, err := result.LastInsertId()
	if err != nil {
		return err
	}
	return s.saveSyncMapping(ctx, syncEntityTerm, syncLocalKey(id), remote.ID, remote.Revision)
}

func isUniqueConstraint(err error) bool {
	return err != nil && strings.Contains(strings.ToLower(err.Error()), "unique constraint")
}

func scanString(row *sql.Row) (string, error) {
	var value string
	err := row.Scan(&value)
	return value, err
}

// resolveRemoteTechnician traduz o id de usuario da nuvem para o tecnico local.
// Sem essa traducao a OS chegava de outro aparelho sem responsavel, e quem
// abrisse a OS depois virava o tecnico dela.
func (s *Store) resolveRemoteTechnician(ctx context.Context, remoteID string) (int64, string, bool) {
	remoteID = strings.TrimSpace(remoteID)
	if remoteID == "" {
		return 0, "", false
	}
	mapping, found, err := s.syncMappingByRemote(ctx, "user", remoteID)
	if err != nil || !found {
		return 0, "", false
	}
	id, err := strconv.ParseInt(strings.TrimSpace(mapping.LocalKey), 10, 64)
	if err != nil || id == 0 {
		return 0, "", false
	}
	var name string
	if err := s.db.QueryRowContext(ctx, `SELECT name FROM app_users WHERE id = ?`, id).Scan(&name); err != nil {
		return 0, "", false
	}
	return id, name, true
}
