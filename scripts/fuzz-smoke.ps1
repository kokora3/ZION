param([ValidateRange(1, 30)][int]$Seconds = 3)

$ErrorActionPreference = "Stop"
$env:GOMAXPROCS = "2"
$Targets = @(
  @{ Package = "./internal/protocol"; Name = "FuzzParseObjectID" },
  @{ Package = "./internal/protocol"; Name = "FuzzDecodeUnsignedObjectCore" },
  @{ Package = "./internal/identity"; Name = "FuzzParseIdentityID" },
  @{ Package = "./internal/identity"; Name = "FuzzParseKeyID" },
  @{ Package = "./internal/identity"; Name = "FuzzPublicKeyValidate" },
  @{ Package = "./internal/identity"; Name = "FuzzSignatureValidate" },
  @{ Package = "./internal/chain"; Name = "FuzzParseTxID" },
  @{ Package = "./internal/governance"; Name = "FuzzParseProposalID" },
  @{ Package = "./internal/consensus"; Name = "FuzzDecodeTransaction" },
  @{ Package = "./internal/p2p"; Name = "FuzzDecodeHello" },
  @{ Package = "./internal/p2p"; Name = "FuzzDecodePEX" },
  @{ Package = "./internal/node"; Name = "FuzzDecodeTxRelayMessage" },
  @{ Package = "./internal/node"; Name = "FuzzDecodeStateSyncMessage" },
  @{ Package = "./internal/objects"; Name = "FuzzDecodeObjectGetRequest" },
  @{ Package = "./internal/objects"; Name = "FuzzDecodeObjectGetResponse" },
  @{ Package = "./internal/board"; Name = "FuzzDecodeBoardContent" },
  @{ Package = "./internal/board"; Name = "FuzzDecodeBoardEvent" },
  @{ Package = "./internal/board"; Name = "FuzzDecodeBoardAnnounce" },
  @{ Package = "./internal/board"; Name = "FuzzDecodeBoardSyncRequest" },
  @{ Package = "./internal/board"; Name = "FuzzDecodeBoardSyncResponse" },
  @{ Package = "./internal/research"; Name = "FuzzParseResearchID" },
  @{ Package = "./internal/research"; Name = "FuzzDecodeResearchEntry" },
  @{ Package = "./internal/resources"; Name = "FuzzParseResourceID" },
  @{ Package = "./internal/resources"; Name = "FuzzDecodeResourceEntry" }
)
foreach ($Target in $Targets) {
  Write-Host "Fuzzing $($Target.Package) $($Target.Name)"
  go test $Target.Package -run '^$' -fuzz "^$($Target.Name)$" -fuzztime "${Seconds}s" -parallel 1
  if ($LASTEXITCODE -ne 0) { exit $LASTEXITCODE }
}
