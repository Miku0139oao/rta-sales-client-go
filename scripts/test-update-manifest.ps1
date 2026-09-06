#Requires -Version 7.5
Set-StrictMode -Version Latest
$ErrorActionPreference = 'Stop'
. (Join-Path $PSScriptRoot 'generate-update-manifest.ps1')
function Fixture {
    return @{
        id=1; tag_name='v0.5.0'; draft=$false; prerelease=$false; body='Release notes'; updated_at='2026-01-01T00:00:00Z'
        assets=@(
            @{id=2; name='RTA-Excel-Filler-portable.exe'; browser_download_url='https://github.com/Miku0139oao/rta-sales-client-go/releases/download/v0.5.0/RTA-Excel-Filler-portable.exe'; size=4; digest=('sha256:'+('a'*64)); updated_at='2026-01-01T00:00:00Z'},
            @{id=3; name='SHA256SUMS.txt'; browser_download_url='https://github.com/Miku0139oao/rta-sales-client-go/releases/download/v0.5.0/SHA256SUMS.txt'; size=100; digest=('sha256:'+('b'*64)); updated_at='2026-01-01T00:00:00Z'}
        )
    }
}
$script:Checks = 0
function Reject([scriptblock]$Action) {
    $rejected=$false
    try { & $Action | Out-Null } catch { $rejected=$true }
    if (-not $rejected) { throw 'Invalid fixture accepted' }
    $script:Checks++
}
$valid=Assert-ReleaseMetadata (Fixture)
if ($valid.tag_name -cne 'v0.5.0' -or $valid.assets.Count -ne 2) { throw 'Valid fixture rejected' }
# Exercise the exact API JSON conversion as well as direct synthetic objects.
$roundTrip = Fixture | ConvertTo-Json -Depth 10 | ConvertFrom-Json -AsHashtable -DateKind String
Assert-ReleaseMetadata $roundTrip | Out-Null
foreach ($mutation in @(
    {param($r) $r.draft=$true}, {param($r) $r.prerelease=$true},
    {param($r) $r.Remove('draft')}, {param($r) $r.prerelease='false'},
    {param($r) $r.tag_name='v0.5.0-beta'}, {param($r) $r.tag_name='v01.5.0'},
    {param($r) $r.tag_name='v999999999999999999999.1.1'},
    {param($r) $r.assets+= $r.assets[0]}, {param($r) $r.assets=@($r.assets[0])},
    {param($r) $r.assets[0].browser_download_url='https://evil.example/x'},
    {param($r) $r.assets[0].browser_download_url+='?credential=x'},
    {param($r) $r.assets[0].size=0}, {param($r) $r.assets[0].size=256MB+1},
    {param($r) $r.assets[1].size=64KB+1}, {param($r) $r.assets[0].size='4'},
    {param($r) $r.assets[0].Remove('digest')}, {param($r) $r.assets[0].digest='sha256:bad'},
    {param($r) $r.assets[1].id=2}, {param($r) $r.Remove('id')},
    {param($r) $r.body=$null}
)) { $r=Fixture; & $mutation $r | Out-Null; Reject { Assert-ReleaseMetadata $r } }
$hash='a'*64; $line="$hash  RTA-Excel-Filler-portable.exe`n"
Assert-Checksums $line $hash
Assert-Checksums "$hash *RTA-Excel-Filler-portable.exe`r`n" $hash
Reject { Assert-Checksums ($line+$line) $hash }
Reject { Assert-Checksums ($line+'bad line here') $hash }
Reject { Assert-Checksums $line ('b'*64) }
Reject { Assert-Checksums "$hash  other.exe" $hash }
$temp=Join-Path ([IO.Path]::GetTempPath()) ([Guid]::NewGuid().ToString('N'))
New-Item -ItemType Directory $temp | Out-Null
try {
    $path=Join-Path $temp 'data.bin'
    [IO.File]::WriteAllText($path,'test',[Text.UTF8Encoding]::new($false))
    $asset=@{size=4;digest=('sha256:'+(Get-FileHash $path).Hash.ToLowerInvariant())}
    Assert-AssetHash $path $asset
    $asset.size=5; Reject { Assert-AssetHash $path $asset }
    $asset.size=4; $asset.digest='sha256:'+('0'*64); Reject { Assert-AssetHash $path $asset }
    Reject { Assert-PortablePE $path '0.5.0' }
    if ($IsWindows) { Reject { Get-ValidatedPublisher $path } }
    Reject { Get-VerifiedDownload 'https://api.github.com/x' $path 10 }
    Reject { Get-VerifiedDownload 'http://github.com/x' $path 10 }
    # Invalid latest metadata cannot create or replace a staged manifest.
    function Get-LatestStable { throw 'Synthetic invalid metadata / API failure' }
    $out=Join-Path $temp 'updates'; New-Item -ItemType Directory $out | Out-Null
    $sentinel=Join-Path $out 'latest.json'; [IO.File]::WriteAllText($sentinel,'previous deployment')
    Reject { New-UpdateManifest $out }
    if ([IO.File]::ReadAllText($sentinel) -cne 'previous deployment') { throw 'Failure replaced existing manifest' }
    # Offline orchestration: all external boundaries stubbed, real size/digest/checksum
    # parsing, identity recheck and output logic exercised. Never run an executable.
    $script:Mode='valid'; $script:Lookups=0
    $script:ExeHash='9f86d081884c7d659a2feaa0c55ad015a3bf4f1b2b0b822cd15d6c15b0f00a08'
    $script:SumsText="$script:ExeHash  RTA-Excel-Filler-portable.exe`n"
    $bytes=[Text.Encoding]::UTF8.GetBytes($script:SumsText)
    $script:SumsHash=[Convert]::ToHexString([Security.Cryptography.SHA256]::HashData($bytes)).ToLowerInvariant()
    function Get-LatestStable {
        $script:Lookups++
        $r=Fixture
        $r.assets[0].digest='sha256:'+$script:ExeHash
        $r.assets[1].digest='sha256:'+$script:SumsHash
        $r.assets[1].size=[Text.Encoding]::UTF8.GetByteCount($script:SumsText)
        if ($script:Mode -eq 'changed' -and $script:Lookups -gt 1) { $r.assets[0].id=99 }
        return Assert-ReleaseMetadata $r
    }
    function Get-VerifiedDownload {
        param($Url,$Path,$Limit)
        $text=if ($Path.EndsWith('SHA256SUMS.txt')) { $script:SumsText } else { 'test' }
        if ($script:Mode -eq 'tamper' -and $Path.EndsWith('portable.exe')) { $text='evil' }
        [IO.File]::WriteAllText($Path,$text,[Text.UTF8Encoding]::new($false))
    }
    function Get-FileHash {
        param($LiteralPath,$Algorithm)
        if ($LiteralPath.EndsWith('publisher-reference.exe')) {
            $hash=if ($script:Mode -eq 'reference') { 'bad' } else { 'db7a3dc5971b7de58a687a9057ca95f53389770fb664b343b9b379abd9399de7' }
            return @{Hash=$hash}
        }
        return Microsoft.PowerShell.Utility\Get-FileHash -LiteralPath $LiteralPath -Algorithm SHA256
    }
    function Get-ValidatedPublisher {
        param($Path)
        if ($script:Mode -eq 'signature') { throw 'Synthetic invalid signature' }
        if ($script:Mode -eq 'publisher' -and $Path.EndsWith('portable.exe')) { return 'different subject' }
        return 'synthetic verified subject'
    }
    function Assert-PortablePE { param($Path,$Version) if ($script:Mode -eq 'pe') { throw 'Synthetic PE mismatch' } }
    foreach ($mode in @('changed','tamper','reference','signature','publisher','pe')) {
        $script:Mode=$mode; $script:Lookups=0
        Reject { New-UpdateManifest $out }
        if ([IO.File]::ReadAllText($sentinel) -cne 'previous deployment') { throw "Failure $mode replaced existing manifest" }
    }
    $script:Mode='valid'; $script:Lookups=0
    New-UpdateManifest $out
    $result=[IO.File]::ReadAllText($sentinel) | ConvertFrom-Json -AsHashtable
    if ($script:Lookups -ne 2 -or $result.tag_name -cne 'v0.5.0' -or $result.assets.Count -ne 2 -or $result.Contains('id') -or $result.assets[0].Contains('id')) { throw 'Incorrect successful manifest' }
} finally { Remove-Item -LiteralPath $temp -Recurse -Force }
Write-Host "Manifest validation passed: $script:Checks rejection cases, valid metadata/JSON/checksum/digest cases; no network, publishing, signing or application execution."
