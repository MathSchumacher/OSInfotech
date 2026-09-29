package main

import (
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"testing"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/test"
)

// writeTestImage grava um PNG colorido para servir de miniatura.
func writeTestImage(t *testing.T, path string, c color.NRGBA, w, h int) {
	t.Helper()
	img := image.NewNRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			shade := color.NRGBA{R: c.R, G: uint8(int(c.G) * y / h), B: uint8(int(c.B) * x / w), A: 255}
			img.SetNRGBA(x, y, shade)
		}
	}
	file, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	if err := png.Encode(file, img); err != nil {
		t.Fatal(err)
	}
}

// pickerFixture monta uma pasta com subpasta, imagens e um arquivo que deve ser
// ignorado.
func pickerFixture(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	if err := os.Mkdir(filepath.Join(dir, "Aparelhos"), 0o755); err != nil {
		t.Fatal(err)
	}
	writeTestImage(t, filepath.Join(dir, "frente.png"), color.NRGBA{R: 220, G: 40, B: 40, A: 255}, 640, 480)
	writeTestImage(t, filepath.Join(dir, "verso.png"), color.NRGBA{R: 40, G: 140, B: 220, A: 255}, 480, 640)
	writeTestImage(t, filepath.Join(dir, "tela.png"), color.NRGBA{R: 60, G: 200, B: 120, A: 255}, 800, 600)
	if err := os.WriteFile(filepath.Join(dir, "anotacao.txt"), []byte("nao e imagem"), 0o644); err != nil {
		t.Fatal(err)
	}
	return dir
}

func newPickerUI(t *testing.T) *mainUI {
	t.Helper()
	app := test.NewApp()
	t.Cleanup(app.Quit)
	desktop := false
	forceMobileLayout = &desktop
	t.Cleanup(func() { forceMobileLayout = nil })
	store := openTestStore(t)
	return newMainUI(app, store, test.NewWindow(nil), "x.db")
}

// TestPickerListsOnlyImagesAndFolders: a grade mostra pastas e imagens, nessa
// ordem, e ignora os outros arquivos.
func TestPickerListsOnlyImagesAndFolders(t *testing.T) {
	ui := newPickerUI(t)
	dir := pickerFixture(t)

	var chosen []string
	picker := &imagePicker{ui: ui, multi: true, selected: map[string]bool{}, onChoose: func(paths []string) { chosen = paths }}
	picker.build("Fotos do aparelho")
	picker.openDir(dir)

	if len(picker.entries) != 4 {
		t.Fatalf("entradas = %d, queria 4 (1 pasta + 3 imagens): %+v", len(picker.entries), picker.entries)
	}
	if !picker.entries[0].isDir || picker.entries[0].name != "Aparelhos" {
		t.Fatalf("a pasta deveria vir primeiro: %+v", picker.entries[0])
	}
	for _, entry := range picker.entries[1:] {
		if entry.isDir || filepath.Ext(entry.name) != ".png" {
			t.Fatalf("entrada inesperada: %+v", entry)
		}
	}

	picker.toggle(picker.entries[1]) // frente.png
	picker.toggle(picker.entries[3]) // verso.png
	if len(picker.selected) != 2 {
		t.Fatalf("selecionadas = %d", len(picker.selected))
	}
	picker.finish()
	if len(chosen) != 2 {
		t.Fatalf("devolveu %d caminhos", len(chosen))
	}
	if filepath.Base(chosen[0]) != "frente.png" || filepath.Base(chosen[1]) != "verso.png" {
		t.Fatalf("ordem inesperada: %v", chosen)
	}
}

// TestPickerSingleSelectionReplaces: sem multipla escolha, clicar em outra
// imagem troca a selecao.
func TestPickerSingleSelectionReplaces(t *testing.T) {
	ui := newPickerUI(t)
	dir := pickerFixture(t)
	picker := &imagePicker{ui: ui, multi: false, selected: map[string]bool{}}
	picker.build("Escolher foto")
	picker.openDir(dir)

	picker.toggle(picker.entries[1])
	picker.toggle(picker.entries[2])
	if len(picker.selected) != 1 {
		t.Fatalf("selecao unica deveria manter 1 imagem, veio %d", len(picker.selected))
	}
}

// TestThumbnailIsGenerated: a miniatura e gerada, cabe no limite e vem do cache
// na segunda vez.
func TestThumbnailIsGenerated(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "foto.png")
	writeTestImage(t, path, color.NRGBA{R: 200, G: 30, B: 30, A: 255}, 1200, 900)
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	key := thumbKey{path: path, mod: info.ModTime().UnixNano(), size: info.Size()}

	img := loadThumb(path, key)
	if img == nil {
		t.Fatal("miniatura nao foi gerada")
	}
	if img.Bounds().Dx() > thumbSide || img.Bounds().Dy() > thumbSide {
		t.Fatalf("miniatura maior que o limite: %v", img.Bounds())
	}
	if _, ok := cachedThumb(key); !ok {
		t.Fatal("miniatura deveria ter ficado no cache")
	}
	if loadThumb(path, key) != img {
		t.Fatal("a segunda leitura deveria vir do cache")
	}
	if loadThumb(filepath.Join(dir, "nao-existe.png"), thumbKey{path: "x"}) != nil {
		t.Fatal("arquivo inexistente deveria devolver nil")
	}
}

// TestShotImagePicker registra o seletor para revisao visual.
func TestShotImagePicker(t *testing.T) {
	dir := shotsDir(t)
	app := test.NewApp()
	defer app.Quit()
	desktop := false
	forceMobileLayout = &desktop
	defer func() { forceMobileLayout = nil }()

	for _, mode := range []themeMode{themeDark, themeLight} {
		app.Settings().SetTheme(newDBSTheme(mode))
		store := openTestStore(t)
		win := test.NewWindow(nil)
		win.Resize(fyne.NewSize(1100, 760))
		ui := newMainUI(app, store, win, "x.db")
		win.SetContent(ui.content)

		fixture := pickerFixture(t)
		picker := &imagePicker{ui: ui, multi: true, selected: map[string]bool{}}
		picker.build("Fotos do aparelho")
		picker.openDir(fixture)
		picker.toggle(picker.entries[1])
		// espera as miniaturas ficarem prontas
		time.Sleep(700 * time.Millisecond)
		picker.grid.Refresh()
		saveShot(t, dir, "picker-"+mode.String(), win.Canvas().Capture())
		win.Close()
	}
}
