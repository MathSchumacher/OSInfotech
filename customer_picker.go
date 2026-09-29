package main

import (
	"context"
	"fmt"
	"image/color"
	"strings"
	"sync/atomic"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/layout"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
)

// customerPicker e a busca rapida de cliente dentro de um formulario: digita-se
// qualquer dado (nome, CPF, telefone, aparelho) e as sugestoes aparecem logo
// abaixo; ao escolher, o formulario e preenchido e a faixa "cliente
// selecionado" toma o lugar da busca.
type customerPicker struct {
	ui      *mainUI
	entry   *dbsEntry
	results *fyne.Container
	strip   fyne.CanvasObject
	search  fyne.CanvasObject
	root    *fyne.Container
	seq     atomic.Uint64

	selected *CustomerProfile
	selAv    *avatar
	selName  *themedText
	selMeta  *themedText
	selNote  *themedText

	onPick  func(CustomerProfile)
	onClear func()
}

func newCustomerPicker(ui *mainUI, placeholder string, onPick func(CustomerProfile), onClear func()) *customerPicker {
	p := &customerPicker{ui: ui, onPick: onPick, onClear: onClear}
	p.entry = newFormEntry()
	p.entry.SetPlaceHolder(placeholder)
	p.entry.ActionItem = widget.NewIcon(theme.SearchIcon())
	p.entry.OnChanged = func(text string) { p.lookup(text) }
	p.results = container.NewVBox()
	p.results.Hide()

	p.selAv = newAvatar(40)
	p.selName = newText("", sizeSection, true, inkPrimary).SetTruncate(true)
	p.selMeta = newText("", sizeCaption, false, inkSecondary).SetTruncate(true)
	p.selNote = newText("", sizeCaption, false, inkSuccess).SetTruncate(true)
	change := ghostButton("Trocar", theme.ViewRefreshIcon(), func() { p.Clear() })
	open := ghostButton("Ver cliente", theme.AccountIcon(), func() {
		if p.selected != nil {
			ui.openCustomer(*p.selected)
		}
	})
	texts := paddedTRBL(container.NewVBox(p.selName, p.selMeta, p.selNote), 0, 0, 0, 10)
	var stripBody fyne.CanvasObject
	if isMobileLayout() {
		stripBody = container.NewVBox(
			container.NewBorder(nil, nil, container.NewCenter(p.selAv), nil, texts),
			container.NewHBox(layout.NewSpacer(), open, change),
		)
	} else {
		stripBody = container.NewBorder(nil, nil, container.NewCenter(p.selAv), container.NewCenter(container.NewHBox(open, change)), texts)
	}
	p.strip = tintedCard(stripBody, 12, toneSuccess)
	p.strip.Hide()

	p.search = container.NewVBox(p.entry, p.results)
	p.root = container.NewVBox(p.search, p.strip)
	return p
}

func (p *customerPicker) Object() fyne.CanvasObject { return p.root }

// Clear volta ao estado de busca (novo cliente ou trocar).
func (p *customerPicker) Clear() {
	p.selected = nil
	p.seq.Add(1)
	p.entry.SetText("")
	p.results.Objects = nil
	p.results.Hide()
	p.strip.Hide()
	p.search.Show()
	p.root.Refresh()
	if p.onClear != nil {
		p.onClear()
	}
}

// Reset limpa sem disparar onClear (usado ao abrir/limpar o formulario).
func (p *customerPicker) Reset() {
	p.selected = nil
	p.seq.Add(1)
	p.entry.SetText("")
	p.results.Objects = nil
	p.results.Hide()
	p.strip.Hide()
	p.search.Show()
	p.root.Refresh()
}

// SetSelected mostra a faixa do cliente escolhido. note e opcional
// ("Reconhecido pelo CPF", por exemplo).
func (p *customerPicker) SetSelected(profile CustomerProfile, note string) {
	copyProfile := profile
	p.selected = &copyProfile
	p.seq.Add(1)
	p.selAv.SetName(profile.Name)
	if profile.AvatarURL != "" {
		p.selAv.SetPhotoURL(profile.AvatarURL)
	}
	p.selName.SetText(profile.Name)
	p.selMeta.SetText(customerMetaLine(profile))
	p.selNote.SetText(note)
	if note == "" {
		p.selNote.Hide()
	} else {
		p.selNote.Show()
	}
	p.results.Objects = nil
	p.results.Hide()
	p.search.Hide()
	p.strip.Show()
	p.root.Refresh()
}

func customerMetaLine(profile CustomerProfile) string {
	parts := make([]string, 0, 3)
	if profile.CPF != "" {
		parts = append(parts, "CPF "+profile.CPF)
	}
	if profile.Phone != "" {
		parts = append(parts, profile.Phone)
	}
	if profile.Orders > 0 {
		parts = append(parts, fmt.Sprintf("%d OS", profile.Orders))
	}
	if len(parts) == 0 {
		return "Sem CPF ou telefone cadastrado"
	}
	return strings.Join(parts, " · ")
}

// lookup busca no banco a partir de 2 letras ou 3 digitos.
func (p *customerPicker) lookup(text string) {
	query := strings.TrimSpace(text)
	seq := p.seq.Add(1)
	if len([]rune(query)) < 2 {
		p.showResults(nil, "")
		return
	}
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		profiles, err := p.ui.store.FindCustomerProfiles(ctx, query, 6)
		if err != nil || seq != p.seq.Load() {
			return
		}
		fyne.Do(func() {
			if seq != p.seq.Load() || strings.TrimSpace(p.entry.Text) != query {
				return
			}
			p.showResults(profiles, query)
		})
	}()
}

func (p *customerPicker) showResults(profiles []CustomerProfile, query string) {
	p.results.Objects = nil
	if query == "" {
		p.results.Hide()
		p.root.Refresh()
		return
	}
	if len(profiles) == 0 {
		hint := newText("Nenhum cliente com esse dado. Preencha os campos abaixo para cadastrar.", sizeCaption, false, inkMuted)
		p.results.Add(paddedTRBL(hint, 6, 4, 2, 4))
	}
	for _, profile := range profiles {
		p.results.Add(p.resultRow(profile))
	}
	p.results.Show()
	p.root.Refresh()
}

func (p *customerPicker) resultRow(profile CustomerProfile) fyne.CanvasObject {
	av := newAvatar(32)
	av.SetName(profile.Name)
	if profile.AvatarURL != "" {
		av.SetPhotoURL(profile.AvatarURL)
	}
	name := newText(profile.Name, sizeBody, true, inkPrimary).SetTruncate(true)
	meta := newText(customerMetaLine(profile), sizeCaption, false, inkSecondary).SetTruncate(true)
	var right fyne.CanvasObject
	if profile.LastDevice != "" && !isMobileLayout() {
		right = container.NewCenter(newText(strings.TrimSpace(profile.LastDevice+" "+profile.LastModel), sizeCaption, false, inkMuted))
	}
	body := container.NewBorder(nil, nil, container.NewCenter(av), right,
		paddedTRBL(container.NewVBox(name, meta), 0, 0, 0, 10))
	row := newTapCard(body, radiusMd, 8, func() {
		p.SetSelected(profile, "")
		if p.onPick != nil {
			p.onPick(profile)
		}
	})
	row.paint = func(t designTokens, hovered bool) (color.Color, color.Color) {
		if hovered {
			return t.Hover, t.BorderStrong
		}
		return t.SurfaceRaised, t.Border
	}
	return row
}

// recognizeByField e chamado quando o usuario digita CPF ou telefone direto
// nos campos: se houver um cliente com aquele dado, ele e reconhecido e os
// campos vazios sao completados.
func (p *customerPicker) recognizeByField(value string, kind string, apply func(CustomerProfile)) {
	digits := onlyDigits(value)
	minLen := 8
	if kind == "cpf" {
		minLen = 11
	}
	if len(digits) < minLen {
		return
	}
	seq := p.seq.Add(1)
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		profiles, err := p.ui.store.FindCustomerProfiles(ctx, digits, 3)
		if err != nil || seq != p.seq.Load() {
			return
		}
		var match *CustomerProfile
		for i := range profiles {
			candidate := profiles[i]
			if kind == "cpf" && onlyDigits(candidate.CPF) == digits {
				match = &candidate
				break
			}
			if kind == "phone" && strings.HasSuffix(onlyDigits(candidate.Phone), digits) {
				match = &candidate
				break
			}
		}
		if match == nil {
			return
		}
		fyne.Do(func() {
			if seq != p.seq.Load() {
				return
			}
			label := "Cliente reconhecido pelo telefone"
			if kind == "cpf" {
				label = "Cliente reconhecido pelo CPF"
			}
			p.SetSelected(*match, label)
			if apply != nil {
				apply(*match)
			}
		})
	}()
}

// spacerRow ajuda a alinhar botoes a direita em faixas.
func spacerRow(objects ...fyne.CanvasObject) fyne.CanvasObject {
	items := append([]fyne.CanvasObject{layout.NewSpacer()}, objects...)
	return container.NewHBox(items...)
}
