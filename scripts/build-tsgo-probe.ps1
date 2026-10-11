[CmdletBinding()]
param([switch]$Test)
$ErrorActionPreference = 'Stop'
$PSNativeCommandUseErrorActionPreference = $false
$root = Split-Path -Parent $PSScriptRoot
$previousLocation = Get-Location
$previousEnvironment = @{}
foreach ($name in @('GOROOT', 'GOTOOLCHAIN', 'TYNEL_PROBE_GO')) {
  $previousEnvironment[$name] = [Environment]::GetEnvironmentVariable($name, 'Process')
}
. (Join-Path $PSScriptRoot 'frontend-common.ps1')
function Get-FrontendInputs { return ((Get-TynelFrontendInputs -Root $root) | ConvertTo-Json -Compress) }
try {
  Set-Location -LiteralPath $root
  $lock = Get-Content -Raw 'toolchain.lock.json' | ConvertFrom-Json
  $sdk = Join-Path $root ('.tynel/sdk/' + $lock.typescript.goVersion)
  $go = Join-Path $sdk 'bin/go.exe'
  if (-not (Test-Path $go)) { throw 'Build the locked private Go SDK with build-bridge.ps1 first.' }
  $env:GOROOT = $sdk; $env:GOTOOLCHAIN = 'local'; $env:TYNEL_PROBE_GO = $go
  $version = (& $go version | Out-String).Trim()
  if ($LASTEXITCODE -ne 0 -or $version -ne ('go version ' + $lock.typescript.goVersion + ' windows/amd64')) { throw "Unsupported frontend probe toolchain: $version" }
  $preparedText = (& python -X utf8 (Join-Path $PSScriptRoot 'prepare-tsgo.py') --verify-only | Out-String)
  if ($LASTEXITCODE -ne 0) { throw 'Pinned frontend sources are unavailable or changed.' }
  $prepared = $preparedText | ConvertFrom-Json
  $source = Join-Path $prepared.sourceRoot 'tsc'
  $stage = Join-Path $root '.tynel/tsgo'
  New-Item -ItemType Directory -Force -Path $stage | Out-Null
  # Empty directory enables package discovery. All source additions are in the
  # overlay, not modifications to the downloaded upstream files.
  New-Item -ItemType Directory -Force -Path (Join-Path $source 'tynelapi') | Out-Null
  $replace = @{}
  Get-ChildItem -LiteralPath (Join-Path $root 'bridge/tsgo') -Filter '*.go' -File | ForEach-Object {
    $replace[(Join-Path $source ('tynelapi/' + $_.Name))] = $_.FullName
  }
  $overlay = Join-Path $stage 'overlay.json'
  [IO.File]::WriteAllText($overlay, (@{Replace=$replace} | ConvertTo-Json), [Text.UTF8Encoding]::new($false))
  $identityPath = Join-Path $stage 'identity.json'
  if (Test-Path -LiteralPath $identityPath) { Remove-Item -LiteralPath $identityPath }
  $before = Get-FrontendInputs
  $binary = Join-Path $stage 'tynel-tsgo-probe.exe'
  $temporaryBinary = Join-Path $stage 'tynel-tsgo-probe.partial.exe'
  Push-Location -LiteralPath (Join-Path $root 'experiments/tsgo-host')
  try {
    & $go build -p 2 -mod=readonly -overlay $overlay -o $temporaryBinary .
    if ($LASTEXITCODE -ne 0) { throw 'tsgo facade build failed.' }
    Move-Item -LiteralPath $temporaryBinary -Destination $binary -Force
    $passed = 0
    if ($Test) {
      $events = @(& $go test -p 2 -mod=readonly -overlay $overlay -count=1 -timeout=120s -json ./... 2>&1)
      $testExit = $LASTEXITCODE
      [IO.File]::WriteAllText((Join-Path $stage 'tests.jsonl'), (($events | ForEach-Object { [string]$_ }) -join "`n") + "`n", [Text.UTF8Encoding]::new($false))
      if ($testExit -ne 0) { $events | Write-Output; throw 'tsgo facade tests failed.' }
      foreach ($line in $events) {
        if ([string]$line -like '{*') { $event = [string]$line | ConvertFrom-Json; if ($event.Action -eq 'pass' -and $event.Test) { $passed++ } }
      }
      if ($passed -lt 10) { throw 'Frontend test evidence incomplete.' }
      & $go vet -mod=readonly -overlay $overlay ./...
      if ($LASTEXITCODE -ne 0) { throw 'Frontend host vet failed.' }
      Write-Output "PASS: frontend facade tests ($passed test/subtest events), external internal-package refusal, host vet"
    }
  } finally { Pop-Location }
  $sampleText = (& $binary (Join-Path $root 'examples/frontend/add.ts') | Out-String)
  if ($LASTEXITCODE -ne 0) { throw 'Standalone frontend probe rejected the add sample.' }
  $sample = $sampleText | ConvertFrom-Json
  if ($sample.protocol -ne $lock.typescript.facadeProtocol -or $sample.upstreamCommit -ne $lock.typescript.commit -or $sample.diagnostics.Count -ne 0) { throw 'Frontend executable identity differs from the lock.' }
  [IO.File]::WriteAllText((Join-Path $stage 'add.json'), $sampleText, [Text.UTF8Encoding]::new($false))
  if ((Get-FrontendInputs) -cne $before) { throw 'Frontend build inputs changed during execution; rebuild.' }
  & python -X utf8 (Join-Path $PSScriptRoot 'prepare-tsgo.py') --verify-only
  if ($LASTEXITCODE -ne 0) { throw 'Upstream sources changed during execution.' }
  $identity = @{ schemaVersion=1; protocol=$lock.typescript.facadeProtocol; upstreamCommit=$lock.typescript.commit; sourceTreeSHA256=$lock.typescript.selectedTreeSHA256; declarationSHA256=$lock.typescript.declarationSHA256; goVersion=$lock.typescript.goVersion; target='windows/amd64'; compilerSHA256=(Get-FileHash $binary -Algorithm SHA256).Hash.ToLowerInvariant(); sourceHashes=($before | ConvertFrom-Json); testsRun=[bool]$Test; passedTestEvents=$passed; scope='single-source query-only reconstructed replay; no TS-to-NIR' }
  [IO.File]::WriteAllText($identityPath, ($identity | ConvertTo-Json -Depth 6), [Text.UTF8Encoding]::new($false))
  Write-Output "Built frontend probe: $binary"
} finally {
  foreach ($name in $previousEnvironment.Keys) { [Environment]::SetEnvironmentVariable($name, $previousEnvironment[$name], 'Process') }
  Set-Location -LiteralPath $previousLocation.Path
}
