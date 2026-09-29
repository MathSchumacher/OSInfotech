package main

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
)

func (ui *mainUI) bootstrapLoginForm() fyne.CanvasObject {
	organization := newFormEntry()
	organization.SetPlaceHolder("Nome da empresa")
	setupToken := newFormPassword()
	setupToken.SetPlaceHolder("Chave configurada neste computador")
	if token, found, err := ui.store.GetSetting(context.Background(), "cloud_setup_token"); err == nil && found {
		setupToken.SetText(strings.TrimSpace(token))
	}
	name := newFormEntry()
	name.SetPlaceHolder("Nome do responsavel")
	login := newFormEntry()
	login.SetPlaceHolder("Ex.: administrador")
	password := newFormPassword()
	password.SetPlaceHolder("Minimo de 8 caracteres")
	confirmation := newFormPassword()
	confirmation.SetPlaceHolder("Repita a senha")
	status := widget.NewLabel("")
	status.Importance = widget.LowImportance
	submit := primaryButton("Ativar empresa", theme.AccountIcon(), nil)
	submit.OnTapped = func() {
		if strings.TrimSpace(setupToken.Text) == "" {
			status.SetText("Este computador nao possui uma chave de ativacao.")
			return
		}
		if password.Text != confirmation.Text {
			status.SetText("As senhas nao conferem.")
			return
		}
		submit.Disable()
		status.SetText("Ativando empresa...")
		go func() {
			config, err := ui.store.APIConfig(context.Background())
			if err == nil {
				var client *WorkerClient
				client, err = NewWorkerClient(config)
				if err == nil {
					var session remoteSession
					session, err = client.Setup(context.Background(), setupToken.Text, organization.Text, name.Text, login.Text, password.Text)
					if err == nil {
						config.Token = session.Token
						err = ui.store.SaveAPIConfig(context.Background(), config)
						if err == nil {
							_ = ui.store.SetSetting(context.Background(), apiTokenExpiry, session.ExpiresAt)
							_ = ui.store.SetSetting(context.Background(), "cloud_setup_token", "")
							var user AppUser
							user, err = ui.store.UpsertRemoteUser(context.Background(), session.User, password.Text)
							if err == nil {
								fyne.Do(func() { ui.completeLogin(user) })
								return
							}
						}
					}
				}
			}
			fyne.Do(func() {
				submit.Enable()
				if err != nil {
					status.SetText(err.Error())
				}
			})
		}()
	}
	return container.NewVBox(
		loginField("Empresa", organization),
		loginField("Token de ativacao", setupToken),
		loginField("Nome", name),
		loginField("Login", login),
		loginField("Senha", password),
		loginField("Confirmar", confirmation),
		gap(1, 4),
		loginSubmitRow(submit),
		status,
	)
}

func (ui *mainUI) hasLocalSetupToken() bool {
	token, found, err := ui.store.GetSetting(context.Background(), "cloud_setup_token")
	return err == nil && found && strings.TrimSpace(token) != ""
}

func (ui *mainUI) activationPendingForm() fyne.CanvasObject {
	message := widget.NewLabel("A empresa ainda nao foi ativada no servidor.")
	message.Wrapping = fyne.TextWrapWord
	message.Importance = widget.LowImportance
	detail := widget.NewLabel("Conclua a ativacao no computador administrador. Em seguida, este aparelho mostrara somente o login da equipe.")
	detail.Wrapping = fyne.TextWrapWord
	detail.Importance = widget.LowImportance
	return container.NewVBox(message, detail)
}

// applySession coloca o usuario na interface (sem trocar o conteudo da
// janela). Usado tanto pelo login quanto pela sessao restaurada no arranque.
func (ui *mainUI) applySession(user AppUser) {
	ui.currentUser = user
	ui.updateSessionLabel()
	ui.clearForm()
	ui.applyRoleAccess()
	ui.showPage(pageHome)
}

func (ui *mainUI) completeLogin(user AppUser) {
	ui.applySession(user)
	if ui.window != nil {
		ui.window.SetContent(ui.content)
	}
	ui.refreshSyncIndicator()
	ui.autoCheckUpdates()
	ui.syncD1Async(false)
}

// lockSession volta para a tela de acesso e cancela a sessao mantida (o login
// lembrado continua preenchido).
func (ui *mainUI) lockSession() {
	ui.clearStoredSession()
	ui.currentUser = AppUser{}
	ui.updateSessionLabel()
	ui.applyRoleAccess()
	if ui.window != nil {
		ui.window.SetContent(ui.loginScreen())
		ui.focusLoginField()
	}
}

// focusLoginField posiciona o cursor: na senha quando o login ja veio
// preenchido, no login caso contrario.
func (ui *mainUI) focusLoginField() {
	if ui.window == nil || ui.currentUser.ID != 0 || ui.loginEntry == nil {
		return
	}
	if strings.TrimSpace(ui.loginEntry.Text) != "" && ui.passwordEntry != nil {
		ui.window.Canvas().Focus(ui.passwordEntry)
		return
	}
	ui.window.Canvas().Focus(ui.loginEntry)
}

func (ui *mainUI) requireAdmin(action string) bool {
	if ui.currentUser.IsAdmin() {
		return true
	}
	dialog.ShowInformation("Acesso restrito", "Somente o administrador pode "+strings.TrimSpace(action)+".", ui.window)
	return false
}

func (ui *mainUI) showUserEditor(user AppUser) {
	if !ui.requireAdmin("editar usuarios") {
		return
	}
	isNew := user.ID == 0
	previousRole := user.Role
	name := newFormEntry()
	name.SetText(user.Name)
	login := newFormEntry()
	login.SetText(user.Login)
	password := newFormPassword()
	if isNew {
		password.SetPlaceHolder("Minimo de 8 caracteres")
	} else {
		password.SetPlaceHolder("Deixe vazio para manter a senha")
	}
	role := widget.NewSelect([]string{roleAdministrator, roleTechnician}, nil)
	role.SetSelected(user.Role)
	if role.Selected == "" {
		role.SetSelected(roleTechnician)
	}
	if isNew {
		role.SetSelected(roleTechnician)
		role.Disable()
	}
	active := widget.NewCheck("Usuario ativo", nil)
	active.SetChecked(user.Active)

	title := "Editar usuario"
	if isNew {
		title = "Novo tecnico"
	}
	dialog.NewForm(title, "Salvar", "Cancelar", []*widget.FormItem{
		widget.NewFormItem("Nome", name),
		widget.NewFormItem("Login", login),
		widget.NewFormItem("Senha", password),
		widget.NewFormItem("Perfil", role),
		widget.NewFormItem("Status", active),
	}, func(ok bool) {
		if !ok {
			return
		}
		user.Name = name.Text
		user.Login = login.Text
		user.Role = role.Selected
		user.Active = active.Checked
		if err := ui.saveManagedUser(context.Background(), &user, password.Text, isNew, previousRole); err != nil {
			dialog.ShowError(err, ui.window)
			return
		}
		if user.ID == ui.currentUser.ID {
			if !user.Active {
				ui.lockSession()
				return
			}
			ui.currentUser = user
			ui.updateSessionLabel()
			ui.applyRoleAccess()
		}
		ui.notify("Usuário salvo")
		if ui.settingsPage != nil {
			ui.settingsPage.refresh()
		}
	}, ui.window).Show()
}

func (ui *mainUI) refreshUsersFromCloud(ctx context.Context) error {
	config, err := ui.store.APIConfig(ctx)
	if err != nil || strings.TrimSpace(config.Token) == "" {
		return err
	}
	client, err := NewWorkerClient(config)
	if err != nil {
		return err
	}
	users, err := client.ListUsers(ctx)
	if err != nil {
		return err
	}
	for _, remote := range users {
		if _, err := ui.store.UpsertRemoteUserProfile(ctx, remote); err != nil {
			return err
		}
	}
	return nil
}

func remoteRoleFromLocal(role string) (string, error) {
	switch role {
	case roleAdministrator:
		return "admin", nil
	case roleTechnician:
		return "technician", nil
	default:
		return "", errors.New("perfil local invalido")
	}
}

func (ui *mainUI) saveManagedUser(ctx context.Context, user *AppUser, password string, isNew bool, previousRole string) error {
	config, err := ui.store.APIConfig(ctx)
	if err != nil || strings.TrimSpace(config.Token) == "" {
		return ui.store.SaveUserAsAdministrator(ctx, ui.currentUser, user, password)
	}
	client, err := NewWorkerClient(config)
	if err != nil {
		return err
	}
	if isNew {
		remote, err := client.CreateTechnician(ctx, user.Name, user.Login, password)
		if err != nil {
			return err
		}
		local, err := ui.store.UpsertRemoteUser(ctx, remote, password)
		if err != nil {
			return err
		}
		*user = local
		return ui.store.LogAuditEvent(ctx, ui.currentUser, "usuario.criado", "usuario", user.ID, user.Name, "Tecnico")
	}
	mapping, found, err := ui.store.existingSyncMapping(ctx, "user", strconv.FormatInt(user.ID, 10))
	if err != nil {
		return err
	}
	if !found {
		return errors.New("este usuario local ainda nao foi vinculado a nuvem; crie um novo tecnico compartilhado")
	}
	remoteRole, err := remoteRoleFromLocal(user.Role)
	if err != nil {
		return err
	}
	// A senha vai junto. Antes so papel e situacao eram enviados, e o app dizia
	// "Usuario salvo" sem trocar senha nenhuma, nem aqui nem na nuvem.
	if strings.TrimSpace(password) != "" {
		if err := validateNewPassword(password); err != nil {
			return err
		}
	}
	remote, err := client.UpdateUserAccess(ctx, mapping.RemoteID, remoteRole, user.Active, password)
	if err != nil {
		return err
	}
	local, err := ui.store.UpsertRemoteUserProfile(ctx, remote)
	if err != nil {
		return err
	}
	// O hash local tambem: e ele que vale quando a loja fica sem internet.
	if strings.TrimSpace(password) != "" {
		if err := ui.store.SetUserPassword(ctx, local.ID, password); err != nil {
			return err
		}
		if err := ui.store.LogAuditEvent(ctx, ui.currentUser, "usuario.senha_trocada", "usuario", local.ID, local.Name, ""); err != nil {
			return err
		}
	}
	*user = local
	action := "usuario.atualizado"
	if previousRole != user.Role {
		if user.IsAdmin() {
			action = "usuario.promovido_administrador"
		} else {
			action = "usuario.rebaixado_tecnico"
		}
	}
	return ui.store.LogAuditEvent(ctx, ui.currentUser, action, "usuario", user.ID, user.Name, user.DisplayRole())
}

// loginForm decide qual tela de acesso mostrar. A nuvem so precisa responder
// aqui para saber se a empresa ainda nao foi ativada; o login em si tenta a
// nuvem na hora de entrar (performLogin), mesmo que esta checagem falhe.
func (ui *mainUI) loginForm() (fyne.CanvasObject, error) {
	count, err := ui.store.UserCount(context.Background())
	if err != nil {
		return nil, err
	}
	if client, err := ui.cloudClient(); err == nil && count == 0 {
		ctx, cancel := context.WithTimeout(context.Background(), 6*time.Second)
		initialized, statusErr := client.SetupStatus(ctx)
		cancel()
		if statusErr == nil && !initialized {
			if !isMobileLayout() && ui.hasLocalSetupToken() {
				return ui.bootstrapLoginForm(), nil
			}
			return ui.activationPendingForm(), nil
		}
	}
	return ui.existingLoginForm(), nil
}

// cloudClient devolve o cliente da nuvem, ou erro quando ela esta desligada
// ("disabled") ou mal configurada neste aparelho.
func (ui *mainUI) cloudClient() (*WorkerClient, error) {
	config, err := ui.store.APIConfig(context.Background())
	if err != nil {
		return nil, err
	}
	return NewWorkerClient(config)
}

// loginOutcome descreve por onde o acesso aconteceu.
type loginOutcome struct {
	User    AppUser
	Cloud   bool   // true = autenticado na nuvem (sessao salva)
	Warning string // aviso nao fatal (ex.: entrou no modo local por falta de conexao)
}

// performLogin tenta a nuvem sempre que ela estiver configurada. Se a nuvem
// responder (mesmo negando), a resposta dela vale. Se nao houver conexao, cai
// para o usuario local quando ele existir neste aparelho.
func (ui *mainUI) performLogin(ctx context.Context, login string, password string) (loginOutcome, error) {
	login = strings.TrimSpace(login)
	if login == "" || password == "" {
		return loginOutcome{}, errors.New("informe login e senha")
	}
	client, cloudErr := ui.cloudClient()
	if cloudErr != nil {
		user, err := ui.store.Authenticate(ctx, login, password)
		if err != nil {
			return loginOutcome{}, err
		}
		return loginOutcome{User: user}, nil
	}

	remoteCtx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	session, err := client.Login(remoteCtx, login, password)
	if err == nil {
		config, cfgErr := ui.store.APIConfig(ctx)
		if cfgErr != nil {
			return loginOutcome{}, cfgErr
		}
		config.Token = session.Token
		if err := ui.store.SaveAPIConfig(ctx, config); err != nil {
			return loginOutcome{}, err
		}
		_ = ui.store.SetSetting(ctx, apiTokenExpiry, session.ExpiresAt)
		user, err := ui.store.UpsertRemoteUser(ctx, session.User, password)
		if err != nil {
			return loginOutcome{}, err
		}
		return loginOutcome{User: user, Cloud: true}, nil
	}

	var apiErr *WorkerAPIError
	if errors.As(err, &apiErr) {
		// A nuvem respondeu: credenciais erradas ou usuario inativo.
		return loginOutcome{}, err
	}
	// Sem resposta da nuvem: tenta o usuario local, se existir.
	user, localErr := ui.store.Authenticate(ctx, login, password)
	if localErr != nil {
		return loginOutcome{}, fmt.Errorf("sem conexão com a nuvem (%v). Verifique a internet e tente de novo", shortNetworkError(err))
	}
	return loginOutcome{User: user, Warning: "Sem conexão com a nuvem: entrou no modo local, sem sincronizar."}, nil
}

// shortNetworkError encurta erros de rede para caber na tela de login.
func shortNetworkError(err error) string {
	msg := err.Error()
	if len(msg) > 90 {
		msg = msg[:90] + "…"
	}
	return msg
}

// cloudModeHint descreve, na tela de login, como o app esta configurado.
func (ui *mainUI) cloudModeHint() string {
	config, err := ui.store.APIConfig(context.Background())
	if err != nil {
		return "Nuvem desligada neste aparelho: acesso com usuário local."
	}
	if _, err := NewWorkerClient(config); err != nil {
		return "Nuvem desligada neste aparelho: acesso com usuário local."
	}
	host := strings.TrimPrefix(strings.TrimPrefix(config.BaseURL, "https://"), "http://")
	return "Conectado à nuvem DBS (" + host + ")"
}

func (ui *mainUI) existingLoginForm() fyne.CanvasObject {
	login := newFormEntry()
	login.SetPlaceHolder("Seu login")
	login.SetText(ui.savedLogin())
	password := newFormPassword()
	password.SetPlaceHolder("Sua senha")
	status := widget.NewLabel("")
	status.Importance = widget.LowImportance
	status.Wrapping = fyne.TextWrapWord
	if ui.loginNotice != "" {
		status.SetText(ui.loginNotice)
		ui.loginNotice = ""
	}
	mode := mutedLabel(ui.cloudModeHint())

	// Lembrar o login preenche o campo na proxima vez; manter conectado entra
	// direto ao abrir o app (e implica lembrar o login).
	remember := widget.NewCheck("Lembrar meu login neste aparelho", nil)
	remember.SetChecked(ui.rememberLoginEnabled())
	keep := widget.NewCheck("Manter conectado", nil)
	keep.SetChecked(ui.keepSessionEnabled())
	remember.OnChanged = func(on bool) {
		if !on && keep.Checked {
			keep.SetChecked(false)
		}
	}
	keep.OnChanged = func(on bool) {
		if on && !remember.Checked {
			remember.SetChecked(true)
		}
	}
	keepHint := newText("Use apenas em computadores e celulares da loja.", sizeCaption, false, inkMuted)
	keepHint.SetTruncate(true)

	submit := primaryButton("Entrar", theme.LoginIcon(), nil)
	submit.OnTapped = func() {
		submit.Disable()
		status.SetText("Entrando…")
		typedLogin := login.Text
		// A senha e lida aqui, na thread da interface: ler o campo dentro da
		// goroutine podia pegar o texto pela metade se o usuario continuasse
		// digitando durante o login.
		typedPassword := password.Text
		go func() {
			outcome, err := ui.performLogin(context.Background(), typedLogin, typedPassword)
			fyne.Do(func() {
				if err != nil {
					submit.Enable()
					status.SetText(err.Error())
					return
				}
				ui.rememberAccess(typedLogin, remember.Checked, keep.Checked, outcome.User)
				ui.completeLogin(outcome.User)
				if outcome.Warning != "" {
					ui.notifyTone(outcome.Warning, toneWarning)
				}
			})
		}()
	}
	login.OnSubmitted = func(string) { ui.window.Canvas().Focus(password) }
	password.OnSubmitted = func(string) { submit.OnTapped() }
	ui.loginEntry = login
	ui.passwordEntry = password
	return container.NewVBox(
		loginField("Login", login),
		loginField("Senha", password),
		gap(1, 6),
		remember,
		keep,
		keepHint,
		gap(1, 6),
		loginSubmitRow(submit),
		status,
		gap(1, 4),
		mode,
	)
}
