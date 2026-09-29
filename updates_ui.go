package main

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"strings"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
)

func (ui *mainUI) autoCheckUpdates() {
	go func() {
		if _, err := ui.updateManifestURL(context.Background()); err != nil {
			return
		}
		ui.checkUpdates(false)
	}()
}

// checkUpdates consulta o manifesto; manual = avisa tambem quando nao ha nada.
func (ui *mainUI) checkUpdates(manual bool) {
	go func() {
		value, err := ui.updateManifestURL(context.Background())
		if err != nil || strings.TrimSpace(value) == "" {
			if manual {
				fyne.Do(func() { ui.notifyTone("Não foi possível verificar atualizações.", toneWarning) })
			}
			return
		}

		ctx, cancel := context.WithTimeout(context.Background(), 25*time.Second)
		defer cancel()

		manifest, hasUpdate, err := checkForUpdate(ctx, value, appVersion)
		if err != nil {
			if manual {
				fyne.Do(func() { ui.notifyTone("Não foi possível verificar atualizações.", toneWarning) })
			}
			return
		}
		if !hasUpdate {
			if manual {
				fyne.Do(func() {
					ui.notify("Você já está na versão mais recente (v" + appVersion + ")")
					if ui.settingsPage != nil && ui.settingsPage.updateStatus != nil {
						ui.settingsPage.updateStatus.SetText("Nenhuma atualização pendente.")
					}
				})
			}
			return
		}
		fyne.Do(func() {
			if ui.settingsPage != nil && ui.settingsPage.updateStatus != nil {
				ui.settingsPage.updateStatus.SetText("Versão " + manifest.Version + " disponível.")
			}
			ui.showUpdateAvailable(manifest)
		})
	}()
}

func (ui *mainUI) updateManifestURL(ctx context.Context) (string, error) {
	// No Android o app se atualiza via APK completo (OTA), com manifesto proprio.
	if isMobileLayout() {
		value, ok, err := ui.store.GetSetting(ctx, androidUpdateManifestURLSetting)
		if err != nil {
			return "", err
		}
		if ok && strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value), nil
		}
		return defaultAndroidUpdateManifestURL, nil
	}

	value, ok, err := ui.store.GetSetting(ctx, updateManifestURLSetting)
	if err != nil {
		return "", err
	}
	if ok && strings.TrimSpace(value) != "" {
		return strings.TrimSpace(value), nil
	}
	return defaultUpdateManifestURL, nil
}

func (ui *mainUI) showUpdateAvailable(manifest UpdateManifest) {
	if isMobileLayout() {
		ui.showAndroidUpdateAvailable(manifest)
		return
	}

	title := newText("Versão "+manifest.Version+" disponível", sizeSection, true, inkPrimary)
	notes := mutedLabel(firstNonEmpty(manifest.Notes, "Novidades e correções para o sistema."))
	status := widget.NewLabel("")
	status.Importance = widget.LowImportance

	installButton := primaryButton("Baixar e instalar", theme.DownloadIcon(), nil)
	installButton.OnTapped = func() {
		installButton.Disable()
		status.SetText("Baixando instalador…")
		go func() {
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
			defer cancel()

			_, err := downloadAndRunSetup(ctx, manifest)
			if err != nil {
				fyne.Do(func() {
					installButton.Enable()
					status.SetText("Falha ao instalar.")
					dialog.ShowError(err, ui.window)
				})
				return
			}
			fyne.Do(func() {
				status.SetText("Instalador iniciado. O app será fechado para atualizar.")
				fyne.CurrentApp().Quit()
			})
		}()
	}

	d := dialog.NewCustom("Atualização disponível", "Depois", container.NewVBox(title, notes, gap(1, 6), installButton, status), ui.window)
	d.Resize(fyne.NewSize(460, 220))
	d.Show()
}

// showAndroidUpdateAvailable abre o fluxo de atualizacao por APK completo (OTA).
// O Android nao permite trocar codigo em runtime; entao baixamos/abrimos o APK
// publicado no R2 e o proprio sistema Android instala por cima da versao atual.
func (ui *mainUI) showAndroidUpdateAvailable(manifest UpdateManifest) {
	apkURL := strings.TrimSpace(manifest.APKURL)
	if apkURL == "" {
		ui.notifyTone("A atualização não possui APK para download.", toneWarning)
		return
	}

	msg := fmt.Sprintf("Versão %s disponível.", manifest.Version)
	if manifest.Mandatory {
		msg += " (obrigatória)"
	}
	title := newText(msg, sizeSection, true, inkPrimary)
	notes := mutedLabel(firstNonEmpty(manifest.Notes, ""))
	hint := mutedLabel("Baixe o APK e confirme a instalação. Se pedir, permita \"instalar apps desconhecidos\" para o navegador.")
	status := widget.NewLabel("")
	status.Importance = widget.LowImportance

	installButton := primaryButton("Baixar e instalar APK", theme.DownloadIcon(), nil)
	installButton.OnTapped = func() {
		parsed, err := url.Parse(apkURL)
		if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") {
			dialog.ShowError(errors.New("URL do APK inválida"), ui.window)
			return
		}
		if err := fyne.CurrentApp().OpenURL(parsed); err != nil {
			status.SetText("Não foi possível abrir o download.")
			dialog.ShowError(err, ui.window)
			return
		}
		status.SetText("Download iniciado. Toque no arquivo baixado para instalar.")
	}

	dialog.ShowCustom("Atualização disponível", "Depois", container.NewVBox(title, notes, installButton, hint, status), ui.window)
}
