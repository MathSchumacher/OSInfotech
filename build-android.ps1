param(
    [string]$Version = "0.6.0",
    [int]$Build = 0,
    [ValidateSet("android/arm64", "android/arm", "android/amd64", "android/386")]
    [string]$Target = "android/arm64",
    [string]$PublishBaseUrl = "",
    [switch]$Publish,
    [switch]$Mandatory,
    [string]$Notes = "",
    [string]$R2EnvFile = ".r2.env.local"
)

$ErrorActionPreference = "Stop"

# O versionCode precisa sempre crescer, senao o Android recusa a atualizacao
# sobre a instalada. Quando nao for informado, ele sai da propria versao:
# 0.8.0 vira 800, no mesmo esquema usado nas versoes anteriores.
if ($Build -le 0) {
    $parts = $Version.Split(".")
    $major = 0; $minor = 0; $patch = 0
    if ($parts.Count -ge 1) { [int]::TryParse($parts[0], [ref]$major) | Out-Null }
    if ($parts.Count -ge 2) { [int]::TryParse($parts[1], [ref]$minor) | Out-Null }
    if ($parts.Count -ge 3) { [int]::TryParse($parts[2], [ref]$patch) | Out-Null }
    $Build = $major * 10000 + $minor * 100 + $patch
    Write-Host "versionCode calculado a partir da versao: $Build"
}

$Root = $PSScriptRoot
$Dist = Join-Path $Root "dist\android"
$Icon = Join-Path $Root "assets\icons\dbs-512.png"
$AppID = "br.com.osinfotech.mobile"

if ([string]::IsNullOrWhiteSpace($env:ANDROID_HOME)) {
	$detectedSdk = Join-Path $env:LOCALAPPDATA "Android\Sdk"
	if (Test-Path -LiteralPath $detectedSdk) {
		$env:ANDROID_HOME = $detectedSdk
		Write-Host "ANDROID_HOME detectado em $detectedSdk"
	}
	else {
		throw "ANDROID_HOME nao esta configurado. Instale o Android SDK/NDK e defina essa variavel antes de gerar o APK."
	}
}
if ([string]::IsNullOrWhiteSpace($env:ANDROID_NDK_HOME)) {
	$ndk = Get-ChildItem -LiteralPath (Join-Path $env:ANDROID_HOME "ndk") -Directory -ErrorAction SilentlyContinue |
		Sort-Object Name -Descending |
		Select-Object -First 1
	if (-not $ndk) {
		throw "Android NDK nao encontrado em $env:ANDROID_HOME\ndk."
	}
	$env:ANDROID_NDK_HOME = $ndk.FullName
	Write-Host "ANDROID_NDK_HOME detectado em $($ndk.FullName)"
}
if (-not (Test-Path -LiteralPath $Icon)) {
    throw "Icone Android nao encontrado: $Icon"
}

# O Fyne CLI e sempre reinstalado na versao fixada. Usar o que estiver no PATH
# muda a assinatura do APK, e o Android recusa a atualizacao sobre a instalada
# com um "app nao instalado" sem explicacao.
Write-Host "Instalando Fyne CLI v2.7.4..."
go install fyne.io/fyne/v2/cmd/fyne@v2.7.4
if ($LASTEXITCODE -ne 0) { throw "Falha ao instalar o Fyne CLI." }
$goPath = (go env GOPATH).Trim()
$fynePath = Join-Path $goPath "bin\fyne.exe"
if (-not (Test-Path -LiteralPath $fynePath)) {
    throw "Nao foi possivel localizar o Fyne CLI apos a instalacao."
}

New-Item -ItemType Directory -Force -Path $Dist | Out-Null
Get-ChildItem -LiteralPath $Dist -Filter "*-Android-*.apk" -File -ErrorAction SilentlyContinue |
    Remove-Item -Force

# O fyne nomeia o APK a partir de --name, entao o arquivo gerado e localizado
# por varredura em vez de nome fixo.
Get-ChildItem -LiteralPath $Root -Filter "*.apk" -File -ErrorAction SilentlyContinue |
    Remove-Item -Force

$shortTemp = "C:\otmp"
New-Item -ItemType Directory -Force -Path $shortTemp | Out-Null
$previousTemp = $env:TEMP
$previousTmp = $env:TMP
$previousGoTmp = $env:GOTMPDIR
$previousGoFlags = $env:GOFLAGS
$env:TEMP = $shortTemp
$env:TMP = $shortTemp
$env:GOTMPDIR = $shortTemp
$env:GOFLAGS = (($previousGoFlags, "-buildvcs=false") -join " ").Trim()

Push-Location $Root
try {
    & $fynePath package `
        --os $Target `
        --name "DBS Assistencia" `
        --appID $AppID `
        --appVersion $Version `
        --appBuild $Build `
        --icon $Icon
    if ($LASTEXITCODE -ne 0) {
        throw "A geracao do APK falhou."
    }
}
finally {
    Pop-Location
    $env:TEMP = $previousTemp
    $env:TMP = $previousTmp
    $env:GOTMPDIR = $previousGoTmp
    $env:GOFLAGS = $previousGoFlags
}

$generatedAPK = Get-ChildItem -LiteralPath $Root -Filter "*.apk" -File -ErrorAction SilentlyContinue |
    Select-Object -First 1 -ExpandProperty FullName
if (-not $generatedAPK) {
    throw "O Fyne nao gerou nenhum APK na raiz do projeto."
}

$apkName = "DBS-assistencia-Android-$Version.apk"
$output = Join-Path $Dist $apkName
Move-Item -LiteralPath $generatedAPK -Destination $output -Force
Write-Host "APK gerado: $output"

# Gera o manifesto de OTA lido pelo app (android/latest.json).
$apkHash = (Get-FileHash -Algorithm SHA256 -LiteralPath $output).Hash.ToLowerInvariant()
$apkSize = (Get-Item -LiteralPath $output).Length
if ([string]::IsNullOrWhiteSpace($PublishBaseUrl)) {
    $apkUrl = "android/$apkName"
}
else {
    $apkUrl = ($PublishBaseUrl.TrimEnd("/") + "/android/" + $apkName)
}
$releaseNotes = if ([string]::IsNullOrWhiteSpace($Notes)) { "DBS Assistencia Tecnica $Version" } else { $Notes }

$manifest = [ordered]@{
    version      = $Version
    build        = $Build
    published_at = (Get-Date).ToUniversalTime().ToString("o")
    notes        = $releaseNotes
    apk_url      = $apkUrl
    apk_sha256   = $apkHash
    size_bytes   = $apkSize
    mandatory    = [bool]$Mandatory
}
$manifestPath = Join-Path $Dist "latest.json"
$manifestJson = $manifest | ConvertTo-Json -Depth 4
[System.IO.File]::WriteAllText($manifestPath, $manifestJson + [Environment]::NewLine, [System.Text.UTF8Encoding]::new($false))
Write-Host "Manifesto OTA: $manifestPath"

if ($Publish) {
    Write-Host ""
    Write-Host "Publishing APK to Cloudflare R2..."
    go run ./cmd/publish-android -version $Version -env $R2EnvFile
    if ($LASTEXITCODE -ne 0) {
        throw "Falha ao publicar o APK no R2."
    }
}
