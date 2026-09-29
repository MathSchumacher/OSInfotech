package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

const updateManifestURLSetting = "update_manifest_url"
const androidUpdateManifestURLSetting = "android_update_manifest_url"

type UpdateManifest struct {
	Version        string `json:"version"`
	PublishedAt    string `json:"published_at"`
	Notes          string `json:"notes"`
	SetupURL       string `json:"setup_url"`
	SetupSHA256    string `json:"setup_sha256"`
	PortableURL    string `json:"portable_url"`
	PortableSHA256 string `json:"portable_sha256"`
	APKURL         string `json:"apk_url"`
	APKSHA256      string `json:"apk_sha256"`
	Mandatory      bool   `json:"mandatory"`
}

func checkForUpdate(ctx context.Context, manifestURL string, currentVersion string) (UpdateManifest, bool, error) {
	manifestURL = strings.TrimSpace(manifestURL)
	if manifestURL == "" {
		return UpdateManifest{}, false, errors.New("URL do manifesto nao configurada")
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, manifestURL, nil)
	if err != nil {
		return UpdateManifest{}, false, err
	}

	client := &http.Client{Timeout: 20 * time.Second}
	res, err := client.Do(req)
	if err != nil {
		return UpdateManifest{}, false, err
	}
	defer res.Body.Close()

	if res.StatusCode < 200 || res.StatusCode > 299 {
		return UpdateManifest{}, false, fmt.Errorf("servidor respondeu %s", res.Status)
	}

	var manifest UpdateManifest
	if err := json.NewDecoder(io.LimitReader(res.Body, 1<<20)).Decode(&manifest); err != nil {
		return UpdateManifest{}, false, err
	}

	manifest.Version = strings.TrimSpace(manifest.Version)
	if manifest.Version == "" {
		return UpdateManifest{}, false, errors.New("manifesto sem versao")
	}

	if manifest.SetupURL != "" {
		manifest.SetupURL, err = resolveUpdateURL(manifestURL, manifest.SetupURL)
		if err != nil {
			return UpdateManifest{}, false, err
		}
	}
	if manifest.PortableURL != "" {
		manifest.PortableURL, err = resolveUpdateURL(manifestURL, manifest.PortableURL)
		if err != nil {
			return UpdateManifest{}, false, err
		}
	}
	if manifest.APKURL != "" {
		manifest.APKURL, err = resolveUpdateURL(manifestURL, manifest.APKURL)
		if err != nil {
			return UpdateManifest{}, false, err
		}
	}

	return manifest, versionNewer(manifest.Version, currentVersion), nil
}

func downloadAndRunSetup(ctx context.Context, manifest UpdateManifest) (string, error) {
	if strings.TrimSpace(manifest.SetupURL) == "" {
		return "", errors.New("manifesto nao possui setup_url")
	}

	parsed, err := url.Parse(manifest.SetupURL)
	if err != nil {
		return "", err
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return "", errors.New("setup_url precisa ser HTTP ou HTTPS")
	}

	name := fmt.Sprintf("DBS-assistencia-Setup-%s.exe", safeFileVersion(manifest.Version))
	outPath := filepath.Join(os.TempDir(), name)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, manifest.SetupURL, nil)
	if err != nil {
		return "", err
	}

	client := &http.Client{Timeout: 10 * time.Minute}
	res, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer res.Body.Close()

	if res.StatusCode < 200 || res.StatusCode > 299 {
		return "", fmt.Errorf("download respondeu %s", res.Status)
	}

	out, err := os.Create(outPath)
	if err != nil {
		return "", err
	}

	hash := sha256.New()
	_, copyErr := io.Copy(io.MultiWriter(out, hash), res.Body)
	closeErr := out.Close()
	if copyErr != nil {
		_ = os.Remove(outPath)
		return "", copyErr
	}
	if closeErr != nil {
		_ = os.Remove(outPath)
		return "", closeErr
	}

	// Sem a impressao digital no manifesto o instalador nao roda: um manifesto
	// adulterado poderia mandar o app baixar e executar qualquer arquivo.
	got := strings.ToLower(hex.EncodeToString(hash.Sum(nil)))
	want := strings.ToLower(strings.TrimSpace(manifest.SetupSHA256))
	if want == "" {
		_ = os.Remove(outPath)
		return "", errors.New("atualizacao sem assinatura SHA-256: instalacao recusada")
	}
	if got != want {
		_ = os.Remove(outPath)
		return "", fmt.Errorf("SHA-256 invalido: esperado %s, obtido %s", want, got)
	}

	if err := exec.Command(outPath).Start(); err != nil {
		return "", err
	}
	return outPath, nil
}

func resolveUpdateURL(baseURL, value string) (string, error) {
	ref, err := url.Parse(strings.TrimSpace(value))
	if err != nil {
		return "", err
	}
	if ref.IsAbs() {
		return ref.String(), nil
	}

	base, err := url.Parse(baseURL)
	if err != nil {
		return "", err
	}
	return base.ResolveReference(ref).String(), nil
}

func versionNewer(remote, current string) bool {
	return compareVersions(remote, current) > 0
}

func compareVersions(a, b string) int {
	ap := versionParts(a)
	bp := versionParts(b)
	if len(ap) == 0 || len(bp) == 0 {
		return 0
	}

	maxLen := len(ap)
	if len(bp) > maxLen {
		maxLen = len(bp)
	}
	for i := 0; i < maxLen; i++ {
		var av, bv int
		if i < len(ap) {
			av = ap[i]
		}
		if i < len(bp) {
			bv = bp[i]
		}
		if av > bv {
			return 1
		}
		if av < bv {
			return -1
		}
	}
	return 0
}

func versionParts(v string) []int {
	v = strings.TrimSpace(strings.TrimPrefix(strings.ToLower(v), "v"))
	if v == "" || v == "dev" {
		return nil
	}

	fields := strings.FieldsFunc(v, func(r rune) bool {
		return r == '.' || r == '-' || r == '_'
	})
	parts := make([]int, 0, len(fields))
	for _, field := range fields {
		if field == "" {
			continue
		}
		n, err := strconv.Atoi(field)
		if err != nil {
			break
		}
		parts = append(parts, n)
	}
	return parts
}

func safeFileVersion(v string) string {
	v = strings.TrimSpace(v)
	if v == "" {
		return "latest"
	}
	var b strings.Builder
	for _, r := range v {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '.' || r == '-' || r == '_' {
			b.WriteRune(r)
		}
	}
	if b.Len() == 0 {
		return "latest"
	}
	return b.String()
}
