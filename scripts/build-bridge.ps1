[CmdletBinding()]
param()
$ErrorActionPreference = 'Stop'
$root = Split-Path -Parent $PSScriptRoot
Set-Location -LiteralPath $root
. (Join-Path $PSScriptRoot 'bridge-common.ps1')
$lock = Get-Content -Raw -LiteralPath 'toolchain.lock.json' | ConvertFrom-Json
$origin = (& go env GOROOT).Trim()
if ($LASTEXITCODE -ne 0) { throw 'Cannot determine GOROOT' }
# Downloaded Go toolchains live in GOMODCACHE, where overlays are forbidden.
# Use a private SDK copy instead; never mutate or weaken the host cache guard.
$goroot = Join-Path $root ('.tynel/sdk/' + $lock.go.version)
$ready = Join-Path $goroot '.tynel-sdk-copy-complete'
if (-not (Test-Path -LiteralPath $ready)) {
  New-Item -ItemType Directory -Path $goroot -Force | Out-Null
  & robocopy $origin $goroot /E /R:1 /W:1 /NFL /NDL /NJH /NJS /NP
  if ($LASTEXITCODE -ge 8) { throw "SDK copy failed: $LASTEXITCODE" }
  [IO.File]::WriteAllText($ready, $origin)
}
$env:GOROOT = $goroot
$go = Join-Path $goroot 'bin/go.exe'
if (-not (Test-Path $go)) { $go = Join-Path $goroot 'bin/go' }
$env:GOTOOLCHAIN = 'local'
$version = (& $go version).Trim()
if ($LASTEXITCODE -ne 0 -or $version -notmatch ('^go version ' + [regex]::Escape($lock.go.version) + ' ')) { throw "Wrong bootstrap: $version" }
$src = Join-Path $goroot 'src/cmd/compile/internal/gc/main.go'
$hash = (Get-FileHash -Algorithm SHA256 -LiteralPath $src).Hash.ToLowerInvariant()
if ($hash -ne $lock.go.gcMainSHA256) { throw "Go compiler source fingerprint mismatch: $hash" }
$stage = Join-Path $root '.tynel/bridge'
New-Item -ItemType Directory -Path $stage -Force | Out-Null
New-Item -ItemType Directory -Path (Join-Path $root '.tynel/bin') -Force | Out-Null
$text = [IO.File]::ReadAllText($src)
$anchor = "`tnoder.LoadPackage(flag.Args())"
if (($text.Split($anchor)).Count -ne 2) { throw 'Compiler seam is not unique' }
$patched = $text.Replace($anchor, $anchor + "`n`tloadTynelIR() // Tynel M1 high-level IR injection")
$utf8 = [Text.UTF8Encoding]::new($false)
$main = Join-Path $stage 'main.go'
[IO.File]::WriteAllText($main, $patched, $utf8)
$replace = @{}
$replace[$src] = $main
$replace[(Join-Path $goroot 'src/cmd/compile/internal/gc/tynel_hook.go')] = (Join-Path $root 'bridge/go/gc_hook.go')
$replace[(Join-Path $goroot 'src/cmd/compile/internal/tynelbridge/load.go')] = (Join-Path $root 'bridge/go/load.go')
Get-ChildItem -LiteralPath (Join-Path $root 'compiler/nir') -Filter '*.go' | Where-Object { $_.Name -notlike '*_test.go' } | ForEach-Object {
  $replace[(Join-Path $goroot ('src/cmd/compile/internal/tynelbridge/nir/' + $_.Name))] = $_.FullName
}
$overlay = Join-Path $stage 'overlay.json'
[IO.File]::WriteAllText($overlay, (@{ Replace = $replace } | ConvertTo-Json -Depth 5), $utf8)
$out = Join-Path $root '.tynel/bin/tynel-compile.exe'
& $go build -overlay $overlay -o $out cmd/compile
if ($LASTEXITCODE -ne 0) { throw "Compiler fork build failed: $LASTEXITCODE" }
$identity = @{ goVersion = $lock.go.version; goroot = $goroot; gcMainSHA256 = $hash; bridgeABI = $lock.bridgeABI; compilerSHA256 = (Get-FileHash $out -Algorithm SHA256).Hash.ToLowerInvariant(); sourceHashes = (Get-TynelBridgeSourceHashes -Root $root) }
[IO.File]::WriteAllText((Join-Path $stage 'identity.json'), ($identity | ConvertTo-Json -Depth 6), $utf8)
Write-Output "Built compiler fork: $out"
Write-Output 'Host GOROOT source and executables were not modified.'
