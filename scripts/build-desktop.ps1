#Requires -Version 5.1
<#
.SYNOPSIS
    Build the Windows portable exe and/or NSIS installer into release/.

.PARAMETER SkipPortable
    Do not produce RTA-Excel-Filler-portable.exe.

.PARAMETER SkipInstaller
    Do not produce RTA-Excel-Filler-setup.exe.
#>
[CmdletBinding()]
param(
    [switch]$SkipPortable,
    [switch]$SkipInstaller
)

Set-StrictMode -Version Latest
$ErrorActionPreference = 'Stop'

if ($SkipPortable -and $SkipInstaller) {
    throw 'Nothing to build: both -SkipPortable and -SkipInstaller were set.'
}

$RepoRoot = (Resolve-Path (Join-Path $PSScriptRoot '..')).Path
$AppDir = Join-Path $RepoRoot 'cmd\rta-excel-filler'
$FrontendDir = Join-Path $RepoRoot 'desktop\frontend'
$ReleaseDir = Join-Path $RepoRoot 'release'
$WailsVersion = 'v2.14.0'
$WailsModule = "github.com/wailsapp/wails/v2/cmd/wails@$WailsVersion"

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

function Get-WailsCommand {
    $existing = Get-Command wails -ErrorAction SilentlyContinue
    if ($null -ne $existing) {
        $reported = (& wails version 2>$null | Out-String)
        if ($reported -match [regex]::Escape($WailsVersion)) {
            return @{ FilePath = $existing.Source; Prefix = @() }
        }
    }

    $goPath = (go env GOPATH).Trim()
    $installed = Join-Path $goPath "bin\wails.exe"
    if (Test-Path -LiteralPath $installed) {
        $reported = (& $installed version 2>$null | Out-String)
        if ($reported -match [regex]::Escape($WailsVersion)) {
            return @{ FilePath = $installed; Prefix = @() }
        }
    }

    Write-Host "Using go run $WailsModule"
    return @{ FilePath = 'go'; Prefix = @('run', $WailsModule) }
}

if (-not (Test-CommandExists 'go')) {
    throw 'Go is not on PATH. Install Go 1.25 or newer.'
}
if (-not (Test-CommandExists 'bun')) {
    throw 'Bun is not on PATH. Install Bun 1.3.14 or newer.'
}
if (-not $SkipInstaller -and -not (Test-CommandExists 'makensis')) {
    throw 'NSIS (makensis) is not on PATH. Install NSIS 3.12, or pass -SkipInstaller.'
}

$WailsCommand = Get-WailsCommand

function Invoke-Wails {
    param([Parameter(Mandatory = $true)][string[]]$WailsArgs)
    $command = $WailsCommand
    $allArgs = @($command.Prefix + $WailsArgs)
    Write-Host ("{0} {1}" -f $command.FilePath, ($allArgs -join ' '))
    & $command.FilePath @allArgs
    if ($LASTEXITCODE -ne 0) {
        throw "wails $($WailsArgs[0]) failed with exit code $LASTEXITCODE"
    }
}

New-Item -ItemType Directory -Force -Path $ReleaseDir | Out-Null

Push-Location $FrontendDir
try {
    Invoke-Checked 'bun install --frozen-lockfile' { bun install --frozen-lockfile }
    Invoke-Checked 'bun run build' { bun run build }
}
finally {
    Pop-Location
}

$firstBuildUsesCleanFrontend = $true
Push-Location $AppDir
try {
    if (-not $SkipPortable) {
        Invoke-Wails @(
            'build', '-s', '-clean', '-trimpath',
            '-platform', 'windows/amd64',
            '-webview2', 'error',
            '-o', 'RTA-Excel-Filler.exe'
        )
        $portableSource = Join-Path $AppDir 'build\bin\RTA-Excel-Filler.exe'
        if (-not (Test-Path -LiteralPath $portableSource)) {
            throw "Wails did not produce $portableSource"
        }
        Copy-Item -LiteralPath $portableSource -Destination (Join-Path $ReleaseDir 'RTA-Excel-Filler-portable.exe') -Force
        $firstBuildUsesCleanFrontend = $false
    }

    if (-not $SkipInstaller) {
        $installerArgs = @(
            'build', '-s', '-trimpath',
            '-platform', 'windows/amd64',
            '-webview2', 'download',
            '-nsis', '-installscope', 'user',
            '-o', 'RTA-Excel-Filler.exe'
        )
        if ($firstBuildUsesCleanFrontend) {
            $installerArgs = @('build', '-s', '-clean') + $installerArgs[2..($installerArgs.Length - 1)]
        }
        else {
            $installerArgs = @('build', '-s', '-skipbindings') + $installerArgs[2..($installerArgs.Length - 1)]
        }
        Invoke-Wails $installerArgs

        $installer = @(Get-ChildItem -LiteralPath (Join-Path $AppDir 'build\bin') -Filter '*-installer.exe' | Select-Object -First 1)
        if ($installer.Count -eq 0) {
            throw 'Wails did not produce an NSIS installer'
        }
        Copy-Item -LiteralPath $installer[0].FullName -Destination (Join-Path $ReleaseDir 'RTA-Excel-Filler-setup.exe') -Force
    }
}
finally {
    Pop-Location
}

$checksums = @(
    Get-ChildItem -LiteralPath $ReleaseDir -Filter '*.exe' |
        Sort-Object Name |
        ForEach-Object {
            $hash = Get-FileHash -Algorithm SHA256 -LiteralPath $_.FullName
            '{0}  {1}' -f $hash.Hash.ToLowerInvariant(), $_.Name
        }
)
if ($checksums.Count -eq 0) {
    throw "No executables were written to $ReleaseDir"
}
$checksums | Set-Content -LiteralPath (Join-Path $ReleaseDir 'SHA256SUMS.txt') -Encoding ascii
Write-Host "Desktop artifacts written to $ReleaseDir"
