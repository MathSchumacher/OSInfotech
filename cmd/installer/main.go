package main

import (
	"embed"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

//go:embed payload/*
var payload embed.FS

var version = "dev"

const (
	// appName aparece para o usuario (atalho, Programas e Recursos). appFolder e
	// o mesmo nome usado como pasta, por isso sem acento.
	appName   = "DBS Assistencia Tecnica"
	appFolder = "DBS Assistencia Tecnica"
	appExe    = "DBS-assistencia.exe"

	// Instalacao antiga, de quando o app se chamava OSInfotech. E removida na
	// atualizacao para o cliente nao ficar com dois apps e dois atalhos. Os dados
	// nao se movem: o banco continua na mesma pasta de sempre.
	legacyName = "OSInfotech"
	legacyExe  = "OSInfotech.exe"
)

func main() {
	if hasArg("/uninstall") || hasArg("--uninstall") {
		if err := uninstall(); err != nil {
			notify("Falha ao desinstalar", err.Error())
			os.Exit(1)
		}
		return
	}

	if err := install(); err != nil {
		notify("Falha ao instalar", err.Error())
		os.Exit(1)
	}
}

func install() error {
	appBytes, err := payload.ReadFile("payload/" + appExe)
	if err != nil {
		return errors.New("payload nao encontrado; gere o instalador usando build.ps1")
	}

	dir, err := installDir()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}

	appPath := filepath.Join(dir, appExe)
	_ = exec.Command("taskkill", "/IM", appExe, "/F").Run()
	if err := os.WriteFile(appPath, appBytes, 0o755); err != nil {
		return err
	}

	setupPath := filepath.Join(dir, "DBS-assistencia-Setup.exe")
	if err := copySelf(setupPath); err != nil {
		return err
	}

	if err := createShortcuts(appPath, dir); err != nil {
		return err
	}
	if err := registerUninstall(dir, setupPath, appPath); err != nil {
		return err
	}
	removeLegacyInstall()

	if !hasArg("/silent") && !hasArg("--silent") {
		_ = exec.Command(appPath).Start()
	}
	return nil
}

func uninstall() error {
	dir, err := installDir()
	if err != nil {
		return err
	}

	_ = exec.Command("taskkill", "/IM", appExe, "/F").Run()
	_ = removeShortcuts()
	_ = exec.Command("reg", "delete", uninstallKey(), "/f").Run()

	cmd := fmt.Sprintf(`ping 127.0.0.1 -n 2 > nul & rmdir /S /Q "%s"`, dir)
	return exec.Command("cmd", "/C", cmd).Start()
}

func installDir() (string, error) {
	base := os.Getenv("LOCALAPPDATA")
	if base == "" {
		return "", errors.New("LOCALAPPDATA nao encontrado")
	}
	return filepath.Join(base, "Programs", appFolder), nil
}

func copySelf(dest string) error {
	src, err := os.Executable()
	if err != nil {
		return err
	}
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()

	out, err := os.Create(dest)
	if err != nil {
		return err
	}
	defer out.Close()

	_, err = io.Copy(out, in)
	return err
}

func createShortcuts(appPath, workingDir string) error {
	startDir := filepath.Join(os.Getenv("APPDATA"), "Microsoft", "Windows", "Start Menu", "Programs", appName)
	if err := os.MkdirAll(startDir, 0o755); err != nil {
		return err
	}

	if err := createShortcut(filepath.Join(startDir, appName+".lnk"), appPath, workingDir); err != nil {
		return err
	}

	desktop := filepath.Join(os.Getenv("USERPROFILE"), "Desktop", appName+".lnk")
	if _, err := os.Stat(filepath.Dir(desktop)); err == nil {
		return createShortcut(desktop, appPath, workingDir)
	}
	return nil
}

func removeShortcuts() error {
	startDir := filepath.Join(os.Getenv("APPDATA"), "Microsoft", "Windows", "Start Menu", "Programs", appName)
	_ = os.RemoveAll(startDir)
	desktop := filepath.Join(os.Getenv("USERPROFILE"), "Desktop", appName+".lnk")
	_ = os.Remove(desktop)
	return nil
}

func createShortcut(linkPath, targetPath, workingDir string) error {
	script := fmt.Sprintf(
		`$w=New-Object -ComObject WScript.Shell; $s=$w.CreateShortcut('%s'); $s.TargetPath='%s'; $s.WorkingDirectory='%s'; $s.IconLocation='%s'; $s.Save()`,
		psQuote(linkPath), psQuote(targetPath), psQuote(workingDir), psQuote(targetPath),
	)
	return exec.Command("powershell", "-NoProfile", "-ExecutionPolicy", "Bypass", "-Command", script).Run()
}

func registerUninstall(dir, setupPath, appPath string) error {
	key := uninstallKey()
	values := [][]string{
		{"DisplayName", appName},
		{"DisplayVersion", version},
		{"Publisher", appName},
		{"InstallLocation", dir},
		{"DisplayIcon", appPath},
		{"UninstallString", quote(setupPath) + " /uninstall"},
	}
	for _, value := range values {
		if err := exec.Command("reg", "add", key, "/v", value[0], "/d", value[1], "/f").Run(); err != nil {
			return err
		}
	}
	return nil
}

func uninstallKey() string {
	return `HKCU\Software\Microsoft\Windows\CurrentVersion\Uninstall\DBSAssistenciaTecnica`
}

func legacyUninstallKey() string {
	return `HKCU\Software\Microsoft\Windows\CurrentVersion\Uninstall\OSInfotech`
}

// removeLegacyInstall apaga a instalacao antiga com o nome OSInfotech: processo,
// atalhos, registro e pasta. Sem isso a atualizacao deixaria o app duplicado na
// maquina do cliente, e ele poderia abrir a versao velha pelo atalho antigo.
func removeLegacyInstall() {
	_ = exec.Command("taskkill", "/IM", legacyExe, "/F").Run()

	startDir := filepath.Join(os.Getenv("APPDATA"), "Microsoft", "Windows", "Start Menu", "Programs", legacyName)
	_ = os.RemoveAll(startDir)
	_ = os.Remove(filepath.Join(os.Getenv("USERPROFILE"), "Desktop", legacyName+".lnk"))
	_ = exec.Command("reg", "delete", legacyUninstallKey(), "/f").Run()

	base := os.Getenv("LOCALAPPDATA")
	if base == "" {
		return
	}
	dir := filepath.Join(base, "Programs", legacyName)
	if _, err := os.Stat(dir); err != nil {
		return
	}
	// O executavel antigo pode estar encerrando, entao a pasta sai depois.
	cmd := fmt.Sprintf(`ping 127.0.0.1 -n 3 > nul & rmdir /S /Q "%s"`, dir)
	_ = exec.Command("cmd", "/C", cmd).Start()
}

func notify(title, body string) {
	script := fmt.Sprintf(
		`Add-Type -AssemblyName PresentationFramework; [System.Windows.MessageBox]::Show('%s','%s') | Out-Null`,
		psQuote(body), psQuote(title),
	)
	_ = exec.Command("powershell", "-NoProfile", "-ExecutionPolicy", "Bypass", "-Command", script).Run()
}

func hasArg(want string) bool {
	for _, arg := range os.Args[1:] {
		if strings.EqualFold(arg, want) {
			return true
		}
	}
	return false
}

func quote(s string) string {
	return `"` + s + `"`
}

func psQuote(s string) string {
	return strings.ReplaceAll(s, `'`, `''`)
}
