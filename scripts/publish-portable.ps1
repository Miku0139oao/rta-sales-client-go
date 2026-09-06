#Requires -Version 5.1
<#
.SYNOPSIS
Build, sign and explicitly publish an existing draft Windows portable release.
Never modifies a published release. Requires a trusted previous portable executable
as the publisher reference (certificate renewal with the same subject is allowed).
#>
[CmdletBinding(SupportsShouldProcess = $true, ConfirmImpact = 'High')]
param(
    [Parameter(Mandatory = $true)][string]$Tag,
    [Parameter(Mandatory = $true)][string]$Commit,
    [Parameter(Mandatory = $true)][string]$PublisherReference
)
Set-StrictMode -Version Latest
$ErrorActionPreference = 'Stop'
$Repo = 'Miku0139oao/rta-sales-client-go'
$Root = (Resolve-Path (Join-Path $PSScriptRoot '..')).Path
if ($Tag -cnotmatch '^v(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)$') { throw 'Stable version tag required' }
if ($Commit -cnotmatch '^[0-9a-f]{40}$') { throw 'Full lowercase commit SHA required' }

function Checked-Git {
    param([string[]]$Arguments)
    $result = & git @Arguments
    if ($LASTEXITCODE -ne 0) { throw "git failed: $($Arguments -join ' ')" }
    return $result
}
function Get-Draft {
    $raw = gh release view $Tag --repo $Repo --json isDraft,isPrerelease,tagName,body,name
    if ($LASTEXITCODE -ne 0) { throw 'An existing draft release is required' }
    $draft = $raw | ConvertFrom-Json
    if (-not $draft.isDraft -or $draft.isPrerelease -or $draft.tagName -cne $Tag) {
        throw 'Refusing to alter a published release, prerelease, or different tag'
    }
    if ([string]::IsNullOrWhiteSpace($draft.body) -or $draft.body -match 'Unsigned CI staging only' -or $draft.name -match 'unsigned draft') {
        throw 'Replace the CI placeholder title and notes with curated release information before publishing'
    }
}
function Get-TrustedPublisher {
    param([string]$Path)
    $signature = Get-AuthenticodeSignature -LiteralPath $Path
    if ($signature.Status -ne 'Valid' -or $null -eq $signature.SignerCertificate -or
        [string]::IsNullOrWhiteSpace($signature.SignerCertificate.Subject)) {
        throw "Authenticode trust validation failed: $Path"
    }
    return $signature.SignerCertificate.Subject
}

Push-Location $Root
try {
    if (Checked-Git -Arguments @('status', '--porcelain')) { throw 'Publishing requires a clean checkout' }
    $version = (Get-Content cmd/rta-excel-filler/wails.json -Raw | ConvertFrom-Json).info.productVersion
    if ($Tag -cne "v$version") { throw 'Tag and configured productVersion differ' }
    $head = Checked-Git -Arguments @('rev-parse', 'HEAD')
    $tagCommit = Checked-Git -Arguments @('rev-parse', '--verify', "$Tag^{commit}")
    if ($head -cne $Commit -or $tagCommit -cne $Commit) { throw 'HEAD, tag and requested commit must match exactly' }
    $remote = @(Checked-Git -Arguments @('ls-remote', "https://github.com/$Repo.git", "refs/tags/$Tag", "refs/tags/$Tag^{}"))
    $peeled = @($remote | Where-Object { $_ -match '\^\{\}$' })
    $resolved = if ($peeled.Count -eq 1) { $peeled[0] } elseif ($remote.Count -eq 1) { $remote[0] } else { throw 'Remote tag cannot be resolved uniquely' }
    if (($resolved -split '\s+')[0] -cne $Commit) { throw 'Remote tag points at a different commit' }
    $publisher = Get-TrustedPublisher -Path (Resolve-Path -LiteralPath $PublisherReference).Path
    Get-Draft
    if (-not $PSCmdlet.ShouldProcess("$Repo $Tag at $Commit", 'Build, sign, replace DRAFT assets, validate remote copies and publish stable release')) { return }
    & (Join-Path $PSScriptRoot 'build-desktop.ps1') -RequireSign
    if ($LASTEXITCODE -ne 0) { throw 'Signed portable build failed' }
    $exe = Join-Path $Root 'release\RTA-Excel-Filler-portable.exe'
    if ((Get-TrustedPublisher -Path $exe) -cne $publisher) { throw 'Publisher differs from trusted reference' }
    $info = [System.Diagnostics.FileVersionInfo]::GetVersionInfo($exe)
    if ($info.ProductVersionRaw.ToString() -cne "$version.0" -or $info.FileVersionRaw.ToString() -cne "$version.0") { throw 'Signed executable version differs from tag' }
    $hash = (Get-FileHash -LiteralPath $exe -Algorithm SHA256).Hash.ToLowerInvariant()
    $expected = "$hash  RTA-Excel-Filler-portable.exe"
    $sums = Join-Path $Root 'release\SHA256SUMS.txt'
    if ((Get-Content -LiteralPath $sums -Raw).TrimEnd("`r", "`n") -cne $expected) { throw 'Checksum file differs from signed output' }
    Get-Draft
    gh release upload $Tag $exe $sums --repo $Repo --clobber
    if ($LASTEXITCODE -ne 0) { throw 'Draft upload failed' }
    $verify = Join-Path ([System.IO.Path]::GetTempPath()) ([Guid]::NewGuid().ToString('N'))
    New-Item -ItemType Directory -Path $verify | Out-Null
    try {
        gh release download $Tag --repo $Repo --pattern RTA-Excel-Filler-portable.exe --pattern SHA256SUMS.txt --dir $verify
        if ($LASTEXITCODE -ne 0) { throw 'Cannot verify uploaded draft assets' }
        $remoteExe = Join-Path $verify 'RTA-Excel-Filler-portable.exe'
        if ((Get-FileHash -LiteralPath $remoteExe -Algorithm SHA256).Hash.ToLowerInvariant() -cne $hash -or
            (Get-Content -LiteralPath (Join-Path $verify 'SHA256SUMS.txt') -Raw).TrimEnd("`r", "`n") -cne $expected -or
            (Get-TrustedPublisher -Path $remoteExe) -cne $publisher) { throw 'Remote draft artifact validation failed' }
        Get-Draft
        # Preserve the draft's curated title, release notes and screenshot links.
        gh release edit $Tag --repo $Repo --draft=false --latest
        if ($LASTEXITCODE -ne 0) { throw 'Publish failed; inspect release state before retrying' }
    }
    finally { Remove-Item -LiteralPath $verify -Recurse -Force }
}
finally { Pop-Location }
