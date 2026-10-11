[CmdletBinding()]
param(
  [Parameter(Mandatory=$true)][string]$InputIR,
  [string]$Output = 'dist/probe.exe'
)
$ErrorActionPreference = 'Stop'
$root = Split-Path -Parent $PSScriptRoot
Set-Location -LiteralPath $root
. (Join-Path $PSScriptRoot 'bridge-common.ps1')
$inputPath = (Resolve-Path -LiteralPath $InputIR).Path
# Validate and compile the same content snapshot, even if the source is edited.
$bytes = [IO.File]::ReadAllBytes($inputPath)
if ($bytes.Length -gt 1048576) { throw 'IR artifact exceeds 1 MiB.' }
$digest = [Convert]::ToHexString([Security.Cryptography.SHA256]::HashData($bytes)).ToLowerInvariant()
$inputs = Join-Path $root '.tynel/inputs'
New-Item -ItemType Directory -Path $inputs -Force | Out-Null
$snapshot = Join-Path $inputs ($digest + '.nir.json')
if (Test-Path -LiteralPath $snapshot) {
  if ((Get-FileHash $snapshot -Algorithm SHA256).Hash.ToLowerInvariant() -ne $digest) { throw 'IR snapshot hash mismatch.' }
} else { [IO.File]::WriteAllBytes($snapshot, $bytes) }
$inputPath = $snapshot
$outputPath = [IO.Path]::GetFullPath($Output, $root)
$compiler = Join-Path $root '.tynel/bin/tynel-compile.exe'
$identityFile = Join-Path $root '.tynel/bridge/identity.json'
if (-not (Test-Path $compiler) -or -not (Test-Path $identityFile)) { throw 'Run scripts/build-bridge.ps1 first.' }
$identity = Get-Content -Raw $identityFile | ConvertFrom-Json
Assert-TynelBridgeFresh -Root $root -Identity $identity
$lock = Get-Content -Raw 'toolchain.lock.json' | ConvertFrom-Json
if ($identity.goVersion -ne $lock.go.version -or $identity.bridgeABI -ne $lock.bridgeABI -or $identity.irVersion -ne $lock.irVersion -or $identity.supportABI -ne $lock.supportABI -or $identity.supportAnchor -ne $lock.supportAnchor) { throw 'Bridge/support identity mismatch; rebuild.' }
$go = Join-Path $identity.goroot 'bin/go.exe'
$env:GOROOT = $identity.goroot
$env:GOTOOLCHAIN = 'local'
New-Item -ItemType Directory -Path (Split-Path $outputPath) -Force | Out-Null
$env:TYNEL_IR = $inputPath
$env:TYNEL_COMPILER = $compiler
$env:TYNEL_COMPILER_SHA256 = $identity.compilerSHA256
$actualHash = (Get-FileHash $compiler -Algorithm SHA256).Hash.ToLowerInvariant()
if ($actualHash -ne $identity.compilerSHA256) { throw 'Compiler binary hash mismatch; rebuild.' }
& $go run ./cmd/tynel check-ir $inputPath
if ($LASTEXITCODE -ne 0) { throw 'IR rejected before native build.' }
$wrapper = Join-Path $root '.tynel/bin/tynel-toolexec.exe'
& $go build -o $wrapper ./cmd/tynel-toolexec
if ($LASTEXITCODE -ne 0) { throw 'Cannot build tool wrapper.' }
Push-Location -LiteralPath (Join-Path $root 'experiments/bootstrap')
try {
  & $go build -p 2 -toolexec ('"' + $wrapper + '"') -o $outputPath .
  if ($LASTEXITCODE -ne 0) { throw "Native build failed: $LASTEXITCODE" }
} finally { Pop-Location }
Write-Output "Built IR program: $outputPath"
