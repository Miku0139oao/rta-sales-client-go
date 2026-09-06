#Requires -Version 7.5
<# Generate staged Pages metadata only after independently verifying published artifacts.
   Dot-source to test pure validation functions without network or executable execution. #>
[CmdletBinding()]
param([string]$OutputDirectory)
Set-StrictMode -Version Latest
$ErrorActionPreference = 'Stop'
$script:UpdateRepo = 'Miku0139oao/rta-sales-client-go'
$script:PortableName = 'RTA-Excel-Filler-portable.exe'
$script:SumsName = 'SHA256SUMS.txt'

function Assert-ReleaseMetadata {
    param([System.Collections.IDictionary]$Release)
    foreach ($key in @('id','tag_name','draft','prerelease','body','assets','updated_at')) {
        if (-not $Release.Contains($key)) { throw "Missing release field: $key" }
    }
    if ($Release.draft -isnot [bool] -or $Release.prerelease -isnot [bool] -or $Release.draft -or $Release.prerelease) { throw 'Only explicitly stable published releases are allowed' }
    if ($Release.id -isnot [long] -and $Release.id -isnot [int]) { throw 'Invalid release identity' }
    if ($Release.id -le 0 -or $Release.body -isnot [string] -or $Release.updated_at -isnot [string]) { throw 'Invalid release fields' }
    if ($Release.tag_name -isnot [string] -or $Release.tag_name -cnotmatch '^v(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)$') { throw 'Invalid stable numeric tag' }
    foreach ($part in $Release.tag_name.Substring(1).Split('.')) {
        $n = 0L
        if (-not [long]::TryParse($part, [ref]$n) -or $n -gt 65535) { throw 'Tag exceeds numeric PE version bounds' }
    }
    if ($Release.assets -isnot [array]) { throw 'Invalid assets array' }
    $selected = @()
    foreach ($name in @($script:PortableName, $script:SumsName)) {
        $assets = @($Release.assets | Where-Object { $_.name -ceq $name })
        if ($assets.Count -ne 1) { throw "Exactly one $name required" }
        $a = $assets[0]
        foreach ($key in @('id','name','browser_download_url','size','digest','updated_at')) {
            if (-not $a.Contains($key)) { throw "Missing asset field: $key" }
        }
        $url = "https://github.com/$script:UpdateRepo/releases/download/$($Release.tag_name)/$name"
        $limit = if ($name -ceq $script:PortableName) { 256MB } else { 64KB }
        if (($a.id -isnot [long] -and $a.id -isnot [int]) -or $a.id -le 0 -or $a.updated_at -isnot [string]) { throw 'Invalid asset identity' }
        if ($a.browser_download_url -cne $url) { throw 'Unsafe asset URL' }
        if (($a.size -isnot [long] -and $a.size -isnot [int]) -or $a.size -le 0 -or $a.size -gt $limit) { throw 'Invalid asset size' }
        if ($a.digest -isnot [string] -or $a.digest -cnotmatch '^sha256:[0-9a-f]{64}$') { throw 'Missing or invalid GitHub SHA256 digest' }
        $selected += [ordered]@{ id=$a.id; name=$name; browser_download_url=$url; size=$a.size; digest=$a.digest; updated_at=$a.updated_at }
    }
    if ($selected[0].id -eq $selected[1].id) { throw 'Duplicate asset identity' }
    return [ordered]@{ id=$Release.id; tag_name=$Release.tag_name; draft=$false; prerelease=$false; body=$Release.body; updated_at=$Release.updated_at; assets=$selected }
}
function Get-LatestStable {
    if ([string]::IsNullOrWhiteSpace($env:GH_TOKEN)) { throw 'Actions GH_TOKEN required (never embedded in app or manifest)' }
    $raw = (& gh api "repos/$script:UpdateRepo/releases/latest" --header 'Accept: application/vnd.github+json' | Out-String)
    if ($LASTEXITCODE -ne 0) { throw 'Authenticated latest release lookup failed' }
    if ([Text.Encoding]::UTF8.GetByteCount($raw) -gt 2MB) { throw 'Release metadata too large' }
    # Keep API timestamps as strings on all PowerShell versions.
    return Assert-ReleaseMetadata ($raw | ConvertFrom-Json -AsHashtable -DateKind String)
}
function Get-VerifiedDownload {
    param([string]$Url, [string]$Path, [long]$Limit)
    $handler = [Net.Http.HttpClientHandler]::new()
    $handler.AllowAutoRedirect = $false
    $client = [Net.Http.HttpClient]::new($handler)
    $client.Timeout = [TimeSpan]::FromMinutes(3)
    $deadline = [Threading.CancellationTokenSource]::new([TimeSpan]::FromMinutes(3))
    try {
        for ($hop=0; $hop -le 5; $hop++) {
            $uri = [Uri]$Url
            if ($uri.Scheme -cne 'https' -or $uri.Port -ne 443 -or $uri.UserInfo -or $uri.Fragment -or $uri.Host -cnotin @('github.com','release-assets.githubusercontent.com','objects.githubusercontent.com')) { throw 'Unsafe artifact redirect' }
            $request = [Net.Http.HttpRequestMessage]::new([Net.Http.HttpMethod]::Get, $uri)
            $request.Headers.UserAgent.ParseAdd('RTA-Pages-manifest-verifier')
            $response = $client.SendAsync($request, [Net.Http.HttpCompletionOption]::ResponseHeadersRead, $deadline.Token).GetAwaiter().GetResult()
            try {
                $status = [int]$response.StatusCode
                if ($status -in @(301,302,303,307,308)) {
                    if ($null -eq $response.Headers.Location -or $hop -eq 5) { throw 'Invalid/excessive artifact redirects' }
                    $Url = [Uri]::new($uri, $response.Headers.Location).AbsoluteUri
                    continue
                }
                if ($status -ne 200) { throw "Artifact service returned HTTP $status" }
                if ($response.Content.Headers.ContentLength -gt $Limit) { throw 'Artifact exceeds download bound' }
                $inputStream = $response.Content.ReadAsStreamAsync($deadline.Token).GetAwaiter().GetResult()
                $output = [IO.File]::Create($Path)
                try {
                    $buffer = [byte[]]::new(65536); $total = 0L
                    while (($count = $inputStream.ReadAsync($buffer,0,$buffer.Length,$deadline.Token).GetAwaiter().GetResult()) -gt 0) {
                        $total += $count
                        if ($total -gt $Limit) { throw 'Artifact exceeds streaming bound' }
                        $output.Write($buffer,0,$count)
                    }
                } finally { $output.Dispose(); $inputStream.Dispose() }
                return
            } finally { $response.Dispose(); $request.Dispose() }
        }
    } finally { $deadline.Dispose(); $client.Dispose(); $handler.Dispose() }
}
function Assert-AssetHash {
    param([string]$Path, [System.Collections.IDictionary]$Asset)
    if ((Get-Item -LiteralPath $Path).Length -ne $Asset.size -or ('sha256:'+(Get-FileHash -LiteralPath $Path -Algorithm SHA256).Hash.ToLowerInvariant()) -cne $Asset.digest) { throw 'Downloaded asset size/digest mismatch' }
}
function Assert-Checksums {
    param([string]$Text, [string]$Hash)
    $found = 0
    foreach ($line in ($Text -split '\r?\n')) {
        if ([string]::IsNullOrWhiteSpace($line)) { continue }
        $fields = $line.Trim() -split '\s+'
        if ($fields.Count -ne 2) { throw 'Malformed checksum entry' }
        if ($fields[1].TrimStart('*') -ceq $script:PortableName) {
            $found++
            if ($fields[0] -notmatch '^[0-9a-fA-F]{64}$' -or $fields[0].ToLowerInvariant() -cne $Hash) { throw 'Portable checksum mismatch' }
        }
    }
    if ($found -ne 1) { throw 'Exactly one portable checksum required' }
}
function Get-ValidatedPublisher {
    param([string]$Path)
    $signature = Get-AuthenticodeSignature -LiteralPath $Path
    if ($signature.Status -ne 'Valid' -or $null -eq $signature.SignerCertificate -or [string]::IsNullOrWhiteSpace($signature.SignerCertificate.Subject)) { throw 'Invalid Authenticode signature' }
    return $signature.SignerCertificate.Subject
}
function Assert-PortablePE {
    param([string]$Path, [string]$Version)
    $stream = [IO.File]::OpenRead($Path); $reader = [IO.BinaryReader]::new($stream)
    try {
        if ($reader.ReadUInt16() -ne 0x5a4d -or $stream.Length -lt 64) { throw 'Not a PE executable' }
        $stream.Position=0x3c; $offset=$reader.ReadUInt32()
        if ($offset -lt 64 -or $offset -gt $stream.Length-26) { throw 'Invalid PE offset' }
        $stream.Position=$offset
        if ($reader.ReadUInt32() -ne 0x4550 -or $reader.ReadUInt16() -ne 0x8664) { throw 'Portable executable must be AMD64' }
        $stream.Position=$offset+24
        if ($reader.ReadUInt16() -ne 0x20b) { throw 'Portable executable must be PE32+' }
    } finally { $reader.Dispose(); $stream.Dispose() }
    $info = [Diagnostics.FileVersionInfo]::GetVersionInfo($Path)
    if ($info.ProductVersionRaw.ToString() -cne "$Version.0" -or $info.FileVersionRaw.ToString() -cne "$Version.0") { throw 'Numeric PE version does not match release tag' }
}
function New-UpdateManifest {
    param([string]$Directory)
    if (-not $IsWindows) { throw 'Windows is required for Authenticode verification' }
    $latest = Get-LatestStable
    $identity = $latest | ConvertTo-Json -Depth 10 -Compress
    $temp = Join-Path ([IO.Path]::GetTempPath()) ([Guid]::NewGuid().ToString('N'))
    New-Item -ItemType Directory -Path $temp | Out-Null
    try {
        foreach ($asset in $latest.assets) {
            $path = Join-Path $temp $asset.name
            Get-VerifiedDownload $asset.browser_download_url $path $asset.size
            Assert-AssetHash $path $asset
        }
        $exe = Join-Path $temp $script:PortableName
        Assert-Checksums ([IO.File]::ReadAllText((Join-Path $temp $script:SumsName))) $latest.assets[0].digest.Substring(7)
        $reference = Join-Path $temp 'publisher-reference.exe'
        Get-VerifiedDownload "https://github.com/$script:UpdateRepo/releases/download/v0.4.8/$script:PortableName" $reference 256MB
        if ((Get-FileHash -LiteralPath $reference -Algorithm SHA256).Hash.ToLowerInvariant() -cne 'db7a3dc5971b7de58a687a9057ca95f53389770fb664b343b9b379abd9399de7') { throw 'Pinned publisher reference hash mismatch' }
        $publisher = Get-ValidatedPublisher $reference
        if ((Get-ValidatedPublisher $exe) -cne $publisher) { throw 'Portable publisher differs from verified pinned reference' }
        Assert-PortablePE $exe $latest.tag_name.Substring(1)
        $rechecked = Get-LatestStable | ConvertTo-Json -Depth 10 -Compress
        if ($rechecked -cne $identity) { throw 'Latest release or asset identities changed during verification; rerun' }
        $manifest = [ordered]@{ tag_name=$latest.tag_name; draft=$false; prerelease=$false; body=$latest.body; assets=@($latest.assets | ForEach-Object { [ordered]@{name=$_.name; browser_download_url=$_.browser_download_url; size=$_.size; digest=$_.digest} }) }
        $json = $manifest | ConvertTo-Json -Depth 10
        if ([Text.Encoding]::UTF8.GetByteCount($json) -gt 2MB) { throw 'Manifest exceeds client limit' }
        New-Item -ItemType Directory -Force -Path $Directory | Out-Null
        $staged = Join-Path $Directory ('.latest-'+[Guid]::NewGuid().ToString('N')+'.tmp')
        try {
            [IO.File]::WriteAllText($staged,$json,[Text.UTF8Encoding]::new($false))
            [IO.File]::Move($staged,(Join-Path $Directory 'latest.json'),$true)
        } finally { if (Test-Path -LiteralPath $staged) { Remove-Item -LiteralPath $staged } }
    } finally { Remove-Item -LiteralPath $temp -Recurse -Force }
}
if ($MyInvocation.InvocationName -ne '.') {
    if ([string]::IsNullOrWhiteSpace($OutputDirectory)) { throw '-OutputDirectory is required' }
    New-UpdateManifest $OutputDirectory
}
