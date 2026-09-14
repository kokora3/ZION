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
$ExpectedByPlatform = @{
  "windows-amd64" = @("zion-node.exe", "zionctl.exe", "run-zion-node.cmd", "README-WINDOWS.md", "configs\normal.yaml", "configs\bootstrap.yaml", "configs\validator.yaml.example", "LICENSE", "CHECKSUMS.txt")
  "linux-amd64" = @("zion-node", "zionctl", "run-zion-node.sh", "README-LINUX.md", "configs\normal.yaml", "configs\bootstrap.yaml", "configs\validator.yaml.example", "LICENSE", "CHECKSUMS.txt")
}
$ReleaseName = Split-Path -Leaf $Root
foreach ($Platform in $ExpectedByPlatform.Keys) {
  $Package = Join-Path $Root "$ReleaseName-$Platform"
  if (-not (Test-Path -LiteralPath $Package -PathType Container)) { throw "missing staged package $Platform" }
  foreach ($Expected in $ExpectedByPlatform[$Platform]) {
    if (-not (Test-Path -LiteralPath (Join-Path $Package $Expected) -PathType Leaf)) {
      throw "package $Platform is missing $Expected"
    }
  }
  $Archive = if ($Platform -eq "windows-amd64") {
    Join-Path $Root "$ReleaseName-$Platform.zip"
  } else {
    Join-Path $Root "$ReleaseName-$Platform.tar.gz"
  }
  $Entries = @(tar -tf $Archive)
  if ($LASTEXITCODE -ne 0 -or $Entries.Count -eq 0) { throw "archive cannot be opened: $Archive" }
  $NormalizedEntries = @($Entries | ForEach-Object { $_.Replace('\', '/').TrimStart('.', '/') })
  foreach ($Expected in $ExpectedByPlatform[$Platform]) {
    $NormalizedExpected = $Expected.Replace('\', '/')
    if ($NormalizedExpected -notin $NormalizedEntries) { throw "archive $Platform is missing $Expected" }
  }
  foreach ($Entry in $NormalizedEntries) {
    if ($Entry -and $ForbiddenNames -contains (Split-Path -Leaf $Entry)) { throw "archive contains secret-bearing file name: $Entry" }
  }
}
$ForbiddenText = '(?i)(BEGIN (RSA |EC |OPENSSH )?PRIVATE KEY|ghp_[A-Za-z0-9]{20,}|github_pat_[A-Za-z0-9_]{20,})'
$TextFiles = Get-ChildItem -LiteralPath $Root -Recurse -File | Where-Object { $_.Length -lt 2MB -and $_.Extension -notin @('.exe', '.gz', '.zip') }
foreach ($File in $TextFiles) {
  if (Select-String -LiteralPath $File.FullName -Pattern $ForbiddenText -Quiet) {
    throw "release contains private-key/token-like text: $($File.FullName)"
  }
}
Write-Host "Checksums, package layouts, and release secret boundaries verified: $Root"
