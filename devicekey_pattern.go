package main

import (
	"fmt"
	"image/color"
	"strconv"
	"strings"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/driver/desktop"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
)

// ---------------------------------------------------------------------------
// Padrao de desenho do aparelho
//
// Metade dos celulares que entram na loja abre por desenho, e nao por numero.
// A base mostra as tentativas de anotar isso com palavras: "SENHA: L",
// "SENHA M", "senha tudo quadrado". Nenhuma dessas devolve o desenho depois.
//
// Aqui o operador desenha o padrao no proprio app, tocando os pontos na ordem,
// e o que fica gravado e a sequencia. Depois, na mesma tela, da para repetir o
// movimento quantas vezes precisar.
//
// A sequencia mora no campo device_key que ja existe, com um prefixo. Assim ela
// sincroniza desde a 0.12.0 sem coluna nova, sem migracao e sem deploy, e
// continua fora do comprovante como todo o resto desse campo.
// ---------------------------------------------------------------------------

// patternPrefix marca o valor como desenho, e nao como senha digitada.
const patternPrefix = "padrao:"

// patternMinDots e o minimo que o Android aceita num padrao.
const patternMinDots = 4

// encodePattern grava a sequencia de pontos (1 a 9, como no teclado numerico:
// 1 no canto superior esquerdo, 9 no inferior direito).
func encodePattern(dots []int) string {
	if len(dots) == 0 {
		return ""
	}
	parts := make([]string, 0, len(dots))
	for _, dot := range dots {
		parts = append(parts, strconv.Itoa(dot))
	}
	return patternPrefix + strings.Join(parts, "-")
}

// decodePattern le a sequencia gravada. Devolve false quando o valor nao e um
// desenho, que e o caso de toda senha digitada.
func decodePattern(value string) ([]int, bool) {
	trimmed := strings.TrimSpace(value)
	if !strings.HasPrefix(trimmed, patternPrefix) {
		return nil, false
	}
	raw := strings.TrimPrefix(trimmed, patternPrefix)
	if raw == "" {
		return nil, false
	}
	parts := strings.Split(raw, "-")
	dots := make([]int, 0, len(parts))
	seen := make(map[int]bool, len(parts))
	for _, part := range parts {
		dot, err := strconv.Atoi(strings.TrimSpace(part))
		if err != nil || dot < 1 || dot > 9 || seen[dot] {
			return nil, false
		}
		seen[dot] = true
		dots = append(dots, dot)
	}
	return dots, true
}

// isPatternKey diz se o que esta gravado e um desenho.
func isPatternKey(value string) bool {
	_, ok := decodePattern(value)
	return ok
}

// patternSummary descreve o desenho em texto, para a linha do formulario.
func patternSummary(value string) string {
	dots, ok := decodePattern(value)
	if !ok {
		return ""
	}
	return fmt.Sprintf("Padrão de %d pontos", len(dots))
}

// ---------------------------------------------------------------------------
// O widget: nove pontos que o operador toca na ordem
// ---------------------------------------------------------------------------

type patternGrid struct {
	widget.BaseWidget

	dots     []int // sequencia tocada
	hover    int   // ponto sob o cursor, 0 quando nenhum
	replay   int   // quantos passos mostrar na animacao; -1 mostra tudo
	anim     *fyne.Animation
	editable bool
	side     float32 // lado da grade; a miniatura do formulario usa menos
	compact  bool    // miniatura: sem numeros, so o traco

	OnChanged func(dots []int)
}

func newPatternGrid(editable bool) *patternGrid {
	p := &patternGrid{replay: -1, editable: editable, side: 200}
	p.ExtendBaseWidget(p)
	return p
}

// newPatternThumb e a miniatura que fica no formulario: mostra o desenho sem
// precisar abrir nada, e um toque repete o movimento. Sem numeros, que nao
// caberiam legiveis nesse tamanho.
func newPatternThumb(dots []int, side float32) *patternGrid {
	p := &patternGrid{replay: -1, editable: false, side: side, compact: true}
	p.ExtendBaseWidget(p)
	p.dots = append([]int(nil), dots...)
	return p
}

func (p *patternGrid) MinSize() fyne.Size { return fyne.NewSize(p.side, p.side) }

// SetDots carrega um desenho existente.
func (p *patternGrid) SetDots(dots []int) {
	p.dots = append([]int(nil), dots...)
	p.replay = -1
	p.Refresh()
}

// Dots devolve a sequencia atual.
func (p *patternGrid) Dots() []int { return append([]int(nil), p.dots...) }

// Clear apaga o desenho.
func (p *patternGrid) Clear() {
	p.dots = nil
	p.replay = -1
	p.Refresh()
	if p.OnChanged != nil {
		p.OnChanged(nil)
	}
}

// Replay repete o movimento do desenho, um ponto por vez. E o que substitui a
// ideia do GIF: o desenho pode ser conferido a qualquer momento, sem virar um
// arquivo solto com a credencial do cliente.
func (p *patternGrid) Replay() {
	if len(p.dots) == 0 {
		return
	}
	if p.anim != nil {
		p.anim.Stop()
	}
	steps := len(p.dots)
	duration := time.Duration(steps) * 260 * time.Millisecond
	p.anim = fyne.NewAnimation(duration, func(f float32) {
		shown := int(f*float32(steps)) + 1
		if shown > steps {
			shown = steps
		}
		if shown != p.replay {
			p.replay = shown
			p.Refresh()
		}
	})
	p.anim.Curve = fyne.AnimationLinear
	p.replay = 1
	p.Refresh()
	p.anim.Start()
	// Ao fim da animacao o desenho inteiro volta a aparecer.
	go func(a *fyne.Animation) {
		time.Sleep(duration + 120*time.Millisecond)
		fyne.Do(func() {
			if p.anim == a {
				p.replay = -1
				p.Refresh()
			}
		})
	}(p.anim)
}

// dotAt devolve o ponto (1 a 9) sob a posicao, ou 0.
func (p *patternGrid) dotAt(pos fyne.Position, size fyne.Size) int {
	cell := fyne.Min(size.Width, size.Height) / 3
	if cell <= 0 {
		return 0
	}
	offsetX := (size.Width - cell*3) / 2
	offsetY := (size.Height - cell*3) / 2
	col := int((pos.X - offsetX) / cell)
	row := int((pos.Y - offsetY) / cell)
	if col < 0 || col > 2 || row < 0 || row > 2 {
		return 0
	}
	return row*3 + col + 1
}

func (p *patternGrid) Tapped(e *fyne.PointEvent) {
	if !p.editable {
		p.Replay()
		return
	}
	dot := p.dotAt(e.Position, p.Size())
	if dot == 0 {
		return
	}
	for _, used := range p.dots {
		if used == dot {
			// Tocar de novo no primeiro ponto recomeca o desenho: e mais rapido
			// que procurar o botao de limpar com o aparelho na outra mao.
			if dot == p.dots[0] {
				p.Clear()
			}
			return
		}
	}
	p.dots = append(p.dots, dot)
	p.replay = -1
	p.Refresh()
	if p.OnChanged != nil {
		p.OnChanged(p.Dots())
	}
}

func (p *patternGrid) MouseIn(e *desktop.MouseEvent) { p.MouseMoved(e) }
func (p *patternGrid) MouseOut()                     { p.hover = 0; p.Refresh() }
func (p *patternGrid) Cursor() desktop.Cursor        { return desktop.PointerCursor }
func (p *patternGrid) MouseMoved(e *desktop.MouseEvent) {
	if !p.editable {
		return
	}
	if dot := p.dotAt(e.Position, p.Size()); dot != p.hover {
		p.hover = dot
		p.Refresh()
	}
}

func (p *patternGrid) CreateRenderer() fyne.WidgetRenderer {
	r := &patternGridRenderer{p: p}
	r.Refresh()
	return r
}

type patternGridRenderer struct {
	p       *patternGrid
	objects []fyne.CanvasObject
}

func (r *patternGridRenderer) Destroy() {}

func (r *patternGridRenderer) MinSize() fyne.Size { return r.p.MinSize() }

func (r *patternGridRenderer) Objects() []fyne.CanvasObject { return r.objects }

func (r *patternGridRenderer) Layout(size fyne.Size) { r.build(size) }

func (r *patternGridRenderer) Refresh() {
	size := r.p.Size()
	if size.Width <= 0 || size.Height <= 0 {
		size = r.p.MinSize()
	}
	r.build(size)
	canvas.Refresh(r.p)
}

// build desenha a grade inteira: primeiro as linhas do caminho, depois os
// pontos por cima, para o traco nao cobrir o circulo.
func (r *patternGridRenderer) build(size fyne.Size) {
	tokens := tk()
	cell := fyne.Min(size.Width, size.Height) / 3
	offsetX := (size.Width - cell*3) / 2
	offsetY := (size.Height - cell*3) / 2
	center := func(dot int) fyne.Position {
		row := float32((dot - 1) / 3)
		col := float32((dot - 1) % 3)
		return fyne.NewPos(offsetX+cell*col+cell/2, offsetY+cell*row+cell/2)
	}

	shown := r.p.dots
	if r.p.replay >= 0 && r.p.replay < len(shown) {
		shown = shown[:r.p.replay]
	}
	order := make(map[int]int, len(shown))
	for i, dot := range shown {
		order[dot] = i + 1
	}

	objects := make([]fyne.CanvasObject, 0, 9+len(shown)*2)
	var labels []fyne.CanvasObject
	for i := 1; i < len(shown); i++ {
		from, to := center(shown[i-1]), center(shown[i])
		line := canvas.NewLine(tokens.Brand)
		line.StrokeWidth = 3
		if r.p.compact {
			line.StrokeWidth = 2
		}
		line.Position1 = from
		line.Position2 = to
		objects = append(objects, line)
	}

	radius := cell * 0.17
	if r.p.compact {
		radius = cell * 0.22
	}
	for dot := 1; dot <= 9; dot++ {
		pos := center(dot)
		circle := canvas.NewCircle(color.Transparent)
		circle.StrokeWidth = 2
		circle.StrokeColor = tokens.Border
		if step, used := order[dot]; used {
			circle.FillColor = tokens.Brand
			circle.StrokeColor = tokens.Brand
			if r.p.compact {
				circle.Move(fyne.NewPos(pos.X-radius, pos.Y-radius))
				circle.Resize(fyne.NewSize(radius*2, radius*2))
				objects = append(objects, circle)
				continue
			}
			label := canvas.NewText(strconv.Itoa(step), tokens.InkOnBrand)
			label.TextSize = sizeCaption
			label.TextStyle = fyne.TextStyle{Bold: true}
			label.Alignment = fyne.TextAlignCenter
			labelSize := label.MinSize()
			label.Move(fyne.NewPos(pos.X-labelSize.Width/2, pos.Y-labelSize.Height/2))
			label.Resize(labelSize)
			// Os numeros vao por ultimo, depois de todos os circulos, senao o
			// ponto seguinte cobre o numero do anterior.
			labels = append(labels, label)
		} else if dot == r.p.hover {
			circle.FillColor = tokens.SurfaceRaised
		}
		circle.Move(fyne.NewPos(pos.X-radius, pos.Y-radius))
		circle.Resize(fyne.NewSize(radius*2, radius*2))
		objects = append(objects, circle)
	}
	r.objects = append(objects, labels...)
}

// ---------------------------------------------------------------------------
// A caixa de desenho, ligada ao formulario da OS
// ---------------------------------------------------------------------------

// showPatternDialog abre a grade para desenhar ou conferir o padrao do
// aparelho. onSave recebe o valor ja codificado, ou vazio quando o operador
// apagou o desenho.
func (ui *mainUI) showPatternDialog(current string, onSave func(value string)) {
	grid := newPatternGrid(true)
	if dots, ok := decodePattern(current); ok {
		grid.SetDots(dots)
	}

	status := newText("", sizeCaption, false, inkMuted)
	var save *dbsButton
	refresh := func(dots []int) {
		switch {
		case len(dots) == 0:
			status.SetText("Toque os pontos na ordem em que o cliente desenha.")
			status.SetInk(inkMuted)
		case len(dots) < patternMinDots:
			status.SetText(fmt.Sprintf("%d pontos. O Android exige pelo menos %d.", len(dots), patternMinDots))
			status.SetInk(inkWarning)
		default:
			status.SetText(fmt.Sprintf("%d pontos. Toque no primeiro de novo para recomeçar.", len(dots)))
			status.SetInk(inkSecondary)
		}
		if save != nil {
			if len(dots) == 0 || len(dots) >= patternMinDots {
				save.Enable()
			} else {
				save.Disable()
			}
		}
	}
	grid.OnChanged = refresh

	var patternDialog *dialog.CustomDialog
	save = primaryButton("Salvar desenho", theme.ConfirmIcon(), func() {
		patternDialog.Hide()
		onSave(encodePattern(grid.Dots()))
	})
	replay := ghostButton("Repetir movimento", theme.MediaPlayIcon(), func() { grid.Replay() })
	clear := ghostButton("Limpar", theme.ContentClearIcon(), func() { grid.Clear(); refresh(nil) })
	cancel := ghostButton("Cancelar", nil, func() { patternDialog.Hide() })

	body := container.NewVBox(
		mutedLabel("O desenho fica só no sistema e nunca sai no comprovante."),
		gap(1, 8),
		container.NewCenter(container.NewStack(gap(220, 220), grid)),
		gap(1, 8),
		status,
		gap(1, 4),
		container.NewCenter(container.NewHBox(replay, clear)),
	)
	refresh(grid.Dots())

	patternDialog = dialog.NewCustom("Padrão de desbloqueio", "", body, ui.window)
	patternDialog.Resize(fyne.NewSize(420, 560))
	patternDialog.SetButtons([]fyne.CanvasObject{cancel, save})
	patternDialog.Show()
}

// setDeviceKey coloca o valor no formulario. O campo de texto mostra o resumo
// quando o valor e um desenho, porque "padrao:1-2-5-9" nao diz nada para quem
// esta no balcao; o valor de verdade fica em currentDeviceKey.
func (ui *mainUI) setDeviceKey(value string) {
	ui.currentDeviceKey = strings.TrimSpace(value)
	if ui.deviceKeyEntry == nil {
		return
	}
	dots, isPattern := decodePattern(ui.currentDeviceKey)
	if isPattern {
		ui.deviceKeyEntry.SetText(patternSummary(ui.currentDeviceKey))
		ui.deviceKeyEntry.Disable()
		if ui.patternBtn != nil {
			ui.patternBtn.SetText("Ver padrão")
		}
	} else {
		ui.deviceKeyEntry.SetText(ui.currentDeviceKey)
		ui.deviceKeyEntry.Enable()
		if ui.patternBtn != nil {
			ui.patternBtn.SetText("Desenhar padrão")
		}
	}
	if ui.deviceKeyRow == nil {
		return
	}
	if isPattern {
		thumb := newPatternThumb(dots, 52)
		resumo := newText(patternSummary(ui.currentDeviceKey), sizeBody, true, inkPrimary)
		dica := newText("Toque no desenho para repetir o movimento", sizeCaption, false, inkMuted)
		texto := container.NewCenter(container.NewVBox(resumo, dica))
		ui.deviceKeyRow.Objects = []fyne.CanvasObject{
			container.NewBorder(nil, nil, paddedTRBL(thumb, 0, 12, 0, 0), ui.patternBtn, texto),
		}
	} else {
		ui.deviceKeyRow.Objects = []fyne.CanvasObject{
			container.NewBorder(nil, nil, nil, ui.patternBtn, ui.deviceKeyEntry),
		}
	}
	ui.deviceKeyRow.Refresh()
	ui.refreshNotePasswordHint()
}

// deviceKeyValue e o que vai para o banco.
func (ui *mainUI) deviceKeyValue() string {
	if isPatternKey(ui.currentDeviceKey) {
		return ui.currentDeviceKey
	}
	if ui.deviceKeyEntry == nil {
		return strings.TrimSpace(ui.currentDeviceKey)
	}
	return strings.TrimSpace(ui.deviceKeyEntry.Text)
}
