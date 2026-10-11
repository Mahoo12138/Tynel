[CmdletBinding()]
param()
$ErrorActionPreference = 'Stop'
$PSNativeCommandUseErrorActionPreference = $false
$root = Split-Path -Parent $PSScriptRoot
$previousLocation = Get-Location
$previousEnvironment = @{}
foreach ($name in @('GOROOT','GOTOOLCHAIN','TYNEL_IR','TYNEL_COMPILER','TYNEL_COMPILER_SHA256')) {
  $previousEnvironment[$name] = [Environment]::GetEnvironmentVariable($name, 'Process')
}
try {
  Set-Location -LiteralPath $root
  $work = Join-Path $root '.tynel/abi'
  New-Item -ItemType Directory -Path $work -Force | Out-Null
  $binary = Join-Path $work 'scalar.exe'
  & (Join-Path $PSScriptRoot 'compile-ir.ps1') -InputIR 'examples/add.nir.json' -Output $binary
  $identity = Get-Content -Raw '.tynel/bridge/identity.json' | ConvertFrom-Json
  $go = Join-Path $identity.goroot 'bin/go.exe'
  $wrapper = Join-Path $root '.tynel/bin/tynel-toolexec.exe'
  $support = 'tynel.local/bootstrap/probesupport'
  $symbols = (& $go tool nm $binary | Out-String)
  if ($LASTEXITCODE -ne 0 -or $symbols -notmatch [regex]::Escape($support + '.ABI_v1')) { throw 'Versioned support anchor was removed from scalar binary.' }
  Write-Output 'PASS: scalar-only program retains versioned support ABI anchor'

  # Ask the same build driver for package archives; then exercise the linker
  # directly so the negative test cannot be satisfied by CLI preflight alone.
  Push-Location -LiteralPath (Join-Path $root 'experiments/bootstrap')
  try {
    $lines = @(& $go list -export -deps -f '{{if .Export}}{{.ImportPath}}={{.Export}}{{end}}' -toolexec ('"' + $wrapper + '"') .)
    if ($LASTEXITCODE -ne 0) { throw 'Cannot obtain matching package archives.' }
  } finally { Pop-Location }
  $packages = [ordered]@{}
  foreach ($line in $lines) {
    if ([string]::IsNullOrWhiteSpace($line)) { continue }
    $pair = $line.Split('=', 2)
    if ($pair.Count -ne 2 -or -not (Test-Path -LiteralPath $pair[1])) { throw "Invalid package archive record: $line" }
    $packages[$pair[0]] = $pair[1]
  }
  $mainArchive = $packages['tynel.local/bootstrap']
  if (-not $mainArchive -or -not $packages[$support] -or -not $packages['runtime']) { throw 'Incomplete link graph.' }
  $goodConfig = Join-Path $work 'good.importcfg'
  $badConfig = Join-Path $work 'bad.importcfg'
  $badArchive = Join-Path $work 'old-support.a'
  $utf8 = [Text.UTF8Encoding]::new($false)
  $goodLines = @($packages.Keys | Where-Object { $_ -ne 'tynel.local/bootstrap' } | ForEach-Object { 'packagefile ' + $_ + '=' + $packages[$_] })
  [IO.File]::WriteAllText($goodConfig, ($goodLines -join "`n") + "`n", $utf8)
  $positive = Join-Path $work 'direct-link.exe'
  & $go tool link -importcfg $goodConfig -o $positive $mainArchive
  if ($LASTEXITCODE -ne 0) { throw 'Positive direct-link control failed.' }
  $result = (& $positive | Out-String).Trim()
  if ($LASTEXITCODE -ne 0 -or $result -cne '42') { throw 'Positive direct-link output failed.' }
  Write-Output 'PASS: direct-link positive control produces 42'

  & $go tool compile -pack -p $support -o $badArchive (Join-Path $root 'tests/fixtures/abi-old/support.go')
  if ($LASTEXITCODE -ne 0) { throw 'Cannot build deliberately old support fixture.' }
  $badLines = @($packages.Keys | Where-Object { $_ -ne 'tynel.local/bootstrap' } | ForEach-Object {
    $archive = if ($_ -eq $support) { $badArchive } else { $packages[$_] }
    'packagefile ' + $_ + '=' + $archive
  })
  [IO.File]::WriteAllText($badConfig, ($badLines -join "`n") + "`n", $utf8)
  $negativeLog = (& $go tool link -importcfg $badConfig -o (Join-Path $work 'must-not-link.exe') $mainArchive 2>&1 | Out-String)
  $negativeExit = $LASTEXITCODE
  [IO.File]::WriteAllText((Join-Path $work 'link-mismatch.log'), $negativeLog, $utf8)
  if ($negativeExit -eq 0 -or $negativeLog -notmatch '(fingerprint mismatch|relocation target.*not defined)' -or $negativeLog -notmatch '(probesupport|ABI_v1)') {
    throw "Old support not rejected for the expected link-contract reason: exit=$negativeExit; $negativeLog"
  }
  Write-Output 'PASS: direct linker rejects old support archive without driver preflight'

  # Keep working sources untouched; substitute one handwritten wrong-layout
  # fixture only for this negative build. The compiler must issue TY4002.
  $overlay = Join-Path $work 'wrong-layout.json'
  $replace = @{}
  $replace[(Join-Path $root 'experiments/bootstrap/probesupport/support.go')] = Join-Path $root 'tests/fixtures/abi-layout/support.go'
  [IO.File]::WriteAllText($overlay, (@{Replace=$replace} | ConvertTo-Json), $utf8)
  Push-Location -LiteralPath (Join-Path $root 'experiments/bootstrap')
  try {
    $layoutLog = (& $go build -overlay $overlay -toolexec ('"' + $wrapper + '"') -o (Join-Path $work 'must-not-build.exe') . 2>&1 | Out-String)
    $layoutExit = $LASTEXITCODE
  } finally { Pop-Location }
  [IO.File]::WriteAllText((Join-Path $work 'layout-mismatch.log'), $layoutLog, $utf8)
  if ($layoutExit -eq 0 -or $layoutLog -notmatch 'TY4002.*fields must be') { throw "Wrong layout was not diagnosed by the bridge: $layoutLog" }
  Write-Output 'PASS: wrong fixed-record layout is rejected by compiler (TY4002)'
  $record = @{ schemaVersion=1; supportABI=$identity.supportABI; supportAnchor=$identity.supportAnchor; positiveDirectLink=0; oldSupportExit=$negativeExit; wrongLayoutExit=$layoutExit; compilerSHA256=$identity.compilerSHA256; scope='M1 anchor and Go package fingerprint prototype; not complete cross-version ABI support' }
  [IO.File]::WriteAllText((Join-Path $work 'result.json'), ($record | ConvertTo-Json), $utf8)
  Write-Output 'ABI probe passed: 4 checks.'
} finally {
  foreach ($name in $previousEnvironment.Keys) { [Environment]::SetEnvironmentVariable($name, $previousEnvironment[$name], 'Process') }
  Set-Location -LiteralPath $previousLocation.Path
}
