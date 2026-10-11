[CmdletBinding()]
param([ValidateRange(1, 100)][int]$Repeat = 20)
$ErrorActionPreference = 'Stop'
$PSNativeCommandUseErrorActionPreference = $false
$root = Split-Path -Parent $PSScriptRoot
$previousLocation = Get-Location
$previousEnvironment = @{}
foreach ($name in @('GOROOT','GOTOOLCHAIN','TYNEL_IR','TYNEL_COMPILER','TYNEL_COMPILER_SHA256','GOGC','GODEBUG')) {
  $previousEnvironment[$name] = [Environment]::GetEnvironmentVariable($name, 'Process')
}
try {
  Set-Location -LiteralPath $root
  # Previous smoke stages set a private GOROOT; PATH may still name another Go.
  # Use the matching executable for fixture generation as well as compilation.
  $identity = Get-Content -Raw '.tynel/bridge/identity.json' | ConvertFrom-Json
  $go = Join-Path $identity.goroot 'bin/go.exe'
  $env:GOROOT = $identity.goroot
  $env:GOTOOLCHAIN = 'local'
  $version = (& $go version | Out-String).Trim()
  if ($LASTEXITCODE -ne 0 -or $version -notmatch ('^go version ' + [regex]::Escape($identity.goVersion) + ' ')) { throw "GC probe toolchain mismatch: $version" }
  $work = Join-Path $root '.tynel/gc'
  New-Item -ItemType Directory -Path $work -Force | Out-Null
  $inputFile = Join-Path $work 'probe.nir.json'
  $binary = Join-Path $work 'probe.exe'
  $irLines = @(& $go run ./cmd/tynel-gc-probe)
  if ($LASTEXITCODE -ne 0) { throw 'GC fixture construction failed.' }
  [IO.File]::WriteAllText($inputFile, ($irLines -join "`n") + "`n", [Text.UTF8Encoding]::new($false))
  & (Join-Path $PSScriptRoot 'compile-ir.ps1') -InputIR $inputFile -Output $binary

  $allocation = (& $go tool objdump -s '^main\.makeNode$' $binary | Out-String)
  if ($LASTEXITCODE -ne 0 -or $allocation -notmatch 'runtime\.newobject') { throw 'No escaping allocation found in generated makeNode.' }
  $store = (& $go tool objdump -s '^main\.linkNode$' $binary | Out-String)
  if ($LASTEXITCODE -ne 0 -or $store -notmatch 'runtime\.gcWriteBarrier') { throw 'No write barrier found in generated linkNode.' }
  [IO.File]::WriteAllText((Join-Path $work 'allocation.asm.txt'), $allocation)
  [IO.File]::WriteAllText((Join-Path $work 'pointer-store.asm.txt'), $store)
  Write-Output 'PASS: generated allocation and pointer store retain Go allocator/write-barrier paths'

  foreach ($mode in @('normal', 'gc-stress')) {
    $env:GOGC = if ($mode -eq 'gc-stress') { '1' } else { '100' }
    $env:GODEBUG = if ($mode -eq 'gc-stress') { 'gccheckmark=1,clobberfree=1' } else { '' }
    for ($i = 0; $i -lt $Repeat; $i++) {
      $info = [Diagnostics.ProcessStartInfo]::new($binary)
      $info.UseShellExecute = $false
      $info.RedirectStandardOutput = $true
      $info.RedirectStandardError = $true
      $process = [Diagnostics.Process]::new()
      $process.StartInfo = $info
      try {
        if (-not $process.Start()) { throw 'Cannot start GC probe.' }
        $stdout = $process.StandardOutput.ReadToEndAsync()
        $stderr = $process.StandardError.ReadToEndAsync()
        if (-not $process.WaitForExit(15000)) { $process.Kill($true); $process.WaitForExit(); throw 'GC probe timed out.' }
        $output = $stdout.GetAwaiter().GetResult().Replace("`r`n", "`n")
        $errors = $stderr.GetAwaiter().GetResult()
        if ($process.ExitCode -ne 0 -or $output -cne "42`n7`n43`n" -or $errors -ne '') {
          throw "GC probe failed in $mode round $i; exit=$($process.ExitCode); stdout=[$output]; stderr=[$errors]"
        }
      } finally { $process.Dispose() }
    }
    Write-Output "PASS: $mode $Repeat/$Repeat runs; returned reference, pointer field, aliases and deep-stack GC"
  }
  $record = @{ schemaVersion = 1; target = 'windows/amd64'; runsPerMode = $Repeat; modes = @('normal','gc-stress'); expectedLines = @('42','7','43'); compilerSHA256 = $identity.compilerSHA256; binarySHA256 = (Get-FileHash $binary -Algorithm SHA256).Hash.ToLowerInvariant(); scope = 'fixed experimental probe_ref, not general objects or closure support' }
  [IO.File]::WriteAllText((Join-Path $work 'result.json'), ($record | ConvertTo-Json -Depth 5))
  Write-Output 'GC probe passed.'
} finally {
  foreach ($name in $previousEnvironment.Keys) { [Environment]::SetEnvironmentVariable($name, $previousEnvironment[$name], 'Process') }
  Set-Location -LiteralPath $previousLocation.Path
}
