package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"osinfotech/internal/r2client"
)

func (ui *mainUI) uploadDocumentToR2(ctx context.Context, key string, path string) (string, error) {
	info, err := os.Stat(path)
	if err != nil {
		return "", err
	}

	cfg, err := ui.loadR2Config(ctx)
	if err != nil {
		return "", err
	}
	client, err := r2client.New(ctx, cfg)
	if err != nil {
		return "", err
	}
	if err := client.EnsureRoom(ctx, map[string]int64{key: info.Size()}, false); err != nil {
		return "", err
	}
	if err := client.PutFile(ctx, key, path); err != nil {
		return "", err
	}
	return client.PublicURL(key), nil
}

func generatedTermPDFObjectKey(id int64) string {
	return fmt.Sprintf("terms/generated/TERMO-%06d.pdf", id)
}

func signedTermObjectKey(id int64, originalPath string) string {
	ext := strings.ToLower(filepath.Ext(originalPath))
	switch ext {
	case ".pdf", ".png", ".jpg", ".jpeg", ".webp":
	default:
		ext = ".pdf"
	}
	base := strings.TrimSuffix(filepath.Base(originalPath), filepath.Ext(originalPath))
	if !strings.HasPrefix(base, fmt.Sprintf("TERMO-%06d-", id)) {
		base = fmt.Sprintf("TERMO-%06d-assinado", id)
	}
	return fmt.Sprintf("terms/signed/%s%s", base, ext)
}

func (ui *mainUI) documentPublicURL(key string) string {
	key = strings.TrimSpace(key)
	if key == "" {
		return ""
	}
	cfg, err := ui.loadR2Config(context.Background())
	if err != nil {
		return ""
	}
	base := strings.TrimRight(cfg.PublicBaseURL, "/")
	if base == "" {
		return ""
	}
	return base + "/" + strings.TrimLeft(key, "/")
}
