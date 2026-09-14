package chain

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"testing"

	"github.com/kokora3/zion/internal/governance"
	"github.com/kokora3/zion/internal/objects"
	"github.com/kokora3/zion/internal/protocol"
	"github.com/kokora3/zion/internal/registry"
)

type deterministicStep struct {
	height int64
	tx     Transaction
}

func TestFinalV01DeterminismAcrossAllCanonicalDomains(t *testing.T) {
	scenario := setupGovernanceState(t)
	initial, err := BootstrapRegistries(scenario.state)
	if err != nil {
		t.Fatal(err)
	}
	rotatedKey := ed25519.NewKeyFromSeed(bytes.Repeat([]byte{0xe1}, ed25519.SeedSize))
	steps := []deterministicStep{{1, rotationTransaction(t, scenario.proofs[0], scenario.keys[0], rotatedKey, 1)}}
	scenario.keys[0] = rotatedKey

	membershipTx := membershipProposalTx(t, scenario, 0, 500)
	steps = append(steps, deterministicStep{2, membershipTx})
	for i := 0; i < 3; i++ {
		steps = append(steps, deterministicStep{int64(3 + i), voteTx(t, membershipTx.GovernanceProposal.ProposalID,
			scenario.proofs[i].IdentityID, scenario.keys[i], governance.Yes)})
	}
	steps = append(steps,
		deterministicStep{12, actionTx(t, GovernanceFinalize, membershipTx.GovernanceProposal.ProposalID, scenario.proofs[0].IdentityID, scenario.keys[0])},
		deterministicStep{13, actionTx(t, GovernanceExecute, membershipTx.GovernanceProposal.ProposalID, scenario.proofs[0].IdentityID, scenario.keys[0])})

	researchBody := registryResearchBody("Final deterministic Research")
	researchTx := registryProposalTx(t, scenario, governance.ResearchAdmission, researchBody, 600)
	steps = appendRegistrySteps(t, steps, scenario, researchTx, 20)
	researchID, _ := researchBody.ID()
	resourceBody := registryResourceBody("Final deterministic Resource")
	resourceBody.CanonicalRefs = []registry.CanonicalReference{{Relation: registry.RelationRelated,
		TargetKind: registry.TargetResearch, TargetID: researchID.String()}}
	resourceTx := registryProposalTx(t, scenario, governance.ResourceAdmission, resourceBody, 700)
	steps = appendRegistrySteps(t, steps, scenario, resourceTx, 40)

	left := replayDeterministicSteps(t, initial, steps)
	// Snapshot reconstruction independently reallocates every map/slice/key.
	right, err := StateFromSnapshot(mustSnapshot(t, initial))
	if err != nil {
		t.Fatal(err)
	}
	right = replayDeterministicSteps(t, right, steps)
	leftBytes, _ := left.CanonicalBytes()
	rightBytes, _ := right.CanonicalBytes()
	leftHash, _ := left.Hash()
	rightHash, _ := right.Hash()
	if !bytes.Equal(leftBytes, rightBytes) || leftHash.String() != rightHash.String() ||
		!bytes.Equal(leftHash.Digest, rightHash.Digest) {
		t.Fatal("same genesis, order, and heights produced different Snapshot/StateHash/AppHash")
	}
	if len(left.Identities) != 4 || left.Identities[scenario.proofs[0].IdentityID.String()].Sequence != 1 ||
		len(left.Governance.Proposals) != 3 || len(left.Research) != 1 || len(left.Resources) != 1 {
		t.Fatal("final scenario did not cover identity rotation, membership, governance, and registries")
	}

	payload := []byte("off-chain final determinism object")
	object := objects.Object{Core: protocol.UnsignedObjectCore{ObjectType: "note", SchemaVersion: protocol.UnsignedObjectSchemaV1,
		CreatedAt: 1, ContentHash: protocol.NewContentHash(payload), SizeBytes: uint64(len(payload)),
		Visibility: protocol.VisibilityPublic, Metadata: map[string]string{}}, Payload: payload}
	store, err := objects.Open(t.TempDir(), objects.DefaultQuotaBytes)
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := store.Put(context.Background(), object); err != nil {
		t.Fatal(err)
	}
	afterOffChain, _ := left.Hash()
	if afterOffChain.String() != leftHash.String() {
		t.Fatal("off-chain object availability changed StateHash")
	}
}

func appendRegistrySteps(t testing.TB, steps []deterministicStep, scenario governanceTestState, tx Transaction, height int64) []deterministicStep {
	t.Helper()
	steps = append(steps, deterministicStep{height, tx})
	for i := 0; i < 3; i++ {
		steps = append(steps, deterministicStep{height + int64(i) + 1,
			voteTx(t, tx.GovernanceProposal.ProposalID, scenario.proofs[i].IdentityID, scenario.keys[i], governance.Yes)})
	}
	return append(steps,
		deterministicStep{height + 10, actionTx(t, GovernanceFinalize, tx.GovernanceProposal.ProposalID, scenario.proofs[0].IdentityID, scenario.keys[0])},
		deterministicStep{height + 11, actionTx(t, GovernanceExecute, tx.GovernanceProposal.ProposalID, scenario.proofs[0].IdentityID, scenario.keys[0])})
}

func replayDeterministicSteps(t testing.TB, state State, steps []deterministicStep) State {
	t.Helper()
	for _, step := range steps {
		var err error
		state, _, err = ApplyWithContext(state, step.tx, ExecutionContext{Height: step.height})
		if err != nil {
			t.Fatalf("apply %s at %d: %v", step.tx.Type, step.height, err)
		}
	}
	return state
}
