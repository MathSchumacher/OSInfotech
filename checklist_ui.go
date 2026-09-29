package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"image"
	"io"
	"net/http"
	"strings"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
	"github.com/chai2010/webp"
	xdraw "golang.org/x/image/draw"
	"osinfotech/internal/r2client"
)

const checklistNoneOption = "— Nenhuma —"

var checklistStateOptions = []string{"Nao verificado", "OK", "Com defeito", "Nao se aplica"}

func stateToLabel(state string) string {
	switch state {
	case checkStateOK:
		return "OK"
	case checkStateDefect:
		return "Com defeito"
	case checkStateNA:
		return "Nao se aplica"
	default:
		return "Nao verificado"
	}
}

func labelToState(label string) string {
	switch label {
	case "OK":
		return checkStateOK
	case "Com defeito":
		return checkStateDefect
	case "Nao se aplica":
		return checkStateNA
	default:
		return checkStateNone
	}
}

func templateDisplayName(t ChecklistTemplate) string {
	if strings.TrimSpace(t.DeviceType) != "" {
		return t.Name + " — " + t.DeviceType
	}
	return t.Name
}

func splitChecklistLines(text string) []string {
	var items []string
	for _, line := range strings.Split(text, "\n") {
		if trimmed := strings.TrimSpace(line); trimmed != "" {
			items = append(items, trimmed)
		}
	}
	return items
}

func ensureSelectOption(sel *widget.Select, option string) {
	if sel == nil || option == "" {
		return
	}
	for _, existing := range sel.Options {
		if existing == option {
			return
		}
	}
	sel.Options = append(sel.Options, option)
}

// ---- Checklist na OS ----

func (ui *mainUI) buildChecklistSection() fyne.CanvasObject {
	ui.checklistSelect = widget.NewSelect(nil, func(name string) { ui.onChecklistTemplateSelected(name) })
	ui.checklistSelect.PlaceHolder = "Selecione uma lista"
	ui.checklistItemsBox = container.NewVBox()
	ui.refreshChecklistTemplates()
	return container.NewVBox(
		ui.checklistSelect,
		gap(1, 4),
		ui.checklistItemsBox,
	)
}

func (ui *mainUI) refreshChecklistTemplates() {
	templates, err := ui.store.ListChecklistTemplates(context.Background())
	if err != nil {
		templates = nil
	}
	ui.checklistTemplates = templates
	if ui.checklistSelect == nil {
		return
	}
	options := []string{checklistNoneOption}
	for _, tpl := range templates {
		options = append(options, templateDisplayName(tpl))
	}
	selected := ui.checklistSelect.Selected
	ui.checklistSelect.Options = options
	ensureSelectOption(ui.checklistSelect, selected)
	ui.checklistSelect.Refresh()
}

func (ui *mainUI) findTemplateByDisplay(display string) (ChecklistTemplate, bool) {
	for _, tpl := range ui.checklistTemplates {
		if templateDisplayName(tpl) == display {
			return tpl, true
		}
	}
	return ChecklistTemplate{}, false
}

func (ui *mainUI) onChecklistTemplateSelected(name string) {
	if name == "" || name == checklistNoneOption {
		ui.currentChecklist = OrderChecklist{}
		ui.renderChecklistItems(nil)
		return
	}
	tpl, ok := ui.findTemplateByDisplay(name)
	if !ok {
		ui.renderChecklistItems(ui.currentChecklist.Items)
		return
	}
	items := make([]ChecklistItem, 0, len(tpl.Items))
	for _, label := range tpl.Items {
		items = append(items, ChecklistItem{Label: label, State: checkStateNone})
	}
	ui.currentChecklist = OrderChecklist{TemplateID: tpl.ID, TemplateName: tpl.Name, DeviceType: tpl.DeviceType, Items: items}
	ui.renderChecklistItems(items)
}

func (ui *mainUI) renderChecklistItems(items []ChecklistItem) {
	ui.checklistItemStates = nil
	if ui.checklistItemsBox == nil {
		return
	}
	ui.checklistItemsBox.Objects = nil
	for _, item := range items {
		sel := widget.NewSelect(checklistStateOptions, nil)
		sel.Selected = stateToLabel(item.State)
		ui.checklistItemStates = append(ui.checklistItemStates, sel)
		label := widget.NewLabel(item.Label)
		label.Wrapping = fyne.TextWrapWord
		ui.checklistItemsBox.Add(container.NewBorder(nil, nil, nil, sel, label))
	}
	ui.checklistItemsBox.Refresh()
}

func (ui *mainUI) collectChecklist() OrderChecklist {
	c := ui.currentChecklist
	if len(ui.checklistItemStates) == len(c.Items) {
		for i := range c.Items {
			c.Items[i].State = labelToState(ui.checklistItemStates[i].Selected)
		}
	}
	return c
}

func (ui *mainUI) applyChecklistToForm(c OrderChecklist) {
	ui.currentChecklist = c
	display := checklistNoneOption
	if c.TemplateName != "" || len(c.Items) > 0 {
		display = c.TemplateName
		if strings.TrimSpace(c.DeviceType) != "" {
			display = c.TemplateName + " — " + c.DeviceType
		}
	}
	if ui.checklistSelect != nil {
		ensureSelectOption(ui.checklistSelect, display)
		ui.checklistSelect.Selected = display
		ui.checklistSelect.Refresh()
	}
	ui.renderChecklistItems(c.Items)
}

func (ui *mainUI) resetChecklistAndPhotos() {
	ui.currentChecklist = OrderChecklist{}
	if ui.checklistSelect != nil {
		ui.checklistSelect.Selected = checklistNoneOption
		ui.checklistSelect.Refresh()
	}
	ui.renderChecklistItems(nil)
	ui.currentPhotos = nil
	ui.refreshPhotoThumbs()
}

// ---- Fotos da OS ----

func (ui *mainUI) buildPhotosSection() fyne.CanvasObject {
	ui.photosBox = container.NewVBox()
	add := secondaryButton("Adicionar fotos", theme.FileImageIcon(), nil)
	add.OnTapped = func() {
		// Varias fotos de uma vez: e o jeito natural de registrar o estado de
		// entrada do aparelho (frente, verso, tela, acessorios).
		ui.pickImages("Fotos do aparelho", true, func(images [][]byte) {
			add.Disable()
			formSeq := ui.orderFormSeq
			go func() {
				for i, data := range images {
					index := i
					fyne.Do(func() {
						add.SetText(fmt.Sprintf("Enviando %d de %d...", index+1, len(images)))
					})
					done := make(chan struct{})
					ui.uploadOrderPhoto(formSeq, data, func() { close(done) })
					<-done
				}
				fyne.Do(func() {
					add.Enable()
					add.SetText("Adicionar fotos")
					if len(images) > 1 {
						ui.notify(fmt.Sprintf("%d fotos anexadas", len(images)))
					}
				})
			}()
		})
	}
	ui.refreshPhotoThumbs()
	return container.NewVBox(add, ui.photosBox)
}

func (ui *mainUI) refreshPhotoThumbs() {
	if ui.photosBox == nil {
		return
	}
	ui.photosBox.Objects = nil
	if len(ui.currentPhotos) == 0 {
		ui.photosBox.Add(newText("Nenhuma foto anexada. Fotos comprovam o estado de entrada.", sizeCaption, false, inkMuted).SetTruncate(true))
		ui.photosBox.Refresh()
		return
	}
	grid := container.NewGridWrap(fyne.NewSize(130, 150))
	for i, photo := range ui.currentPhotos {
		index := i
		img := canvas.NewImageFromResource(theme.FileImageIcon())
		img.FillMode = canvas.ImageFillContain
		img.SetMinSize(fyne.NewSize(130, 110))
		ui.loadPhotoThumb(img, photo.URL)
		img.CornerRadius = radiusMd
		remove := ghostButton("Remover", theme.DeleteIcon(), func() { ui.removeOrderPhoto(index) })
		grid.Add(container.NewVBox(img, remove))
	}
	ui.photosBox.Add(grid)
	ui.photosBox.Refresh()
}

func (ui *mainUI) removeOrderPhoto(index int) {
	if index < 0 || index >= len(ui.currentPhotos) {
		return
	}
	ui.currentPhotos = append(ui.currentPhotos[:index], ui.currentPhotos[index+1:]...)
	ui.refreshPhotoThumbs()
}

func (ui *mainUI) loadPhotoThumb(img *canvas.Image, rawURL string) {
	rawURL = strings.TrimSpace(rawURL)
	if rawURL == "" {
		return
	}
	go func() {
		req, err := http.NewRequest(http.MethodGet, rawURL, nil)
		if err != nil {
			return
		}
		client := &http.Client{Timeout: 30 * time.Second}
		res, err := client.Do(req)
		if err != nil {
			return
		}
		defer res.Body.Close()
		if res.StatusCode < 200 || res.StatusCode > 299 {
			return
		}
		decoded, _, err := image.Decode(io.LimitReader(res.Body, 12*1024*1024))
		if err != nil {
			return
		}
		fyne.Do(func() {
			img.Resource = nil
			img.Image = decoded
			img.Refresh()
		})
	}()
}

func (ui *mainUI) uploadOrderPhoto(formSeq uint64, data []byte, done func()) {
	if done != nil {
		defer done()
	}
	webpData, err := makeOrderPhotoWebP(data)
	if err != nil {
		fyne.Do(func() { dialog.ShowError(err, ui.window) })
		return
	}
	cfg, err := ui.loadR2Config(context.Background())
	if err != nil {
		fyne.Do(func() { dialog.ShowError(fmt.Errorf("R2 nao configurado: %w", err), ui.window) })
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	client, err := r2client.New(ctx, cfg)
	if err != nil {
		fyne.Do(func() { dialog.ShowError(fmt.Errorf("R2 nao configurado: %w", err), ui.window) })
		return
	}
	key := orderPhotoObjectKey(webpData)
	if err := client.EnsureRoom(ctx, map[string]int64{key: int64(len(webpData))}, false); err != nil {
		fyne.Do(func() { dialog.ShowError(fmt.Errorf("limite R2 atingido: %w", err), ui.window) })
		return
	}
	if err := client.PutBytes(ctx, key, webpData, "image/webp"); err != nil {
		fyne.Do(func() { dialog.ShowError(fmt.Errorf("falha no upload: %w", err), ui.window) })
		return
	}
	publicURL := client.PublicURL(key)
	if publicURL == "" {
		fyne.Do(func() { dialog.ShowError(errors.New("Public URL do R2 nao configurada"), ui.window) })
		return
	}
	fyne.Do(func() {
		// O envio demora; se o usuario ja abriu outra OS, a foto nao pode entrar
		// no aparelho de outro cliente.
		if ui.orderFormSeq != formSeq {
			return
		}
		ui.currentPhotos = append(ui.currentPhotos, OrderPhoto{URL: publicURL, Key: key})
		ui.refreshPhotoThumbs()
	})
}

func makeOrderPhotoWebP(data []byte) ([]byte, error) {
	src, _, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("imagem invalida: %w", err)
	}
	b := src.Bounds()
	w, h := b.Dx(), b.Dy()
	if w <= 0 || h <= 0 {
		return nil, errors.New("imagem sem dimensoes validas")
	}
	const maxSide = 1280
	nw, nh := w, h
	if w > maxSide || h > maxSide {
		if w >= h {
			nw = maxSide
			nh = h * maxSide / w
		} else {
			nh = maxSide
			nw = w * maxSide / h
		}
	}
	dst := image.NewRGBA(image.Rect(0, 0, nw, nh))
	xdraw.CatmullRom.Scale(dst, dst.Bounds(), src, b, xdraw.Over, nil)
	var out bytes.Buffer
	if err := webp.Encode(&out, dst, &webp.Options{Quality: 80}); err != nil {
		return nil, err
	}
	return out.Bytes(), nil
}

func orderPhotoObjectKey(data []byte) string {
	sum := sha256.Sum256(data)
	return "os-photos/" + hex.EncodeToString(sum[:]) + ".webp"
}

// ---- Gerenciador de listas (templates) ----

func (ui *mainUI) showChecklistTemplateEditor(existing *ChecklistTemplate, onSaved func()) {
	name := newFormEntry()
	name.SetPlaceHolder("Ex: Celular")
	deviceType := newFormEntry()
	deviceType.SetPlaceHolder("Ex: Celular, Console, TV (opcional)")
	items := newFormMultiLine()
	items.SetPlaceHolder("Um item por linha")
	items.SetMinRowsVisible(10)

	title := "Nova lista"
	if existing != nil {
		title = "Editar lista"
		name.SetText(existing.Name)
		deviceType.SetText(existing.DeviceType)
		items.SetText(strings.Join(existing.Items, "\n"))
	}

	form := widget.NewForm(
		widget.NewFormItem("Nome", name),
		widget.NewFormItem("Tipo de aparelho", deviceType),
		widget.NewFormItem("Itens", items),
	)
	editor := dialog.NewCustomConfirm(title, "Salvar", "Cancelar", form, func(ok bool) {
		if !ok {
			return
		}
		tpl := ChecklistTemplate{}
		if existing != nil {
			tpl = *existing
		}
		tpl.Name = strings.TrimSpace(name.Text)
		tpl.DeviceType = strings.TrimSpace(deviceType.Text)
		tpl.Items = splitChecklistLines(items.Text)
		if err := ui.store.SaveChecklistTemplate(context.Background(), &tpl); err != nil {
			dialog.ShowError(err, ui.window)
			return
		}
		if onSaved != nil {
			onSaved()
		}
	}, ui.window)
	editor.Resize(fyne.NewSize(500, 540))
	editor.Show()
}
