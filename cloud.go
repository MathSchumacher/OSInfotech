package main

import (
	"context"
	"strconv"
	"strings"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
	"osinfotech/internal/r2client"
)

const (
	defaultR2AccountID     = "0f19a0cab136b6a2e9cd7ceee5a29575"
	defaultR2BucketName    = "osinfotech"
	defaultR2Endpoint      = "https://0f19a0cab136b6a2e9cd7ceee5a29575.r2.cloudflarestorage.com"
	defaultR2PublicBaseURL = "https://pub-edaac30d04ca45e1a0f8a19919ac915c.r2.dev"
	defaultR2AccessKeyID   = "65778ac67599073de905da2d46ba8ab6"
	defaultR2SecretKey     = "137679f9b0e01bbd38e0e16ffe8e1880f3a7fd8ed40e98108835d241c3f68c87"

	defaultUpdateManifestURL        = defaultR2PublicBaseURL + "/latest.json"
	defaultAndroidUpdateManifestURL = defaultR2PublicBaseURL + "/android/latest.json"

	r2AccountIDSetting       = "r2_account_id"
	r2EndpointSetting        = "r2_endpoint"
	r2BucketNameSetting      = "r2_bucket_name"
	r2AccessKeyIDSetting     = "r2_access_key_id"
	r2SecretAccessKeySetting = "r2_secret_access_key"
	r2PublicBaseURLSetting   = "r2_public_base_url"
	r2MaxBytesSetting        = "r2_max_bytes"
)

func (ui *mainUI) loadR2Config(ctx context.Context) (r2client.Config, error) {
	cfg := r2client.FromEnv()
	cfg.AccountID = firstNonEmpty(cfg.AccountID, defaultR2AccountID)
	cfg.Endpoint = firstNonEmpty(cfg.Endpoint, defaultR2Endpoint)
	cfg.Bucket = firstNonEmpty(cfg.Bucket, defaultR2BucketName)
	cfg.AccessKeyID = firstNonEmpty(cfg.AccessKeyID, defaultR2AccessKeyID)
	cfg.SecretAccessKey = firstNonEmpty(cfg.SecretAccessKey, defaultR2SecretKey)
	cfg.PublicBaseURL = firstNonEmpty(cfg.PublicBaseURL, defaultR2PublicBaseURL)

	values := map[string]*string{
		r2AccountIDSetting:       &cfg.AccountID,
		r2EndpointSetting:        &cfg.Endpoint,
		r2BucketNameSetting:      &cfg.Bucket,
		r2AccessKeyIDSetting:     &cfg.AccessKeyID,
		r2SecretAccessKeySetting: &cfg.SecretAccessKey,
		r2PublicBaseURLSetting:   &cfg.PublicBaseURL,
	}
	for key, target := range values {
		value, ok, err := ui.store.GetSetting(ctx, key)
		if err != nil {
			return r2client.Config{}, err
		}
		if ok && strings.TrimSpace(value) != "" {
			*target = strings.TrimSpace(value)
		}
	}

	if value, ok, err := ui.store.GetSetting(ctx, r2MaxBytesSetting); err != nil {
		return r2client.Config{}, err
	} else if ok && strings.TrimSpace(value) != "" {
		if parsed, err := strconv.ParseInt(strings.TrimSpace(value), 10, 64); err == nil {
			cfg.MaxBucketBytes = parsed
		}
	}

	return cfg.WithDefaults(), nil
}

func (ui *mainUI) showR2SettingsDialog() {
	cfg, err := ui.loadR2Config(context.Background())
	if err != nil {
		dialog.ShowError(err, ui.window)
		return
	}

	accountEntry := newFormEntry()
	accountEntry.SetText(cfg.AccountID)
	bucketEntry := newFormEntry()
	bucketEntry.SetText(cfg.Bucket)
	endpointEntry := newFormEntry()
	endpointEntry.SetText(cfg.Endpoint)
	publicEntry := newFormEntry()
	publicEntry.SetText(cfg.PublicBaseURL)
	accessEntry := newFormEntry()
	accessEntry.SetText(cfg.AccessKeyID)
	secretEntry := newFormPassword()
	secretEntry.SetText(cfg.SecretAccessKey)
	maxEntry := newFormEntry()
	maxEntry.SetText(strconv.FormatInt(cfg.MaxBucketBytes, 10))

	status := widget.NewLabel("")
	status.Importance = widget.LowImportance

	saveButton := primaryButton("Salvar", theme.DocumentSaveIcon(), func() {
		settings := map[string]string{
			r2AccountIDSetting:       accountEntry.Text,
			r2BucketNameSetting:      bucketEntry.Text,
			r2EndpointSetting:        endpointEntry.Text,
			r2PublicBaseURLSetting:   publicEntry.Text,
			r2AccessKeyIDSetting:     accessEntry.Text,
			r2SecretAccessKeySetting: secretEntry.Text,
			r2MaxBytesSetting:        maxEntry.Text,
		}
		for key, value := range settings {
			if err := ui.store.SetSetting(context.Background(), key, value); err != nil {
				dialog.ShowError(err, ui.window)
				return
			}
		}

		updateURL := strings.TrimRight(publicEntry.Text, "/") + "/latest.json"
		_ = ui.store.SetSetting(context.Background(), updateManifestURLSetting, updateURL)
		status.SetText("Configuracao R2 salva. URL de atualizacao ajustada.")
	})

	testButton := secondaryButton("Testar", theme.ConfirmIcon(), func() {
		status.SetText("Testando R2...")
		go func() {
			_ = ui.store.SetSetting(context.Background(), r2AccountIDSetting, accountEntry.Text)
			_ = ui.store.SetSetting(context.Background(), r2BucketNameSetting, bucketEntry.Text)
			_ = ui.store.SetSetting(context.Background(), r2EndpointSetting, endpointEntry.Text)
			_ = ui.store.SetSetting(context.Background(), r2PublicBaseURLSetting, publicEntry.Text)
			_ = ui.store.SetSetting(context.Background(), r2AccessKeyIDSetting, accessEntry.Text)
			_ = ui.store.SetSetting(context.Background(), r2SecretAccessKeySetting, secretEntry.Text)
			_ = ui.store.SetSetting(context.Background(), r2MaxBytesSetting, maxEntry.Text)

			cfg, err := ui.loadR2Config(context.Background())
			if err == nil {
				var client *r2client.Client
				client, err = r2client.New(context.Background(), cfg)
				if err == nil {
					_, _, err = client.ListObjects(context.Background(), "")
				}
			}

			fyne.Do(func() {
				if err != nil {
					status.SetText("Falha no teste.")
					dialog.ShowError(err, ui.window)
					return
				}
				status.SetText("Conexao R2 OK.")
			})
		}()
	})

	form := widget.NewForm(
		widget.NewFormItem("Account ID", accountEntry),
		widget.NewFormItem("Bucket", bucketEntry),
		widget.NewFormItem("Endpoint S3", endpointEntry),
		widget.NewFormItem("Public URL", publicEntry),
		widget.NewFormItem("Access Key", accessEntry),
		widget.NewFormItem("Secret Key", secretEntry),
		widget.NewFormItem("Limite bytes", maxEntry),
	)

	dialog.ShowCustom("Nuvem R2", "Fechar", container.NewVBox(
		form,
		container.NewHBox(saveButton, testButton),
		status,
	), ui.window)
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}
