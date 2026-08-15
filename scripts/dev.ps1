#Requires -Version 5.1
<#
.SYNOPSIS
    Start the Wails desktop app with frontend hot reload.
#>
[CmdletBinding()]
param()

Set-StrictMode -Version Latest
$ErrorActionPreference = 'Stop'

$RepoRoot = (Resolve-Path (Join-Path $PSScriptRoot '..')).Path
$AppDir = Join-Path $RepoRoot 'cmd\rta-excel-filler'
$WailsModule = 'github.com/wailsapp/wails/v2/cmd/wails@v2.14.0'

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

Write-Host "Starting Wails dev with $WailsModule"
Push-Location $AppDir
try {
    & go run $WailsModule dev
    if ($LASTEXITCODE -ne 0) {
        throw "wails dev failed with exit code $LASTEXITCODE"
    }
}
finally {
    Pop-Location
}
