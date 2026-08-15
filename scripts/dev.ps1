#Requires -Version 5.1
<#
.SYNOPSIS
    Start the Wails v3 desktop app with frontend hot reload.
#>
[CmdletBinding()]
param()

Set-StrictMode -Version Latest
$ErrorActionPreference = 'Stop'

$RepoRoot = (Resolve-Path (Join-Path $PSScriptRoot '..')).Path
$AppDir = Join-Path $RepoRoot 'cmd\rta-excel-filler'
$WailsVersion = 'v3.0.0-beta.8'
$WailsModule = "github.com/wailsapp/wails/v3/cmd/wails3@$WailsVersion"

function Test-CommandExists {
    param([Parameter(Mandatory = $true)][string]$Name)
    return $null -ne (Get-Command $Name -ErrorAction SilentlyContinue)
}

if (-not (Test-CommandExists 'go')) {
    throw 'Go is not on PATH. Install Go 1.25 or newer.'
}
if (-not (Test-CommandExists 'bun')) {
    throw 'Bun is not on PATH. Install Bun 1.3.14 or newer.'
}

$env:FRONTEND_DEVSERVER_URL = 'http://127.0.0.1:9245'
$env:WAILS_VITE_PORT = '9245'

function Get-Wails3Command {
    $existing = Get-Command wails3 -ErrorAction SilentlyContinue
    if ($null -ne $existing) {
        $reported = (& wails3 version 2>$null | Out-String)
        if ($reported -match [regex]::Escape($WailsVersion)) {
            return @{ FilePath = $existing.Source; Prefix = @() }
        }
    }

    $goPath = (go env GOPATH).Trim()
    $installed = Join-Path $goPath 'bin\wails3.exe'
    if (Test-Path -LiteralPath $installed) {
        $reported = (& $installed version 2>$null | Out-String)
        if ($reported -match [regex]::Escape($WailsVersion)) {
            return @{ FilePath = $installed; Prefix = @() }
        }
    }

    Write-Host "Using go run $WailsModule"
    return @{ FilePath = 'go'; Prefix = @('run', $WailsModule) }
}

$WailsCommand = Get-Wails3Command
Write-Host "Starting Wails v3 dev with $WailsVersion"
Push-Location $AppDir
try {
    $allArgs = @($WailsCommand.Prefix + @('dev', '-config', '.\build\config.yml', '-port', '9245'))
    & $WailsCommand.FilePath @allArgs
    if ($LASTEXITCODE -ne 0) {
        throw "wails3 dev failed with exit code $LASTEXITCODE"
    }
}
finally {
    Pop-Location
}
