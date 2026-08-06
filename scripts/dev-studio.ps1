# ============================================================
# AgentPrimordia Studio - One-click launcher (Windows)
#
# Starts backend (:8090) + frontend (:5173) together.
# Ctrl+C stops both. Opens browser when ready.
#
# Usage:
#   powershell -ExecutionPolicy Bypass -File scripts\dev-studio.ps1
# ============================================================

param(
    [int]$BackendPort = 8090,
    [int]$FrontendPort = 5173,
    [switch]$NoBrowser
)

$ErrorActionPreference = 'Stop'
$Root = Split-Path -Parent $PSScriptRoot
$BackendDir = Join-Path $Root 'agentprimordia'
$FrontendDir = Join-Path $Root 'agentprimordia\studio\web'

Write-Host "==> AgentPrimordia Studio launcher" -ForegroundColor Cyan
Write-Host "    backend  :$BackendPort  (go run ./cmd/studio)"
Write-Host "    frontend :$FrontendPort  (npm run dev)"

foreach ($cmd in 'go', 'npm') {
    if (-not (Get-Command $cmd -ErrorAction SilentlyContinue)) {
        Write-Error "Missing command: $cmd (install Go and Node.js first)"
    }
}

if (-not (Test-Path (Join-Path $FrontendDir 'node_modules'))) {
    Write-Host "==> First run: installing frontend deps..." -ForegroundColor Yellow
    Push-Location $FrontendDir
    try { npm install --no-audit --no-fund } finally { Pop-Location }
}

Write-Host "==> Starting backend..." -ForegroundColor Green
$backend = Start-Process -FilePath 'go' -ArgumentList @('run', './cmd/studio', "-addr", ":$BackendPort") `
    -WorkingDirectory $BackendDir -WindowStyle Hidden -PassThru

Write-Host "==> Starting frontend..." -ForegroundColor Green
$frontend = Start-Process -FilePath 'npm.cmd' -ArgumentList @('run', 'dev', '--', '--port', "$FrontendPort", '--strictPort') `
    -WorkingDirectory $FrontendDir -WindowStyle Hidden -PassThru

Write-Host ""

$backendUp = $false
$frontendUp = $false
for ($i = 0; $i -lt 30 -and -not ($backendUp -and $frontendUp); $i++) {
    Start-Sleep -Milliseconds 1000
    if (-not $backendUp) {
        try { Invoke-WebRequest -Uri "http://localhost:$BackendPort/api/v1/cluster/status" -UseBasicParsing -TimeoutSec 1 | Out-Null; $backendUp = $true } catch {}
    }
    if (-not $frontendUp) {
        try { Invoke-WebRequest -Uri "http://localhost:$FrontendPort/" -UseBasicParsing -TimeoutSec 1 | Out-Null; $frontendUp = $true } catch {}
    }
}

if (-not ($backendUp -and $frontendUp)) {
    Write-Host "Warning: services not ready within 30s - check output." -ForegroundColor Yellow
}

$url = "http://localhost:$FrontendPort"
Write-Host "==> Ready!" -ForegroundColor Green
Write-Host "    Open $url"
if (-not $NoBrowser) {
    try { Start-Process $url } catch {}
}
Write-Host ""
Write-Host "Press Ctrl+C to stop both servers..." -ForegroundColor DarkGray

try {
    Wait-Process -Id $backend.Id -ErrorAction SilentlyContinue
    Wait-Process -Id $frontend.Id -ErrorAction SilentlyContinue
} finally {
    Stop-Process -Id $backend.Id -Force -ErrorAction SilentlyContinue
    Stop-Process -Id $frontend.Id -Force -ErrorAction SilentlyContinue
    Write-Host ""
    Write-Host "Backend and frontend stopped." -ForegroundColor Cyan
}
