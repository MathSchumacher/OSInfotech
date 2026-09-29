package main

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"
)

// O cliente e identificado pelo nome normalizado (normalized_name). Por isso
// trocar o nome criava um segundo cadastro, e nada impedia dois clientes com o
// mesmo CPF ou telefone. Este arquivo cuida da identidade do cliente:
// renomear sem duplicar, recusar documento repetido e unificar duplicados.

// ErrCustomerNameTaken indica que ja existe outro cliente com o nome novo.
var ErrCustomerNameTaken = errors.New("ja existe um cliente com esse nome")

// customerConflict descreve um cadastro que ja usa o mesmo documento ou telefone.
type customerConflict struct {
	Profile CustomerProfile
	Field   string // "CPF" ou "telefone"
	Value   string
}

// isFillerNumber reconhece os numeros de preenchimento que a equipe digita
// quando o cliente nao informa contato (0000000000, 99999999999...). Eles se
// repetem em varios cadastros e nao identificam ninguem.
func isFillerNumber(digits string) bool {
	if digits == "" {
		return true
	}
	first := digits[0]
	for i := 0; i < len(digits); i++ {
		if digits[i] != first {
			return false
		}
	}
	return true
}

// documentKey devolve os digitos do CPF/CNPJ quando o valor tem tamanho de
// documento real. Valores curtos ou repetidos nao servem para identificar.
func documentKey(document string) string {
	digits := onlyDigits(document)
	if len(digits) != 11 && len(digits) != 14 {
		return ""
	}
	if isFillerNumber(digits) {
		return ""
	}
	return digits
}

// phoneKey normaliza o telefone para comparacao: so digitos, sem o 55 do país.
func phoneKey(phone string) string {
	digits := onlyDigits(phone)
	if len(digits) > 11 && strings.HasPrefix(digits, "55") {
		digits = digits[2:]
	}
	if len(digits) < 10 || len(digits) > 11 || isFillerNumber(digits) {
		return ""
	}
	// (51) 99999-9999 tambem e preenchimento: o DDD e de verdade, mas o numero
	// e um digito repetido. Sem isto, clientes diferentes apareciam como o
	// mesmo telefone.
	if isFillerNumber(digits[2:]) {
		return ""
	}
	return digits
}

// listCustomerRecords le apenas a tabela de cadastros (sem juntar com as OS),
// para as verificacoes feitas a cada salvamento.
func (s *Store) listCustomerRecords(ctx context.Context) ([]CustomerProfile, error) {
	rows, err := s.db.QueryContext(ctx, `
SELECT normalized_name, name, cpf, phone FROM customer_profiles`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var profiles []CustomerProfile
	for rows.Next() {
		var profile CustomerProfile
		if err := rows.Scan(&profile.Key, &profile.Name, &profile.CPF, &profile.Phone); err != nil {
			return nil, err
		}
		profiles = append(profiles, profile)
	}
	return profiles, rows.Err()
}

// FindCustomerConflict procura outro cliente que ja use o mesmo CPF ou
// telefone. exceptKey e o cliente que esta sendo salvo (nao conta como
// conflito consigo mesmo).
func (s *Store) FindCustomerConflict(ctx context.Context, exceptKey string, document string, phone string) (customerConflict, bool, error) {
	doc := documentKey(document)
	tel := phoneKey(phone)
	if doc == "" && tel == "" {
		return customerConflict{}, false, nil
	}
	profiles, err := s.listCustomerRecords(ctx)
	if err != nil {
		return customerConflict{}, false, err
	}
	exceptKey = normalizeCustomerName(exceptKey)
	for _, profile := range profiles {
		if profile.Key == exceptKey {
			continue
		}
		if doc != "" && documentKey(profile.CPF) == doc {
			return customerConflict{Profile: profile, Field: "CPF", Value: profile.CPF}, true, nil
		}
		if tel != "" && phoneKey(profile.Phone) == tel {
			return customerConflict{Profile: profile, Field: "telefone", Value: profile.Phone}, true, nil
		}
	}
	return customerConflict{}, false, nil
}

// customerProfileByKey carrega um cadastro pela chave.
func (s *Store) customerProfileByKey(ctx context.Context, key string) (CustomerProfile, bool, error) {
	var profile CustomerProfile
	var lastSeen, createdAt string
	err := s.db.QueryRowContext(ctx, `
SELECT normalized_name, name, cpf, phone, avatar_url, avatar_key, last_device, last_model, last_seen, created_at
FROM customer_profiles WHERE normalized_name = ?`, key).Scan(
		&profile.Key, &profile.Name, &profile.CPF, &profile.Phone, &profile.AvatarURL, &profile.AvatarKey,
		&profile.LastDevice, &profile.LastModel, &lastSeen, &createdAt,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return CustomerProfile{}, false, nil
	}
	if err != nil {
		return CustomerProfile{}, false, err
	}
	profile.LastSeen = decodeTime(lastSeen)
	return profile, true, nil
}

// ordersOfCustomer devolve as OS cujo cliente e exatamente esse nome.
func (s *Store) ordersOfCustomer(ctx context.Context, key string) ([]ServiceOrder, error) {
	rows, err := s.db.QueryContext(ctx,
		"SELECT "+orderSelectColumns+" FROM service_orders WHERE osfold(IFNULL(customer, '')) = osfold(?)", key)
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
		// osfold ignora acento; confirma com a mesma normalizacao da chave.
		if normalizeCustomerName(order.Customer) == key {
			orders = append(orders, order)
		}
	}
	return orders, rows.Err()
}

// RenameCustomerProfile troca o nome de um cadastro existente, levando junto as
// OS e o vinculo com a nuvem. Sem isso, editar o nome criava um segundo
// cliente e o historico ficava dividido.
func (s *Store) RenameCustomerProfile(ctx context.Context, oldKey string, profile CustomerProfile) error {
	oldKey = normalizeCustomerName(oldKey)
	newKey := normalizeCustomerName(profile.Name)
	if newKey == "" {
		return errors.New("informe o nome do cliente")
	}
	if oldKey == "" || oldKey == newKey {
		profile.Key = newKey
		return s.SaveCustomerProfile(ctx, profile)
	}
	if _, exists, err := s.customerProfileByKey(ctx, newKey); err != nil {
		return err
	} else if exists {
		return fmt.Errorf("%w: %s", ErrCustomerNameTaken, profile.Name)
	}
	previous, found, err := s.customerProfileByKey(ctx, oldKey)
	if err != nil {
		return err
	}
	if !found {
		profile.Key = newKey
		return s.SaveCustomerProfile(ctx, profile)
	}

	// Mantem o que o formulario nao edita (foto, ultimo aparelho, datas).
	if profile.AvatarURL == "" {
		profile.AvatarURL = previous.AvatarURL
	}
	if profile.AvatarKey == "" {
		profile.AvatarKey = previous.AvatarKey
	}
	now := time.Now().UTC()
	lastSeen := previous.LastSeen
	if lastSeen.IsZero() {
		lastSeen = now
	}

	if _, err := s.db.ExecContext(ctx, `
INSERT INTO customer_profiles (
	normalized_name, name, cpf, phone, avatar_url, avatar_key, last_device, last_model, last_seen, created_at, updated_at
) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		newKey, profile.Name, profile.CPF, profile.Phone, profile.AvatarURL, profile.AvatarKey,
		previous.LastDevice, previous.LastModel, encodeTime(lastSeen), encodeTime(now), encodeTime(now),
	); err != nil {
		return err
	}
	if _, err := s.db.ExecContext(ctx, `DELETE FROM customer_profiles WHERE normalized_name = ?`, oldKey); err != nil {
		return err
	}

	// O vinculo com a nuvem segue o cliente: assim o servidor recebe uma
	// alteracao de nome, e nao um cliente novo.
	if _, err := s.db.ExecContext(ctx, `
UPDATE sync_entity_map SET local_key = ? WHERE entity_type = ? AND local_key = ?`,
		newKey, syncEntityCustomer, oldKey); err != nil {
		return err
	}
	if _, err := s.db.ExecContext(ctx, `
UPDATE sync_queue SET local_key = ? WHERE entity_type = ? AND local_key = ?`,
		newKey, syncEntityCustomer, oldKey); err != nil {
		return err
	}
	for _, column := range []string{"left_key", "right_key"} {
		if _, err := s.db.ExecContext(ctx, "UPDATE OR IGNORE customer_distinct SET "+column+" = ? WHERE "+column+" = ?", newKey, oldKey); err != nil {
			return err
		}
	}

	if err := s.renameOrdersCustomer(ctx, oldKey, profile); err != nil {
		return err
	}
	return s.queueCustomerByKey(ctx, newKey)
}

// renameOrdersCustomer atualiza o nome (e o contato) nas OS do cliente.
func (s *Store) renameOrdersCustomer(ctx context.Context, oldKey string, profile CustomerProfile) error {
	orders, err := s.ordersOfCustomer(ctx, oldKey)
	if err != nil {
		return err
	}
	// A data de atualizacao da OS nao muda: ela e o que o relatorio financeiro
	// usa como data de entrega, e trocar o nome do cliente nao pode mover uma
	// OS entregue em julho para o mes atual.
	for _, order := range orders {
		cpf, phone := order.CPF, order.Phone
		if strings.TrimSpace(cpf) == "" {
			cpf = profile.CPF
		}
		if strings.TrimSpace(phone) == "" {
			phone = profile.Phone
		}
		if _, err := s.db.ExecContext(ctx, `
UPDATE service_orders SET customer = ?, cpf = ?, phone = ? WHERE id = ?`,
			profile.Name, cpf, phone, order.ID); err != nil {
			return err
		}
		order.Customer = profile.Name
		order.CPF = cpf
		order.Phone = phone
		if err := s.queueOrder(ctx, order); err != nil {
			return err
		}
	}
	return nil
}

// MergeCustomerProfiles junta dois cadastros: mantem keepKey, move as OS de
// mergeKey e completa os campos vazios. O cadastro extra e apagado.
func (s *Store) MergeCustomerProfiles(ctx context.Context, keepKey string, mergeKey string) error {
	keepKey = normalizeCustomerName(keepKey)
	mergeKey = normalizeCustomerName(mergeKey)
	if keepKey == "" || mergeKey == "" || keepKey == mergeKey {
		return errors.New("selecione dois clientes diferentes")
	}
	keep, foundKeep, err := s.customerProfileByKey(ctx, keepKey)
	if err != nil {
		return err
	}
	merge, foundMerge, err := s.customerProfileByKey(ctx, mergeKey)
	if err != nil {
		return err
	}
	if !foundKeep || !foundMerge {
		return errors.New("cliente nao encontrado")
	}

	// Completa o cadastro que fica com o que so existe no outro.
	if strings.TrimSpace(keep.CPF) == "" {
		keep.CPF = merge.CPF
	}
	if strings.TrimSpace(keep.Phone) == "" {
		keep.Phone = merge.Phone
	}
	if strings.TrimSpace(keep.AvatarURL) == "" {
		keep.AvatarURL = merge.AvatarURL
		keep.AvatarKey = merge.AvatarKey
	}
	if strings.TrimSpace(keep.LastDevice) == "" {
		keep.LastDevice = merge.LastDevice
		keep.LastModel = merge.LastModel
	}
	if merge.LastSeen.After(keep.LastSeen) {
		keep.LastSeen = merge.LastSeen
	}
	if err := s.SaveCustomerProfile(ctx, keep); err != nil {
		return err
	}
	// As OS do cadastro extra passam a apontar para o nome que fica.
	if err := s.renameOrdersCustomer(ctx, mergeKey, keep); err != nil {
		return err
	}
	if _, err := s.db.ExecContext(ctx, `DELETE FROM customer_distinct WHERE left_key = ? OR right_key = ?`, mergeKey, mergeKey); err != nil {
		return err
	}
	return s.DeleteCustomerProfile(ctx, mergeKey)
}

// duplicateGroup e um conjunto de cadastros que compartilham CPF ou telefone.
type duplicateGroup struct {
	Field    string // "CPF" ou "telefone"
	Value    string
	Profiles []CustomerProfile
}

// DuplicateCustomerGroups lista os cadastros que parecem ser a mesma pessoa.
func (s *Store) DuplicateCustomerGroups(ctx context.Context) ([]duplicateGroup, error) {
	profiles, err := s.ListCustomerProfiles(ctx, "")
	if err != nil {
		return nil, err
	}
	distinct, err := s.distinctCustomerPairs(ctx)
	if err != nil {
		return nil, err
	}
	byDocument := map[string][]CustomerProfile{}
	byPhone := map[string][]CustomerProfile{}
	for _, profile := range profiles {
		if doc := documentKey(profile.CPF); doc != "" {
			byDocument[doc] = append(byDocument[doc], profile)
		}
		if tel := phoneKey(profile.Phone); tel != "" {
			byPhone[tel] = append(byPhone[tel], profile)
		}
	}

	seen := map[string]bool{}
	var groups []duplicateGroup
	collect := func(field string, buckets map[string][]CustomerProfile, display func(CustomerProfile) string) {
		keys := make([]string, 0, len(buckets))
		for key := range buckets {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		for _, key := range keys {
			group := buckets[key]
			if len(group) < 2 {
				continue
			}
			// Evita mostrar o mesmo par duas vezes (por CPF e por telefone).
			names := make([]string, 0, len(group))
			for _, profile := range group {
				names = append(names, profile.Key)
			}
			sort.Strings(names)
			id := strings.Join(names, "|")
			if seen[id] {
				continue
			}
			seen[id] = true
			// Pares ja conferidos pelo operador ("nao e a mesma pessoa") somem.
			if allPairsDistinct(group, distinct) {
				continue
			}
			sort.SliceStable(group, func(i, j int) bool { return group[i].Orders > group[j].Orders })
			groups = append(groups, duplicateGroup{Field: field, Value: display(group[0]), Profiles: group})
		}
	}
	collect("CPF", byDocument, func(p CustomerProfile) string { return p.CPF })
	collect("telefone", byPhone, func(p CustomerProfile) string { return p.Phone })
	return groups, nil
}

// Pares conferidos: dois cadastros podem dividir o telefone sem serem a mesma
// pessoa (marido e esposa, mae e filho, a loja que atende pelo numero do
// balcao). Quando o operador confirma que sao pessoas diferentes, o par fica
// registrado aqui e o aviso de repetidos para de apontar os dois.

func orderedPair(a string, b string) (string, string) {
	if a > b {
		return b, a
	}
	return a, b
}

// MarkCustomersDistinct registra que os cadastros da lista sao pessoas
// diferentes entre si.
func (s *Store) MarkCustomersDistinct(ctx context.Context, keys []string) error {
	now := encodeTime(time.Now().UTC())
	for i := 0; i < len(keys); i++ {
		for j := i + 1; j < len(keys); j++ {
			left, right := orderedPair(normalizeCustomerName(keys[i]), normalizeCustomerName(keys[j]))
			if left == "" || right == "" || left == right {
				continue
			}
			if _, err := s.db.ExecContext(ctx, `
INSERT INTO customer_distinct (left_key, right_key, created_at) VALUES (?, ?, ?)
ON CONFLICT(left_key, right_key) DO NOTHING`, left, right, now); err != nil {
				return err
			}
		}
	}
	return nil
}

// distinctCustomerPairs devolve os pares ja conferidos, nas duas ordens, para a
// consulta nao depender de quem vem primeiro.
func (s *Store) distinctCustomerPairs(ctx context.Context) (map[string]bool, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT left_key, right_key FROM customer_distinct`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	pairs := map[string]bool{}
	for rows.Next() {
		var left, right string
		if err := rows.Scan(&left, &right); err != nil {
			return nil, err
		}
		pairs[left+"|"+right] = true
		pairs[right+"|"+left] = true
	}
	return pairs, rows.Err()
}

// allPairsDistinct diz se todos os cadastros do grupo ja foram conferidos como
// pessoas diferentes. Basta um par em aberto para o grupo continuar no aviso.
func allPairsDistinct(group []CustomerProfile, distinct map[string]bool) bool {
	if len(distinct) == 0 {
		return false
	}
	for i := 0; i < len(group); i++ {
		for j := i + 1; j < len(group); j++ {
			if !distinct[group[i].Key+"|"+group[j].Key] {
				return false
			}
		}
	}
	return true
}
