[CmdletBinding()]
param(
  [ValidateRange(0, 60)][int]$FuzzSeconds = 5,
  [switch]$WithFrontend
)
$ErrorActionPreference = 'Stop'
$PSNativeCommandUseErrorActionPreference = $false
$root = Split-Path -Parent $PSScriptRoot
$previousLocation = Get-Location
$previousEnvironment = @{}
foreach ($name in @('GOROOT', 'GOTOOLCHAIN', 'TYNEL_IR', 'TYNEL_COMPILER', 'TYNEL_COMPILER_SHA256')) {
  $previousEnvironment[$name] = [Environment]::GetEnvironmentVariable($name, 'Process')
}

try {
  Set-Location -LiteralPath $root
  # A go.mod toolchain directive is not an exact upper version bound. Resolve
  # an explicit SDK so root tests, fixture generation and the fork agree.
  $lock = Get-Content -Raw 'toolchain.lock.json' | ConvertFrom-Json
  $sdk = Join-Path $root ('.tynel/sdk/' + $lock.go.version)
  if (-not (Test-Path -LiteralPath (Join-Path $sdk 'bin/go.exe'))) {
    $sdk = (& go env GOROOT | Out-String).Trim()
    if ($LASTEXITCODE -ne 0) { throw 'Cannot locate regression SDK.' }
  }
  $go = Join-Path $sdk 'bin/go.exe'
  $gofmt = Join-Path $sdk 'bin/gofmt.exe'
  $env:GOROOT = $sdk
  $env:GOTOOLCHAIN = 'local'
  $version = (& $go version | Out-String).Trim()
  if ($LASTEXITCODE -ne 0 -or $version -notmatch ('^go version ' + [regex]::Escape($lock.go.version) + ' ')) { throw "Install the locked Go SDK before regression: $version" }
  Write-Output "Regression toolchain: $version"
  # The bridge and bootstrap are separate modules: root go test ./... does
  # not cover them. Rebuild and exercise the actual compiler below as well.
  $goFiles = @(
    foreach ($dir in @('cmd', 'compiler', 'internal', 'tests', 'bridge/go', 'bridge/tsgo', 'experiments/bootstrap', 'experiments/tsgo-host')) {
      Get-ChildItem -LiteralPath (Join-Path $root $dir) -Recurse -Filter '*.go' -File | ForEach-Object { $_.FullName }
    }
  )
  $unformatted = @(& $gofmt -l @goFiles)
  if ($LASTEXITCODE -ne 0) { throw 'gofmt check failed.' }
  if ($unformatted.Count -ne 0) { throw ('Run gofmt on: ' + ($unformatted -join ', ')) }
  foreach ($script in (Get-ChildItem -LiteralPath $PSScriptRoot -Filter '*.ps1' -File)) {
    $tokens = $null
    $parseErrors = $null
    [void][System.Management.Automation.Language.Parser]::ParseFile($script.FullName, [ref]$tokens, [ref]$parseErrors)
    if ($parseErrors.Count -ne 0) { throw ("PowerShell syntax errors in $($script.Name): " + ($parseErrors -join '; ')) }
  }
  Write-Output 'PASS: Go formatting and PowerShell syntax'

  & python -X utf8 (Join-Path $PSScriptRoot 'test-prepare-go-source.py')
  if ($LASTEXITCODE -ne 0) { throw 'Go source comparison tests failed.' }
  & python -X utf8 (Join-Path $PSScriptRoot 'check-stage.py')
  if ($LASTEXITCODE -ne 0) { throw 'Stage metadata or document links failed.' }
  & python -X utf8 (Join-Path $PSScriptRoot 'test-check-stage.py')
  if ($LASTEXITCODE -ne 0) { throw 'Stage metadata tests failed.' }

  & $go test ./... -count=1
  if ($LASTEXITCODE -ne 0) { throw 'Go unit tests failed.' }
  & $go vet ./...
  if ($LASTEXITCODE -ne 0) { throw 'go vet failed.' }
  if ($FuzzSeconds -gt 0) {
    & $go test ./compiler/nir '-run=^$' '-fuzz=^FuzzNumberBits$' "-fuzztime=${FuzzSeconds}s" '-parallel=2'
    if ($LASTEXITCODE -ne 0) { throw 'Binary64 fuzz regression failed.' }
  }

  & (Join-Path $PSScriptRoot 'build-bridge.ps1')
  & (Join-Path $PSScriptRoot 'smoke-native.ps1')
  & (Join-Path $PSScriptRoot 'smoke-gc.ps1')
  & (Join-Path $PSScriptRoot 'smoke-abi.ps1')
  # Leave the documented public example built from the final sources;
  # the smoke binary itself intentionally ends with an empty-main case.
  & (Join-Path $PSScriptRoot 'compile-ir.ps1') -InputIR 'examples/add.nir.json' -Output 'dist/add.exe'
  $output = (& (Join-Path $root 'dist/add.exe') | Out-String).Replace("`r`n", "`n").Trim()
  if ($LASTEXITCODE -ne 0 -or $output -cne '42') { throw "Final add example failed: [$output]" }
  Write-Output 'PASS: final dist/add.exe produces 42'
  if ($WithFrontend) {
    & python -X utf8 (Join-Path $PSScriptRoot 'test-prepare-tsgo.py')
    if ($LASTEXITCODE -ne 0) { throw 'Source-lock negative tests failed.' }
    & (Join-Path $PSScriptRoot 'build-tsgo-probe.ps1') -Test
    & python -X utf8 (Join-Path $PSScriptRoot 'smoke-ts.py')
    if ($LASTEXITCODE -ne 0) { throw 'TypeScript-to-native scalar regression failed.' }
    & python -X utf8 (Join-Path $PSScriptRoot 'smoke-control.py')
    if ($LASTEXITCODE -ne 0) { throw 'Boolean/control-flow native regression failed.' }
    & python -X utf8 (Join-Path $PSScriptRoot 'smoke-short-circuit.py')
    if ($LASTEXITCODE -ne 0) { throw 'Short-circuit native regression failed.' }
  } else { Write-Output 'Frontend probe not requested; use -WithFrontend after prepare-tsgo.py.' }
  Write-Output 'Tynel regression passed.'
} finally {
  foreach ($name in $previousEnvironment.Keys) {
    [Environment]::SetEnvironmentVariable($name, $previousEnvironment[$name], 'Process')
  }
  Set-Location -LiteralPath $previousLocation.Path
}
