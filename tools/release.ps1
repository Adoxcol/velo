# Builds a Velo release into dist\:
#   Velo-Setup-<version>.exe       installer (embeds the app and the extension)
#   velo-extension-<version>.zip   extension package for the Chrome Web Store / Edge Add-ons
#
# Usage (from the repository root):  .\tools\release.ps1 -Version 0.1.0
param([Parameter(Mandatory)][string]$Version)
$ErrorActionPreference = 'Stop'
$root = Split-Path -Parent $PSScriptRoot
Set-Location -LiteralPath $root

if ($Version -notmatch '^\d+\.\d+\.\d+$') { throw "Version must look like 1.2.3" }
$ld = "-X velo/internal/version.Version=$Version"
$payload = Join-Path $root 'cmd\velo-setup\payload'
$dist = Join-Path $root 'dist'

# Fresh payload (keep the README that explains the folder).
Get-ChildItem -LiteralPath $payload | Where-Object Name -ne 'README.md' | Remove-Item -Recurse -Force
New-Item -ItemType Directory -Force $dist | Out-Null

Write-Host "Building Velo $Version..."
go build -trimpath -ldflags "$ld -H windowsgui -s -w" -o (Join-Path $payload 'velo.exe') ./cmd/velo
go build -trimpath -ldflags "$ld -H windowsgui -s -w" -o (Join-Path $payload 'velo-host.exe') ./cmd/velo-host
go build -trimpath -ldflags "$ld -s -w" -o (Join-Path $payload 'veloctl.exe') ./cmd/veloctl
if ($LASTEXITCODE) { throw 'build failed' }

# The installed copy keeps the development key, so loading it unpacked gives
# the same extension ID that the native host allows.
Copy-Item -Recurse -LiteralPath (Join-Path $root 'extension') -Destination (Join-Path $payload 'extension')
Remove-Item -LiteralPath (Join-Path $payload 'extension\EXTENSION_ID.txt') -ErrorAction SilentlyContinue

$setup = Join-Path $dist "Velo-Setup-$Version.exe"
go build -trimpath -ldflags "$ld -H windowsgui -s -w" -o $setup ./cmd/velo-setup
if ($LASTEXITCODE) { throw 'setup build failed' }

go run ./tools/packext -version $Version -out (Join-Path $dist "velo-extension-$Version.zip")
if ($LASTEXITCODE) { throw 'extension package failed' }

Get-ChildItem -LiteralPath $payload | Where-Object Name -ne 'README.md' | Remove-Item -Recurse -Force
Get-ChildItem -LiteralPath $dist | ForEach-Object {
    $hash = (Get-FileHash -Algorithm SHA256 -LiteralPath $_.FullName).Hash.ToLower()
    '{0,-34} {1,8:N0} KB  sha256 {2}' -f $_.Name, ($_.Length / 1KB), $hash
}
