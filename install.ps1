# Builds Velo and registers the native messaging host for the current user
# (Chrome and Edge). Run from the project folder:  .\install.ps1
$ErrorActionPreference = 'Stop'
# Stop a running copy so its exe can be replaced (progress is saved).
Get-Process velo, mydm -ErrorAction SilentlyContinue | Stop-Process -Force
Start-Sleep -Milliseconds 300
$root = $PSScriptRoot
$bin = Join-Path $root 'bin'

go build -ldflags '-H windowsgui' -o (Join-Path $bin 'velo.exe') ./cmd/velo
go build -o (Join-Path $bin 'veloctl.exe') ./cmd/veloctl
go build -ldflags '-H windowsgui' -o (Join-Path $bin 'velo-host.exe') ./cmd/velo-host

$extId = (Get-Content (Join-Path $root 'extension\EXTENSION_ID.txt')).Trim()
$manifest = Join-Path $bin 'com.velo.host.json'
@{
    name            = 'com.velo.host'
    description     = 'Velo native messaging host'
    path            = (Join-Path $bin 'velo-host.exe')
    type            = 'stdio'
    allowed_origins = @("chrome-extension://$extId/")
} | ConvertTo-Json | Set-Content -Encoding utf8 $manifest

foreach ($browser in 'Google\Chrome', 'Microsoft\Edge') {
    $key = "HKCU:\Software\$browser\NativeMessagingHosts\com.velo.host"
    New-Item -Path $key -Force | Out-Null
    Set-ItemProperty -Path $key -Name '(default)' -Value $manifest
}
# Remove leftovers from the app's old name (MyDM).
foreach ($browser in 'Google\Chrome', 'Microsoft\Edge') {
    Remove-Item "HKCU:\Software\$browser\NativeMessagingHosts\com.mydm.host" -ErrorAction SilentlyContinue
}
Remove-Item "$env:APPDATA\Microsoft\Windows\Start Menu\Programs\MyDM.lnk" -ErrorAction SilentlyContinue

# Start menu shortcut
$lnk = (New-Object -ComObject WScript.Shell).CreateShortcut("$env:APPDATA\Microsoft\Windows\Start Menu\Programs\Velo.lnk")
$lnk.TargetPath = Join-Path $bin 'velo.exe'
$lnk.Save()

Write-Host "Installed. Load '$root\extension' as an unpacked extension (ID $extId)."
