#Requires -Version 5.1
$ErrorActionPreference = 'Stop'
$here = Split-Path -Parent $MyInvocation.MyCommand.Path
$out = Join-Path $here 'frames'
New-Item -ItemType Directory -Force -Path $out | Out-Null
$edge = @(
    "${env:ProgramFiles(x86)}\Microsoft\Edge\Application\msedge.exe",
    "$env:ProgramFiles\Microsoft\Edge\Application\msedge.exe"
) | Where-Object { Test-Path $_ } | Select-Object -First 1
if (-not $edge) { throw 'Microsoft Edge not found' }

$html = (Resolve-Path (Join-Path $here 'slides.html')).Path
$uriBase = ([Uri]$html).AbsoluteUri
$count = 19
for ($i = 0; $i -lt $count; $i++) {
    $shot = Join-Path $out ('slide-{0:d2}.png' -f $i)
    $url = $uriBase + '?s=' + $i
    Write-Host "render $i"
    if (Test-Path $shot) { Remove-Item -LiteralPath $shot -Force }
    $proc = Start-Process -FilePath $edge -ArgumentList @(
        '--headless=new', '--disable-gpu', '--hide-scrollbars',
        '--force-device-scale-factor=1', '--window-size=1920,1080',
        "--screenshot=$shot", $url
    ) -PassThru -WindowStyle Hidden
    $deadline = (Get-Date).AddSeconds(20)
    while (-not (Test-Path $shot) -and (Get-Date) -lt $deadline) {
        Start-Sleep -Milliseconds 200
    }
    if (-not $proc.HasExited) { Wait-Process -Id $proc.Id -Timeout 15 -ErrorAction SilentlyContinue }
    if (-not (Test-Path $shot) -or ((Get-Item $shot).Length -lt 10000)) { throw "missing or tiny $shot" }
}
Write-Host "rendered $count frames"
