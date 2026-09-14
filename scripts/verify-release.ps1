param([string]$ReleaseDirectory = "dist/zion-v0.1.0-alpha.1")

$ErrorActionPreference = "Stop"
$Repository = (Resolve-Path (Join-Path $PSScriptRoot "..")).Path
$Root = [System.IO.Path]::GetFullPath((Join-Path $Repository $ReleaseDirectory))
$Dist = [System.IO.Path]::GetFullPath((Join-Path $Repository "dist"))
if (-not $Root.StartsWith($Dist + [System.IO.Path]::DirectorySeparatorChar)) {
  throw "Release directory must stay inside repository dist"
}
$Manifest = Join-Path $Root "SHA256SUMS"
if (-not (Test-Path -LiteralPath $Manifest)) { throw "missing SHA256SUMS" }
foreach ($Line in Get-Content -LiteralPath $Manifest) {
  if ($Line -notmatch '^([0-9a-f]{64})  (.+)$') { throw "invalid checksum line: $Line" }
  $Path = Join-Path $Root $Matches[2]
  $Actual = (Get-FileHash -Algorithm SHA256 -LiteralPath $Path).Hash.ToLowerInvariant()
  if ($Actual -ne $Matches[1]) { throw "checksum mismatch: $($Matches[2])" }
}
$ForbiddenNames = @('peer.key', 'priv_validator_key.json', 'priv_validator_state.json', 'node_key.json', '.env', '.env.local')
$Found = Get-ChildItem -LiteralPath $Root -Recurse -File | Where-Object { $ForbiddenNames -contains $_.Name }
if ($Found) { throw "release contains secret-bearing file name: $($Found.FullName -join ', ')" }
Write-Host "Checksums and release secret-file boundary verified: $Root"
