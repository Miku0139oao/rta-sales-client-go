#Requires -Version 5.1
<#
.SYNOPSIS
    Build the Windows portable exe and/or NSIS installer into release/.

.PARAMETER SkipPortable
    Do not produce RTA-Excel-Filler-portable.exe.

.PARAMETER SkipInstaller
    Do not produce RTA-Excel-Filler-setup.exe.

.PARAMETER RequireSign
    Fail the build if Microsoft Trusted Signing is not available.
#>
[CmdletBinding()]
param(
    [switch]$SkipPortable,
    [switch]$SkipInstaller,
    [switch]$RequireSign
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
if (-not $SkipInstaller -and -not (Test-CommandExists 'makensis')) {
    throw 'NSIS (makensis) is not on PATH. Install NSIS 3.12, or pass -SkipInstaller.'
}

$WailsCommand = Get-Wails3Command

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
        go build -tags production -trimpath -ldflags '-w -s -H windowsgui' -o $builtExe .
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

if (-not $SkipPortable) {
    Copy-Item -LiteralPath $builtExe -Destination (Join-Path $ReleaseDir 'RTA-Excel-Filler-portable.exe') -Force
}

if (-not $SkipInstaller) {
    $nsisDir = Join-Path $AppDir 'build\windows\installer'
    Invoke-Wails3 @('generate', 'webview2bootstrapper', '-dir', $nsisDir)
    $generatedBootstrapper = Join-Path $nsisDir 'MicrosoftEdgeWebview2Setup.exe'
    $bootstrapper = Join-Path $nsisDir 'tmp\MicrosoftEdgeWebview2Setup.exe'
    New-Item -ItemType Directory -Force -Path (Split-Path $bootstrapper) | Out-Null
    if (Test-Path -LiteralPath $generatedBootstrapper) {
        Copy-Item -LiteralPath $generatedBootstrapper -Destination $bootstrapper -Force
    }
    if (-not (Test-Path -LiteralPath $bootstrapper)) {
        throw "WebView2 bootstrapper was not generated at $bootstrapper"
    }

    $wailsInfo = Get-Content -LiteralPath (Join-Path $AppDir 'wails.json') -Raw | ConvertFrom-Json
    $productName = [string]$wailsInfo.info.productName
    $productVersion = [string]$wailsInfo.info.productVersion
    if ([string]::IsNullOrWhiteSpace($productName) -or [string]::IsNullOrWhiteSpace($productVersion)) {
        throw 'wails.json is missing info.productName or info.productVersion'
    }

    Push-Location $nsisDir
    try {
        Invoke-Checked 'makensis installer' {
            makensis `
                -DREQUEST_EXECUTION_LEVEL=user `
                -DWAILS_INSTALL_SCOPE=user `
                "-DARG_WAILS_AMD64_BINARY=$builtExe" `
                "-DINFO_PRODUCTNAME=$productName" `
                "-DINFO_PRODUCTVERSION=$productVersion" `
                project.nsi
        }
    }
    finally {
        Pop-Location
    }

    $installer = @(Get-ChildItem -LiteralPath $BinDir -Filter '*-installer.exe' | Select-Object -First 1)
    if ($installer.Count -eq 0) {
        throw 'makensis did not produce an NSIS installer'
    }
    $setupPath = Join-Path $ReleaseDir 'RTA-Excel-Filler-setup.exe'
    Copy-Item -LiteralPath $installer[0].FullName -Destination $setupPath -Force
    Invoke-AuthenticodeSign -Files @($setupPath)
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
