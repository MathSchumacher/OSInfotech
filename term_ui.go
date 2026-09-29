package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/storage"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
)

var salesTermTypes = []string{
	defaultTermType,
	"Compra de equipamento usado",
	"Venda de equipamento usado",
	"Termo personalizado",
}

const (
	termFilterAll               = "Todos"
	termFilterDrafts            = "Rascunhos"
	termFilterAwaitingSignature = "Aguardando assinatura"
	termFilterSigned            = "Assinados"
	termFilterWithoutPDF        = "Sem PDF"
)

var termDocumentFilters = []string{
	termFilterAll,
	termFilterDrafts,
	termFilterAwaitingSignature,
	termFilterSigned,
	termFilterWithoutPDF,
}

func (ui *mainUI) cnpjLookupField(entry *dbsEntry, tapped func()) fyne.CanvasObject {
	button := ghostButton("Consultar", theme.SearchIcon(), tapped)
	return container.NewBorder(nil, nil, nil, button, entry)
}

func (ui *mainUI) applyTermDefaultsToForm(defaults TermDefaults) {
	ui.termTypeSelect.SetSelected(firstNonEmpty(defaults.TermType, defaultTermType))
	ui.termSellerNameEntry.SetText(defaults.SellerName)
	ui.termSellerCPFEntry.SetText(defaults.SellerDocument)
	ui.termSellerRGEntry.SetText(defaults.SellerRG)
	ui.termSellerPhoneEntry.SetText(defaults.SellerPhone)
	ui.termSellerAddressEntry.SetText(defaults.SellerAddress)
	ui.termBuyerCompanyEntry.SetText(defaults.BuyerCompany)
	ui.termBuyerDocumentEntry.SetText(defaults.BuyerDocument)
	ui.termBuyerAddressEntry.SetText(defaults.BuyerAddress)
	ui.termBuyerPhoneEntry.SetText(defaults.BuyerPhone)
	ui.termAmountEntry.SetText(formatBRL(defaults.AmountCents))
	ui.termPaymentMethodEntry.SetText(defaults.PaymentMethod)
	ui.termCityEntry.SetText(defaults.City)
	ui.termDeclarationEntry.SetText(firstNonEmpty(defaults.Declaration, defaultTermDeclaration()))
}

func (ui *mainUI) lookupTermCNPJ(target string) {
	switch target {
	case "seller":
		ui.lookupCNPJInto(ui.termSellerCPFEntry, ui.termSellerNameEntry, ui.termSellerPhoneEntry, ui.termSellerAddressEntry, func() {
			ui.suppressTermLookup = true
		}, func() {
			ui.suppressTermLookup = false
			ui.refreshTermPreview()
		})
	case "buyer":
		ui.lookupCNPJInto(ui.termBuyerDocumentEntry, ui.termBuyerCompanyEntry, ui.termBuyerPhoneEntry, ui.termBuyerAddressEntry, nil, func() {
			ui.refreshTermPreview()
		})
	}
}

func (ui *mainUI) lookupCNPJInto(documentEntry *dbsEntry, nameEntry *dbsEntry, phoneEntry *dbsEntry, addressEntry *dbsEntry, beforeApply func(), afterApply func()) {
	document := strings.TrimSpace(documentEntry.Text)
	if len(onlyDigits(document)) != 14 {
		dialog.ShowError(errors.New("informe um CNPJ com 14 digitos"), ui.window)
		return
	}

	status := widget.NewLabel("Buscando CNPJ...")
	statusDialog := dialog.NewCustom("Consulta CNPJ", "Fechar", status, ui.window)
	statusDialog.Show()

	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
		defer cancel()

		company, err := lookupCNPJ(ctx, document)
		fyne.Do(func() {
			if err != nil {
				status.SetText("Nao foi possivel consultar o CNPJ.")
				dialog.ShowError(err, ui.window)
				return
			}
			if beforeApply != nil {
				beforeApply()
			}
			documentEntry.SetText(company.Document)
			if company.Name != "" {
				nameEntry.SetText(company.Name)
			}
			if company.Phone != "" {
				phoneEntry.SetText(company.Phone)
			}
			if company.Address != "" {
				addressEntry.SetText(company.Address)
			}
			if afterApply != nil {
				afterApply()
			}
			statusDialog.Hide()
		})
	}()
}

func (ui *mainUI) collectTerm() (ServiceTerm, error) {
	seller := strings.TrimSpace(ui.termSellerNameEntry.Text)
	device := strings.TrimSpace(ui.termDeviceKindEntry.Text)
	if seller == "" {
		return ServiceTerm{}, errors.New("informe o nome do vendedor")
	}
	if device == "" {
		return ServiceTerm{}, errors.New("informe o equipamento")
	}

	amount, err := parseBRL(ui.termAmountEntry.Text)
	if err != nil {
		return ServiceTerm{}, err
	}
	termDate, err := parseTermDate(ui.termDateEntry.Text)
	if err != nil {
		return ServiceTerm{}, err
	}

	term := ServiceTerm{ID: ui.currentTermID}
	if ui.currentTermID != 0 {
		existing, err := ui.store.GetTerm(context.Background(), ui.currentTermID)
		if err != nil {
			return ServiceTerm{}, err
		}
		term.PDFKey = existing.PDFKey
		term.PDFURL = existing.PDFURL
		term.SignedKey = existing.SignedKey
		term.SignedURL = existing.SignedURL
	}

	term.TermType = firstNonEmpty(ui.termTypeSelect.Selected, defaultTermType)
	term.Status = firstNonEmpty(ui.termStatusSelect.Selected, termStatusDraft)
	term.SellerName = seller
	term.SellerCPF = strings.TrimSpace(ui.termSellerCPFEntry.Text)
	term.SellerRG = strings.TrimSpace(ui.termSellerRGEntry.Text)
	term.SellerPhone = strings.TrimSpace(ui.termSellerPhoneEntry.Text)
	term.SellerAddress = strings.TrimSpace(ui.termSellerAddressEntry.Text)
	term.BuyerCompany = strings.TrimSpace(ui.termBuyerCompanyEntry.Text)
	term.BuyerDocument = strings.TrimSpace(ui.termBuyerDocumentEntry.Text)
	term.BuyerAddress = strings.TrimSpace(ui.termBuyerAddressEntry.Text)
	term.BuyerPhone = strings.TrimSpace(ui.termBuyerPhoneEntry.Text)
	term.DeviceKind = device
	term.Brand = strings.TrimSpace(ui.termBrandEntry.Text)
	term.Model = strings.TrimSpace(ui.termModelEntry.Text)
	term.Color = strings.TrimSpace(ui.termColorEntry.Text)
	term.IMEI1 = strings.TrimSpace(ui.termIMEI1Entry.Text)
	term.IMEI2 = strings.TrimSpace(ui.termIMEI2Entry.Text)
	term.Accessories = strings.TrimSpace(ui.termAccessoriesEntry.Text)
	term.Declaration = firstNonEmpty(ui.termDeclarationEntry.Text, defaultTermDeclaration())
	term.AmountCents = amount
	term.PaymentMethod = strings.TrimSpace(ui.termPaymentMethodEntry.Text)
	term.City = strings.TrimSpace(ui.termCityEntry.Text)
	term.TermDate = termDate
	return term, nil
}

func (ui *mainUI) refreshTermPreview() {
	if ui.termPreview == nil {
		return
	}
	ui.termPreview.Update(ui.collectTermPreview())
}

func (ui *mainUI) collectTermPreview() ServiceTerm {
	amount, _ := parseBRL(ui.termAmountEntry.Text)
	termDate, _ := parseTermDate(ui.termDateEntry.Text)
	return ServiceTerm{
		ID:            ui.currentTermID,
		TermType:      firstNonEmpty(ui.termTypeSelect.Selected, defaultTermType),
		Status:        firstNonEmpty(ui.termStatusSelect.Selected, termStatusDraft),
		SellerName:    strings.TrimSpace(ui.termSellerNameEntry.Text),
		SellerCPF:     strings.TrimSpace(ui.termSellerCPFEntry.Text),
		SellerRG:      strings.TrimSpace(ui.termSellerRGEntry.Text),
		SellerPhone:   strings.TrimSpace(ui.termSellerPhoneEntry.Text),
		SellerAddress: strings.TrimSpace(ui.termSellerAddressEntry.Text),
		BuyerCompany:  strings.TrimSpace(ui.termBuyerCompanyEntry.Text),
		BuyerDocument: strings.TrimSpace(ui.termBuyerDocumentEntry.Text),
		BuyerAddress:  strings.TrimSpace(ui.termBuyerAddressEntry.Text),
		BuyerPhone:    strings.TrimSpace(ui.termBuyerPhoneEntry.Text),
		DeviceKind:    strings.TrimSpace(ui.termDeviceKindEntry.Text),
		Brand:         strings.TrimSpace(ui.termBrandEntry.Text),
		Model:         strings.TrimSpace(ui.termModelEntry.Text),
		Color:         strings.TrimSpace(ui.termColorEntry.Text),
		IMEI1:         strings.TrimSpace(ui.termIMEI1Entry.Text),
		IMEI2:         strings.TrimSpace(ui.termIMEI2Entry.Text),
		Accessories:   strings.TrimSpace(ui.termAccessoriesEntry.Text),
		Declaration:   firstNonEmpty(ui.termDeclarationEntry.Text, defaultTermDeclaration()),
		AmountCents:   amount,
		PaymentMethod: strings.TrimSpace(ui.termPaymentMethodEntry.Text),
		City:          strings.TrimSpace(ui.termCityEntry.Text),
		TermDate:      termDate,
	}
}

func (ui *mainUI) saveTerm() {
	term, err := ui.saveTermData()
	if err != nil {
		dialog.ShowError(err, ui.window)
		return
	}
	ui.notify(fmt.Sprintf("Termo %06d salvo", term.ID))
}

func (ui *mainUI) saveTermData() (ServiceTerm, error) {
	term, err := ui.collectTerm()
	if err != nil {
		return ServiceTerm{}, err
	}
	if err := ui.store.SaveTerm(context.Background(), &term); err != nil {
		return ServiceTerm{}, err
	}

	ui.currentTermID = term.ID
	ui.termAmountEntry.SetText(formatBRL(term.AmountCents))
	ui.termDetailTitle.SetText(fmt.Sprintf("Termo %06d", term.ID))
	ui.termDetailMeta.SetText(fmt.Sprintf("Criado em %s - Atualizado em %s", formatDate(term.CreatedAt), formatDate(term.UpdatedAt)))
	ui.updateTermDocumentLabels(term)
	ui.refreshTerms()
	ui.syncD1Async(false)
	return term, nil
}

func (ui *mainUI) deleteTerm() {
	// Mesma regra da OS e do cliente: excluir e so do administrador.
	if !ui.requireAdmin("excluir termos") {
		return
	}
	if ui.currentTermID == 0 {
		ui.notifyTone("Abra um termo para excluir.", toneWarning)
		return
	}

	id := ui.currentTermID
	dialog.ShowConfirm("Excluir termo", fmt.Sprintf("Excluir o termo %06d? Os arquivos ja enviados ao R2 nao serao removidos.", id), func(ok bool) {
		if !ok {
			return
		}
		if err := ui.store.DeleteTerm(context.Background(), id); err != nil {
			dialog.ShowError(err, ui.window)
			return
		}
		ui.clearTermForm()
		ui.showPage(pageTerms)
		ui.syncD1Async(false)
		ui.notify(fmt.Sprintf("Termo %06d excluído", id))
	}, ui.window)
}

func (ui *mainUI) generateTermPDF() {
	term, err := ui.saveTermData()
	if err != nil {
		dialog.ShowError(err, ui.window)
		return
	}

	outDir := filepath.Join(filepath.Dir(ui.dbPath), "termos", "gerados")
	path, err := writeTermPDF(term, outDir)
	if err != nil {
		dialog.ShowError(err, ui.window)
		return
	}

	ui.termPDFStatus.SetText("PDF gerado localmente. Enviando para R2...")
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
		defer cancel()

		key := generatedTermPDFObjectKey(term.ID)
		publicURL, err := ui.uploadDocumentToR2(ctx, key, path)
		if err == nil {
			err = ui.store.AttachTermPDF(context.Background(), term.ID, key, publicURL)
		}

		fyne.Do(func() {
			if err != nil {
				ui.termPDFStatus.SetText("PDF local gerado. Falha no upload R2.")
				dialog.ShowError(err, ui.window)
				ui.showGeneratedFileDialog("Termo gerado", "PDF gerado localmente (o envio para a nuvem falhou).", path, "")
				return
			}
			updated, loadErr := ui.store.GetTerm(context.Background(), term.ID)
			if loadErr == nil {
				ui.loadTerm(updated)
				ui.refreshTerms()
			}
			ui.termPDFStatus.SetText("PDF salvo no R2.")
			ui.showGeneratedFileDialog("Termo gerado", "PDF pronto para impressão e assinatura.", path, publicURL)
		})
	}()
}

func (ui *mainUI) openTermPDF() {
	if ui.currentTermID == 0 {
		ui.notifyTone("Salve e gere o termo antes de abrir o PDF.", toneWarning)
		return
	}
	term, err := ui.store.GetTerm(context.Background(), ui.currentTermID)
	if err != nil {
		dialog.ShowError(err, ui.window)
		return
	}
	url := strings.TrimSpace(term.PDFURL)
	if url == "" && strings.TrimSpace(term.PDFKey) != "" {
		url = ui.documentPublicURL(term.PDFKey)
	}
	if url != "" {
		if err := openExternalURL(url); err != nil {
			dialog.ShowError(err, ui.window)
		}
		return
	}
	path := filepath.Join(filepath.Dir(ui.dbPath), "termos", "gerados", fmt.Sprintf("TERMO-%06d.pdf", term.ID))
	if _, err := os.Stat(path); err != nil {
		ui.notifyTone("Nenhum PDF gerado para este termo ainda.", toneWarning)
		return
	}
	uri, err := fileURL(path)
	if err != nil {
		dialog.ShowError(err, ui.window)
		return
	}
	if err := fyne.CurrentApp().OpenURL(uri); err != nil {
		dialog.ShowError(err, ui.window)
	}
}

func (ui *mainUI) openSignedTerm() {
	if ui.currentTermID == 0 {
		ui.notifyTone("Salve o termo antes de abrir o documento assinado.", toneWarning)
		return
	}
	term, err := ui.store.GetTerm(context.Background(), ui.currentTermID)
	if err != nil {
		dialog.ShowError(err, ui.window)
		return
	}
	url := strings.TrimSpace(term.SignedURL)
	if url == "" && strings.TrimSpace(term.SignedKey) != "" {
		url = ui.documentPublicURL(term.SignedKey)
	}
	if url != "" {
		if err := openExternalURL(url); err != nil {
			dialog.ShowError(err, ui.window)
		}
		return
	}
	if strings.TrimSpace(term.SignedKey) != "" {
		path := filepath.Join(filepath.Dir(ui.dbPath), "termos", "assinados", filepath.Base(term.SignedKey))
		if _, err := os.Stat(path); err == nil {
			uri, err := fileURL(path)
			if err != nil {
				dialog.ShowError(err, ui.window)
				return
			}
			if err := fyne.CurrentApp().OpenURL(uri); err != nil {
				dialog.ShowError(err, ui.window)
			}
			return
		}
	}
	ui.notifyTone("Nenhum documento assinado anexado a este termo.", toneWarning)
}

func (ui *mainUI) attachSignedTerm() {
	term, err := ui.saveTermData()
	if err != nil {
		dialog.ShowError(err, ui.window)
		return
	}

	fileDialog := dialog.NewFileOpen(func(reader fyne.URIReadCloser, err error) {
		if err != nil {
			dialog.ShowError(err, ui.window)
			return
		}
		if reader == nil {
			return
		}
		defer reader.Close()

		const maxDocumentBytes = 40 * 1024 * 1024
		data, err := io.ReadAll(io.LimitReader(reader, maxDocumentBytes+1))
		if err != nil {
			dialog.ShowError(err, ui.window)
			return
		}
		if len(data) > maxDocumentBytes {
			dialog.ShowError(errors.New("arquivo muito grande para anexar"), ui.window)
			return
		}

		ext := strings.ToLower(reader.URI().Extension())
		if ext == "" {
			ext = strings.ToLower(filepath.Ext(reader.URI().Name()))
		}
		switch ext {
		case ".pdf", ".png", ".jpg", ".jpeg", ".webp":
		default:
			dialog.ShowError(errors.New("use PDF ou imagem PNG/JPG/WebP"), ui.window)
			return
		}

		dir := filepath.Join(filepath.Dir(ui.dbPath), "termos", "assinados")
		if err := os.MkdirAll(dir, 0o755); err != nil {
			dialog.ShowError(err, ui.window)
			return
		}
		localPath := filepath.Join(dir, fmt.Sprintf("TERMO-%06d-assinado-%s%s", term.ID, time.Now().Format("20060102-150405"), ext))
		if err := os.WriteFile(localPath, data, 0o644); err != nil {
			dialog.ShowError(err, ui.window)
			return
		}

		ui.termSignedStatus.SetText("Enviando assinado/escaneado para R2...")
		go func() {
			ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
			defer cancel()

			key := signedTermObjectKey(term.ID, localPath)
			publicURL, err := ui.uploadDocumentToR2(ctx, key, localPath)
			if err == nil {
				err = ui.store.AttachSignedTerm(context.Background(), term.ID, key, publicURL)
			}

			fyne.Do(func() {
				if err != nil {
					ui.termSignedStatus.SetText("Falha no upload do assinado.")
					dialog.ShowError(err, ui.window)
					return
				}
				updated, loadErr := ui.store.GetTerm(context.Background(), term.ID)
				if loadErr == nil {
					ui.loadTerm(updated)
					ui.refreshTerms()
				}
				ui.termSignedStatus.SetText("Documento assinado salvo no R2.")
				ui.notify("Documento assinado salvo na nuvem")
			})
		}()
	}, ui.window)
	fileDialog.SetFilter(storage.NewExtensionFileFilter([]string{".pdf", ".png", ".jpg", ".jpeg", ".webp"}))
	fileDialog.Show()
}

func (ui *mainUI) updateTermDocumentLabels(term ServiceTerm) {
	if term.PDFURL != "" {
		ui.termPDFStatus.SetText("PDF gerado e salvo no R2.")
	} else if term.PDFKey != "" {
		ui.termPDFStatus.SetText("PDF gerado.")
	} else {
		ui.termPDFStatus.SetText("PDF nao gerado")
	}

	if term.SignedURL != "" {
		ui.termSignedStatus.SetText("Assinado/escaneado salvo no R2.")
	} else if term.SignedKey != "" {
		ui.termSignedStatus.SetText("Assinado anexado.")
	} else {
		ui.termSignedStatus.SetText("Assinado nao anexado")
	}
}

func parseTermDate(input string) (time.Time, error) {
	text := strings.TrimSpace(input)
	if text == "" {
		return time.Now(), nil
	}
	layouts := []string{"02/01/2006", "2/1/2006", "2006-01-02", "02/01/06"}
	for _, layout := range layouts {
		if parsed, err := time.ParseInLocation(layout, text, time.Local); err == nil {
			return parsed, nil
		}
	}
	return time.Time{}, errors.New("data invalida: use dd/mm/aaaa")
}

func openExternalURL(rawURL string) error {
	parsed, err := url.Parse(strings.TrimSpace(rawURL))
	if err != nil {
		return err
	}
	return fyne.CurrentApp().OpenURL(parsed)
}

func termDocumentSummary(term ServiceTerm) string {
	hasPDF := strings.TrimSpace(term.PDFKey) != "" || strings.TrimSpace(term.PDFURL) != ""
	hasSigned := strings.TrimSpace(term.SignedKey) != "" || strings.TrimSpace(term.SignedURL) != ""
	switch {
	case hasPDF && hasSigned:
		return "PDF normal + assinado anexado"
	case hasSigned:
		return "Assinado anexado"
	case hasPDF:
		return "PDF pronto para imprimir, aguardando assinatura"
	default:
		return "Sem PDF gerado"
	}
}
