package main

import (
	"context"
	"strconv"
	"strings"
	"time"

	"fyne.io/fyne/v2"
)

// Preferencias de acesso. Ficam em fyne.Preferences, ou seja: valem so para
// este aparelho e nunca sao sincronizadas com a nuvem.
const (
	prefRememberLogin = "login_remember"      // bool: preencher o login na proxima vez
	prefSavedLogin    = "login_saved"         // string: o login lembrado
	prefKeepSession   = "login_keep_session"  // bool: entrar direto ao abrir o app
	prefSessionUserID = "session_user_id"     // int: usuario da sessao mantida
	prefSessionUntil  = "session_expires_at"  // string RFC3339: validade da sessao
	prefSessionDevice = "session_device_note" // string: nome do usuario, so para exibir
)

// sessionLifetime e por quanto tempo o app continua entrando sozinho sem pedir
// a senha de novo. Alinhado com a validade do token da nuvem (30 dias).
const sessionLifetime = 30 * 24 * time.Hour

func (ui *mainUI) prefs() fyne.Preferences {
	if ui.app == nil {
		return nil
	}
	return ui.app.Preferences()
}

func (ui *mainUI) rememberLoginEnabled() bool {
	p := ui.prefs()
	if p == nil {
		return false
	}
	return p.BoolWithFallback(prefRememberLogin, false)
}

func (ui *mainUI) savedLogin() string {
	p := ui.prefs()
	if p == nil || !ui.rememberLoginEnabled() {
		return ""
	}
	return strings.TrimSpace(p.String(prefSavedLogin))
}

func (ui *mainUI) keepSessionEnabled() bool {
	p := ui.prefs()
	if p == nil {
		return false
	}
	return p.BoolWithFallback(prefKeepSession, false)
}

// rememberAccess grava as preferencias de acesso depois de um login que deu
// certo. Manter a sessao implica lembrar o login.
func (ui *mainUI) rememberAccess(login string, remember bool, keep bool, user AppUser) {
	p := ui.prefs()
	if p == nil {
		return
	}
	if keep {
		remember = true
	}
	p.SetBool(prefRememberLogin, remember)
	if remember {
		p.SetString(prefSavedLogin, strings.TrimSpace(login))
	} else {
		p.SetString(prefSavedLogin, "")
	}
	p.SetBool(prefKeepSession, keep)
	if keep && user.ID != 0 {
		p.SetString(prefSessionUserID, strconv.FormatInt(user.ID, 10))
		p.SetString(prefSessionUntil, time.Now().Add(sessionLifetime).UTC().Format(time.RFC3339))
		p.SetString(prefSessionDevice, user.Name)
		return
	}
	ui.clearStoredSession()
}

// clearStoredSession apaga so a sessao mantida (o login lembrado continua).
func (ui *mainUI) clearStoredSession() {
	p := ui.prefs()
	if p == nil {
		return
	}
	p.SetBool(prefKeepSession, false)
	p.SetString(prefSessionUserID, "")
	p.SetString(prefSessionUntil, "")
	p.SetString(prefSessionDevice, "")
}

// forgetDevice apaga tudo: sessao mantida e login lembrado.
func (ui *mainUI) forgetDevice() {
	p := ui.prefs()
	if p == nil {
		return
	}
	ui.clearStoredSession()
	p.SetBool(prefRememberLogin, false)
	p.SetString(prefSavedLogin, "")
}

// restoreSession devolve o usuario da sessao mantida, quando ela ainda vale.
// O segundo retorno e um aviso para mostrar na tela de login (sessao expirada,
// usuario desativado...).
func (ui *mainUI) restoreSession() (AppUser, string) {
	p := ui.prefs()
	if p == nil || !ui.keepSessionEnabled() {
		return AppUser{}, ""
	}
	id, err := strconv.ParseInt(strings.TrimSpace(p.String(prefSessionUserID)), 10, 64)
	if err != nil || id == 0 {
		ui.clearStoredSession()
		return AppUser{}, ""
	}
	until, err := time.Parse(time.RFC3339, strings.TrimSpace(p.String(prefSessionUntil)))
	if err != nil || time.Now().After(until) {
		ui.clearStoredSession()
		return AppUser{}, "Sua sessão expirou neste aparelho. Entre novamente."
	}

	ctx := context.Background()
	users, err := ui.store.ListUsers(ctx)
	if err != nil {
		return AppUser{}, ""
	}
	var user AppUser
	for _, candidate := range users {
		if candidate.ID == id {
			user = candidate
			break
		}
	}
	if user.ID == 0 {
		ui.clearStoredSession()
		return AppUser{}, ""
	}
	if !user.Active {
		ui.clearStoredSession()
		return AppUser{}, "Seu acesso foi desativado pelo administrador."
	}

	// Com a nuvem ligada, a sessao so vale enquanto o token dela valer: e o
	// token que autoriza a sincronizacao.
	if _, err := ui.cloudClient(); err == nil {
		if value, found, err := ui.store.GetSetting(ctx, apiTokenExpiry); err == nil && found && strings.TrimSpace(value) != "" {
			if expiry, err := time.Parse(time.RFC3339, strings.TrimSpace(value)); err == nil && time.Now().After(expiry) {
				ui.clearStoredSession()
				return AppUser{}, "Sua sessão na nuvem expirou. Entre novamente para sincronizar."
			}
		}
	}
	return user, ""
}

// accessSummary explica em uma linha o estado do acesso neste aparelho.
func (ui *mainUI) accessSummary() string {
	switch {
	case ui.keepSessionEnabled():
		p := ui.prefs()
		if p != nil {
			if until, err := time.Parse(time.RFC3339, strings.TrimSpace(p.String(prefSessionUntil))); err == nil {
				return "Entrando direto até " + until.Local().Format("02/01/2006") + " sem digitar a senha."
			}
		}
		return "Entrando direto neste aparelho."
	case ui.rememberLoginEnabled():
		return "O login \"" + ui.savedLogin() + "\" vem preenchido; a senha continua sendo pedida."
	default:
		return "Login e senha são pedidos toda vez que o app abre."
	}
}

// startupContent decide a primeira tela: o app ja logado (sessao mantida) ou a
// tela de acesso.
func (ui *mainUI) startupContent() fyne.CanvasObject {
	user, notice := ui.restoreSession()
	ui.loginNotice = notice
	if user.ID == 0 {
		screen := ui.loginScreen()
		// O cursor so pode ser posicionado depois que a janela aparece.
		go func() {
			time.Sleep(400 * time.Millisecond)
			fyne.Do(func() { ui.focusLoginField() })
		}()
		return screen
	}
	ui.applySession(user)
	// As tarefas de rede esperam a janela abrir para nao concorrer com o
	// primeiro desenho.
	go func() {
		time.Sleep(400 * time.Millisecond)
		fyne.Do(func() {
			ui.refreshSyncIndicator()
			ui.autoCheckUpdates()
			ui.syncD1Async(false)
		})
	}()
	return ui.content
}
