#Requires -Version 5.1
<#
.SYNOPSIS
    Run the same local checks that CI expects before a desktop build.

.PARAMETER SkipRace
    Skip `go test -race` for a faster edit loop.

.PARAMETER SkipFrontend
    Skip Bun install and `bun run verify`.

.PARAMETER SkipBuild
    Skip `go build ./...`.
#>
[CmdletBinding()]
param(
    [switch]$SkipRace,
    [switch]$SkipFrontend,
    [switch]$SkipBuild
)

Set-StrictMode -Version Latest
$ErrorActionPreference = 'Stop'

$RepoRoot = (Resolve-Path (Join-Path $PSScriptRoot '..')).Path

function Test-CommandExists {
    param([Parameter(Mandatory = $true)][string]$Name)
    return $null -ne (Get-Command $Name -ErrorAction SilentlyContinue)
}

function Invoke-Checked {
    param(
        [Parameter(Mandatory = $true)][string]$Label,
        [Parameter(Mandatory = $true)][scriptblock]$Script
    )
    Write-Host "== $Label =="
    & $Script
    if ($LASTEXITCODE -ne 0) {
        throw "$Label failed with exit code $LASTEXITCODE"
    }
}

if (-not (Test-CommandExists 'go')) {
    throw 'Go is not on PATH. Install Go 1.25 or newer.'
}
if (-not $SkipFrontend -and -not (Test-CommandExists 'bun')) {
    throw 'Bun is not on PATH. Install Bun 1.3.14 or newer, or pass -SkipFrontend.'
}

Push-Location $RepoRoot
try {
    Invoke-Checked 'go test ./...' { go test ./... }

    if (-not $SkipRace) {
        Invoke-Checked 'go test -race ./...' { go test -race ./... }
    }

    Invoke-Checked 'go vet ./...' { go vet ./... }

    if (-not $SkipBuild) {
        Invoke-Checked 'go build ./...' { go build ./... }
    }

    if (-not $SkipFrontend) {
        $frontend = Join-Path $RepoRoot 'desktop\frontend'
        Push-Location $frontend
        try {
            Invoke-Checked 'bun install --frozen-lockfile' { bun install --frozen-lockfile }
            Invoke-Checked 'bun run verify' { bun run verify }
        }
        finally {
            Pop-Location
        }
    }
}
finally {
    Pop-Location
}

Write-Host 'All requested checks passed.'
