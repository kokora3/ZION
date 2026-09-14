param(
  [ValidateRange(1, 86400)]
  [int]$Seconds = 120
)

$ErrorActionPreference = "Stop"
$env:ZION_SOAK_SECONDS = "$Seconds"
$env:GOMAXPROCS = "2"
go test -p=1 ./internal/node -run '^TestOptionalSoak$' -count=1 -v
