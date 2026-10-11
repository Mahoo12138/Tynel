[CmdletBinding()]
param(
  [string]$SdkSource,
  [string]$Proxy,
  [switch]$WithFrontend,
  [ValidateRange(0,60)][int]$FuzzSeconds = 5
)
$ErrorActionPreference = 'Stop'
$PSNativeCommandUseErrorActionPreference = $false
$root = Split-Path -Parent $PSScriptRoot
$oldLocation = Get-Location
$saved = @{}
foreach ($name in @('PATH','GOROOT','GOTOOLCHAIN','HTTP_PROXY','HTTPS_PROXY')) {
  $saved[$name] = [Environment]::GetEnvironmentVariable($name, 'Process')
}
try {
  Set-Location -LiteralPath $root
  $lock = Get-Content -Raw toolchain.lock.json | ConvertFrom-Json
  $sdk = Join-Path $root ('.tynel/sdk/' + $lock.go.version)
  $ready = Join-Path $sdk '.tynel-sdk-copy-complete'
  if (-not (Test-Path -LiteralPath $ready)) {
    if (Test-Path -LiteralPath $sdk) { throw 'Incomplete SDK destination exists; inspect it before retrying.' }
    if (-not $SdkSource) { throw 'Supply -SdkSource with an existing exact Go SDK root, not its bin directory.' }
    $origin = (Resolve-Path -LiteralPath $SdkSource).Path
    $env:GOROOT = $origin; $env:GOTOOLCHAIN = 'local'
    $sourceGo = Join-Path $origin 'bin/go.exe'
    $version = (& $sourceGo version | Out-String).Trim()
    if ($LASTEXITCODE -ne 0 -or $version -ne ('go version ' + $lock.go.version + ' windows/amd64')) { throw "Wrong source SDK: $version" }
    if ((Get-FileHash (Join-Path $origin 'src/cmd/compile/internal/gc/main.go') -Algorithm SHA256).Hash.ToLowerInvariant() -ne $lock.go.gcMainSHA256) { throw 'Source SDK seam mismatch.' }
    New-Item -ItemType Directory -Force -Path $sdk | Out-Null
    & robocopy $origin $sdk /E /XJ /R:1 /W:1 /NFL /NDL /NJH /NJS /NP /XF .tynel-sdk-copy-complete
    if ($LASTEXITCODE -ge 8) { throw "SDK copy failed: $LASTEXITCODE" }
    [IO.File]::WriteAllText($ready, $origin)
  }
  $env:GOROOT = $sdk; $env:GOTOOLCHAIN = 'local'
  $env:PATH = (Join-Path $sdk 'bin') + [IO.Path]::PathSeparator + $saved['PATH']
  $version = (& (Join-Path $sdk 'bin/go.exe') version | Out-String).Trim()
  if ($LASTEXITCODE -ne 0 -or $version -ne ('go version ' + $lock.go.version + ' windows/amd64')) { throw "Wrong replay SDK: $version" }
  if ($Proxy) { $env:HTTP_PROXY = $Proxy; $env:HTTPS_PROXY = $Proxy }
  if ($WithFrontend) {
    if (-not (Test-Path -LiteralPath 'scripts/prepare-tsgo.py')) { throw 'This stage has no TypeScript frontend.' }
    & python -X utf8 scripts/prepare-tsgo.py
    if ($LASTEXITCODE -ne 0) { throw 'Pinned TypeScript preparation failed.' }
    & (Join-Path $PSScriptRoot 'test.ps1') -FuzzSeconds $FuzzSeconds -WithFrontend
  } else {
    & (Join-Path $PSScriptRoot 'test.ps1') -FuzzSeconds $FuzzSeconds
  }
  Write-Output 'Replay passed using this checkout and a private SDK.'
} finally {
  foreach ($name in $saved.Keys) { [Environment]::SetEnvironmentVariable($name, $saved[$name], 'Process') }
  Set-Location -LiteralPath $oldLocation.Path
}
