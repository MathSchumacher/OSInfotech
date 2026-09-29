param(
    [string]$Version = "",
    [string]$EnvFile = ".r2.env.local"
)

$ErrorActionPreference = "Stop"
$argsList = @("./cmd/publish", "-env", $EnvFile)
if (-not [string]::IsNullOrWhiteSpace($Version)) {
    $argsList += @("-version", $Version)
}

go run @argsList

