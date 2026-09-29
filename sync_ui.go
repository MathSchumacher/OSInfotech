package main

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
)

// syncD1Async sincroniza com a nuvem em segundo plano. manual = disparado pelo
// usuario (mostra resultado); automatico = silencioso, so atualiza o indicador.
//
// A primeira carga de um aparelho novo pode levar minutos, entao: uma unica
// sincronizacao por vez, com andamento no indicador da barra superior.
func (ui *mainUI) syncD1Async(manual bool) {
	if !ui.syncRunning.CompareAndSwap(false, true) {
		if manual {
			ui.notifyTone("A sincronização já está em andamento.", toneInfo)
		}
		return
	}
	if ui.syncPill != nil {
		ui.syncPill.Set(syncWorking, "Sincronizando…")
	}
	go func() {
		defer ui.syncRunning.Store(false)

		ctx := context.Background()
		config, err := ui.store.APIConfig(ctx)
		offline := false
		if err == nil && strings.TrimSpace(config.Token) == "" {
			offline = true
			err = errors.New("faça login na nuvem antes de sincronizar")
		}
		var client *WorkerClient
		if err == nil {
			client, err = NewWorkerClient(config)
		}
		var summary SyncSummary
		if err == nil {
			summary, err = ui.store.SyncD1WithProgress(ctx, client, func(progress SyncSummary) {
				fyne.Do(func() {
					if ui.syncPill != nil {
						ui.syncPill.Set(syncWorking, syncProgressText(progress))
					}
					if ui.homeStatus != nil {
						ui.homeStatus.update(ui)
					}
				})
			})
		}
		if err != nil && !offline {
			_ = ui.store.SetSetting(ctx, syncLastFailureKey, err.Error())
		}

		// A nuvem traz cadastros repetidos de outros aparelhos e da base antiga.
		// O app une o que e certo (mesmo CPF, mesmo nome) sem perguntar e devolve
		// a limpeza para a nuvem na mesma passada.
		// Modelos de checklist repetidos vem da mesma raiz: cada aparelho semeou
		// os padroes antes da primeira leitura.
		_, _ = ui.store.MergeDuplicateChecklistTemplates(ctx)
		// As OS que chegaram da nuvem ainda nao tem modelo e servico
		// reconhecidos. Sao campos derivados, entao cada aparelho preenche os
		// seus sozinho e todos chegam ao mesmo resultado.
		_, _ = ui.store.BackfillOrderKeys(ctx, 800)
		// A senha do aparelho escrita na anotacao sai para o campo proprio, que
		// nunca e impresso. So o que nao deixa duvida; o resto vira aviso na
		// lista de OS para uma pessoa olhar.
		_, _ = ui.store.MoveDevicePasswordsFromNotes(ctx)
		merged := 0
		if count, mergeErr := ui.store.MergeObviousDuplicates(ctx); mergeErr == nil && count > 0 {
			merged = count
			// A limpeza volta para a nuvem na mesma passada; se a sincronizacao
			// falhou, ela fica na fila e sobe na proxima.
			if err == nil && client != nil {
				if extra, pushErr := ui.store.SyncD1WithProgress(ctx, client, nil); pushErr == nil {
					summary.Pushed += extra.Pushed
					summary.Pulled += extra.Pulled
				}
			}
		}
		fyne.Do(func() {
			// A liberacao fica so no defer acima: fyne.Do e assincrono, e soltar a
			// trava aqui deixava duas sincronizacoes rodarem juntas.
			ui.refreshSyncIndicator()
			if err != nil {
				if merged > 0 {
					ui.refreshList()
					ui.refreshClients()
				}
				if manual && offline {
					ui.showCloudActivationDialog()
					return
				}
				if manual {
					ui.notifyTone("Falha ao sincronizar: "+err.Error(), toneDanger)
				}
				return
			}
			ui.refreshList()
			ui.refreshClients()
			ui.refreshTerms()
			if ui.current == pageHome {
				ui.refreshHome()
			}
			switch {
			case merged > 0:
				text := fmt.Sprintf("%d cadastros repetidos unidos automaticamente", merged)
				if merged == 1 {
					text = "1 cadastro repetido unido automaticamente"
				}
				ui.notify(text)
			case manual, summary.Pulled > 0:
				ui.notify(syncDoneText(summary))
			}
		})
	}()
}

// syncProgressText descreve o andamento em uma linha curta.
func syncProgressText(progress SyncSummary) string {
	switch {
	case progress.Pulled > 0 && progress.Pushed > 0:
		return fmt.Sprintf("Sincronizando… %d enviados, %d recebidos", progress.Pushed, progress.Pulled)
	case progress.Pulled > 0:
		return fmt.Sprintf("Sincronizando… %d recebidos", progress.Pulled)
	case progress.Pushed > 0:
		return fmt.Sprintf("Sincronizando… %d enviados", progress.Pushed)
	default:
		return "Sincronizando…"
	}
}

// syncDoneText resume o resultado para o aviso de conclusao.
func syncDoneText(summary SyncSummary) string {
	if summary.Pushed == 0 && summary.Pulled == 0 && summary.Conflicts == 0 && summary.Rejected == 0 {
		return "Nuvem já estava atualizada"
	}
	text := fmt.Sprintf("Nuvem atualizada: %d enviados, %d recebidos", summary.Pushed, summary.Pulled)
	if summary.Conflicts > 0 {
		text += fmt.Sprintf(", %d conflitos resolvidos", summary.Conflicts)
	}
	// A recusa precisa aparecer: a alteracao saiu da fila para nao travar o
	// resto, e o operador tem que saber que aquela OS nao subiu.
	if summary.Rejected > 0 {
		text += fmt.Sprintf(". %d não foram aceitas (%s)", summary.Rejected, summary.RejectedReason)
	}
	return text
}

func (ui *mainUI) showOrganizationNameDialog(currentName string, onSaved func()) {
	if !ui.requireAdmin("editar os dados da empresa") {
		return
	}
	currentName = strings.TrimSpace(currentName)
	if currentName == "" || currentName == "ativa no servidor" || currentName == "indisponível" || currentName == "nenhuma" {
		currentName = ""
	}
	nameEntry := newFormEntry()
	nameEntry.SetText(currentName)
	nameEntry.SetPlaceHolder("Nome da empresa")
	status := widget.NewLabel("")
	status.Importance = widget.LowImportance
	save := primaryButton("Salvar nome", theme.DocumentSaveIcon(), nil)
	save.OnTapped = func() {
		name := strings.TrimSpace(nameEntry.Text)
		if len([]rune(name)) < 2 {
			status.SetText("Informe o nome da empresa.")
			return
		}
		save.Disable()
		status.SetText("Salvando na nuvem…")
		go func() {
			config, err := ui.store.APIConfig(context.Background())
			if err == nil {
				var client *WorkerClient
				client, err = NewWorkerClient(config)
				if err == nil {
					_, err = client.UpdateOrganizationName(context.Background(), name)
				}
			}
			fyne.Do(func() {
				save.Enable()
				if err != nil {
					status.SetText(err.Error())
					return
				}
				status.SetText("Nome da empresa salvo.")
				if onSaved != nil {
					onSaved()
				}
			})
		}()
	}
	d := dialog.NewCustom("Editar empresa", "Fechar", container.NewVBox(
		field("Nome da empresa", nameEntry, ""),
		gap(1, 4),
		save,
		status,
	), ui.window)
	d.Resize(fyne.NewSize(420, 200))
	d.Show()
}

func (ui *mainUI) showCloudActivationDialog() {
	config, err := ui.store.APIConfig(context.Background())
	if err != nil {
		dialog.ShowError(err, ui.window)
		return
	}
	apiURL := newFormEntry()
	apiURL.SetText(config.BaseURL)
	setupToken := newFormPassword()
	if value, found, _ := ui.store.GetSetting(context.Background(), "cloud_setup_token"); found {
		setupToken.SetText(value)
	}
	organization := newFormEntry()
	organization.SetPlaceHolder("Nome da empresa")
	name := newFormEntry()
	name.SetText(ui.currentUser.Name)
	login := newFormEntry()
	login.SetText(ui.currentUser.Login)
	password := newFormPassword()
	password.SetPlaceHolder("Senha do administrador")
	status := widget.NewLabel("")
	status.Importance = widget.LowImportance
	status.Wrapping = fyne.TextWrapWord

	activate := primaryButton("Ativar dados compartilhados", theme.ConfirmIcon(), nil)
	activate.OnTapped = func() {
		activate.Disable()
		status.SetText("Ativando a empresa na nuvem…")
		go func() {
			updated := APIConfig{BaseURL: strings.TrimSpace(apiURL.Text)}
			client, err := NewWorkerClient(updated)
			if err == nil {
				var session remoteSession
				session, err = client.Setup(context.Background(), setupToken.Text, organization.Text, name.Text, login.Text, password.Text)
				if err == nil {
					updated.Token = session.Token
					err = ui.store.SaveAPIConfig(context.Background(), updated)
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
			fyne.Do(func() {
				activate.Enable()
				if err != nil {
					status.SetText(err.Error())
				}
			})
		}()
	}

	form := container.NewVBox(
		field("URL da API", apiURL, ""),
		field("Token de ativação", setupToken, ""),
		field("Empresa", organization, ""),
		field("Responsável", name, ""),
		field("Login", login, ""),
		field("Senha", password, ""),
	)
	d := dialog.NewCustom("Ativar dados compartilhados", "Fechar", container.NewVBox(form, gap(1, 4), activate, status), ui.window)
	d.Resize(fyne.NewSize(480, 520))
	d.Show()
}
