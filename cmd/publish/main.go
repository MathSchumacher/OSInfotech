package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"osinfotech/internal/r2client"
)

type manifest struct {
	Version string `json:"version"`
}

func main() {
	distDir := flag.String("dist", "dist", "dist directory")
	version := flag.String("version", "", "version to publish; defaults to dist/latest.json")
	envFile := flag.String("env", ".r2.env.local", "optional local env file")
	flag.Parse()

	if err := loadEnvFile(*envFile); err != nil && !os.IsNotExist(err) {
		fatal(err)
	}

	v := strings.TrimSpace(*version)
	if v == "" {
		var err error
		v, err = readVersion(filepath.Join(*distDir, "latest.json"))
		if err != nil {
			fatal(err)
		}
	}

	files := []r2client.UploadFile{
		{Key: fmt.Sprintf("DBS-assistencia-Portable-%s.zip", v), Path: filepath.Join(*distDir, fmt.Sprintf("DBS-assistencia-Portable-%s.zip", v))},
		{Key: fmt.Sprintf("setup/DBS-assistencia-Setup-%s.exe", v), Path: filepath.Join(*distDir, "setup", fmt.Sprintf("DBS-assistencia-Setup-%s.exe", v))},
		{Key: "latest.json", Path: filepath.Join(*distDir, "latest.json")},
	}

	incoming := make(map[string]int64, len(files))
	for _, file := range files {
		size, err := r2client.FileSize(file.Path)
		if err != nil {
			fatal(err)
		}
		incoming[file.Key] = size
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()

	client, err := r2client.New(ctx, r2client.FromEnv())
	if err != nil {
		fatal(err)
	}
	if err := client.EnsureRoom(ctx, incoming, true); err != nil {
		fatal(err)
	}

	for _, file := range files[:len(files)-1] {
		fmt.Printf("Uploading %s...\n", file.Key)
		if err := client.PutFile(ctx, file.Key, file.Path); err != nil {
			fatal(err)
		}
	}

	latest := files[len(files)-1]
	fmt.Printf("Uploading %s...\n", latest.Key)
	if err := client.PutFile(ctx, latest.Key, latest.Path); err != nil {
		fatal(err)
	}

	fmt.Println("Published successfully.")
	if url := client.PublicURL("latest.json"); url != "" {
		fmt.Println("Manifest:", url)
	}
}

func readVersion(path string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	var m manifest
	if err := json.Unmarshal(data, &m); err != nil {
		return "", err
	}
	if strings.TrimSpace(m.Version) == "" {
		return "", fmt.Errorf("%s sem campo version", path)
	}
	return strings.TrimSpace(m.Version), nil
}

func loadEnvFile(path string) error {
	if strings.TrimSpace(path) == "" {
		return nil
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		key, value, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		key = strings.TrimSpace(key)
		value = strings.Trim(strings.TrimSpace(value), `"'`)
		if key != "" && os.Getenv(key) == "" {
			_ = os.Setenv(key, value)
		}
	}
	return nil
}

func fatal(err error) {
	fmt.Fprintln(os.Stderr, "publish failed:", err)
	os.Exit(1)
}
