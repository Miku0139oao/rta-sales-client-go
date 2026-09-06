#Requires -Version 5.1
[CmdletBinding()]
param([string]$PlaywrightModule = "$env:USERPROFILE\.cache\codex-runtimes\codex-primary-runtime\dependencies\node\node_modules\playwright\index.mjs")
Set-StrictMode -Version Latest
$ErrorActionPreference = 'Stop'
$repo = (Resolve-Path (Join-Path $PSScriptRoot '..')).Path
$nonce = [guid]::NewGuid().ToString('N')
$root = Join-Path $env:USERPROFILE "rta-update-smoke-$nonce"
New-Item -ItemType Directory -Path $root | Out-Null
# Only this newly created directory gets a private DACL; never alter ancestry.
$sid = [System.Security.Principal.WindowsIdentity]::GetCurrent().User.Value
& icacls $root /inheritance:r /grant:r "*${sid}:(OI)(CI)F" '*S-1-5-18:(OI)(CI)F' '*S-1-5-32-544:(OI)(CI)F' | Out-Null
if ($LASTEXITCODE) { throw 'private ACL failed' }
[IO.File]::WriteAllText((Join-Path $root 'nonce'),$nonce)
Start-Transcript -Path (Join-Path $root 'harness.log') | Out-Null
Write-Host "Sandbox: $root"
$oldEnv = $env:RTA_PORTABLE_SMOKE_MANIFEST
try {
 $snapshot = Join-Path $root 'source'
 New-Item -ItemType Directory -Path $snapshot | Out-Null
 Push-Location $repo
 try {
  # Tracked source plus narrowly scoped Go task additions; no arbitrary untracked data.
  $files = @(git ls-files) + @(git ls-files --others --exclude-standard -- 'cmd/rta-excel-filler/*.go' 'desktop/*.go' 'internal/**/*.go' 'desktop/frontend/src/**/*.ts' 'desktop/frontend/src/**/*.svelte')
  foreach ($rel in ($files | Sort-Object -Unique)) {
   if ($rel -notmatch '^(cmd/|desktop/|internal/|rtasales/|securestore/|xlsx|go\.(mod|sum)$)' -or $rel -match '(^|/)(\.env[^/]*|node_modules|release|\.cache)(/|$)' -or $rel -match '\.(xlsx|csv|exe|syso)$') { continue }
   $src = Join-Path $repo $rel
   if (-not (Test-Path -LiteralPath $src -PathType Leaf)) { continue }
   $dst = Join-Path $snapshot $rel
   New-Item -ItemType Directory -Force -Path (Split-Path $dst) | Out-Null
   Copy-Item -LiteralPath $src -Destination $dst
  }
 } finally { Pop-Location }
 # Reuse dependencies read-only via a junction, but build only disposable source.
 $dependencyLink = Join-Path $snapshot 'desktop/frontend/node_modules'
 New-Item -ItemType Junction -Path $dependencyLink -Target (Join-Path $repo 'desktop/frontend/node_modules') | Out-Null
 Push-Location (Join-Path $snapshot 'desktop/frontend')
 try { & bun run build; if ($LASTEXITCODE) { throw 'frontend build failed' } } finally { Pop-Location; [IO.Directory]::Delete($dependencyLink) }
 $app = Join-Path $snapshot 'cmd/rta-excel-filler'
 $wails = Join-Path ((& go env GOPATH).Trim()) 'bin/wails3.exe'
 foreach ($version in @('0.4.6','0.4.7')) {
  $infoPath = Join-Path $app 'build/windows/info.json'
  $info = Get-Content -Raw -LiteralPath $infoPath | ConvertFrom-Json
  $info.fixed.product_version = "$version.0"; $info.fixed.file_version = "$version.0"; $info.info.'0409'.ProductVersion = $version
  [IO.File]::WriteAllText($infoPath,($info | ConvertTo-Json -Depth 20))
  Push-Location $app
  try {
   $argsW = @('generate','syso','-arch','amd64','-icon','build/windows/icon.ico','-manifest','build/windows/wails.exe.manifest','-info','build/windows/info.json','-out','wails_windows_amd64.syso')
   if (Test-Path $wails) { & $wails @argsW } else { & go run github.com/wailsapp/wails/v3/cmd/wails3@v3.0.0-beta.8 @argsW }
   if ($LASTEXITCODE) { throw 'resource generation failed' }
   $name = if ($version -eq '0.4.6') {'fixture.exe'} else {'next.exe'}
   & go build -tags production,native_smoke,portable_update_smoke -trimpath -ldflags "-w -s -H windowsgui -X github.com/Miku0139oao/rta-sales-client-go/internal/buildinfo.Version=$version" -o (Join-Path $root $name) .
   if ($LASTEXITCODE) { throw 'fixture build failed' }
  } finally { Pop-Location }
 }
 & (Join-Path $repo 'scripts/sign-windows.ps1') -Files @((Join-Path $root 'fixture.exe'),(Join-Path $root 'next.exe')) -Required
 $oldHash = (Get-FileHash (Join-Path $root 'fixture.exe')).Hash.ToLowerInvariant()
 $newHash = (Get-FileHash (Join-Path $root 'next.exe')).Hash.ToLowerInvariant()
 [IO.File]::WriteAllText((Join-Path $root 'SHA256SUMS.txt'),"$newHash  RTA-Excel-Filler-portable.exe`n")
 $base = 'https://github.com/Miku0139oao/rta-sales-client-go/releases/download/v0.4.7/'
 $release = @{tag_name='v0.4.7';draft=$false;prerelease=$false;body="Signed isolated smoke $nonce";assets=@(@{name='RTA-Excel-Filler-portable.exe';browser_download_url=($base+'RTA-Excel-Filler-portable.exe');size=(Get-Item (Join-Path $root 'next.exe')).Length;digest=('sha256:'+$newHash)},@{name='SHA256SUMS.txt';browser_download_url=($base+'SHA256SUMS.txt');size=(Get-Item (Join-Path $root 'SHA256SUMS.txt')).Length;digest=('sha256:'+(Get-FileHash (Join-Path $root 'SHA256SUMS.txt')).Hash.ToLowerInvariant())})}
 [IO.File]::WriteAllText((Join-Path $root 'latest.json'),($release | ConvertTo-Json -Depth 10))
 $listener = [Net.Sockets.TcpListener]::new([Net.IPAddress]::Loopback,0); $listener.Start(); $port=$listener.LocalEndpoint.Port; $listener.Stop()
 $manifest = @{root=$root;nonce=$nonce;port=$port;version='0.4.7';oldHash=$oldHash;newHash=$newHash}
 $env:RTA_PORTABLE_SMOKE_MANIFEST = Join-Path $root 'manifest.json'
 [IO.File]::WriteAllText($env:RTA_PORTABLE_SMOKE_MANIFEST,($manifest | ConvertTo-Json))
 $p = Start-Process -FilePath (Join-Path $root 'fixture.exe') -WorkingDirectory $root -PassThru
 [IO.File]::WriteAllText((Join-Path $root 'parent.pid'),[string]$p.Id)
 $env:PLAYWRIGHT_MODULE = ([uri](Resolve-Path -LiteralPath $PlaywrightModule).Path).AbsoluteUri
 & node (Join-Path $repo 'desktop/frontend/scripts/update-native-smoke.mjs')
 if ($LASTEXITCODE) { throw "Signed native smoke failed; evidence: $root" }
 if (-not $p.WaitForExit(10000) -or $p.ExitCode -ne 0) { throw 'original Wails parent did not exit normally' }
 $evidencePath = Join-Path $root 'e2e-result.json'
 $evidence = Get-Content -Raw $evidencePath | ConvertFrom-Json
 $restartPID = [int]($evidence.starts[1].Split(' ')[0])
 $restarted = Get-Process -Id $restartPID
 if ($restarted.Path -ne (Join-Path $root 'fixture.exe') -or $restarted.MainWindowHandle -eq 0) { throw 'restart missing exact executable/window' }
 $helperPath = Join-Path (Join-Path $root $evidence.stage.name) 'helper.exe'
 $deadline = [DateTime]::UtcNow.AddSeconds(10)
 do {
  $helpers = @(Get-Process -Name helper -ErrorAction SilentlyContinue | Where-Object { $_.Path -eq $helperPath })
  if ($helpers.Count -eq 0) { break }
  Start-Sleep -Milliseconds 100
 } while ([DateTime]::UtcNow -lt $deadline)
 if ($helpers.Count) { throw 'fixture helper has not exited' }
 $evidence | Add-Member parentNormalExit $true
 $evidence | Add-Member helperExited $true
 $evidence | Add-Member restartedWindowHandle ([string]$restarted.MainWindowHandle)
 [IO.File]::WriteAllText($evidencePath,($evidence | ConvertTo-Json -Depth 10))
} finally {
 # Only recorded bootstrap PIDs with exact fixture executable path; graceful close only.
 $starts = Join-Path $root 'starts.log'
 if (Test-Path $starts) {
  foreach ($line in Get-Content $starts) {
   $fixturePid = [int]($line.Split(' ')[0])
   $proc = Get-Process -Id $fixturePid -ErrorAction SilentlyContinue
   if ($proc -and $proc.Path -eq (Join-Path $root 'fixture.exe')) { [void]$proc.CloseMainWindow() }
  }
 }
 $env:RTA_PORTABLE_SMOKE_MANIFEST = $oldEnv
 Stop-Transcript | Out-Null
}
