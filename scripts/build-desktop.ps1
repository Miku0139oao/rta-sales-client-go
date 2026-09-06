#Requires -Version 5.1
<#
.SYNOPSIS
    Build only the Windows portable exe into release/.
    WebView2 must be installed manually if absent.

.PARAMETER RequireSign
    Fail the build if Microsoft Trusted Signing is not available.
#>
[CmdletBinding()]
param(
    [switch]$RequireSign
)

Set-StrictMode -Version Latest
$ErrorActionPreference = 'Stop'

$RepoRoot = (Resolve-Path (Join-Path $PSScriptRoot '..')).Path
$AppDir = Join-Path $RepoRoot 'cmd\rta-excel-filler'
$FrontendDir = Join-Path $RepoRoot 'desktop\frontend'
$ReleaseDir = Join-Path $RepoRoot 'release'
$BinDir = Join-Path $AppDir 'build\bin'
$WailsVersion = 'v3.0.0-beta.8'
$WailsModule = "github.com/wailsapp/wails/v3/cmd/wails3@$WailsVersion"

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
    if ($null -ne $LASTEXITCODE -and $LASTEXITCODE -ne 0) {
        throw "$Label failed with exit code $LASTEXITCODE"
    }
}

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

if (-not (Test-CommandExists 'go')) {
    throw 'Go is not on PATH. Install Go 1.25 or newer.'
}
if (-not (Test-CommandExists 'bun')) {
    throw 'Bun is not on PATH. Install Bun 1.3.14 or newer.'
}

$WailsCommand = Get-Wails3Command
$wailsInfo = Get-Content -LiteralPath (Join-Path $AppDir 'wails.json') -Raw | ConvertFrom-Json
$Version = [string]$wailsInfo.info.productVersion
if ($Version -notmatch '^(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)$') {
    throw 'wails.json must contain a stable numeric productVersion.'
}
$resourceInfo = Get-Content -LiteralPath (Join-Path $AppDir 'build\windows\info.json') -Raw | ConvertFrom-Json
if ($resourceInfo.fixed.product_version -ne "$Version.0" -or
    $resourceInfo.fixed.file_version -ne "$Version.0" -or
    $resourceInfo.info.'0409'.ProductVersion -ne $Version) {
    throw 'Windows resource versions must match wails.json.'
}
if ($env:GITHUB_REF_TYPE -eq 'tag' -and $env:GITHUB_REF_NAME -cne "v$Version") {
    throw 'Build tag does not match productVersion.'
}

function Invoke-AuthenticodeSign {
    param([Parameter(Mandatory = $true)][string[]]$Files)
    $signer = Join-Path $PSScriptRoot 'sign-windows.ps1'
    $signArgs = @{ Files = $Files }
    if ($RequireSign) {
        $signArgs.Required = $true
    }
    & $signer @signArgs
    if ($null -ne $LASTEXITCODE -and $LASTEXITCODE -ne 0) {
        throw "Authenticode signing failed with exit code $LASTEXITCODE"
    }
}

function Invoke-Wails3 {
    param([Parameter(Mandatory = $true)][string[]]$WailsArgs)
    $allArgs = @($WailsCommand.Prefix + $WailsArgs)
    Write-Host ("{0} {1}" -f $WailsCommand.FilePath, ($allArgs -join ' '))
    & $WailsCommand.FilePath @allArgs
    if ($LASTEXITCODE -ne 0) {
        throw "wails3 $($WailsArgs[0]) failed with exit code $LASTEXITCODE"
    }
}

New-Item -ItemType Directory -Force -Path $ReleaseDir, $BinDir | Out-Null

Push-Location $FrontendDir
try {
    Invoke-Checked 'bun install --frozen-lockfile' { bun install --frozen-lockfile }
    Invoke-Checked 'bun run build' { bun run build }
}
finally {
    Pop-Location
}

$exeName = 'RTA-Excel-Filler.exe'
$builtExe = Join-Path $BinDir $exeName
$syso = Join-Path $AppDir 'wails_windows_amd64.syso'

Push-Location $AppDir
try {
    Invoke-Wails3 @(
        'generate', 'syso',
        '-arch', 'amd64',
        '-icon', 'build\windows\icon.ico',
        '-manifest', 'build\windows\wails.exe.manifest',
        '-info', 'build\windows\info.json',
        '-out', $syso
    )

    Invoke-Checked 'go build desktop' {
        go build -tags production -trimpath -ldflags "-w -s -H windowsgui -X github.com/Miku0139oao/rta-sales-client-go/internal/buildinfo.Version=$Version" -o $builtExe .
    }
}
finally {
    if (Test-Path -LiteralPath $syso) {
        Remove-Item -LiteralPath $syso -Force
    }
    Pop-Location
}

if (-not (Test-Path -LiteralPath $builtExe)) {
    throw "go build did not produce $builtExe"
}

Invoke-AuthenticodeSign -Files @($builtExe)

$portablePath = Join-Path $ReleaseDir 'RTA-Excel-Filler-portable.exe'
Copy-Item -LiteralPath $builtExe -Destination $portablePath -Force
# Hash this build's exact output only, after signing. Ignore stale release files.
$hash = Get-FileHash -Algorithm SHA256 -LiteralPath $portablePath
('{0}  RTA-Excel-Filler-portable.exe' -f $hash.Hash.ToLowerInvariant()) |
    Set-Content -LiteralPath (Join-Path $ReleaseDir 'SHA256SUMS.txt') -Encoding ascii
Write-Host "Desktop artifacts written to $ReleaseDir"
