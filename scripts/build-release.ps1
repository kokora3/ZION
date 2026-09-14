param(
  [string]$Version = "v0.1.0-alpha.1",
  [string]$OutputDirectory = "dist"
)

$ErrorActionPreference = "Stop"
$Repository = (Resolve-Path (Join-Path $PSScriptRoot "..")).Path
$OutputRoot = [System.IO.Path]::GetFullPath((Join-Path $Repository $OutputDirectory))
$ExpectedRoot = [System.IO.Path]::GetFullPath((Join-Path $Repository "dist"))
if ($OutputRoot -ne $ExpectedRoot) {
  throw "Release output must resolve to the repository dist directory"
}
if ($Version -notmatch '^v[0-9]+\.[0-9]+\.[0-9]+-alpha\.[0-9]+$') {
  throw "Release version must use the vMAJOR.MINOR.PATCH-alpha.N form"
}
$ReleaseRoot = Join-Path $OutputRoot "zion-$Version"
if (Test-Path -LiteralPath $ReleaseRoot) {
  Remove-Item -LiteralPath $ReleaseRoot -Recurse -Force
}
New-Item -ItemType Directory -Force -Path $ReleaseRoot | Out-Null

$Commit = (git -C $Repository rev-parse --short=12 HEAD).Trim()
if ($LASTEXITCODE -ne 0) { throw "git revision lookup failed" }
$Epoch = $env:SOURCE_DATE_EPOCH
if ($Epoch) {
  $BuildDate = [DateTimeOffset]::FromUnixTimeSeconds([int64]$Epoch).UtcDateTime.ToString("yyyy-MM-ddTHH:mm:ssZ")
} else {
  $BuildDate = (git -C $Repository log -1 --format=%cI).Trim()
}
$LdFlags = "-s -w -X github.com/kokora3/zion/internal/buildinfo.Version=$Version -X github.com/kokora3/zion/internal/buildinfo.Commit=$Commit -X github.com/kokora3/zion/internal/buildinfo.BuildDate=$BuildDate"

function Build-Package([string]$GoOS, [string]$GoArch, [string]$Suffix) {
  $Name = "zion-$Version-$GoOS-$GoArch"
  $Stage = Join-Path $ReleaseRoot $Name
  New-Item -ItemType Directory -Force -Path $Stage | Out-Null
  $env:GOOS = $GoOS
  $env:GOARCH = $GoArch
  $env:CGO_ENABLED = if ($GoOS -eq "windows") { "1" } else { "0" }
  go build -trimpath -ldflags $LdFlags -o (Join-Path $Stage "zion-node$Suffix") ./cmd/zion-node
  if ($LASTEXITCODE -ne 0) { throw "zion-node $GoOS/$GoArch build failed" }
  go build -trimpath -ldflags $LdFlags -o (Join-Path $Stage "zionctl$Suffix") ./cmd/zionctl
  if ($LASTEXITCODE -ne 0) { throw "zionctl $GoOS/$GoArch build failed" }
  Copy-Item -LiteralPath (Join-Path $Repository "README.md") -Destination $Stage
  Copy-Item -LiteralPath (Join-Path $Repository "LICENSE") -Destination $Stage
  Copy-Item -LiteralPath (Join-Path $Repository "SECURITY.md") -Destination $Stage
  New-Item -ItemType Directory -Force -Path (Join-Path $Stage "configs"), (Join-Path $Stage "docs") | Out-Null
  Copy-Item -LiteralPath (Join-Path $Repository "configs\alpha-1") -Destination (Join-Path $Stage "configs\alpha-1") -Recurse
  Copy-Item -LiteralPath (Join-Path $Repository "docs\operations") -Destination (Join-Path $Stage "docs\operations") -Recurse
  @("version=$Version", "commit=$Commit", "build_date=$BuildDate", "go_version=$(go version)") | Set-Content -LiteralPath (Join-Path $Stage "BUILDINFO") -Encoding utf8
  if ($GoOS -eq "windows") {
    Compress-Archive -Path (Join-Path $Stage "*") -DestinationPath (Join-Path $ReleaseRoot "$Name.zip") -CompressionLevel Optimal
  } else {
    tar -C $Stage -czf (Join-Path $ReleaseRoot "$Name.tar.gz") .
    if ($LASTEXITCODE -ne 0) { throw "Linux archive creation failed" }
  }
}

Push-Location $Repository
try {
  Build-Package "windows" "amd64" ".exe"
  Build-Package "linux" "amd64" ""
  $GoInventory = @(
    "zion-node (windows/amd64):"
    (go version -m (Join-Path $ReleaseRoot "zion-$Version-windows-amd64\zion-node.exe"))
    ""
    "zionctl (windows/amd64):"
    (go version -m (Join-Path $ReleaseRoot "zion-$Version-windows-amd64\zionctl.exe"))
    ""
    "zion-node (linux/amd64):"
    (go version -m (Join-Path $ReleaseRoot "zion-$Version-linux-amd64\zion-node"))
    ""
    "zionctl (linux/amd64):"
    (go version -m (Join-Path $ReleaseRoot "zion-$Version-linux-amd64\zionctl"))
  )
  if ($LASTEXITCODE -ne 0) { throw "Go binary dependency inventory failed" }
  $GoInventory | Set-Content -LiteralPath (Join-Path $ReleaseRoot "GO-MODULES.txt") -Encoding utf8
  $WebInventory = npm --prefix apps/web ls --all --json
  if ($LASTEXITCODE -ne 0) { throw "Web dependency inventory failed" }
  $WebInventory | Set-Content -LiteralPath (Join-Path $ReleaseRoot "WEB-DEPENDENCIES.json") -Encoding utf8
  $Archives = Get-ChildItem -LiteralPath $ReleaseRoot -File | Where-Object { $_.Name -match '\.(zip|tar\.gz)$' } | Sort-Object Name
  $Checksums = foreach ($Archive in $Archives) {
    $Hash = (Get-FileHash -Algorithm SHA256 -LiteralPath $Archive.FullName).Hash.ToLowerInvariant()
    "$Hash  $($Archive.Name)"
  }
  $Checksums | Set-Content -LiteralPath (Join-Path $ReleaseRoot "SHA256SUMS") -Encoding ascii
  Write-Host "Release artifacts: $ReleaseRoot"
} finally {
  Pop-Location
  Remove-Item Env:GOOS -ErrorAction SilentlyContinue
  Remove-Item Env:GOARCH -ErrorAction SilentlyContinue
}
