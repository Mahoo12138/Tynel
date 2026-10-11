function Get-TynelFrontendInputs {
  param([Parameter(Mandatory=$true)][string]$Root)
  $names = @('go.mod', 'toolchain.lock.json', 'scripts/prepare-tsgo.py', 'scripts/build-tsgo-probe.ps1', 'scripts/frontend-common.ps1')
  foreach ($dir in @('bridge/tsgo', 'experiments/tsgo-host', 'compiler/frontend', 'compiler/nir')) {
    foreach ($file in (Get-ChildItem -LiteralPath (Join-Path $Root $dir) -File -Recurse)) {
      if ($file.Extension -eq '.go' -or $file.Name -in @('go.mod','go.sum')) { $names += [IO.Path]::GetRelativePath($Root,$file.FullName).Replace('\','/') }
    }
  }
  $hashes = [ordered]@{}
  foreach ($name in ($names | Sort-Object -Unique)) { $hashes[$name] = (Get-FileHash -LiteralPath (Join-Path $Root $name) -Algorithm SHA256).Hash.ToLowerInvariant() }
  return $hashes
}

function Assert-TynelFrontendFresh {
  param([Parameter(Mandatory=$true)][string]$Root, [Parameter(Mandatory=$true)]$Identity)
  if ($null -eq $Identity.sourceHashes) { throw 'Frontend has no source fingerprints; rebuild.' }
  $current = Get-TynelFrontendInputs -Root $Root
  $recorded = @($Identity.sourceHashes.PSObject.Properties)
  if ($current.Count -ne $recorded.Count) { throw 'Frontend source set changed; rebuild.' }
  foreach ($p in $recorded) { if ($current[$p.Name] -ne $p.Value) { throw ('Frontend source changed: ' + $p.Name + '; rebuild.') } }
  $binary = Join-Path $Root '.tynel/tsgo/tynel-tsgo-probe.exe'
  if ((Get-FileHash -LiteralPath $binary -Algorithm SHA256).Hash.ToLowerInvariant() -ne $Identity.compilerSHA256) { throw 'Frontend binary fingerprint mismatch; rebuild.' }
}
