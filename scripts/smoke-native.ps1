[CmdletBinding()]
param()
$ErrorActionPreference = 'Stop'
$PSNativeCommandUseErrorActionPreference = $false
$root = Split-Path -Parent $PSScriptRoot
Set-Location -LiteralPath $root
$scaffold = Join-Path $root 'experiments/bootstrap/main.go'
$before = (Get-FileHash $scaffold -Algorithm SHA256).Hash
$work = Join-Path $root '.tynel/smoke'
New-Item -ItemType Directory -Path $work -Force | Out-Null
$inputFile = Join-Path $work 'input.nir.json'
$binary = Join-Path $work 'probe.exe'
$utf8 = [Text.UTF8Encoding]::new($false)
$module = Get-Content -Raw 'examples/add.nir.json' | ConvertFrom-Json
function Save-IR($value) { [IO.File]::WriteAllText($inputFile, ($value | ConvertTo-Json -Depth 80), $utf8) }
function Build-And-Expect([string]$expected) {
  & (Join-Path $PSScriptRoot 'compile-ir.ps1') -InputIR $inputFile -Output $binary
  $output = (& $binary | Out-String).Replace("`r`n", "`n").Trim()
  if ($LASTEXITCODE -ne 0 -or $output -cne $expected) { throw "Native output mismatch: expected [$expected], got [$output], exit=$LASTEXITCODE" }
}
Save-IR $module
Build-And-Expect '42'
$nodeOutput = (& node 'examples/add.reference.ts' | Out-String).Trim()
if ($LASTEXITCODE -ne 0 -or $nodeOutput -cne '42') { throw 'Node reference mismatch' }
Write-Output 'PASS: NIR add and Node reference both produce 42'
# Same IR path and output path, new input bytes: exercise the go build cache.
$module.functions[1].body[0].value.args[0].bits = '4044800000000000'
Save-IR $module
Build-And-Expect '43'
Write-Output 'PASS: changed IR at the same path produces 43 (cache invalidation)'
$loc = @{ file = 'special.reference.ts'; line = 1; column = 1 }
function Number-Expr([string]$bits) { return @{ op = 'number'; type = 'number'; bits = $bits; loc = $loc } }
$body = @()
foreach ($bits in @('8000000000000000','7ff8000000000042','7ff0000000000000','fff0000000000000')) {
  $body += @{ op = 'print_number'; loc = $loc; value = (Number-Expr $bits) }
}
$body += @{ op = 'print_number'; loc = $loc; value = @{ op = 'add'; type = 'number'; loc = $loc; args = @((Number-Expr '3fb999999999999a'), (Number-Expr '3fc999999999999a')) } }
$module.functions[1].body = $body
Save-IR $module
Build-And-Expect "-0`nNaN`n+Inf`n-Inf`n0.30000000000000004"
Write-Output 'PASS: native negative zero, NaN, infinities and binary64 addition'
$module.functions[1].body = @()
Save-IR $module
Build-And-Expect ''
Write-Output 'PASS: empty main generates a real native function'
# Bypass the CLI precheck ONLY in this negative test: the compiler itself
# must reject malformed IR, not trust a caller's previous validation.
$module.irVersion = 999
Save-IR $module
$identity = Get-Content -Raw '.tynel/bridge/identity.json' | ConvertFrom-Json
$go = Join-Path $identity.goroot 'bin/go.exe'
$env:GOROOT = $identity.goroot
$env:GOTOOLCHAIN = 'local'
$env:TYNEL_IR = $inputFile
$env:TYNEL_COMPILER = Join-Path $root '.tynel/bin/tynel-compile.exe'
$env:TYNEL_COMPILER_SHA256 = $identity.compilerSHA256
$wrapper = Join-Path $root '.tynel/bin/tynel-toolexec.exe'
Push-Location -LiteralPath (Join-Path $root 'experiments/bootstrap')
try {
  $log = (& $go build -p 2 -toolexec ('"' + $wrapper + '"') -o (Join-Path $work 'rejected.exe') . 2>&1 | Out-String)
  if ($LASTEXITCODE -eq 0 -or $log -notmatch 'TY4001') { throw "Compiler failed to reject bad IR correctly: $log" }
} finally { Pop-Location }
Write-Output 'PASS: compiler-side version rejection (TY4001)'
if ((Get-FileHash $scaffold -Algorithm SHA256).Hash -ne $before) { throw 'Bootstrap source changed during compilation.' }
Write-Output 'PASS: handwritten Go scaffold unchanged across all input variants'
. (Join-Path $PSScriptRoot 'bridge-common.ps1')
$staleIdentity = Get-Content -Raw '.tynel/bridge/identity.json' | ConvertFrom-Json
$staleIdentity.sourceHashes.'compiler/nir/ir.go' = 'stale'
$rejected = $false
try { Assert-TynelBridgeFresh -Root $root -Identity $staleIdentity } catch { $rejected = $_.Exception.Message -like '*Bridge source changed*' }
if (-not $rejected) { throw 'Stale bridge identity was accepted.' }
Write-Output 'PASS: changed source fingerprints require a bridge rebuild'
Write-Output 'Native smoke: 7 checks passed.'
