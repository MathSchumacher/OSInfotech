package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// syncPullPageSize e quantas alteracoes cada pagina do pull traz.
const syncPullPageSize = 50

const (
	defaultAPIBaseURL = "https://osinfotech-api.lanhousedbs.workers.dev"

	apiBaseURLSetting   = "api_base_url"
	apiTokenSetting     = "api_session_token"
	apiTokenExpiry      = "api_session_expires_at"
	syncCursorSetting   = "d1_sync_cursor"
	syncLastSuccessKey  = "d1_sync_last_success"
	syncLastFailureKey  = "d1_sync_last_failure"
	apiRemoteUserPrefix = "api_remote_user_"
)

type APIConfig struct {
	BaseURL string
	Token   string
}

func (s *Store) APIConfig(ctx context.Context) (APIConfig, error) {
	config := APIConfig{BaseURL: defaultAPIBaseURL}
	if value, found, err := s.GetSetting(ctx, apiBaseURLSetting); err != nil {
		return APIConfig{}, err
	} else if found && strings.TrimSpace(value) != "" {
		config.BaseURL = strings.TrimSpace(value)
	}
	if value, found, err := s.GetSetting(ctx, apiTokenSetting); err != nil {
		return APIConfig{}, err
	} else if found {
		config.Token = strings.TrimSpace(value)
	}
	config.BaseURL = strings.TrimRight(config.BaseURL, "/")
	if _, err := url.ParseRequestURI(config.BaseURL); err != nil {
		return APIConfig{}, fmt.Errorf("URL da API invalida: %w", err)
	}
	return config, nil
}

func (s *Store) SaveAPIConfig(ctx context.Context, config APIConfig) error {
	if err := s.SetSetting(ctx, apiBaseURLSetting, strings.TrimRight(strings.TrimSpace(config.BaseURL), "/")); err != nil {
		return err
	}
	return s.SetSetting(ctx, apiTokenSetting, strings.TrimSpace(config.Token))
}

type WorkerClient struct {
	baseURL string
	token   string
	http    *http.Client
}

func NewWorkerClient(config APIConfig) (*WorkerClient, error) {
	baseURL := strings.TrimRight(strings.TrimSpace(config.BaseURL), "/")
	if _, err := url.ParseRequestURI(baseURL); err != nil {
		return nil, fmt.Errorf("URL da API invalida: %w", err)
	}
	return &WorkerClient{
		baseURL: baseURL,
		token:   strings.TrimSpace(config.Token),
		// Timeout alto de seguranca: quem limita cada chamada e o contexto. A
		// primeira sincronizacao de um aparelho novo baixa tudo em paginas e o
		// worker leva alguns segundos por pagina.
		http: &http.Client{Timeout: 120 * time.Second},
	}, nil
}

type WorkerAPIError struct {
	Status  int
	Code    string
	Message string
}

func (err *WorkerAPIError) Error() string {
	if err.Message != "" {
		return err.Message
	}
	return fmt.Sprintf("erro da API (%d)", err.Status)
}

type remoteSetupStatus struct {
	Initialized bool `json:"initialized"`
}

type RemoteUser struct {
	ID             string `json:"id"`
	OrganizationID string `json:"organization_id"`
	Name           string `json:"name"`
	Login          string `json:"login"`
	Role           string `json:"role"`
	Active         bool   `json:"active"`
}

type RemoteOrganization struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	CreatedAt string `json:"created_at"`
	UpdatedAt string `json:"updated_at"`
}

type remoteOrganizationStatus struct {
	Organization RemoteOrganization `json:"organization"`
	User         RemoteUser         `json:"user"`
}

type remoteSession struct {
	User      RemoteUser `json:"user"`
	Token     string     `json:"token"`
	ExpiresAt string     `json:"expires_at"`
}

type syncPushResult struct {
	EntityID   string          `json:"entity_id"`
	EntityType string          `json:"entity_type"`
	Status     string          `json:"status"`
	Revision   int64           `json:"revision"`
	Message    string          `json:"message"`
	Entity     json.RawMessage `json:"entity"`
}

type syncPullChange struct {
	Sequence   int64           `json:"sequence"`
	EntityType string          `json:"entity_type"`
	EntityID   string          `json:"entity_id"`
	Action     string          `json:"action"`
	Revision   int64           `json:"revision"`
	ChangedAt  string          `json:"changed_at"`
	Entity     json.RawMessage `json:"entity"`
}

func (client *WorkerClient) SetupStatus(ctx context.Context) (bool, error) {
	var response remoteSetupStatus
	if err := client.request(ctx, http.MethodGet, "/v1/setup/status", nil, &response, false); err != nil {
		return false, err
	}
	return response.Initialized, nil
}

func (client *WorkerClient) Login(ctx context.Context, login string, password string) (remoteSession, error) {
	var response remoteSession
	err := client.request(ctx, http.MethodPost, "/v1/auth/login", map[string]string{
		"login": login, "password": password,
	}, &response, false)
	return response, err
}

func (client *WorkerClient) Setup(ctx context.Context, setupToken string, organization string, name string, login string, password string) (remoteSession, error) {
	var response remoteSession
	err := client.requestWithHeaders(ctx, http.MethodPost, "/v1/setup", map[string]string{
		"organization_name": organization,
		"name":              name,
		"login":             login,
		"password":          password,
	}, &response, map[string]string{"X-Setup-Token": strings.TrimSpace(setupToken)}, false)
	return response, err
}

func (client *WorkerClient) OrganizationStatus(ctx context.Context) (remoteOrganizationStatus, error) {
	var response remoteOrganizationStatus
	err := client.request(ctx, http.MethodGet, "/v1/organization", nil, &response, true)
	return response, err
}

func (client *WorkerClient) UpdateOrganizationName(ctx context.Context, name string) (remoteOrganizationStatus, error) {
	var response remoteOrganizationStatus
	err := client.request(ctx, http.MethodPatch, "/v1/organization", map[string]string{
		"name": name,
	}, &response, true)
	return response, err
}

func (client *WorkerClient) Push(ctx context.Context, changes []SyncQueueEntry) ([]syncPushResult, error) {
	var response struct {
		Results []syncPushResult `json:"results"`
	}
	if err := client.request(ctx, http.MethodPost, "/v1/sync/push", map[string]any{"changes": changes}, &response, true); err != nil {
		return nil, err
	}
	return response.Results, nil
}

func (client *WorkerClient) Pull(ctx context.Context, after int64) (int64, []syncPullChange, bool, error) {
	// Paginas menores: o worker responde ~0,3 s por alteracao, entao 100 de uma
	// vez estourava o tempo limite e a carga inicial nunca terminava.
	path := fmt.Sprintf("/v1/sync/pull?after=%d&limit=%d", after, syncPullPageSize)
	var response struct {
		Cursor  int64            `json:"cursor"`
		Changes []syncPullChange `json:"changes"`
		HasMore bool             `json:"has_more"`
	}
	if err := client.request(ctx, http.MethodGet, path, nil, &response, true); err != nil {
		return 0, nil, false, err
	}
	return response.Cursor, response.Changes, response.HasMore, nil
}

func (client *WorkerClient) ListUsers(ctx context.Context) ([]RemoteUser, error) {
	var response struct {
		Users []RemoteUser `json:"users"`
	}
	if err := client.request(ctx, http.MethodGet, "/v1/users", nil, &response, true); err != nil {
		return nil, err
	}
	return response.Users, nil
}

func (client *WorkerClient) CreateTechnician(ctx context.Context, name string, login string, password string) (RemoteUser, error) {
	var response struct {
		User RemoteUser `json:"user"`
	}
	err := client.request(ctx, http.MethodPost, "/v1/users", map[string]string{
		"name": name, "login": login, "password": password, "role": "technician",
	}, &response, true)
	return response.User, err
}

// UpdateUserAccess altera papel, situacao e, quando password vem preenchida, a
// senha. Senha vazia mantem a atual: e o que a tela promete no campo.
func (client *WorkerClient) UpdateUserAccess(ctx context.Context, remoteID string, role string, active bool, password string) (RemoteUser, error) {
	var response struct {
		User RemoteUser `json:"user"`
	}
	body := map[string]any{"role": role, "active": active}
	if strings.TrimSpace(password) != "" {
		body["password"] = password
	}
	err := client.request(ctx, http.MethodPatch, "/v1/users/"+url.PathEscape(strings.TrimSpace(remoteID)), body, &response, true)
	return response.User, err
}

func (client *WorkerClient) request(ctx context.Context, method string, path string, body any, target any, authenticated bool) error {
	return client.requestWithHeaders(ctx, method, path, body, target, nil, authenticated)
}

func (client *WorkerClient) requestWithHeaders(ctx context.Context, method string, path string, body any, target any, headers map[string]string, authenticated bool) error {
	var payload io.Reader
	if body != nil {
		encoded, err := json.Marshal(body)
		if err != nil {
			return err
		}
		payload = bytes.NewReader(encoded)
	}
	req, err := http.NewRequestWithContext(ctx, method, client.baseURL+path, payload)
	if err != nil {
		return err
	}
	req.Header.Set("Accept", "application/json")
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if authenticated {
		if client.token == "" {
			return errors.New("sessao da nuvem ausente")
		}
		req.Header.Set("Authorization", "Bearer "+client.token)
	}
	for key, value := range headers {
		req.Header.Set(key, value)
	}

	response, err := client.http.Do(req)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	data, err := io.ReadAll(io.LimitReader(response.Body, 2<<20))
	if err != nil {
		return err
	}
	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		var apiError struct {
			Code    string `json:"error"`
			Message string `json:"message"`
		}
		_ = json.Unmarshal(data, &apiError)
		return &WorkerAPIError{Status: response.StatusCode, Code: apiError.Code, Message: apiError.Message}
	}
	if target == nil || len(data) == 0 {
		return nil
	}
	return json.Unmarshal(data, target)
}
