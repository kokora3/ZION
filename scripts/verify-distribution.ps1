$ErrorActionPreference = "Stop"
$Repository = (Resolve-Path (Join-Path $PSScriptRoot "..")).Path
$Version = "v0.1.0-alpha.1"

$BuildInfo = Get-Content -LiteralPath (Join-Path $Repository "internal/buildinfo/buildinfo.go") -Raw
$Package = Get-Content -LiteralPath (Join-Path $Repository "apps/web/package.json") -Raw | ConvertFrom-Json
$Compose = Get-Content -LiteralPath (Join-Path $Repository "docker-compose.yml") -Raw
if ($BuildInfo -notmatch [regex]::Escape('Version   = "' + $Version + '"')) { throw "zion-node version mismatch" }
if ("v$($Package.version)" -ne $Version) { throw "Web version mismatch" }
if ($Compose -notmatch [regex]::Escape("zion-node:$Version") -or $Compose -notmatch [regex]::Escape("zion-web:$Version")) {
  throw "Compose image version mismatch"
}

$Required = @(
  ".dockerignore", "docker-compose.yml",
  "deploy/docker/zion-node.Dockerfile", "deploy/docker/zion-web.Dockerfile",
  "deploy/public-host/.env.example", "deploy/public-host/bootstrap.yaml",
  "deploy/public-host/compose.override.yml", "deploy/public-host/compose.test.yml",
  "deploy/public-host/lib.sh", "deploy/public-host/test.sh", "deploy/public-host/deploy.sh",
  "deploy/public-host/status.sh", "deploy/public-host/logs.sh",
  "deploy/public-host/stop.sh", "deploy/public-host/restart.sh",
  "deploy/public-host/update.sh", "deploy/public-host/backup.sh",
  "deploy/public-host/README.md", "deploy/public-host/PUBLIC-HOST-CHECKLIST.md",
  "docs/deployment/public-internet-node.md",
  "configs/alpha-1/normal.yaml", "configs/alpha-1/bootstrap.yaml", "configs/alpha-1/validator.yaml.example",
  "deploy/portable/run-zion-node.cmd", "deploy/portable/run-zion-node.sh",
  "deploy/systemd/zion-node.service"
)
foreach ($Relative in $Required) {
  if (-not (Test-Path -LiteralPath (Join-Path $Repository $Relative) -PathType Leaf)) { throw "missing $Relative" }
}

$Forbidden = '(?i)(privileged:\s*true|network_mode:\s*host|pid:\s*host|NEXT_PUBLIC_[A-Z0-9_]*(TOKEN|SECRET|KEY))'
foreach ($Relative in @("docker-compose.yml", "deploy/docker/zion-node.Dockerfile", "deploy/docker/zion-web.Dockerfile")) {
  $Text = Get-Content -LiteralPath (Join-Path $Repository $Relative) -Raw
  if ($Text -match $Forbidden) { throw "unsafe distribution setting in $Relative" }
}

$GenesisID = (Get-Content -LiteralPath (Join-Path $Repository "configs/alpha-1/genesis-id.txt") -Raw).Trim()
$PublicConfig = Get-Content -LiteralPath (Join-Path $Repository "deploy/public-host/bootstrap.yaml") -Raw
$PublicOverride = Get-Content -LiteralPath (Join-Path $Repository "deploy/public-host/compose.override.yml") -Raw
if ($GenesisID -notmatch '^[0-9a-f]{64}$' -or $PublicConfig -notmatch [regex]::Escape("genesis_id: $GenesisID")) {
  throw "public-host config does not consume the frozen G1 GenesisID"
}
if ($PublicConfig -notmatch 'roles:\s*\[NORMAL, BOOTSTRAP\]' -or $PublicConfig -notmatch 'consensus:\s*\r?\n\s+enabled:\s*false') {
  throw "public-host config is not authority-free NORMAL + BOOTSTRAP"
}
if ($PublicOverride -notmatch 'target:\s*42000' -or $PublicOverride -notmatch 'protocol:\s*udp' -or
    $PublicOverride -match 'target:\s*(42001|26656|3000)') {
  throw "public-host override does not expose exactly the required P2P/UDP transport"
}
if ($PublicOverride -notmatch 'max-size:\s*"10m"' -or $PublicOverride -notmatch 'max-file:\s*"5"') {
  throw "public-host Docker logs are not bounded"
}
$RuntimeMaterial = @(
  "deploy/public-host/.env.example", "deploy/public-host/bootstrap.yaml", "deploy/public-host/compose.override.yml",
  "deploy/public-host/lib.sh", "deploy/public-host/deploy.sh", "deploy/public-host/status.sh",
  "deploy/public-host/logs.sh", "deploy/public-host/stop.sh", "deploy/public-host/restart.sh",
  "deploy/public-host/update.sh", "deploy/public-host/backup.sh"
)
foreach ($Relative in $RuntimeMaterial) {
  $Text = Get-Content -LiteralPath (Join-Path $Repository $Relative) -Raw
  if ($Text -match '(?i)(digitalocean|droplet|aws|ec2|azure|hetzner|linode)') {
    throw "provider dependency in runtime material: $Relative"
  }
  if ($Text -match '(?i)(BEGIN (RSA |EC |OPENSSH )?PRIVATE KEY|ghp_[A-Za-z0-9]{20,}|github_pat_[A-Za-z0-9_]{20,})') {
    throw "secret-like material in public-host runtime material: $Relative"
  }
  if ($Relative.EndsWith('.sh') -and $Text -notmatch 'set -euo pipefail') {
    throw "strict shell mode missing from $Relative"
  }
}

Write-Host "Distribution, public-host identity, provider neutrality, and secret/privilege boundaries verified"
