# Builds a Velo release into dist\:
#   Velo-Setup-<version>.exe       installer (embeds the app and the extension)
#   velo-extension-<version>.zip   extension package for the Chrome Web Store / Edge Add-ons
#   Velo-Portable-<version>.zip    no install: unzip and run velo.exe
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

# Portable: the app, the extension and a "portable" marker, which keeps
# settings in .\data and connects the browser to this folder when run.
$portable = Join-Path ([IO.Path]::GetTempPath()) "Velo-Portable-$Version"
Remove-Item -LiteralPath $portable -Recurse -Force -ErrorAction SilentlyContinue
New-Item -ItemType Directory -Force $portable | Out-Null
Get-ChildItem -LiteralPath $payload | Where-Object Name -ne 'README.md' | Copy-Item -Destination $portable -Recurse
Set-Content -LiteralPath (Join-Path $portable 'portable') -Value 'This file makes Velo portable: settings and history stay in the data folder next to it.'
@"
Velo $Version (portable)

1. Run velo.exe. Settings and history are kept in the data folder here.
2. Browser extension: open chrome://extensions (or edge://extensions),
   turn on Developer mode, click "Load unpacked" and pick the extension
   folder here. Velo connects the browser to this folder when it runs.
3. To remove Velo, quit it from the tray and delete this folder.

Portable copies don't update themselves: download the new zip from
https://github.com/Adoxcol/velo/releases
"@ | Set-Content -LiteralPath (Join-Path $portable 'README.txt')
$pzip = Join-Path $dist "Velo-Portable-$Version.zip"
Remove-Item -LiteralPath $pzip -ErrorAction SilentlyContinue
Compress-Archive -Path (Join-Path $portable '*') -DestinationPath $pzip
Remove-Item -LiteralPath $portable -Recurse -Force

go run ./tools/packext -version $Version -out (Join-Path $dist "velo-extension-$Version.zip")
if ($LASTEXITCODE) { throw 'extension package failed' }

Get-ChildItem -LiteralPath $payload | Where-Object Name -ne 'README.md' | Remove-Item -Recurse -Force
Get-ChildItem -LiteralPath $dist | ForEach-Object {
    $hash = (Get-FileHash -Algorithm SHA256 -LiteralPath $_.FullName).Hash.ToLower()
    '{0,-34} {1,8:N0} KB  sha256 {2}' -f $_.Name, ($_.Length / 1KB), $hash
}
