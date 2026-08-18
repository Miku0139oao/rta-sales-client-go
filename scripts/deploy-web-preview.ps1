# Build the live web edition and deploy UI + rta-web to rtasales.com.
# Usage (from repo root): .\scripts\deploy-web-preview.ps1

$ErrorActionPreference = 'Stop'

$root = Resolve-Path (Join-Path $PSScriptRoot '..')
$frontend = Join-Path $root 'desktop\frontend'
$dist = Join-Path $frontend 'dist-web'
$hostName = 'root@miku.zerotwo02.net'
$remote = '/srv/pre-rtasales'
$binary = Join-Path $root 'rta-web-linux-amd64'

Set-Location $frontend
bun run build:web

if (-not (Test-Path (Join-Path $dist 'index.html'))) {
    throw "web build did not produce dist-web/index.html"
}

Set-Location $root
$env:GOOS = 'linux'
$env:GOARCH = 'amd64'
$env:CGO_ENABLED = '0'
go build -trimpath -ldflags '-s -w' -o $binary ./cmd/rta-web
Remove-Item Env:GOOS, Env:GOARCH, Env:CGO_ENABLED -ErrorAction SilentlyContinue

if (-not (Test-Path $binary)) {
    throw 'linux rta-web binary was not built'
}

ssh $hostName "mkdir -p $remote /usr/local/lib/systemd/system"
scp -r "$dist\*" "${hostName}:${remote}/"
scp $binary "${hostName}:/usr/local/bin/rta-web"
scp (Join-Path $root 'deploy\rta-web.service') "${hostName}:/etc/systemd/system/rta-web.service"
scp (Join-Path $root 'deploy\rtasales.caddy') "${hostName}:/tmp/rtasales.caddy"
scp (Join-Path $root 'deploy\apply-rtasales-caddy.py') "${hostName}:/tmp/apply-rtasales-caddy.py"
ssh $hostName @"
set -euo pipefail
chmod 755 /usr/local/bin/rta-web
chown caddy:caddy /usr/local/bin/rta-web
find $remote -type d -exec chmod 755 {} \;
find $remote -type f -exec chmod 644 {} \;
chown -R caddy:caddy $remote
python3 /tmp/apply-rtasales-caddy.py
systemctl daemon-reload
systemctl enable --now rta-web
systemctl restart rta-web
caddy validate --config /etc/caddy/Caddyfile
systemctl reload caddy
"@
Write-Host "Deployed live web edition to ${hostName}:$remote (rtasales.com)"
