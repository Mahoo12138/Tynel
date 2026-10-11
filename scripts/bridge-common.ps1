function Get-TynelBridgeSourceHashes {
  param([Parameter(Mandatory=$true)][string]$Root)
  $paths = @('toolchain.lock.json', 'scripts/build-bridge.ps1', 'scripts/bridge-common.ps1', 'experiments/bootstrap/go.mod')
  foreach ($dir in @('compiler/nir', 'bridge/go', 'experiments/bootstrap')) {
    foreach ($file in (Get-ChildItem -LiteralPath (Join-Path $Root $dir) -Recurse -File -Filter '*.go' | Where-Object { $_.Name -notlike '*_test.go' })) {
      $paths += [IO.Path]::GetRelativePath($Root, $file.FullName).Replace('\', '/')
    }
  }
  $hashes = [ordered]@{}
  foreach ($path in ($paths | Sort-Object)) {
    $hashes[$path] = (Get-FileHash -LiteralPath (Join-Path $Root $path) -Algorithm SHA256).Hash.ToLowerInvariant()
  }
  return $hashes
}

function Assert-TynelBridgeFresh {
  param([string]$Root, $Identity)
  if ($null -eq $Identity.sourceHashes) { throw 'Bridge has no source fingerprints; rebuild.' }
  $current = Get-TynelBridgeSourceHashes -Root $Root
  $recorded = @($Identity.sourceHashes.PSObject.Properties)
  if ($current.Count -ne $recorded.Count) { throw 'Bridge source set changed; rebuild.' }
  foreach ($property in $recorded) {
    if ($current[$property.Name] -ne $property.Value) { throw ('Bridge source changed: ' + $property.Name + '; rebuild.') }
  }
}
