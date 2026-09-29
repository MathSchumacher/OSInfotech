package main

import (
	"image"
	"image/color"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/driver/desktop"
	"fyne.io/fyne/v2/storage"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
	xdraw "golang.org/x/image/draw"
)

// O seletor de imagens do Fyne mostra so um icone com a extensao: no desktop
// nao existe miniatura. Como aqui a foto e o conteudo (aparelho do cliente,
// avatar), este arquivo traz um seletor proprio com miniaturas. No celular o
// Fyne delega ao seletor do proprio sistema, que ja mostra as fotos.

var pickerImageExtensions = map[string]bool{
	".png": true, ".jpg": true, ".jpeg": true, ".webp": true, ".gif": true, ".bmp": true,
}

const (
	thumbCell   = 148 // lado da celula da grade
	thumbSide   = 116 // lado maximo da miniatura
	prefLastDir = "image_picker_last_dir"
)

// ---------------------------------------------------------------------------
// Cache de miniaturas
// ---------------------------------------------------------------------------

type thumbKey struct {
	path string
	mod  int64
	size int64
}

var (
	thumbCacheMu sync.Mutex
	thumbCache   = map[thumbKey]image.Image{}
	// thumbSlots limita quantas imagens sao decodificadas ao mesmo tempo:
	// uma pasta com centenas de fotos nao pode travar o app.
	thumbSlots = make(chan struct{}, 3)
)

func cachedThumb(key thumbKey) (image.Image, bool) {
	thumbCacheMu.Lock()
	defer thumbCacheMu.Unlock()
	img, ok := thumbCache[key]
	return img, ok
}

func storeThumb(key thumbKey, img image.Image) {
	thumbCacheMu.Lock()
	defer thumbCacheMu.Unlock()
	if len(thumbCache) > 400 {
		for k := range thumbCache { // remove algumas entradas antigas
			delete(thumbCache, k)
			if len(thumbCache) <= 300 {
				break
			}
		}
	}
	thumbCache[key] = img
}

// loadThumb decodifica e reduz a imagem, respeitando o limite de decodificacoes
// simultaneas. Devolve nil quando o arquivo nao e uma imagem valida.
func loadThumb(path string, key thumbKey) image.Image {
	if img, ok := cachedThumb(key); ok {
		return img
	}
	thumbSlots <- struct{}{}
	defer func() { <-thumbSlots }()
	if img, ok := cachedThumb(key); ok {
		return img
	}

	file, err := os.Open(path)
	if err != nil {
		return nil
	}
	defer file.Close()
	src, _, err := image.Decode(file)
	if err != nil {
		return nil
	}
	bounds := src.Bounds()
	w, h := bounds.Dx(), bounds.Dy()
	if w <= 0 || h <= 0 {
		return nil
	}
	nw, nh := w, h
	if w > thumbSide || h > thumbSide {
		if w >= h {
			nw, nh = thumbSide, h*thumbSide/w
		} else {
			nh, nw = thumbSide, w*thumbSide/h
		}
	}
	dst := image.NewNRGBA(image.Rect(0, 0, nw, nh))
	xdraw.ApproxBiLinear.Scale(dst, dst.Bounds(), src, bounds, xdraw.Src, nil)
	storeThumb(key, dst)
	return dst
}

// ---------------------------------------------------------------------------
// Celula da grade
// ---------------------------------------------------------------------------

type pickerEntry struct {
	name  string
	path  string
	isDir bool
	key   thumbKey
}

type thumbItem struct {
	widget.BaseWidget
	entry     pickerEntry
	selected  bool
	hovered   bool
	onTap     func()
	onConfirm func()

	bg    *canvas.Rectangle
	img   *canvas.Image
	icon  *canvas.Image
	label *canvas.Text
	want  string // caminho que esta celula esta mostrando agora
}

func newThumbItem() *thumbItem {
	t := &thumbItem{}
	t.bg = canvas.NewRectangle(color.Transparent)
	t.bg.CornerRadius = radiusMd
	t.bg.StrokeWidth = 1
	t.img = canvas.NewImageFromImage(nil)
	t.img.FillMode = canvas.ImageFillContain
	t.img.ScaleMode = canvas.ImageScaleSmooth
	t.icon = canvas.NewImageFromResource(nil)
	t.icon.FillMode = canvas.ImageFillContain
	t.label = canvas.NewText("", color.Black)
	t.label.TextSize = sizeCaption
	t.label.Alignment = fyne.TextAlignCenter
	t.ExtendBaseWidget(t)
	return t
}

func (t *thumbItem) Tapped(*fyne.PointEvent) {
	if t.onTap != nil {
		t.onTap()
	}
}

func (t *thumbItem) DoubleTapped(*fyne.PointEvent) {
	if t.onConfirm != nil {
		t.onConfirm()
	}
}

func (t *thumbItem) MouseIn(*desktop.MouseEvent)    { t.hovered = true; t.Refresh() }
func (t *thumbItem) MouseMoved(*desktop.MouseEvent) {}
func (t *thumbItem) MouseOut()                      { t.hovered = false; t.Refresh() }
func (t *thumbItem) Cursor() desktop.Cursor         { return desktop.PointerCursor }

func (t *thumbItem) MinSize() fyne.Size { return fyne.NewSize(thumbCell, thumbCell+22) }

func (t *thumbItem) CreateRenderer() fyne.WidgetRenderer {
	r := &thumbItemRenderer{t: t}
	r.Refresh()
	return r
}

type thumbItemRenderer struct{ t *thumbItem }

func (r *thumbItemRenderer) Layout(size fyne.Size) {
	t := r.t
	t.bg.Resize(size)
	area := fyne.NewSize(size.Width-16, size.Height-34)
	t.img.Resize(area)
	t.img.Move(fyne.NewPos(8, 6))
	t.icon.Resize(fyne.NewSize(44, 44))
	t.icon.Move(fyne.NewPos((size.Width-44)/2, (area.Height-44)/2+6))
	ls := t.label.MinSize()
	t.label.Resize(fyne.NewSize(size.Width-8, ls.Height))
	t.label.Move(fyne.NewPos(4, size.Height-ls.Height-6))
}

func (r *thumbItemRenderer) MinSize() fyne.Size { return r.t.MinSize() }

func (r *thumbItemRenderer) Refresh() {
	tk := tk()
	t := r.t
	switch {
	case t.selected:
		t.bg.FillColor = tk.BrandSoft
		t.bg.StrokeColor = tk.Brand
		t.bg.StrokeWidth = 2
	case t.hovered:
		t.bg.FillColor = tk.Hover
		t.bg.StrokeColor = tk.BorderStrong
		t.bg.StrokeWidth = 1
	default:
		t.bg.FillColor = tk.GlassSunken
		t.bg.StrokeColor = tk.GlassBorder
		t.bg.StrokeWidth = 1
	}
	t.label.Color = tk.Ink
	t.bg.Refresh()
	t.label.Refresh()
	t.img.Refresh()
	t.icon.Refresh()
}

func (r *thumbItemRenderer) Objects() []fyne.CanvasObject {
	return []fyne.CanvasObject{r.t.bg, r.t.img, r.t.icon, r.t.label}
}

func (r *thumbItemRenderer) Destroy() {}

// ---------------------------------------------------------------------------
// Seletor
// ---------------------------------------------------------------------------

type imagePicker struct {
	ui       *mainUI
	multi    bool
	dir      string
	entries  []pickerEntry
	selected map[string]bool
	gen      atomic.Uint64

	grid     *widget.GridWrap
	pathBox  *dbsEntry
	status   *themedText
	empty    fyne.CanvasObject
	confirm  *dbsButton
	dialog   *dialog.CustomDialog
	onChoose func(paths []string)
}

// showImagePicker abre o seletor de imagens com miniaturas. No celular usa o
// seletor do sistema, que ja mostra as fotos.
func (ui *mainUI) showImagePicker(title string, multi bool, onChoose func(paths []string)) {
	p := &imagePicker{ui: ui, multi: multi, selected: map[string]bool{}, onChoose: onChoose}
	p.build(title)
}

func (p *imagePicker) build(title string) {
	ui := p.ui
	p.pathBox = newFormEntry()
	p.pathBox.OnSubmitted = func(path string) { p.openDir(strings.TrimSpace(path)) }
	up := ghostButton("Acima", theme.MoveUpIcon(), func() { p.openDir(filepath.Dir(p.dir)) })
	reload := iconButton(theme.ViewRefreshIcon(), func() { p.openDir(p.dir) })
	top := container.NewBorder(nil, nil, up, reload, p.pathBox)

	places := container.NewVBox(newEyebrow("Locais"))
	for _, place := range pickerPlaces() {
		target := place.path
		places.Add(ghostButton(place.label, place.icon, func() { p.openDir(target) }))
	}
	sidebar := container.NewVScroll(places)

	p.status = newText("", sizeCaption, false, inkMuted)
	p.empty = container.NewCenter(newText("Nenhuma imagem nesta pasta.", sizeBody, false, inkMuted))
	p.empty.Hide()

	p.grid = widget.NewGridWrap(
		func() int { return len(p.entries) },
		func() fyne.CanvasObject { return newThumbItem() },
		func(id widget.GridWrapItemID, obj fyne.CanvasObject) { p.updateCell(id, obj.(*thumbItem)) },
	)

	body := container.NewBorder(
		container.NewVBox(top, gap(1, 6)),
		container.NewVBox(gap(1, 4), p.status),
		container.NewStack(gap(190, 1), sidebar), nil,
		container.NewStack(p.grid, p.empty),
	)

	cancel := ghostButton("Cancelar", theme.CancelIcon(), func() { p.dialog.Hide() })
	p.confirm = primaryButton("Usar imagem", theme.ConfirmIcon(), func() { p.finish() })
	if p.multi {
		p.confirm.SetText("Usar imagens")
	}
	p.confirm.Disable()

	p.dialog = dialog.NewCustomWithoutButtons(title, body, ui.window)
	p.dialog.SetButtons([]fyne.CanvasObject{cancel, p.confirm})
	p.dialog.Resize(fyne.NewSize(940, 640))
	p.openDir(p.startDir())
	p.dialog.Show()
}

// startDir devolve a ultima pasta usada, ou a pasta de imagens do usuario.
func (p *imagePicker) startDir() string {
	if prefs := p.ui.prefs(); prefs != nil {
		if last := strings.TrimSpace(prefs.String(prefLastDir)); last != "" {
			if info, err := os.Stat(last); err == nil && info.IsDir() {
				return last
			}
		}
	}
	if places := pickerPlaces(); len(places) > 0 {
		return places[0].path
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "."
	}
	return home
}

type pickerPlace struct {
	label string
	path  string
	icon  fyne.Resource
}

// pickerPlaces monta os atalhos de pastas que existem neste computador.
func pickerPlaces() []pickerPlace {
	var places []pickerPlace
	home, err := os.UserHomeDir()
	if err == nil {
		candidates := []struct {
			label string
			sub   string
			icon  fyne.Resource
		}{
			{"Imagens", "Pictures", theme.FileImageIcon()},
			{"Downloads", "Downloads", theme.DownloadIcon()},
			{"Área de trabalho", "Desktop", theme.ComputerIcon()},
			{"Documentos", "Documents", theme.DocumentIcon()},
		}
		for _, c := range candidates {
			path := filepath.Join(home, c.sub)
			if info, err := os.Stat(path); err == nil && info.IsDir() {
				places = append(places, pickerPlace{c.label, path, c.icon})
			}
		}
		places = append(places, pickerPlace{"Pasta pessoal", home, theme.HomeIcon()})
	}
	if runtime.GOOS == "windows" {
		for letter := 'C'; letter <= 'Z'; letter++ {
			drive := string(letter) + ":\\"
			if info, err := os.Stat(drive); err == nil && info.IsDir() {
				places = append(places, pickerPlace{string(letter) + ":", drive, theme.StorageIcon()})
			}
		}
	} else {
		places = append(places, pickerPlace{"Sistema", "/", theme.StorageIcon()})
	}
	return places
}

// openDir carrega as pastas e imagens do caminho informado.
func (p *imagePicker) openDir(dir string) {
	dir = strings.TrimSpace(dir)
	if dir == "" {
		return
	}
	info, err := os.Stat(dir)
	if err != nil || !info.IsDir() {
		p.status.SetText("Pasta não encontrada: " + dir)
		return
	}
	p.gen.Add(1)
	p.dir = dir
	p.pathBox.SetText(dir)
	if prefs := p.ui.prefs(); prefs != nil {
		prefs.SetString(prefLastDir, dir)
	}

	items, err := os.ReadDir(dir)
	if err != nil {
		p.status.SetText("Sem permissão para abrir esta pasta.")
		p.entries = nil
		p.grid.Refresh()
		return
	}
	var dirs, files []pickerEntry
	for _, item := range items {
		name := item.Name()
		if strings.HasPrefix(name, ".") {
			continue
		}
		full := filepath.Join(dir, name)
		if item.IsDir() {
			dirs = append(dirs, pickerEntry{name: name, path: full, isDir: true})
			continue
		}
		if !pickerImageExtensions[strings.ToLower(filepath.Ext(name))] {
			continue
		}
		entry := pickerEntry{name: name, path: full}
		if fileInfo, err := item.Info(); err == nil {
			entry.key = thumbKey{path: full, mod: fileInfo.ModTime().UnixNano(), size: fileInfo.Size()}
		} else {
			entry.key = thumbKey{path: full}
		}
		files = append(files, entry)
	}
	sortEntries(dirs)
	sortEntries(files)
	p.entries = append(dirs, files...)
	p.selected = map[string]bool{}
	p.updateStatus(len(files))
	p.confirm.Disable()
	p.grid.Refresh()
	p.grid.ScrollToTop()
	if len(p.entries) == 0 {
		p.empty.Show()
	} else {
		p.empty.Hide()
	}
}

func sortEntries(entries []pickerEntry) {
	sort.SliceStable(entries, func(i, j int) bool {
		return strings.ToLower(entries[i].name) < strings.ToLower(entries[j].name)
	})
}

func (p *imagePicker) updateStatus(images int) {
	switch {
	case len(p.selected) == 1:
		for path := range p.selected {
			p.status.SetText("Selecionada: " + filepath.Base(path))
		}
	case len(p.selected) > 1:
		p.status.SetText(strconv.Itoa(len(p.selected)) + " imagens selecionadas")
	case images == 0:
		p.status.SetText("Nenhuma imagem nesta pasta.")
	case images == 1:
		p.status.SetText("1 imagem. Clique para escolher, duplo clique para confirmar.")
	default:
		p.status.SetText(strconv.Itoa(images) + " imagens. Clique para escolher, duplo clique para confirmar.")
	}
}

// updateCell preenche uma celula reciclada da grade.
func (p *imagePicker) updateCell(id widget.GridWrapItemID, cell *thumbItem) {
	if id < 0 || id >= len(p.entries) {
		return
	}
	entry := p.entries[id]
	cell.entry = entry
	cell.want = entry.path
	cell.selected = p.selected[entry.path]
	cell.label.Text = shortFileName(entry.name)
	cell.img.Image = nil
	cell.img.Hide()
	cell.icon.Show()

	if entry.isDir {
		cell.icon.Resource = theme.NewColoredResource(theme.FolderIcon(), theme.ColorNamePrimary)
		cell.onTap = func() { p.openDir(entry.path) }
		cell.onConfirm = func() { p.openDir(entry.path) }
		cell.Refresh()
		return
	}

	cell.icon.Resource = theme.NewColoredResource(theme.FileImageIcon(), theme.ColorNameDisabled)
	cell.onTap = func() { p.toggle(entry) }
	cell.onConfirm = func() {
		p.toggle(entry)
		p.finish()
	}
	cell.Refresh()

	gen := p.gen.Load()
	go func() {
		img := loadThumb(entry.path, entry.key)
		if img == nil || gen != p.gen.Load() {
			return
		}
		fyne.Do(func() {
			if cell.want != entry.path {
				return // a celula ja foi reaproveitada por outro arquivo
			}
			cell.img.Image = img
			cell.img.Show()
			cell.icon.Hide()
			cell.Refresh()
		})
	}()
}

// shortFileName encurta o nome para caber na celula (canvas.Text nao trunca).
func shortFileName(name string) string {
	const maxRunes = 20
	runes := []rune(name)
	if len(runes) <= maxRunes {
		return name
	}
	ext := filepath.Ext(name)
	base := []rune(strings.TrimSuffix(name, ext))
	keep := maxRunes - len([]rune(ext)) - 1
	if keep < 4 {
		return string(runes[:maxRunes-1]) + "…"
	}
	return string(base[:keep]) + "…" + ext
}

// toggle marca ou desmarca uma imagem.
func (p *imagePicker) toggle(entry pickerEntry) {
	if !p.multi {
		p.selected = map[string]bool{entry.path: true}
	} else if p.selected[entry.path] {
		delete(p.selected, entry.path)
	} else {
		p.selected[entry.path] = true
	}
	if len(p.selected) == 0 {
		p.confirm.Disable()
	} else {
		p.confirm.Enable()
	}
	images := 0
	for _, e := range p.entries {
		if !e.isDir {
			images++
		}
	}
	p.updateStatus(images)
	p.grid.Refresh()
}

// finish devolve as imagens escolhidas.
func (p *imagePicker) finish() {
	if len(p.selected) == 0 {
		return
	}
	paths := make([]string, 0, len(p.selected))
	for _, entry := range p.entries { // mantem a ordem da pasta
		if p.selected[entry.path] {
			paths = append(paths, entry.path)
		}
	}
	p.dialog.Hide()
	if p.onChoose != nil {
		p.onChoose(paths)
	}
}

// chooseFileNative usa o seletor do proprio sistema (celular) e devolve os
// bytes lidos: no Android o endereco e um content:// que so o Fyne sabe abrir.
func (ui *mainUI) chooseFileNative(done func(data []byte)) {
	fileDialog := dialog.NewFileOpen(func(reader fyne.URIReadCloser, err error) {
		if err != nil {
			dialog.ShowError(err, ui.window)
			return
		}
		if reader == nil {
			return
		}
		defer reader.Close()
		data, err := io.ReadAll(io.LimitReader(reader, maxPickedImageBytes+1))
		if err != nil {
			dialog.ShowError(err, ui.window)
			return
		}
		if len(data) > maxPickedImageBytes {
			ui.notifyTone("Imagem muito grande (limite de 20 MB).", toneDanger)
			return
		}
		done(data)
	}, ui.window)
	fileDialog.SetFilter(storage.NewExtensionFileFilter([]string{".png", ".jpg", ".jpeg", ".webp", ".gif"}))
	fileDialog.Show()
}

// maxPickedImageBytes limita o tamanho de cada foto aceita.
const maxPickedImageBytes = 20 * 1024 * 1024

// pickImages abre o seletor e entrega o conteudo das imagens escolhidas.
func (ui *mainUI) pickImages(title string, multi bool, done func(images [][]byte)) {
	if isMobileLayout() {
		ui.chooseFileNative(func(data []byte) { done([][]byte{data}) })
		return
	}
	ui.showImagePicker(title, multi, func(paths []string) {
		images := make([][]byte, 0, len(paths))
		for _, path := range paths {
			info, err := os.Stat(path)
			if err != nil {
				continue
			}
			if info.Size() > maxPickedImageBytes {
				ui.notifyTone(filepath.Base(path)+": imagem muito grande (limite de 20 MB).", toneDanger)
				continue
			}
			data, err := os.ReadFile(path)
			if err != nil {
				ui.notifyTone("Não foi possível ler "+filepath.Base(path)+".", toneDanger)
				continue
			}
			images = append(images, data)
		}
		if len(images) == 0 {
			return
		}
		done(images)
	})
}
