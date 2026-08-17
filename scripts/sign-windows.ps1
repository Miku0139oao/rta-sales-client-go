#Requires -Version 5.1
<#
.SYNOPSIS
    Sign Windows PE files with Microsoft Trusted Signing (Azure Code Signing).

    Uses the machine-local SignTool, Azure.CodeSigning.Dlib, and metadata.json.
    Skips when those tools are absent (CI), unless -Required is set.

.PARAMETER Files
    Files to sign. Non-existent paths are an error.

.PARAMETER Required
    Fail if Trusted Signing is not configured or signing fails.
#>
[CmdletBinding()]
param(
    [Parameter(Mandatory = $true)]
    [string[]]$Files,
    [switch]$Required
)

Set-StrictMode -Version Latest
$ErrorActionPreference = 'Stop'

if ($env:RTA_SIGN_SKIP -eq '1') {
    Write-Host 'Skipping Authenticode signing because RTA_SIGN_SKIP=1'
    if ($Required) {
        throw 'RTA_SIGN_SKIP=1 cannot be combined with -Required'
    }
    return
}

function Resolve-ExistingFile {
    param([string[]]$Candidates)
    foreach ($candidate in $Candidates) {
        if (-not [string]::IsNullOrWhiteSpace($candidate) -and (Test-Path -LiteralPath $candidate -PathType Leaf)) {
            return (Resolve-Path -LiteralPath $candidate).Path
        }
    }
    return $null
}

$homeDir = [Environment]::GetFolderPath('UserProfile')
$kitRoot = Join-Path ${env:ProgramFiles(x86)} 'Windows Kits\10\bin'
$kitSignTool = $null
if (Test-Path -LiteralPath $kitRoot) {
    $kitSignTool = Get-ChildItem -LiteralPath $kitRoot -Recurse -Filter 'signtool.exe' -ErrorAction SilentlyContinue |
        Where-Object { $_.Directory.Name -eq 'x64' } |
        Sort-Object FullName -Descending |
        Select-Object -First 1 -ExpandProperty FullName
}

$signTool = Resolve-ExistingFile @(
    $env:RTA_SIGNTOOL,
    (Join-Path $homeDir 'lib\Microsoft.Windows.SDK.BuildTools\bin\10.0.26100.0\x64\signtool.exe'),
    $kitSignTool
)
$dlib = Resolve-ExistingFile @(
    $env:RTA_SIGN_DLIB,
    (Join-Path $homeDir 'lib\Microsoft.Trusted.Signing.Client\bin\x64\Azure.CodeSigning.Dlib.dll')
)
$metadata = Resolve-ExistingFile @(
    $env:RTA_SIGN_METADATA,
    (Join-Path $homeDir 'lib\Microsoft.Trusted.Signing.Client\bin\x64\metadata.json')
)

if (-not $signTool -or -not $dlib -or -not $metadata) {
    $missing = @()
    if (-not $signTool) { $missing += 'signtool.exe' }
    if (-not $dlib) { $missing += 'Azure.CodeSigning.Dlib.dll' }
    if (-not $metadata) { $missing += 'metadata.json' }
    $message = "Microsoft Trusted Signing is not configured (missing $($missing -join ', '))"
    if ($Required) {
        throw $message
    }
    Write-Warning "$message; shipping unsigned binaries. Set RTA_SIGN_METADATA / RTA_SIGNTOOL / RTA_SIGN_DLIB, or sign later with scripts/sign-windows.ps1 -Required."
    return
}

foreach ($file in $Files) {
    if (-not (Test-Path -LiteralPath $file -PathType Leaf)) {
        throw "Cannot sign missing file: $file"
    }
}

Write-Host "== Authenticode (Microsoft Trusted Signing) =="
Write-Host "signtool  $signTool"
Write-Host "dlib      $dlib"
Write-Host "metadata  $metadata"

foreach ($file in $Files) {
    $path = (Resolve-Path -LiteralPath $file).Path
    Write-Host "Signing $path"
    & $signTool sign `
        /fd sha256 `
        /td sha256 `
        /tr 'http://timestamp.acs.microsoft.com' `
        /dlib $dlib `
        /dmdf $metadata `
        $path
    if ($LASTEXITCODE -ne 0) {
        throw "signtool failed for $path with exit code $LASTEXITCODE"
    }
    $signature = Get-AuthenticodeSignature -FilePath $path
    if ($signature.Status -ne 'Valid') {
        throw "signature for $path is $($signature.Status)"
    }
    Write-Host ("Signed {0} as {1}" -f $path, $signature.SignerCertificate.Subject)
}
