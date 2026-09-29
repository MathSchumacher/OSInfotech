package main

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"
)

const (
	syncEntityCustomer          = "customer"
	syncEntityOrder             = "service_order"
	syncEntityTerm              = "sales_term"
	syncEntityChecklistTemplate = "checklist_template"
)

type SyncQueueEntry struct {
	EntityType   string          `json:"entity_type"`
	EntityRemote string          `json:"entity_id"`
	LocalKey     string          `json:"-"`
	Action       string          `json:"action"`
	BaseRevision int64           `json:"base_revision"`
	Payload      json.RawMessage `json:"payload,omitempty"`
}

type syncEntityMapping struct {
	EntityType string
	LocalKey   string
	RemoteID   string
	Revision   int64
}

type syncCustomerPayload struct {
	Name      string `json:"name"`
	Document  string `json:"document"`
	Phone     string `json:"phone"`
	AvatarKey string `json:"avatar_key"`
	AvatarURL string `json:"avatar_url"`
}

type syncOrderPayload struct {
	Status           string          `json:"status"`
	CustomerID       string          `json:"customer_id,omitempty"`
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
	TechnicianID     string          `json:"technician_id,omitempty"`
	Checklist        *OrderChecklist `json:"checklist,omitempty"`
	Photos           []OrderPhoto    `json:"photos,omitempty"`
	// Campos que ate agora ficavam presos num computador so. device_key e a
	// senha do aparelho, contacted_at a data da cobranca, e os tres de
	// fechamento respondem quando a OS foi entregue e se foi paga. model_key e
	// service_key continuam de fora de proposito: sao derivados do texto ja
	// gravado, entao cada aparelho chega ao mesmo valor sozinho.
	DeviceKey     string `json:"device_key"`
	DeliveredAt   string `json:"delivered_at"`
	PaidAt        string `json:"paid_at"`
	PaymentMethod string `json:"payment_method"`
	ContactedAt   string `json:"contacted_at"`
	ContactName   string `json:"contact_name"`
	ContactPhone  string `json:"contact_phone"`
	// PaidCents segue a mesma guarda dos outros: zero vindo da nuvem nao apaga
	// o que existe aqui. O preco disso e que corrigir um pagamento para zero
	// vale so no aparelho onde foi corrigido; o preco de nao guardar seria uma
	// versao antiga do app zerar o pagamento de todo mundo.
	PaidCents int64 `json:"paid_cents"`
}

type syncChecklistTemplatePayload struct {
	Name       string   `json:"name"`
	DeviceType string   `json:"device_type"`
	Items      []string `json:"items"`
}

type syncTermPayload struct {
	TermType    string      `json:"term_type"`
	Status      string      `json:"status"`
	Payload     ServiceTerm `json:"payload"`
	AmountCents int64       `json:"amount_cents"`
	PDFKey      string      `json:"pdf_key"`
	SignedKey   string      `json:"signed_key"`
}

func newSyncID() (string, error) {
	bytes := make([]byte, 16)
	if _, err := rand.Read(bytes); err != nil {
		return "", err
	}
	bytes[6] = (bytes[6] & 0x0f) | 0x40
	bytes[8] = (bytes[8] & 0x3f) | 0x80
	encoded := hex.EncodeToString(bytes)
	return encoded[0:8] + "-" + encoded[8:12] + "-" + encoded[12:16] + "-" + encoded[16:20] + "-" + encoded[20:], nil
}

func (s *Store) syncMapping(ctx context.Context, entityType string, localKey string) (syncEntityMapping, error) {
	localKey = strings.TrimSpace(localKey)
	if localKey == "" {
		return syncEntityMapping{}, errors.New("chave local de sincronizacao invalida")
	}
	var mapping syncEntityMapping
	err := s.db.QueryRowContext(ctx, `
SELECT entity_type, local_key, remote_id, revision
FROM sync_entity_map
WHERE entity_type = ? AND local_key = ?`, entityType, localKey).Scan(
		&mapping.EntityType, &mapping.LocalKey, &mapping.RemoteID, &mapping.Revision,
	)
	if err == nil {
		return mapping, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return syncEntityMapping{}, err
	}

	remoteID, err := newSyncID()
	if err != nil {
		return syncEntityMapping{}, err
	}
	mapping = syncEntityMapping{EntityType: entityType, LocalKey: localKey, RemoteID: remoteID}
	_, err = s.db.ExecContext(ctx, `
INSERT INTO sync_entity_map(entity_type, local_key, remote_id, revision, updated_at)
VALUES (?, ?, ?, 0, ?)`, entityType, localKey, remoteID, encodeTime(time.Now().UTC()))
	if err != nil {
		return syncEntityMapping{}, err
	}
	return mapping, nil
}

func (s *Store) existingSyncMapping(ctx context.Context, entityType string, localKey string) (syncEntityMapping, bool, error) {
	var mapping syncEntityMapping
	err := s.db.QueryRowContext(ctx, `
SELECT entity_type, local_key, remote_id, revision
FROM sync_entity_map
WHERE entity_type = ? AND local_key = ?`, entityType, strings.TrimSpace(localKey)).Scan(
		&mapping.EntityType, &mapping.LocalKey, &mapping.RemoteID, &mapping.Revision,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return syncEntityMapping{}, false, nil
	}
	if err != nil {
		return syncEntityMapping{}, false, err
	}
	return mapping, true, nil
}

func (s *Store) syncMappingByRemote(ctx context.Context, entityType string, remoteID string) (syncEntityMapping, bool, error) {
	var mapping syncEntityMapping
	err := s.db.QueryRowContext(ctx, `
SELECT entity_type, local_key, remote_id, revision
FROM sync_entity_map
WHERE entity_type = ? AND remote_id = ?`, entityType, strings.TrimSpace(remoteID)).Scan(
		&mapping.EntityType, &mapping.LocalKey, &mapping.RemoteID, &mapping.Revision,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return syncEntityMapping{}, false, nil
	}
	if err != nil {
		return syncEntityMapping{}, false, err
	}
	return mapping, true, nil
}

func (s *Store) saveSyncMapping(ctx context.Context, entityType, localKey, remoteID string, revision int64) error {
	if _, err := s.db.ExecContext(ctx, `
DELETE FROM sync_entity_map
WHERE entity_type = ? AND remote_id = ? AND local_key <> ?`, entityType, remoteID, localKey); err != nil {
		return err
	}
	_, err := s.db.ExecContext(ctx, `
INSERT INTO sync_entity_map(entity_type, local_key, remote_id, revision, updated_at)
VALUES (?, ?, ?, ?, ?)
ON CONFLICT(entity_type, local_key) DO UPDATE SET
	remote_id = excluded.remote_id,
	revision = excluded.revision,
	updated_at = excluded.updated_at`,
		entityType, localKey, remoteID, revision, encodeTime(time.Now().UTC()))
	return err
}

func (s *Store) removeSyncMapping(ctx context.Context, entityType, remoteID string) error {
	_, err := s.db.ExecContext(ctx, "DELETE FROM sync_entity_map WHERE entity_type = ? AND remote_id = ?", entityType, remoteID)
	return err
}

func (s *Store) enqueueSync(ctx context.Context, entityType, localKey, action string, payload any) error {
	mapping, err := s.syncMapping(ctx, entityType, localKey)
	if err != nil {
		return err
	}
	data := []byte("{}")
	if action == "upsert" {
		data, err = json.Marshal(payload)
		if err != nil {
			return err
		}
	}
	_, err = s.db.ExecContext(ctx, `
INSERT INTO sync_queue(entity_type, entity_remote_id, local_key, action, base_revision, payload_json, queued_at)
VALUES (?, ?, ?, ?, ?, ?, ?)
ON CONFLICT(entity_type, entity_remote_id) DO UPDATE SET
	local_key = excluded.local_key,
	action = excluded.action,
	base_revision = excluded.base_revision,
	payload_json = excluded.payload_json,
	queued_at = excluded.queued_at`,
		entityType, mapping.RemoteID, localKey, action, mapping.Revision, string(data), encodeTime(time.Now().UTC()))
	return err
}

func (s *Store) PendingSyncChanges(ctx context.Context, limit int) ([]SyncQueueEntry, error) {
	if limit <= 0 || limit > 200 {
		limit = 100
	}
	rows, err := s.db.QueryContext(ctx, `
SELECT entity_type, entity_remote_id, local_key, action, base_revision, payload_json
FROM sync_queue
ORDER BY queued_at ASC, entity_type ASC
LIMIT ?`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	changes := make([]SyncQueueEntry, 0, limit)
	for rows.Next() {
		var change SyncQueueEntry
		var payload string
		if err := rows.Scan(&change.EntityType, &change.EntityRemote, &change.LocalKey, &change.Action, &change.BaseRevision, &payload); err != nil {
			return nil, err
		}
		change.Payload = json.RawMessage(payload)
		changes = append(changes, change)
	}
	return changes, rows.Err()
}

func (s *Store) completeSyncChange(ctx context.Context, change SyncQueueEntry, revision int64) error {
	// Exclusao confirmada: o vinculo com a nuvem tem de sair junto. Mantendo o
	// vinculo, a proxima leitura da nuvem tentava atualizar uma linha que nao
	// existe mais e a OS sumia deste aparelho para sempre.
	if change.Action == "delete" {
		if err := s.removeSyncMapping(ctx, change.EntityType, change.EntityRemote); err != nil {
			return err
		}
		_, err := s.db.ExecContext(ctx, "DELETE FROM sync_queue WHERE entity_type = ? AND entity_remote_id = ?", change.EntityType, change.EntityRemote)
		return err
	}
	if err := s.saveSyncMapping(ctx, change.EntityType, change.LocalKey, change.EntityRemote, revision); err != nil {
		return err
	}
	_, err := s.db.ExecContext(ctx, "DELETE FROM sync_queue WHERE entity_type = ? AND entity_remote_id = ?", change.EntityType, change.EntityRemote)
	return err
}

func (s *Store) discardSyncChange(ctx context.Context, change SyncQueueEntry) error {
	_, err := s.db.ExecContext(ctx, "DELETE FROM sync_queue WHERE entity_type = ? AND entity_remote_id = ?", change.EntityType, change.EntityRemote)
	return err
}

func (s *Store) SyncQueueSize(ctx context.Context) (int, error) {
	var count int
	err := s.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM sync_queue").Scan(&count)
	return count, err
}

func (s *Store) queueOrder(ctx context.Context, order ServiceOrder) error {
	if order.ID == 0 {
		return errors.New("salve a OS antes de sincronizar")
	}
	technicianID := ""
	if order.TechnicianID != 0 {
		if mapping, found, err := s.existingSyncMapping(ctx, "user", strconv.FormatInt(order.TechnicianID, 10)); err == nil && found {
			technicianID = mapping.RemoteID
		}
	}
	customerID := ""
	if key := normalizeCustomerName(order.Customer); key != "" {
		if mapping, err := s.syncMapping(ctx, syncEntityCustomer, key); err == nil {
			customerID = mapping.RemoteID
		}
	}
	payload := syncOrderPayload{
		Status: order.Status, CustomerID: customerID, CustomerName: order.Customer, CustomerDocument: order.CPF,
		CustomerPhone: order.Phone, Device: order.Device, BrandModel: order.BrandModel, Serial: order.Serial,
		Issue: order.Issue, Accessories: order.Accessories, Notes: order.Notes, GrossCents: order.AmountCents,
		CostCents: order.CostCents, TechnicianID: technicianID, Photos: order.Photos,
		DeviceKey: order.DeviceKey, DeliveredAt: encodeSecond(order.DeliveredAt),
		PaidAt: encodeSecond(order.PaidAt), PaymentMethod: order.PaymentMethod,
		ContactedAt: encodeSecond(order.ContactedAt),
		ContactName: order.ContactName, ContactPhone: order.ContactPhone,
		PaidCents: order.PaidCents,
	}
	if len(order.Checklist.Items) > 0 || order.Checklist.TemplateName != "" {
		checklist := order.Checklist
		payload.Checklist = &checklist
	}
	return s.enqueueSync(ctx, syncEntityOrder, strconv.FormatInt(order.ID, 10), "upsert", payload)
}

func (s *Store) queueChecklistTemplate(ctx context.Context, tpl ChecklistTemplate) error {
	if tpl.ID == 0 {
		return errors.New("salve o template antes de sincronizar")
	}
	payload := syncChecklistTemplatePayload{
		Name: tpl.Name, DeviceType: tpl.DeviceType, Items: tpl.Items,
	}
	return s.enqueueSync(ctx, syncEntityChecklistTemplate, strconv.FormatInt(tpl.ID, 10), "upsert", payload)
}

func (s *Store) queueCustomerByKey(ctx context.Context, key string) error {
	key = normalizeCustomerName(key)
	if key == "" {
		return nil
	}
	var profile CustomerProfile
	err := s.db.QueryRowContext(ctx, `
SELECT normalized_name, name, cpf, phone, avatar_url, avatar_key, last_device, last_model, last_seen
FROM customer_profiles WHERE normalized_name = ?`, key).Scan(
		&profile.Key, &profile.Name, &profile.CPF, &profile.Phone, &profile.AvatarURL, &profile.AvatarKey,
		&profile.LastDevice, &profile.LastModel, new(string),
	)
	if errors.Is(err, sql.ErrNoRows) {
		return nil
	}
	if err != nil {
		return err
	}
	payload := syncCustomerPayload{Name: profile.Name, Document: profile.CPF, Phone: profile.Phone, AvatarKey: profile.AvatarKey, AvatarURL: profile.AvatarURL}
	return s.enqueueSync(ctx, syncEntityCustomer, key, "upsert", payload)
}

func (s *Store) queueTerm(ctx context.Context, term ServiceTerm) error {
	if term.ID == 0 {
		return errors.New("salve o termo antes de sincronizar")
	}
	payload := syncTermPayload{
		TermType: term.TermType, Status: term.Status, Payload: term, AmountCents: term.AmountCents,
		PDFKey: term.PDFKey, SignedKey: term.SignedKey,
	}
	return s.enqueueSync(ctx, syncEntityTerm, strconv.FormatInt(term.ID, 10), "upsert", payload)
}

func (s *Store) queueDelete(ctx context.Context, entityType, localKey string) error {
	return s.enqueueSync(ctx, entityType, localKey, "delete", nil)
}

func (s *Store) SeedSyncQueue(ctx context.Context) error {
	if seeded, ok, err := s.GetSetting(ctx, "d1_sync_seeded"); err != nil {
		return err
	} else if ok && seeded == "1" {
		return nil
	}

	profiles, err := s.ListCustomerProfiles(ctx, "")
	if err != nil {
		return err
	}
	for _, profile := range profiles {
		if err := s.queueCustomerByKey(ctx, profile.Key); err != nil {
			return err
		}
	}
	orders, err := s.List(ctx, "Todos", "")
	if err != nil {
		return err
	}
	for _, order := range orders {
		if err := s.queueOrder(ctx, order); err != nil {
			return err
		}
	}
	terms, err := s.ListTerms(ctx, "", termFilterAll)
	if err != nil {
		return err
	}
	for _, term := range terms {
		if err := s.queueTerm(ctx, term); err != nil {
			return err
		}
	}
	templates, err := s.ListChecklistTemplates(ctx)
	if err != nil {
		return err
	}
	for _, tpl := range templates {
		if err := s.queueChecklistTemplate(ctx, tpl); err != nil {
			return err
		}
	}
	return s.SetSetting(ctx, "d1_sync_seeded", "1")
}

func syncLocalKey(id int64) string {
	return strconv.FormatInt(id, 10)
}

func parseSyncLocalID(value string) (int64, error) {
	id, err := strconv.ParseInt(value, 10, 64)
	if err != nil || id <= 0 {
		return 0, fmt.Errorf("chave local de sincronizacao invalida: %q", value)
	}
	return id, nil
}
