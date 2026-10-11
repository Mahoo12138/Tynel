[CmdletBinding()]
param(
  [Parameter(Mandatory=$true)][string]$InputTS,
  [string]$Output = 'dist/ts-program.exe'
)
$ErrorActionPreference = 'Stop'
$PSNativeCommandUseErrorActionPreference = $false
$root = Split-Path -Parent $PSScriptRoot
$previousLocation = Get-Location
$previousEnvironment = @{}
foreach ($name in @('GOROOT','GOTOOLCHAIN','TYNEL_IR','TYNEL_COMPILER','TYNEL_COMPILER_SHA256')) {
  $previousEnvironment[$name] = [Environment]::GetEnvironmentVariable($name,'Process')
}
. (Join-Path $PSScriptRoot 'frontend-common.ps1')
. (Join-Path $PSScriptRoot 'bridge-common.ps1')
function Invoke-Frontend([string[]]$Arguments) {
  $info = [Diagnostics.ProcessStartInfo]::new($frontend)
  $info.UseShellExecute = $false
  $info.RedirectStandardOutput = $true
  $info.RedirectStandardError = $true
  $info.StandardOutputEncoding = [Text.UTF8Encoding]::new($false)
  $info.StandardErrorEncoding = [Text.UTF8Encoding]::new($false)
  foreach ($arg in $Arguments) { $info.ArgumentList.Add($arg) }
  $process = [Diagnostics.Process]::new()
  $process.StartInfo = $info
  try {
    if (-not $process.Start()) { throw 'Cannot start scalar frontend.' }
    $stdout = $process.StandardOutput.ReadToEndAsync()
    $stderr = $process.StandardError.ReadToEndAsync()
    if (-not $process.WaitForExit(30000)) { $process.Kill($true); $process.WaitForExit(); throw 'Scalar frontend timed out.' }
    $text = $stdout.GetAwaiter().GetResult()
    $errors = $stderr.GetAwaiter().GetResult()
    if ($process.ExitCode -ne 0) { throw ("Frontend rejected the source (exit $($process.ExitCode)):`n" + $errors) }
    if ($errors -ne '') { throw ('Unexpected frontend stderr: ' + $errors) }
    return $text
  } finally { $process.Dispose() }
}
try {
  Set-Location -LiteralPath $root
  $inputPath = (Resolve-Path -LiteralPath $InputTS).Path
  $outputPath = [IO.Path]::GetFullPath($Output,$root)
  $prefix = $root.TrimEnd('\','/') + [IO.Path]::DirectorySeparatorChar
  if (-not $inputPath.StartsWith($prefix,[StringComparison]::OrdinalIgnoreCase) -or [IO.Path]::GetExtension($inputPath) -cne '.ts') { throw 'Input must be a .ts file inside this project.' }
  $relativeOutput = [IO.Path]::GetRelativePath($root,$outputPath).Replace('\','/')
  if ($relativeOutput -notmatch '^(dist|\.tynel)/.+\.exe$') { throw 'Output must be an .exe under dist/ or .tynel/.' }
  $frontendIdentity = Get-Content -Raw -LiteralPath '.tynel/tsgo/identity.json' | ConvertFrom-Json
  Assert-TynelFrontendFresh -Root $root -Identity $frontendIdentity
  $frontend = Join-Path $root '.tynel/tsgo/tynel-tsgo-probe.exe'
  $bytes = [IO.File]::ReadAllBytes($inputPath)
  if ($bytes.Length -gt 65536) { throw 'Scalar source exceeds 64 KiB.' }
  $sourceHash = [Convert]::ToHexString([Security.Cryptography.SHA256]::HashData($bytes)).ToLowerInvariant()
  $sourceName = [IO.Path]::GetRelativePath($root,$inputPath).Replace('\','/')
  $stage = Join-Path $root ('.tynel/ts-build/' + [guid]::NewGuid().ToString('N'))
  New-Item -ItemType Directory -Path $stage -Force | Out-Null
  $snapshot = Join-Path $stage 'input.ts'
  [IO.File]::WriteAllBytes($snapshot,$bytes)
  $factsText = Invoke-Frontend @('-emit','facts','-source-name',$sourceName,$snapshot)
  $facts = $factsText | ConvertFrom-Json
  if ($facts.sourceSHA256 -cne $sourceHash -or $facts.profile -cne 'tynel.scalar/3' -or $facts.version -ne 3) { throw 'Frontend source/profile identity mismatch.' }
  $irText = Invoke-Frontend @('-emit','nir','-source-name',$sourceName,$snapshot)
  $utf8 = [Text.UTF8Encoding]::new($false)
  $factsPath = Join-Path $stage 'source.facts.json'
  $irPath = Join-Path $stage 'main.nir.json'
  [IO.File]::WriteAllText($factsPath,$factsText,$utf8)
  [IO.File]::WriteAllText($irPath,$irText,$utf8)
  $bridge = Get-Content -Raw '.tynel/bridge/identity.json' | ConvertFrom-Json
  $temporaryBinary = Join-Path $stage 'program.exe'
  & (Join-Path $PSScriptRoot 'compile-ir.ps1') -InputIR $irPath -Output $temporaryBinary
  Assert-TynelFrontendFresh -Root $root -Identity $frontendIdentity
  Assert-TynelBridgeFresh -Root $root -Identity $bridge
  if ((Get-FileHash -LiteralPath '.tynel/bin/tynel-compile.exe' -Algorithm SHA256).Hash.ToLowerInvariant() -cne $bridge.compilerSHA256) { throw 'Backend changed during source compilation.' }
  $record = [ordered]@{
    schemaVersion=1; profile='tynel.scalar/3'; source=$sourceName; sourceSHA256=$sourceHash
    frontendSHA256=$frontendIdentity.compilerSHA256; tsgoCommit=$frontendIdentity.upstreamCommit
    backendSHA256=$bridge.compilerSHA256; goVersion=$bridge.goVersion; irVersion=$bridge.irVersion
    supportABI=$bridge.supportABI; target='windows/amd64'
    facts=[IO.Path]::GetRelativePath($root,$factsPath).Replace('\','/')
    nir=[IO.Path]::GetRelativePath($root,$irPath).Replace('\','/')
    nirSHA256=(Get-FileHash -LiteralPath $irPath -Algorithm SHA256).Hash.ToLowerInvariant()
    binarySHA256=(Get-FileHash -LiteralPath $temporaryBinary -Algorithm SHA256).Hash.ToLowerInvariant()
    scope='single-source scalar pilot; handwritten host support; no JS engine or generated business Go'
  }
  $manifest = Join-Path $stage 'build.json'
  [IO.File]::WriteAllText($manifest,($record | ConvertTo-Json -Depth 5),$utf8)
  New-Item -ItemType Directory -Path (Split-Path -Parent $outputPath) -Force | Out-Null
  # Failed frontend/backend work never replaces a previously successful binary.
  [IO.File]::Move($temporaryBinary,$outputPath,$true)
  Copy-Item -LiteralPath $manifest -Destination ($outputPath + '.tynel.json') -Force
  Write-Output "Built TypeScript scalar program: $outputPath"
  Write-Output "NIR replay input: $irPath"
} finally {
  foreach ($name in $previousEnvironment.Keys) { [Environment]::SetEnvironmentVariable($name,$previousEnvironment[$name],'Process') }
  Set-Location -LiteralPath $previousLocation.Path
}
