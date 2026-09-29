param(
    [string]$Version = "0.1.0",
    [string]$PublishBaseUrl = "",
    [switch]$Publish,
    [string]$R2EnvFile = ".r2.env.local",
    [switch]$SkipSetup
)

$ErrorActionPreference = "Stop"
$Root = $PSScriptRoot
$Dist = Join-Path $Root "dist"
$PortableDir = Join-Path $Dist "portable"
$SetupDir = Join-Path $Dist "setup"
$PayloadDir = Join-Path $Root "cmd\installer\payload"

New-Item -ItemType Directory -Force -Path $PortableDir, $SetupDir, $PayloadDir | Out-Null

$AppExe = Join-Path $PortableDir "DBS-assistencia.exe"
$ZipPath = Join-Path $Dist "DBS-assistencia-Portable-$Version.zip"
$SetupExe = Join-Path $SetupDir "DBS-assistencia-Setup-$Version.exe"
$LatestPath = Join-Path $Dist "latest.json"

function Clear-OldReleaseArtifacts {
    $currentPortable = "DBS-assistencia-Portable-$Version.zip"
    $currentSetup = "DBS-assistencia-Setup-$Version.exe"

    Get-ChildItem -LiteralPath $Dist -Filter "*-Portable-*.zip" -File -ErrorAction SilentlyContinue |
        Where-Object { $_.Name -ne $currentPortable } |
        Remove-Item -Force

    Get-ChildItem -LiteralPath $SetupDir -Filter "*-Setup-*.exe" -File -ErrorAction SilentlyContinue |
        Where-Object { $_.Name -ne $currentSetup } |
        Remove-Item -Force

    Get-ChildItem -LiteralPath $PortableDir -Filter "*.exe~" -File -ErrorAction SilentlyContinue |
        Remove-Item -Force -ErrorAction SilentlyContinue
}

function Close-RunningPortableApp {
    $running = Get-Process -Name "DBS-assistencia" -ErrorAction SilentlyContinue | Where-Object {
        try {
            $_.Path -eq $AppExe
        }
        catch {
            $false
        }
    }

    foreach ($process in $running) {
        Write-Host "Closing running portable app before build: PID $($process.Id)"
        $null = $process.CloseMainWindow()
        if (-not $process.WaitForExit(2500)) {
            Stop-Process -Id $process.Id -Force
        }
    }
}

function Join-PublishUrl {
    param([string]$FileName)
    if ([string]::IsNullOrWhiteSpace($PublishBaseUrl)) {
        return $FileName
    }
    return ($PublishBaseUrl.TrimEnd("/") + "/" + $FileName)
}

function Get-RcVersion {
    $parts = $Version.Split(".") | ForEach-Object {
        $n = 0
        if ([int]::TryParse($_, [ref]$n)) { $n } else { 0 }
    }
    while ($parts.Count -lt 4) {
        $parts += 0
    }
    return ($parts[0..3] -join ",")
}

function New-WindowsResource {
    param(
        [string]$RcPath,
        [string]$OutPath
    )

    $windres = Get-Command windres -ErrorAction SilentlyContinue
    if (-not $windres) {
        Write-Warning "windres nao encontrado; o .exe sera gerado sem icone de arquivo Windows."
        return
    }

    $tmpRc = Join-Path $env:TEMP ("osinfotech-" + [guid]::NewGuid().ToString("N") + ".rc")
    $rcVersion = Get-RcVersion
    $content = Get-Content -Raw -LiteralPath $RcPath
    $content = $content -replace 'FILEVERSION\s+[0-9,]+', "FILEVERSION $rcVersion"
    $content = $content -replace 'PRODUCTVERSION\s+[0-9,]+', "PRODUCTVERSION $rcVersion"
    $content = $content -replace 'VALUE "FileVersion", "[^"]*"', "VALUE `"FileVersion`", `"$Version`""
    $content = $content -replace 'VALUE "ProductVersion", "[^"]*"', "VALUE `"ProductVersion`", `"$Version`""
    Set-Content -LiteralPath $tmpRc -Value $content -Encoding ASCII

    Push-Location $Root
    try {
        & $windres.Source -O coff -F pe-x86-64 -i $tmpRc -o $OutPath
    }
    finally {
        Pop-Location
        Remove-Item -LiteralPath $tmpRc -Force -ErrorAction SilentlyContinue
    }
}

Close-RunningPortableApp
Clear-OldReleaseArtifacts

New-WindowsResource -RcPath (Join-Path $Root "app.rc") -OutPath (Join-Path $Root "app_windows.syso")
New-WindowsResource -RcPath (Join-Path $Root "cmd\installer\app.rc") -OutPath (Join-Path $Root "cmd\installer\app_windows.syso")

Write-Host "Building portable app..."
go build -trimpath -buildvcs=false -ldflags "-H=windowsgui -s -w -X main.appVersion=$Version" -o $AppExe .

if (Test-Path $ZipPath) {
    Remove-Item -LiteralPath $ZipPath -Force
}
Compress-Archive -Path $AppExe -DestinationPath $ZipPath -Force

if (-not $SkipSetup) {
    Write-Host "Building setup installer..."
    Copy-Item -LiteralPath $AppExe -Destination (Join-Path $PayloadDir "DBS-assistencia.exe") -Force
    try {
        go build -trimpath -buildvcs=false -ldflags "-H=windowsgui -s -w -X main.version=$Version" -o $SetupExe ./cmd/installer
    }
    finally {
        Remove-Item -LiteralPath (Join-Path $PayloadDir "DBS-assistencia.exe") -Force -ErrorAction SilentlyContinue
    }
}

$manifest = [ordered]@{
    version = $Version
    published_at = (Get-Date).ToUniversalTime().ToString("o")
    notes = "DBS Assistencia Tecnica $Version"
    setup_url = $null
    setup_sha256 = $null
    portable_url = Join-PublishUrl ("DBS-assistencia-Portable-$Version.zip")
    portable_sha256 = (Get-FileHash -Algorithm SHA256 -LiteralPath $ZipPath).Hash.ToLowerInvariant()
}

if (-not $SkipSetup) {
    $setupName = "DBS-assistencia-Setup-$Version.exe"
    $manifest.setup_url = Join-PublishUrl ("setup/$setupName")
    $manifest.setup_sha256 = (Get-FileHash -Algorithm SHA256 -LiteralPath $SetupExe).Hash.ToLowerInvariant()
}

$manifestJson = $manifest | ConvertTo-Json -Depth 4
[System.IO.File]::WriteAllText($LatestPath, $manifestJson + [Environment]::NewLine, [System.Text.UTF8Encoding]::new($false))

Write-Host ""
Write-Host "Portable EXE: $AppExe"
Write-Host "Portable ZIP: $ZipPath"
if (-not $SkipSetup) {
    Write-Host "Setup EXE:    $SetupExe"
}
Write-Host "Update JSON:  $LatestPath"

if ($Publish) {
    Write-Host ""
    Write-Host "Publishing to Cloudflare R2..."
    & (Join-Path $Root "publish-r2.ps1") -Version $Version -EnvFile $R2EnvFile
}

Close-RunningPortableApp
Clear-OldReleaseArtifacts
