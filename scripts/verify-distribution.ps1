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

Write-Host "Distribution structure, version consistency, and static secret/privilege boundaries verified"

