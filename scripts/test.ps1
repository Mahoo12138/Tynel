[CmdletBinding()]
param(
  [ValidateRange(0, 60)][int]$FuzzSeconds = 5
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
  # The bridge and bootstrap are separate modules: root go test ./... does
  # not cover them. Rebuild and exercise the actual compiler below as well.
  $goFiles = @(
    foreach ($dir in @('cmd', 'compiler', 'bridge/go', 'experiments/bootstrap')) {
      Get-ChildItem -LiteralPath (Join-Path $root $dir) -Recurse -Filter '*.go' -File | ForEach-Object { $_.FullName }
    }
  )
  $unformatted = @(& gofmt -l @goFiles)
  if ($LASTEXITCODE -ne 0) { throw 'gofmt check failed.' }
  if ($unformatted.Count -ne 0) { throw ('Run gofmt on: ' + ($unformatted -join ', ')) }
  foreach ($script in (Get-ChildItem -LiteralPath $PSScriptRoot -Filter '*.ps1' -File)) {
    $tokens = $null
    $parseErrors = $null
    [void][System.Management.Automation.Language.Parser]::ParseFile($script.FullName, [ref]$tokens, [ref]$parseErrors)
    if ($parseErrors.Count -ne 0) { throw ("PowerShell syntax errors in $($script.Name): " + ($parseErrors -join '; ')) }
  }
  Write-Output 'PASS: Go formatting and PowerShell syntax'

  & go test ./... -count=1
  if ($LASTEXITCODE -ne 0) { throw 'Go unit tests failed.' }
  & go vet ./...
  if ($LASTEXITCODE -ne 0) { throw 'go vet failed.' }
  if ($FuzzSeconds -gt 0) {
    & go test ./compiler/nir '-run=^$' '-fuzz=^FuzzNumberBits$' "-fuzztime=${FuzzSeconds}s" '-parallel=2'
    if ($LASTEXITCODE -ne 0) { throw 'Binary64 fuzz regression failed.' }
  }

  & (Join-Path $PSScriptRoot 'build-bridge.ps1')
  & (Join-Path $PSScriptRoot 'smoke-native.ps1')
  # Leave the documented public example built from the final sources;
  # the smoke binary itself intentionally ends with an empty-main case.
  & (Join-Path $PSScriptRoot 'compile-ir.ps1') -InputIR 'examples/add.nir.json' -Output 'dist/add.exe'
  $output = (& (Join-Path $root 'dist/add.exe') | Out-String).Replace("`r`n", "`n").Trim()
  if ($LASTEXITCODE -ne 0 -or $output -cne '42') { throw "Final add example failed: [$output]" }
  Write-Output 'PASS: final dist/add.exe produces 42'
  Write-Output 'Tynel regression passed.'
} finally {
  foreach ($name in $previousEnvironment.Keys) {
    [Environment]::SetEnvironmentVariable($name, $previousEnvironment[$name], 'Process')
  }
  Set-Location -LiteralPath $previousLocation.Path
}
