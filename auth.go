package main

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"golang.org/x/crypto/bcrypt"
)

const (
	roleAdministrator = "Administrador"
	roleTechnician    = "Tecnico"
)

var (
	ErrInvalidCredentials = errors.New("login ou senha invalidos")
	ErrAdminAlreadyExists = errors.New("ja existe um administrador cadastrado")
)

type AppUser struct {
	ID        int64
	Name      string
	Login     string
	Role      string
	Active    bool
	CreatedAt time.Time
	UpdatedAt time.Time
}

type AuditEvent struct {
	ID          int64
	ActorUserID int64
	ActorName   string
	Action      string
	SubjectType string
	SubjectID   int64
	SubjectName string
	Details     string
	CreatedAt   time.Time
}

func (user AppUser) IsAdmin() bool {
	return user.Role == roleAdministrator
}

func (user AppUser) DisplayRole() string {
	if user.IsAdmin() {
		return "Administrador"
	}
	return "Tecnico"
}

func normalizeLogin(login string) string {
	return strings.ToLower(strings.TrimSpace(login))
}

func validateUser(name, login, role string) error {
	if strings.TrimSpace(name) == "" {
		return errors.New("informe o nome do usuario")
	}
	login = normalizeLogin(login)
	if len(login) < 3 {
		return errors.New("o login precisa ter pelo menos 3 caracteres")
	}
	for _, r := range login {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '.' || r == '_' || r == '-' || r == '@' {
			continue
		}
		return errors.New("o login aceita letras, numeros, ponto, hifen, underline ou @")
	}
	if role != roleAdministrator && role != roleTechnician {
		return errors.New("perfil de usuario invalido")
	}
	return nil
}

func validateNewPassword(password string) error {
	if len([]rune(password)) < 8 {
		return errors.New("a senha precisa ter pelo menos 8 caracteres")
	}
	return nil
}

func (s *Store) UserCount(ctx context.Context) (int, error) {
	var count int
	err := s.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM app_users").Scan(&count)
	return count, err
}

func (s *Store) BootstrapAdministrator(ctx context.Context, name, login, password string) (AppUser, error) {
	if err := validateUser(name, login, roleAdministrator); err != nil {
		return AppUser{}, err
	}
	if err := validateNewPassword(password); err != nil {
		return AppUser{}, err
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return AppUser{}, err
	}
	defer func() { _ = tx.Rollback() }()

	var count int
	if err := tx.QueryRowContext(ctx, "SELECT COUNT(*) FROM app_users").Scan(&count); err != nil {
		return AppUser{}, err
	}
	if count > 0 {
		return AppUser{}, ErrAdminAlreadyExists
	}

	user := AppUser{
		Name:   strings.TrimSpace(name),
		Login:  normalizeLogin(login),
		Role:   roleAdministrator,
		Active: true,
	}
	if err := insertUser(ctx, tx, &user, password); err != nil {
		return AppUser{}, err
	}
	if err := tx.Commit(); err != nil {
		return AppUser{}, err
	}
	return user, nil
}

func (s *Store) Authenticate(ctx context.Context, login, password string) (AppUser, error) {
	login = normalizeLogin(login)
	if login == "" || password == "" {
		return AppUser{}, ErrInvalidCredentials
	}

	var (
		user      AppUser
		hash      string
		active    int
		createdAt string
		updatedAt string
	)
	err := s.db.QueryRowContext(ctx, `
SELECT id, name, login, password_hash, role, active, created_at, updated_at
FROM app_users
WHERE login = ? AND active = 1`, login).Scan(
		&user.ID, &user.Name, &user.Login, &hash, &user.Role, &active, &createdAt, &updatedAt,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return AppUser{}, ErrInvalidCredentials
	}
	if err != nil {
		return AppUser{}, err
	}
	if bcrypt.CompareHashAndPassword([]byte(hash), []byte(password)) != nil {
		return AppUser{}, ErrInvalidCredentials
	}
	user.Active = active != 0
	user.CreatedAt = decodeTime(createdAt)
	user.UpdatedAt = decodeTime(updatedAt)
	return user, nil
}

func localRoleFromRemote(role string) (string, error) {
	switch strings.ToLower(strings.TrimSpace(role)) {
	case "admin":
		return roleAdministrator, nil
	case "technician":
		return roleTechnician, nil
	default:
		return "", errors.New("perfil remoto invalido")
	}
}

func (s *Store) UpsertRemoteUser(ctx context.Context, remote RemoteUser, password string) (AppUser, error) {
	if strings.TrimSpace(remote.ID) == "" || strings.TrimSpace(remote.Login) == "" {
		return AppUser{}, errors.New("usuario remoto invalido")
	}
	role, err := localRoleFromRemote(remote.Role)
	if err != nil {
		return AppUser{}, err
	}
	if err := validateNewPassword(password); err != nil {
		return AppUser{}, err
	}
	passwordHash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return AppUser{}, err
	}

	now := time.Now().UTC()
	login := normalizeLogin(remote.Login)
	user := AppUser{Name: strings.TrimSpace(remote.Name), Login: login, Role: role, Active: remote.Active, UpdatedAt: now}
	var existingID int64
	err = s.db.QueryRowContext(ctx, "SELECT id FROM app_users WHERE login = ?", login).Scan(&existingID)
	switch {
	case errors.Is(err, sql.ErrNoRows):
		res, err := s.db.ExecContext(ctx, `
INSERT INTO app_users(name, login, password_hash, role, active, created_at, updated_at)
VALUES (?, ?, ?, ?, ?, ?, ?)`,
			user.Name, user.Login, string(passwordHash), user.Role, boolToInt(user.Active), encodeTime(now), encodeTime(now))
		if err != nil {
			return AppUser{}, err
		}
		user.ID, err = res.LastInsertId()
		if err != nil {
			return AppUser{}, err
		}
		user.CreatedAt = now
	case err != nil:
		return AppUser{}, err
	default:
		user.ID = existingID
		if _, err := s.db.ExecContext(ctx, `
UPDATE app_users SET name = ?, password_hash = ?, role = ?, active = ?, updated_at = ? WHERE id = ?`,
			user.Name, string(passwordHash), user.Role, boolToInt(user.Active), encodeTime(now), user.ID); err != nil {
			return AppUser{}, err
		}
		var createdAt string
		if err := s.db.QueryRowContext(ctx, "SELECT created_at FROM app_users WHERE id = ?", user.ID).Scan(&createdAt); err == nil {
			user.CreatedAt = decodeTime(createdAt)
		}
	}
	if err := s.saveSyncMapping(ctx, "user", strconv.FormatInt(user.ID, 10), remote.ID, 0); err != nil {
		return AppUser{}, err
	}
	return user, nil
}

func (s *Store) UpsertRemoteUserProfile(ctx context.Context, remote RemoteUser) (AppUser, error) {
	if strings.TrimSpace(remote.ID) == "" || strings.TrimSpace(remote.Login) == "" {
		return AppUser{}, errors.New("usuario remoto invalido")
	}
	role, err := localRoleFromRemote(remote.Role)
	if err != nil {
		return AppUser{}, err
	}
	now := time.Now().UTC()
	login := normalizeLogin(remote.Login)
	user := AppUser{Name: strings.TrimSpace(remote.Name), Login: login, Role: role, Active: remote.Active, UpdatedAt: now}
	var existingID int64
	err = s.db.QueryRowContext(ctx, "SELECT id FROM app_users WHERE login = ?", login).Scan(&existingID)
	switch {
	case errors.Is(err, sql.ErrNoRows):
		res, err := s.db.ExecContext(ctx, `
INSERT INTO app_users(name, login, password_hash, role, active, created_at, updated_at)
VALUES (?, ?, '!', ?, ?, ?, ?)`, user.Name, user.Login, user.Role, boolToInt(user.Active), encodeTime(now), encodeTime(now))
		if err != nil {
			return AppUser{}, err
		}
		user.ID, err = res.LastInsertId()
		if err != nil {
			return AppUser{}, err
		}
		user.CreatedAt = now
	case err != nil:
		return AppUser{}, err
	default:
		user.ID = existingID
		if _, err := s.db.ExecContext(ctx, `
UPDATE app_users SET name = ?, role = ?, active = ?, updated_at = ? WHERE id = ?`,
			user.Name, user.Role, boolToInt(user.Active), encodeTime(now), user.ID); err != nil {
			return AppUser{}, err
		}
		var createdAt string
		if err := s.db.QueryRowContext(ctx, "SELECT created_at FROM app_users WHERE id = ?", user.ID).Scan(&createdAt); err == nil {
			user.CreatedAt = decodeTime(createdAt)
		}
	}
	if err := s.saveSyncMapping(ctx, "user", strconv.FormatInt(user.ID, 10), remote.ID, 0); err != nil {
		return AppUser{}, err
	}
	return user, nil
}

func (s *Store) ListUsers(ctx context.Context) ([]AppUser, error) {
	rows, err := s.db.QueryContext(ctx, `
SELECT id, name, login, role, active, created_at, updated_at
FROM app_users
ORDER BY active DESC, CASE role WHEN ? THEN 0 ELSE 1 END, lower(name), id`, roleAdministrator)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	users := make([]AppUser, 0, 8)
	for rows.Next() {
		var (
			user      AppUser
			active    int
			createdAt string
			updatedAt string
		)
		if err := rows.Scan(&user.ID, &user.Name, &user.Login, &user.Role, &active, &createdAt, &updatedAt); err != nil {
			return nil, err
		}
		user.Active = active != 0
		user.CreatedAt = decodeTime(createdAt)
		user.UpdatedAt = decodeTime(updatedAt)
		users = append(users, user)
	}
	return users, rows.Err()
}

func (s *Store) SaveUser(ctx context.Context, user *AppUser, password string) error {
	if user == nil {
		return errors.New("usuario invalido")
	}
	if err := validateUser(user.Name, user.Login, user.Role); err != nil {
		return err
	}
	user.Name = strings.TrimSpace(user.Name)
	user.Login = normalizeLogin(user.Login)

	if user.ID == 0 {
		if err := validateNewPassword(password); err != nil {
			return err
		}
		return insertUser(ctx, s.db, user, password)
	}

	if !user.IsAdmin() || !user.Active {
		var otherActiveAdmins int
		if err := s.db.QueryRowContext(ctx, `
SELECT COUNT(*) FROM app_users
WHERE role = ? AND active = 1 AND id <> ?`, roleAdministrator, user.ID).Scan(&otherActiveAdmins); err != nil {
			return err
		}
		if otherActiveAdmins == 0 {
			return errors.New("mantenha pelo menos um administrador ativo")
		}
	}

	var passwordHash string
	if password != "" {
		if err := validateNewPassword(password); err != nil {
			return err
		}
		hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
		if err != nil {
			return err
		}
		passwordHash = string(hash)
	} else {
		if err := s.db.QueryRowContext(ctx, "SELECT password_hash FROM app_users WHERE id = ?", user.ID).Scan(&passwordHash); err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return errors.New("usuario nao encontrado")
			}
			return err
		}
	}

	now := time.Now().UTC()
	_, err := s.db.ExecContext(ctx, `
UPDATE app_users
SET name = ?, login = ?, password_hash = ?, role = ?, active = ?, updated_at = ?
WHERE id = ?`,
		user.Name, user.Login, passwordHash, user.Role, boolToInt(user.Active), encodeTime(now), user.ID,
	)
	if err != nil {
		return err
	}
	user.UpdatedAt = now
	return nil
}

// SetUserPassword troca so a senha local. A conta compartilhada e trocada na
// nuvem, mas o hash daqui e o que vale quando a loja fica sem internet: sem
// isto a senha antiga continuaria abrindo o app offline.
func (s *Store) SetUserPassword(ctx context.Context, id int64, password string) error {
	if err := validateNewPassword(password); err != nil {
		return err
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return err
	}
	result, err := s.db.ExecContext(ctx,
		"UPDATE app_users SET password_hash = ?, updated_at = ? WHERE id = ?",
		string(hash), encodeTime(time.Now().UTC()), id)
	if err != nil {
		return err
	}
	if affected, err := result.RowsAffected(); err == nil && affected == 0 {
		return errors.New("usuario nao encontrado")
	}
	return nil
}

// SaveUserAsAdministrator applies the user-management policy used by the UI.
// New accounts are always technicians; only an administrator can promote one later.
func (s *Store) SaveUserAsAdministrator(ctx context.Context, actor AppUser, user *AppUser, password string) error {
	if !actor.IsAdmin() || !actor.Active {
		return errors.New("somente um administrador ativo pode gerenciar usuarios")
	}
	if user == nil {
		return errors.New("usuario invalido")
	}

	isNew := user.ID == 0
	previousRole := ""
	if isNew {
		if user.Role != roleTechnician {
			return errors.New("novos acessos devem ser criados como tecnicos")
		}
	} else {
		if err := s.db.QueryRowContext(ctx, "SELECT role FROM app_users WHERE id = ?", user.ID).Scan(&previousRole); err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return errors.New("usuario nao encontrado")
			}
			return err
		}
	}

	if err := s.SaveUser(ctx, user, password); err != nil {
		return err
	}

	action := "usuario.atualizado"
	details := user.DisplayRole()
	if isNew {
		action = "usuario.criado"
		details = "Tecnico"
	} else if previousRole != user.Role {
		if user.IsAdmin() {
			action = "usuario.promovido_administrador"
		} else {
			action = "usuario.rebaixado_tecnico"
		}
	}
	return s.LogAuditEvent(ctx, actor, action, "usuario", user.ID, user.Name, details)
}

func (s *Store) LogAuditEvent(ctx context.Context, actor AppUser, action string, subjectType string, subjectID int64, subjectName string, details string) error {
	if actor.ID == 0 {
		return errors.New("usuario responsavel pela auditoria invalido")
	}
	_, err := s.db.ExecContext(ctx, `
INSERT INTO audit_events(actor_user_id, actor_name, action, subject_type, subject_id, subject_name, details, created_at)
VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		actor.ID,
		strings.TrimSpace(actor.Name),
		strings.TrimSpace(action),
		strings.TrimSpace(subjectType),
		subjectID,
		strings.TrimSpace(subjectName),
		strings.TrimSpace(details),
		encodeTime(time.Now().UTC()),
	)
	return err
}

func (s *Store) ListAuditEvents(ctx context.Context, limit int) ([]AuditEvent, error) {
	if limit <= 0 || limit > 500 {
		limit = 100
	}
	rows, err := s.db.QueryContext(ctx, `
SELECT id, actor_user_id, actor_name, action, subject_type, subject_id, subject_name, details, created_at
FROM audit_events
ORDER BY created_at DESC, id DESC
LIMIT ?`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	events := make([]AuditEvent, 0, limit)
	for rows.Next() {
		var event AuditEvent
		var createdAt string
		if err := rows.Scan(
			&event.ID,
			&event.ActorUserID,
			&event.ActorName,
			&event.Action,
			&event.SubjectType,
			&event.SubjectID,
			&event.SubjectName,
			&event.Details,
			&createdAt,
		); err != nil {
			return nil, err
		}
		event.CreatedAt = decodeTime(createdAt)
		events = append(events, event)
	}
	return events, rows.Err()
}

type userDB interface {
	ExecContext(context.Context, string, ...any) (sql.Result, error)
}

func insertUser(ctx context.Context, db userDB, user *AppUser, password string) error {
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return err
	}
	now := time.Now().UTC()
	res, err := db.ExecContext(ctx, `
INSERT INTO app_users(name, login, password_hash, role, active, created_at, updated_at)
VALUES (?, ?, ?, ?, ?, ?, ?)`,
		user.Name, user.Login, string(hash), user.Role, boolToInt(user.Active), encodeTime(now), encodeTime(now),
	)
	if err != nil {
		return fmt.Errorf("salvar usuario: %w", err)
	}
	id, err := res.LastInsertId()
	if err != nil {
		return err
	}
	user.ID = id
	user.CreatedAt = now
	user.UpdatedAt = now
	return nil
}

func boolToInt(value bool) int {
	if value {
		return 1
	}
	return 0
}
