package main

import (
	"image/color"
	"strings"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/layout"
)

type termPreview struct {
	root fyne.CanvasObject

	title       *canvas.Text
	sellerName  *previewField
	sellerCPF   *previewField
	sellerRG    *previewField
	sellerPhone *previewField
	sellerAddr  *previewField
	buyerName   *previewField
	buyerDoc    *previewField
	buyerAddr   *previewField
	buyerPhone  *previewField
	deviceKind  *previewField
	brand       *previewField
	model       *previewField
	deviceColor *previewField
	imei1       *previewField
	imei2       *previewField
	accessories *previewField
	amount      *previewField
	payment     *previewField
	city        *previewField
	date        *previewField
	declaration *previewParagraph
	footer      *canvas.Text
}

type previewField struct {
	value *canvas.Text
}

type previewParagraph struct {
	lines []*canvas.Text
}

func newTermPreview() *termPreview {
	p := &termPreview{}

	pageBg := canvas.NewRectangle(color.NRGBA{R: 255, G: 255, B: 255, A: 255})
	pageBg.StrokeColor = color.NRGBA{R: 192, G: 203, B: 214, A: 255}
	pageBg.StrokeWidth = 1
	pageBg.SetMinSize(fyne.NewSize(360, 510))

	p.title = previewText(defaultTermType, 11, true)
	p.title.Alignment = fyne.TextAlignCenter
	intro := previewText("Pelo presente instrumento particular, as partes abaixo identificadas realizam a compra e venda do equipamento descrito.", 7.5, false)

	var sellerNameObj, sellerCPFObj, sellerRGObj, sellerPhoneObj, sellerAddrObj fyne.CanvasObject
	p.sellerName, sellerNameObj = newPreviewField("Nome")
	p.sellerCPF, sellerCPFObj = newPreviewField("CPF/CNPJ")
	p.sellerRG, sellerRGObj = newPreviewField("RG")
	p.sellerPhone, sellerPhoneObj = newPreviewField("Telefone")
	p.sellerAddr, sellerAddrObj = newPreviewField("Endereco")

	var buyerNameObj, buyerDocObj, buyerAddrObj, buyerPhoneObj fyne.CanvasObject
	p.buyerName, buyerNameObj = newPreviewField("Empresa")
	p.buyerDoc, buyerDocObj = newPreviewField("CNPJ/CPF")
	p.buyerAddr, buyerAddrObj = newPreviewField("Endereco")
	p.buyerPhone, buyerPhoneObj = newPreviewField("Telefone")

	var deviceObj, brandObj, modelObj, colorObj, imei1Obj, imei2Obj, accessoriesObj fyne.CanvasObject
	p.deviceKind, deviceObj = newPreviewField("Tipo")
	p.brand, brandObj = newPreviewField("Marca")
	p.model, modelObj = newPreviewField("Modelo")
	p.deviceColor, colorObj = newPreviewField("Cor")
	p.imei1, imei1Obj = newPreviewField("IMEI/Serie 1")
	p.imei2, imei2Obj = newPreviewField("IMEI/Serie 2")
	p.accessories, accessoriesObj = newPreviewField("Acessorios")

	var amountObj, paymentObj, cityObj, dateObj fyne.CanvasObject
	p.amount, amountObj = newPreviewField("Valor")
	p.payment, paymentObj = newPreviewField("Forma de pagamento")
	p.city, cityObj = newPreviewField("Cidade")
	p.date, dateObj = newPreviewField("Data")

	p.declaration = newPreviewParagraph(4)
	p.footer = previewText("", 6.8, false)
	p.footer.Alignment = fyne.TextAlignCenter

	content := container.NewVBox(
		p.title,
		intro,
		previewSection("VENDEDOR",
			previewRow(sellerNameObj, sellerCPFObj),
			previewRow(sellerRGObj, sellerPhoneObj),
			sellerAddrObj,
		),
		previewSection("COMPRADOR",
			previewRow(buyerNameObj, buyerDocObj),
			buyerAddrObj,
			buyerPhoneObj,
		),
		previewNumberedSection("1. DADOS DO EQUIPAMENTO"),
		previewRow(deviceObj, brandObj, modelObj, colorObj),
		previewRow(imei1Obj, imei2Obj),
		accessoriesObj,
		previewNumberedSection("2. DECLARACAO"),
		p.declaration.object(),
		previewNumberedSection("3. VALOR DA VENDA"),
		previewRow(amountObj, paymentObj),
		previewRow(cityObj, dateObj),
		layout.NewSpacer(),
		previewSignatureRow(),
		p.footer,
	)

	page := container.NewStack(pageBg, container.NewPadded(content))
	surface := canvas.NewRectangle(color.NRGBA{R: 229, G: 235, B: 241, A: 255})
	p.root = container.NewStack(surface, container.NewPadded(container.NewCenter(page)))
	p.Update(ServiceTerm{TermType: defaultTermType, Declaration: defaultTermDeclaration()})
	return p
}

func (p *termPreview) CanvasObject() fyne.CanvasObject {
	return p.root
}

func (p *termPreview) Update(term ServiceTerm) {
	if p == nil {
		return
	}
	p.title.Text = strings.ToUpper(firstNonEmpty(term.TermType, defaultTermType))
	p.sellerName.Set(term.SellerName)
	p.sellerCPF.Set(term.SellerCPF)
	p.sellerRG.Set(term.SellerRG)
	p.sellerPhone.Set(term.SellerPhone)
	p.sellerAddr.Set(term.SellerAddress)
	p.buyerName.Set(term.BuyerCompany)
	p.buyerDoc.Set(term.BuyerDocument)
	p.buyerAddr.Set(term.BuyerAddress)
	p.buyerPhone.Set(term.BuyerPhone)
	p.deviceKind.Set(term.DeviceKind)
	p.brand.Set(term.Brand)
	p.model.Set(term.Model)
	p.deviceColor.Set(term.Color)
	p.imei1.Set(term.IMEI1)
	p.imei2.Set(term.IMEI2)
	p.accessories.Set(term.Accessories)
	p.declaration.Set(firstNonEmpty(term.Declaration, defaultTermDeclaration()))
	p.amount.Set(formatBRL(term.AmountCents))
	p.payment.Set(term.PaymentMethod)
	p.city.Set(term.City)
	p.date.Set(formatTermDate(term.TermDate))
	if term.ID > 0 {
		p.footer.Text = previewValue("Gerado pelo "+appDisplayName+" - Termo "+formatID(term.ID), 80)
	} else {
		p.footer.Text = "Gerado pelo " + appDisplayName
	}
	p.root.Refresh()
}

func (f *previewField) Set(value string) {
	f.value.Text = previewValue(value, 34)
	f.value.Refresh()
}

func newPreviewField(label string) (*previewField, fyne.CanvasObject) {
	caption := previewText(label, 6.8, true)
	caption.Color = color.NRGBA{R: 89, G: 102, B: 115, A: 255}
	value := previewText("", 8.2, false)
	line := previewHairline(color.NRGBA{R: 95, G: 107, B: 122, A: 255})
	field := &previewField{value: value}
	return field, container.NewVBox(caption, value, line)
}

func newPreviewParagraph(maxLines int) *previewParagraph {
	paragraph := &previewParagraph{lines: make([]*canvas.Text, maxLines)}
	for i := range paragraph.lines {
		paragraph.lines[i] = previewText("", 7.7, false)
	}
	return paragraph
}

func (p *previewParagraph) object() fyne.CanvasObject {
	objects := make([]fyne.CanvasObject, 0, len(p.lines))
	for _, line := range p.lines {
		objects = append(objects, line)
	}
	return container.NewVBox(objects...)
}

func (p *previewParagraph) Set(text string) {
	lines := wrapPreviewText(text, 92, len(p.lines))
	for i, line := range p.lines {
		if i < len(lines) {
			line.Text = lines[i]
		} else {
			line.Text = ""
		}
		line.Refresh()
	}
}

func previewSection(title string, rows ...fyne.CanvasObject) fyne.CanvasObject {
	headerBg := canvas.NewRectangle(color.NRGBA{R: 232, G: 238, B: 243, A: 255})
	headerBg.StrokeColor = color.NRGBA{R: 47, G: 59, B: 72, A: 255}
	headerBg.StrokeWidth = 1
	headerBg.SetMinSize(fyne.NewSize(1, 18))
	label := previewText(title, 8.5, true)
	objects := []fyne.CanvasObject{container.NewStack(headerBg, container.NewPadded(label))}
	objects = append(objects, rows...)
	return container.NewVBox(objects...)
}

func previewNumberedSection(title string) fyne.CanvasObject {
	label := previewText(title, 8.5, true)
	return container.NewPadded(label)
}

func previewRow(objects ...fyne.CanvasObject) fyne.CanvasObject {
	return container.NewGridWithColumns(len(objects), objects...)
}

func previewSignatureRow() fyne.CanvasObject {
	left := previewSignature("Assinatura do Vendedor")
	right := previewSignature("Assinatura do Comprador")
	return container.NewGridWithColumns(2, left, right)
}

func previewSignature(label string) fyne.CanvasObject {
	line := previewHairline(color.NRGBA{R: 47, G: 59, B: 72, A: 255})
	text := previewText(label, 7.2, false)
	text.Alignment = fyne.TextAlignCenter
	return container.NewVBox(line, text)
}

func previewHairline(c color.Color) *canvas.Rectangle {
	line := canvas.NewRectangle(c)
	line.SetMinSize(fyne.NewSize(1, 1))
	return line
}

func previewText(text string, size float32, bold bool) *canvas.Text {
	t := canvas.NewText(text, color.NRGBA{R: 24, G: 33, B: 43, A: 255})
	t.TextSize = size
	t.TextStyle = fyne.TextStyle{Bold: bold}
	return t
}

func previewValue(value string, maxRunes int) string {
	value = strings.Join(strings.Fields(strings.TrimSpace(value)), " ")
	if value == "" {
		return " "
	}
	runes := []rune(value)
	if len(runes) <= maxRunes {
		return value
	}
	if maxRunes <= 1 {
		return string(runes[:maxRunes])
	}
	return string(runes[:maxRunes-1]) + "..."
}

func wrapPreviewText(text string, maxRunes int, maxLines int) []string {
	words := strings.Fields(strings.TrimSpace(text))
	if len(words) == 0 || maxLines <= 0 {
		return nil
	}
	lines := make([]string, 0, maxLines)
	current := ""
	for _, word := range words {
		next := word
		if current != "" {
			next = current + " " + word
		}
		if len([]rune(next)) <= maxRunes {
			current = next
			continue
		}
		if current != "" {
			lines = append(lines, current)
		}
		current = word
		if len(lines) == maxLines {
			break
		}
	}
	if current != "" && len(lines) < maxLines {
		lines = append(lines, current)
	}
	if len(lines) == maxLines && len(words) > 0 {
		last := []rune(lines[len(lines)-1])
		if len(last) > 3 {
			lines[len(lines)-1] = string(last[:len(last)-3]) + "..."
		}
	}
	return lines
}

func formatID(id int64) string {
	return strings.TrimLeft(strings.TrimSpace(strings.ReplaceAll(formatThousands(id), ".", "")), "0")
}
