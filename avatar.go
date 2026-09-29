package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"image"
	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"
	"io"
	"net/http"
	"strings"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
	"github.com/chai2010/webp"
	xdraw "golang.org/x/image/draw"
	_ "golang.org/x/image/webp"
	"osinfotech/internal/r2client"
)

func (ui *mainUI) chooseAvatar() {
	name := strings.TrimSpace(ui.nameEntry.Text)
	if name == "" {
		dialog.ShowInformation("Foto do cliente", "Informe o nome do cliente antes de adicionar uma foto.", ui.window)
		return
	}
	ui.chooseAvatarFile(func(data []byte) {
		ui.processAndUploadAvatar(name, strings.TrimSpace(ui.cpfEntry.Text), strings.TrimSpace(ui.phoneEntry.Text), data, ui.avatarStatus, ui.setAvatarPreview, func(profile CustomerProfile) {
			ui.currentAvatarURL = profile.AvatarURL
			ui.currentAvatarKey = profile.AvatarKey
		})
	})
}

func (ui *mainUI) chooseClientAvatar() {
	name := strings.TrimSpace(ui.clientNameEntry.Text)
	if name == "" {
		dialog.ShowInformation("Foto do cliente", "Informe o nome do cliente antes de adicionar uma foto.", ui.window)
		return
	}
	ui.chooseAvatarFile(func(data []byte) {
		ui.processAndUploadAvatar(name, strings.TrimSpace(ui.clientCPFEntry.Text), strings.TrimSpace(ui.clientPhoneEntry.Text), data, ui.clientAvatarStatus, ui.setClientAvatarPreview, func(profile CustomerProfile) {
			ui.clientAvatarURL = profile.AvatarURL
			ui.clientAvatarKey = profile.AvatarKey
			ui.currentClientKey = profile.Key
			ui.refreshClients()
		})
	})
}

// chooseAvatarFile abre o seletor de imagens (com miniaturas no computador) e
// entrega o conteudo da foto escolhida.
func (ui *mainUI) chooseAvatarFile(done func([]byte)) {
	ui.pickImages("Escolher foto do cliente", false, func(images [][]byte) {
		if len(images) > 0 {
			done(images[0])
		}
	})
}

func (ui *mainUI) processAndUploadAvatar(name string, cpf string, phone string, data []byte, status *widget.Label, setPreview func(image.Image), done func(CustomerProfile)) {
	status.SetText("Preparando foto...")

	go func() {
		webpData, preview, err := makeAvatarWebP(data)
		if err != nil {
			fyne.Do(func() {
				status.SetText("Falha na foto.")
				dialog.ShowError(err, ui.window)
			})
			return
		}

		fyne.Do(func() {
			setPreview(preview)
			status.SetText("Enviando para R2...")
		})

		cfg, err := ui.loadR2Config(context.Background())
		if err != nil {
			fyne.Do(func() {
				status.SetText("R2 nao configurado.")
				dialog.ShowError(err, ui.window)
			})
			return
		}

		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
		defer cancel()

		client, err := r2client.New(ctx, cfg)
		if err != nil {
			fyne.Do(func() {
				status.SetText("R2 nao configurado.")
				dialog.ShowError(err, ui.window)
			})
			return
		}

		objectKey := avatarObjectKey(name)
		if err := client.EnsureRoom(ctx, map[string]int64{objectKey: int64(len(webpData))}, false); err != nil {
			fyne.Do(func() {
				status.SetText("Limite R2 atingido.")
				dialog.ShowError(err, ui.window)
			})
			return
		}

		if err := client.PutBytes(ctx, objectKey, webpData, "image/webp"); err != nil {
			fyne.Do(func() {
				status.SetText("Falha no upload.")
				dialog.ShowError(err, ui.window)
			})
			return
		}

		publicURL := client.PublicURL(objectKey)
		if publicURL == "" {
			fyne.Do(func() {
				status.SetText("Foto enviada, mas Public URL nao configurada.")
			})
			return
		}

		profile := CustomerProfile{
			Key:       normalizeCustomerName(name),
			Name:      name,
			CPF:       cpf,
			Phone:     phone,
			AvatarURL: publicURL,
			AvatarKey: objectKey,
		}
		if err := ui.store.SaveCustomerAvatar(context.Background(), profile); err != nil {
			fyne.Do(func() {
				status.SetText("Upload OK, falha ao salvar perfil.")
				dialog.ShowError(err, ui.window)
			})
			return
		}

		fyne.Do(func() {
			done(profile)
			status.SetText("Foto salva em WebP.")
		})
	}()
}

func makeAvatarWebP(data []byte) ([]byte, image.Image, error) {
	src, _, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		return nil, nil, fmt.Errorf("imagem invalida: %w", err)
	}

	b := src.Bounds()
	w, h := b.Dx(), b.Dy()
	if w <= 0 || h <= 0 {
		return nil, nil, errors.New("imagem sem dimensoes validas")
	}

	side := w
	if h < side {
		side = h
	}
	x0 := b.Min.X + (w-side)/2
	y0 := b.Min.Y + (h-side)/2
	srcRect := image.Rect(x0, y0, x0+side, y0+side)

	dst := image.NewRGBA(image.Rect(0, 0, 512, 512))
	xdraw.CatmullRom.Scale(dst, dst.Bounds(), src, srcRect, xdraw.Over, nil)

	var out bytes.Buffer
	if err := webp.Encode(&out, dst, &webp.Options{Quality: 82}); err != nil {
		return nil, nil, err
	}
	return out.Bytes(), dst, nil
}

func (ui *mainUI) setAvatarPreview(img image.Image) {
	ui.avatarImage.Image = img
	ui.avatarImage.Resource = nil
	ui.avatarImage.Refresh()
}

func (ui *mainUI) clearAvatarPreview() {
	ui.avatarImage.Image = nil
	ui.avatarImage.Resource = theme.AccountIcon()
	ui.avatarImage.Refresh()
	ui.avatarStatus.SetText("Sem foto")
	ui.currentAvatarURL = ""
	ui.currentAvatarKey = ""
}

func (ui *mainUI) setClientAvatarPreview(img image.Image) {
	ui.clientAvatarImage.Image = img
	ui.clientAvatarImage.Resource = nil
	ui.clientAvatarImage.Refresh()
}

func (ui *mainUI) clearClientAvatarPreview() {
	ui.clientAvatarImage.Image = nil
	ui.clientAvatarImage.Resource = theme.AccountIcon()
	ui.clientAvatarImage.Refresh()
	ui.clientAvatarStatus.SetText("Sem foto")
	ui.clientAvatarURL = ""
	ui.clientAvatarKey = ""
}

func (ui *mainUI) loadAvatarPreviewFromURL(rawURL string) {
	rawURL = strings.TrimSpace(rawURL)
	if rawURL == "" {
		ui.clearAvatarPreview()
		return
	}

	ui.avatarStatus.SetText("Carregando foto...")
	ui.avatarPreviewSeq++
	seq := ui.avatarPreviewSeq
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()

		req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
		if err != nil {
			return
		}
		res, err := http.DefaultClient.Do(req)
		if err != nil {
			fyne.Do(func() { ui.avatarStatus.SetText("Foto salva") })
			return
		}
		defer res.Body.Close()
		if res.StatusCode < 200 || res.StatusCode > 299 {
			fyne.Do(func() { ui.avatarStatus.SetText("Foto salva") })
			return
		}

		img, _, err := image.Decode(io.LimitReader(res.Body, 4*1024*1024))
		if err != nil {
			fyne.Do(func() { ui.avatarStatus.SetText("Foto salva") })
			return
		}

		fyne.Do(func() {
			if ui.avatarPreviewSeq != seq {
				return
			}
			ui.setAvatarPreview(img)
			ui.avatarStatus.SetText("Foto salva")
		})
	}()
}

func (ui *mainUI) loadClientAvatarPreviewFromURL(rawURL string) {
	rawURL = strings.TrimSpace(rawURL)
	if rawURL == "" {
		ui.clearClientAvatarPreview()
		return
	}

	ui.clientAvatarStatus.SetText("Carregando foto...")
	ui.clientAvatarPreviewSeq++
	seq := ui.clientAvatarPreviewSeq
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()

		req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
		if err != nil {
			return
		}
		res, err := http.DefaultClient.Do(req)
		if err != nil {
			fyne.Do(func() { ui.clientAvatarStatus.SetText("Foto salva") })
			return
		}
		defer res.Body.Close()
		if res.StatusCode < 200 || res.StatusCode > 299 {
			fyne.Do(func() { ui.clientAvatarStatus.SetText("Foto salva") })
			return
		}

		img, _, err := image.Decode(io.LimitReader(res.Body, 4*1024*1024))
		if err != nil {
			fyne.Do(func() { ui.clientAvatarStatus.SetText("Foto salva") })
			return
		}

		fyne.Do(func() {
			if ui.clientAvatarPreviewSeq != seq {
				return
			}
			ui.setClientAvatarPreview(img)
			ui.clientAvatarStatus.SetText("Foto salva")
		})
	}()
}

func avatarObjectKey(name string) string {
	sum := sha256.Sum256([]byte(normalizeCustomerName(name)))
	return "avatars/" + hex.EncodeToString(sum[:])[:16] + ".webp"
}
