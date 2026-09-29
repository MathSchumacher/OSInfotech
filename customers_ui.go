package main

import (
	"context"
	"errors"
	"fmt"
	"image/color"
	"strings"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/theme"
)

// Telas de identidade do cliente: aviso de CPF/telefone repetido, renomear sem
// duplicar e unificacao dos cadastros que ja nasceram repetidos.

// collectCustomerForm monta o cadastro a partir do formulario.
func (ui *mainUI) collectCustomerForm() (CustomerProfile, error) {
	name := strings.TrimSpace(ui.clientNameEntry.Text)
	if name == "" {
		return CustomerProfile{}, errors.New("informe o nome do cliente")
	}
	return CustomerProfile{
		Key:       normalizeCustomerName(name),
		Name:      name,
		CPF:       strings.TrimSpace(ui.clientCPFEntry.Text),
		Phone:     strings.TrimSpace(ui.clientPhoneEntry.Text),
		AvatarURL: ui.clientAvatarURL,
		AvatarKey: ui.clientAvatarKey,
	}, nil
}

func (ui *mainUI) saveCustomer() {
	ctx := context.Background()
	profile, err := ui.collectCustomerForm()
	if err != nil {
		ui.notifyTone(err.Error(), toneDanger)
		return
	}

	// Um CPF (ou telefone) so pode pertencer a um cadastro.
	conflict, found, err := ui.store.FindCustomerConflict(ctx, ui.currentClientKey, profile.CPF, profile.Phone)
	if err != nil {
		dialog.ShowError(err, ui.window)
		return
	}
	if found {
		ui.showCustomerConflict(conflict, profile)
		return
	}

	renamed := ui.currentClientKey != "" && ui.currentClientKey != profile.Key
	if renamed {
		previousKey := ui.currentClientKey
		if err := ui.store.RenameCustomerProfile(ctx, previousKey, profile); err != nil {
			if errors.Is(err, ErrCustomerNameTaken) {
				ui.askMergeIntoExisting(previousKey, profile)
				return
			}
			dialog.ShowError(err, ui.window)
			return
		}
	} else if err := ui.store.SaveCustomerProfile(ctx, profile); err != nil {
		dialog.ShowError(err, ui.window)
		return
	}

	ui.currentClientKey = profile.Key
	ui.clientDetailTitle.SetText(profile.Name)
	ui.clientDetailMeta.SetText("Cliente salvo.")
	ui.refreshClients()
	ui.loadCustomerHistory(profile.Key)
	ui.refreshList()
	ui.syncD1Async(false)
	if renamed {
		ui.notify("Cliente renomeado; as OS antigas continuam com ele")
	} else {
		ui.notify("Cliente salvo")
	}
}

// showCustomerConflict explica que o documento ja esta em uso e deixa o
// operador decidir: abrir o cadastro existente ou unificar os dois.
func (ui *mainUI) showCustomerConflict(conflict customerConflict, attempted CustomerProfile) {
	title := "CPF já cadastrado"
	if conflict.Field == "telefone" {
		title = "Telefone já cadastrado"
	}
	message := fmt.Sprintf("O %s %s já pertence ao cliente %q. Cada cliente precisa de um %s próprio.",
		conflict.Field, firstNonEmpty(conflict.Value, "informado"), conflict.Profile.Name, conflict.Field)

	var conflictDialog dialog.Dialog
	open := primaryButton("Abrir "+conflict.Profile.Name, theme.AccountIcon(), func() {
		conflictDialog.Hide()
		ui.openCustomer(conflict.Profile)
	})
	actions := []fyne.CanvasObject{open}
	if ui.currentClientKey != "" && ui.currentClientKey != conflict.Profile.Key {
		merge := secondaryButton("Unificar os dois", theme.ContentAddIcon(), func() {
			conflictDialog.Hide()
			ui.confirmMerge(conflict.Profile.Key, ui.currentClientKey, attempted.Name)
		})
		actions = append(actions, merge)
	}
	content := container.NewVBox(mutedLabel(message), gap(1, 6), container.New(newColumnsLayout(8, 1, 1), actions...))
	conflictDialog = dialog.NewCustom(title, "Cancelar", content, ui.window)
	conflictDialog.Resize(fyne.NewSize(480, 220))
	conflictDialog.Show()
}

// askMergeIntoExisting aparece quando o nome novo ja e de outro cadastro.
func (ui *mainUI) askMergeIntoExisting(currentKey string, profile CustomerProfile) {
	message := fmt.Sprintf("Já existe um cliente chamado %q. Quer juntar os dois cadastros? As OS dos dois ficam no mesmo cliente.", profile.Name)
	dialog.NewCustomConfirm("Nome já usado", "Unificar", "Cancelar", mutedLabel(message), func(ok bool) {
		if !ok {
			return
		}
		ui.confirmMerge(profile.Key, currentKey, profile.Name)
	}, ui.window).Show()
}

// confirmMerge junta keepKey (que fica) com mergeKey (que desaparece).
func (ui *mainUI) confirmMerge(keepKey string, mergeKey string, keepName string) {
	ctx := context.Background()
	if err := ui.store.MergeCustomerProfiles(ctx, keepKey, mergeKey); err != nil {
		dialog.ShowError(err, ui.window)
		return
	}
	ui.currentClientKey = keepKey
	ui.refreshClients()
	ui.refreshList()
	ui.syncD1Async(false)
	ui.notify("Cadastros unificados em " + keepName)
	if profile, found, err := ui.store.customerProfileByKey(ctx, keepKey); err == nil && found {
		ui.openCustomer(profile)
	}
}

// refreshDuplicateWarning mostra (ou esconde) o aviso de cadastros repetidos na
// lista de clientes.
func (ui *mainUI) refreshDuplicateWarning() {
	if ui.clientsDuplicateBox == nil {
		return
	}
	go func() {
		groups, err := ui.store.DuplicateCustomerGroups(context.Background())
		if err != nil {
			return
		}
		fyne.Do(func() {
			if ui.clientsDuplicateBox == nil {
				return
			}
			ui.clientsDuplicateBox.Objects = nil
			if len(groups) == 0 {
				ui.clientsDuplicateBox.Hide()
				ui.clientsDuplicateBox.Refresh()
				return
			}
			text := fmt.Sprintf("%d clientes parecem repetidos (mesmo CPF ou telefone).", countDuplicateProfiles(groups))
			if len(groups) == 1 && len(groups[0].Profiles) == 2 {
				text = fmt.Sprintf("%q e %q têm o mesmo %s.", groups[0].Profiles[0].Name, groups[0].Profiles[1].Name, groups[0].Field)
			}
			label := newText(text, sizeBody, false, inkPrimary)
			label.SetTruncate(true)
			review := secondaryButton("Revisar", theme.SearchIcon(), func() { ui.showDuplicateCustomers() })
			body := container.NewBorder(nil, nil, container.NewCenter(newToneIcon(theme.WarningIcon(), toneWarning, 20)),
				container.NewCenter(review), paddedTRBL(label, 0, 8, 0, 10))
			ui.clientsDuplicateBox.Add(newGlassPanel(body, 12, tintGlass(toneWarning)))
			ui.clientsDuplicateBox.Add(gap(1, 10))
			ui.clientsDuplicateBox.Show()
			ui.clientsDuplicateBox.Refresh()
		})
	}()
}

func countDuplicateProfiles(groups []duplicateGroup) int {
	total := 0
	for _, group := range groups {
		total += len(group.Profiles)
	}
	return total
}

// showDuplicateCustomers lista os grupos repetidos e deixa o operador escolher
// qual cadastro fica. As unioes certas (mesmo CPF) o app ja faz sozinho depois
// de cada sincronizacao; aqui sobra o que depende de decisao humana.
func (ui *mainUI) showDuplicateCustomers() {
	ctx := context.Background()
	groups, err := ui.store.DuplicateCustomerGroups(ctx)
	if err != nil {
		dialog.ShowError(err, ui.window)
		return
	}
	body := container.NewVBox()
	var duplicatesDialog dialog.Dialog
	if len(groups) == 0 {
		body.Add(mutedLabel("Nenhum cliente repetido. Tudo certo."))
	}
	for _, group := range groups {
		current := group
		head := container.NewHBox(newEyebrow("Mesmo "+current.Field), gap(6, 1), newText(current.Value, sizeCaption, true, inkSecondary))
		apart := ghostButton("Não são a mesma pessoa", theme.CancelIcon(), func() {
			duplicatesDialog.Hide()
			ui.markCustomersApart(current)
		})
		rows := container.NewVBox(container.NewBorder(nil, nil, nil, container.NewCenter(apart), head), gap(1, 4))
		for _, item := range current.Profiles {
			profile := item
			others := make([]string, 0, len(current.Profiles)-1)
			for _, other := range current.Profiles {
				if other.Key != profile.Key {
					others = append(others, other.Key)
				}
			}
			name := newText(profile.Name, sizeBody, true, inkPrimary)
			name.SetTruncate(true)
			meta := newText(customerMetaLine(profile), sizeCaption, false, inkSecondary)
			meta.SetTruncate(true)
			keep := ghostButton("Manter este", theme.ConfirmIcon(), func() {
				duplicatesDialog.Hide()
				ui.mergeGroupInto(profile, others)
			})
			row := container.NewBorder(nil, nil, nil, container.NewCenter(keep),
				paddedTRBL(container.NewVBox(name, meta), 0, 8, 0, 0))
			rows.Add(newGlassPanel(row, 10, sunkenGlass))
			rows.Add(gap(1, 4))
		}
		body.Add(card(rows))
		body.Add(gap(1, 8))
	}
	content := container.NewVScroll(body)
	content.SetMinSize(fyne.NewSize(560, 380))
	duplicatesDialog = dialog.NewCustom("Clientes repetidos", "Fechar", content, ui.window)
	duplicatesDialog.Resize(fyne.NewSize(620, 480))
	duplicatesDialog.Show()
}

// markCustomersApart guarda que os cadastros do grupo sao pessoas diferentes
// que dividem o telefone, para o aviso parar de apontar os dois.
func (ui *mainUI) markCustomersApart(group duplicateGroup) {
	keys := make([]string, 0, len(group.Profiles))
	for _, profile := range group.Profiles {
		keys = append(keys, profile.Key)
	}
	if err := ui.store.MarkCustomersDistinct(context.Background(), keys); err != nil {
		dialog.ShowError(err, ui.window)
		return
	}
	ui.refreshClients()
	ui.notify("Anotado: sao clientes diferentes")
}

// mergeGroupInto junta todos os cadastros do grupo no escolhido.
func (ui *mainUI) mergeGroupInto(keep CustomerProfile, others []string) {
	message := fmt.Sprintf("Manter %q e juntar %d cadastro(s) nele? As OS de todos passam para esse cliente.", keep.Name, len(others))
	dialog.ShowConfirm("Unificar clientes", message, func(ok bool) {
		if !ok {
			return
		}
		ctx := context.Background()
		for _, other := range others {
			if err := ui.store.MergeCustomerProfiles(ctx, keep.Key, other); err != nil {
				dialog.ShowError(err, ui.window)
				return
			}
		}
		ui.refreshClients()
		ui.refreshList()
		ui.syncD1Async(false)
		ui.notify("Cadastros unificados em " + keep.Name)
	}, ui.window)
}

// warnOrderCustomerConflict avisa, ao salvar a OS, que o CPF digitado pertence
// a outro cliente. Devolve true quando o salvamento deve esperar a decisao.
func (ui *mainUI) warnOrderCustomerConflict(order ServiceOrder, onContinue func()) bool {
	if documentKey(order.CPF) == "" {
		return false
	}
	key := normalizeCustomerName(order.Customer)
	conflict, found, err := ui.store.FindCustomerConflict(context.Background(), key, order.CPF, "")
	if err != nil || !found {
		return false
	}
	if normalizeCustomerName(conflict.Profile.Name) == key {
		return false
	}
	message := fmt.Sprintf("O CPF %s está cadastrado para %q, e não para %q. Usar o cliente já cadastrado?",
		firstNonEmpty(order.CPF, "informado"), conflict.Profile.Name, order.Customer)
	var conflictDialog dialog.Dialog
	use := primaryButton("Usar "+conflict.Profile.Name, theme.AccountIcon(), func() {
		conflictDialog.Hide()
		ui.suppressCustomerLookup = true
		ui.nameEntry.SetText(conflict.Profile.Name)
		ui.suppressCustomerLookup = false
		ui.customerPicker.SetSelected(conflict.Profile, "Cliente reconhecido pelo CPF")
		onContinue()
	})
	keepTyped := ghostButton("Manter "+order.Customer, theme.DocumentIcon(), func() {
		conflictDialog.Hide()
		onContinue()
	})
	content := container.NewVBox(mutedLabel(message), gap(1, 6),
		container.New(newColumnsLayout(8, 1, 1), use, keepTyped))
	conflictDialog = dialog.NewCustom("CPF de outro cliente", "Cancelar", content, ui.window)
	conflictDialog.Resize(fyne.NewSize(520, 220))
	conflictDialog.Show()
	return true
}

// glassWarnStyle deixa o aviso de duplicados com a cor de alerta.
var _ = func() color.Color { return nil }
