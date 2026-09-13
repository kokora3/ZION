package chain

import (
	"bytes"
	"context"
	"testing"

	"github.com/kokora3/zion/internal/governance"
	"github.com/kokora3/zion/internal/objects"
	"github.com/kokora3/zion/internal/protocol"
	"github.com/kokora3/zion/internal/registry"
	"github.com/kokora3/zion/internal/research"
	"github.com/kokora3/zion/internal/resources"
)

func registryResearchBody(title string) research.EntryBody {
	return research.EntryBody{SchemaVersion: research.Schema, Title: title, Summary: "Deterministic research metadata.",
		Contributors: []research.Contributor{{DisplayName: "Alice"}}, ExternalIdentifiers: []research.ExternalIdentifier{},
		ObjectRefs: []protocol.ObjectID{}, CanonicalRefs: []registry.CanonicalReference{}}
}

func registryResourceBody(name string) resources.EntryBody {
	return resources.EntryBody{SchemaVersion: resources.Schema, Kind: resources.Dataset, Name: name,
		Summary: "Deterministic resource metadata.", Maintainers: []resources.Maintainer{{DisplayName: "Alice"}},
		ObjectRefs: []protocol.ObjectID{}, CanonicalRefs: []registry.CanonicalReference{}}
}

func registryProposalTx(t testing.TB, scenario governanceTestState, kind governance.ProposalKind, payload any, created protocol.ProtocolTimestamp) Transaction {
	t.Helper()
	body := governance.ProposalBody{SchemaVersion: governance.Schema, NetworkID: protocol.Alpha1NetworkID,
		Kind: kind, Proposer: scenario.proofs[0].IdentityID, CreatedAt: created}
	switch value := payload.(type) {
	case research.EntryBody:
		body.Research = &value
	case resources.EntryBody:
		body.Resource = &value
	default:
		t.Fatal("unsupported registry test payload")
	}
	authorization, err := governance.CreateProposal(body, scenario.keys[0])
	if err != nil {
		t.Fatal(err)
	}
	return Transaction{SchemaVersion: TransactionSchema, NetworkID: protocol.Alpha1NetworkID,
		Type: GovernanceProposal, GovernanceProposal: &authorization}
}

func executeRegistryProposal(t testing.TB, state State, scenario governanceTestState, tx Transaction, height int64) State {
	t.Helper()
	state, _ = applyAt(t, state, tx, height)
	id := tx.GovernanceProposal.ProposalID
	for i := 0; i < 3; i++ {
		state, _ = applyAt(t, state, voteTx(t, id, scenario.proofs[i].IdentityID, scenario.keys[i], governance.Yes), height+int64(i)+1)
	}
	state, _ = applyAt(t, state, actionTx(t, GovernanceFinalize, id, scenario.proofs[0].IdentityID, scenario.keys[0]), height+10)
	state, _ = applyAt(t, state, actionTx(t, GovernanceExecute, id, scenario.proofs[0].IdentityID, scenario.keys[0]), height+11)
	return state
}

func TestRegistryGovernanceAdmissionAndCanonicalDeterminism(t *testing.T) {
	scenario := setupGovernanceState(t)
	state, err := BootstrapRegistries(scenario.state)
	if err != nil {
		t.Fatal(err)
	}
	researchBody := registryResearchBody("ZION Research")
	emptyHash, _ := state.Hash()
	researchTx := registryProposalTx(t, scenario, governance.ResearchAdmission, researchBody, 100)
	state = executeRegistryProposal(t, state, scenario, researchTx, 1)
	researchID, _ := researchBody.ID()
	storedResearch, exists := state.Research[researchID.String()]
	if !exists || storedResearch.ProposalID.String() != researchTx.GovernanceProposal.ProposalID.String() || storedResearch.AdmittedHeight != 12 {
		t.Fatal("research admission was not recorded with its governance provenance")
	}

	resourceBody := registryResourceBody("ZION Dataset")
	resourceBody.CanonicalRefs = []registry.CanonicalReference{{Relation: registry.RelationRelated, TargetKind: registry.TargetResearch, TargetID: researchID.String()}}
	resourceTx := registryProposalTx(t, scenario, governance.ResourceAdmission, resourceBody, 200)
	state = executeRegistryProposal(t, state, scenario, resourceTx, 20)
	registryHash, _ := state.Hash()
	if registryHash.String() == emptyHash.String() {
		t.Fatal("registry state did not contribute to StateHash")
	}
	resourceID, _ := resourceBody.ID()
	if _, exists := state.Resources[resourceID.String()]; !exists {
		t.Fatal("resource admission missing")
	}
	canonical, err := state.CanonicalBytes()
	if err != nil {
		t.Fatal(err)
	}
	restored, err := StateFromSnapshot(mustSnapshot(t, state))
	if err != nil {
		t.Fatal(err)
	}
	restoredBytes, _ := restored.CanonicalBytes()
	if !bytes.Equal(canonical, restoredBytes) {
		t.Fatal("registry snapshot restore changed canonical state")
	}
	if researchID.String() == resourceID.String() {
		t.Fatal("registry namespaces are not distinct")
	}
}

func TestRegistryAdmissionFailuresAreAtomic(t *testing.T) {
	scenario := setupGovernanceState(t)
	state, _ := BootstrapRegistries(scenario.state)
	body := registryResearchBody("Duplicate")
	first := registryProposalTx(t, scenario, governance.ResearchAdmission, body, 100)
	state = executeRegistryProposal(t, state, scenario, first, 1)
	rejectAt(t, state, actionTx(t, GovernanceExecute, first.GovernanceProposal.ProposalID, scenario.proofs[0].IdentityID, scenario.keys[0]), 13, AlreadyExecuted)
	second := registryProposalTx(t, scenario, governance.ResearchAdmission, body, 101)
	state, _ = applyAt(t, state, second, 20)
	for i := 0; i < 3; i++ {
		state, _ = applyAt(t, state, voteTx(t, second.GovernanceProposal.ProposalID, scenario.proofs[i].IdentityID, scenario.keys[i], governance.Yes), int64(21+i))
	}
	state, _ = applyAt(t, state, actionTx(t, GovernanceFinalize, second.GovernanceProposal.ProposalID, scenario.proofs[0].IdentityID, scenario.keys[0]), 30)
	rejectAt(t, state, actionTx(t, GovernanceExecute, second.GovernanceProposal.ProposalID, scenario.proofs[0].IdentityID, scenario.keys[0]), 31, RegistryExists)

	missingID := research.ID{HashDigest: protocol.HashBytes([]byte("missing"))}
	resourceBody := registryResourceBody("Broken reference")
	resourceBody.CanonicalRefs = []registry.CanonicalReference{{Relation: registry.RelationUses, TargetKind: registry.TargetResearch, TargetID: missingID.String()}}
	broken := registryProposalTx(t, scenario, governance.ResourceAdmission, resourceBody, 102)
	state, _ = applyAt(t, state, broken, 40)
	for i := 0; i < 3; i++ {
		state, _ = applyAt(t, state, voteTx(t, broken.GovernanceProposal.ProposalID, scenario.proofs[i].IdentityID, scenario.keys[i], governance.Yes), int64(41+i))
	}
	state, _ = applyAt(t, state, actionTx(t, GovernanceFinalize, broken.GovernanceProposal.ProposalID, scenario.proofs[0].IdentityID, scenario.keys[0]), 50)
	rejectAt(t, state, actionTx(t, GovernanceExecute, broken.GovernanceProposal.ProposalID, scenario.proofs[0].IdentityID, scenario.keys[0]), 51, NotFound)

	for _, transactionType := range []TransactionType{"ResearchRegister", "ResourceRegister"} {
		rejectAt(t, state, Transaction{SchemaVersion: TransactionSchema, NetworkID: protocol.Alpha1NetworkID, Type: transactionType}, 60, Unsupported)
	}
}

func TestRegistryAdmissionKeepsGovernanceDecisionBoundaries(t *testing.T) {
	for name, testCase := range map[string]struct {
		choices []governance.VoteChoice
		status  governance.ProposalStatus
	}{
		"exactly two thirds": {[]governance.VoteChoice{governance.Yes, governance.Yes, governance.No}, governance.Rejected},
		"only two voters":    {[]governance.VoteChoice{governance.Yes, governance.Yes}, governance.Expired},
	} {
		t.Run(name, func(t *testing.T) {
			scenario := setupGovernanceState(t)
			state, _ := BootstrapRegistries(scenario.state)
			tx := registryProposalTx(t, scenario, governance.ResearchAdmission, registryResearchBody(name), 100)
			state, _ = applyAt(t, state, tx, 1)
			for position, choice := range testCase.choices {
				state, _ = applyAt(t, state, voteTx(t, tx.GovernanceProposal.ProposalID, scenario.proofs[position].IdentityID, scenario.keys[position], choice), int64(position+2))
			}
			state, _ = applyAt(t, state, actionTx(t, GovernanceFinalize, tx.GovernanceProposal.ProposalID, scenario.proofs[0].IdentityID, scenario.keys[0]), 11)
			if state.Governance.Proposals[tx.GovernanceProposal.ProposalID.String()].Status != testCase.status || len(state.Research) != 0 {
				t.Fatal("Phase 6 policy boundary changed or registry mutated")
			}
		})
	}
}

func TestRegistryStateSchemaMigration(t *testing.T) {
	scenario := setupGovernanceState(t)
	before, err := scenario.state.CanonicalBytes()
	if err != nil {
		t.Fatal(err)
	}
	restoredV2, err := StateFromSnapshot(mustSnapshot(t, scenario.state))
	if err != nil {
		t.Fatal(err)
	}
	after, _ := restoredV2.CanonicalBytes()
	if !bytes.Equal(before, after) {
		t.Fatal("schema V2 bytes changed")
	}
	v3, err := BootstrapRegistries(scenario.state)
	if err != nil {
		t.Fatal(err)
	}
	if v3.SchemaVersion != StateSchemaV3 || len(v3.Research) != 0 || len(v3.Resources) != 0 {
		t.Fatal("invalid V2 to V3 registry migration")
	}
	if _, err := BootstrapRegistries(v3); err == nil {
		t.Fatal("registry migration replay accepted")
	}
}

func TestRegistryObjectAvailabilityIsNotApplyInput(t *testing.T) {
	scenario := setupGovernanceState(t)
	state, _ := BootstrapRegistries(scenario.state)
	payload := []byte("off-chain-object")
	object := objects.Object{Core: protocol.UnsignedObjectCore{ObjectType: "note", SchemaVersion: protocol.UnsignedObjectSchemaV1,
		CreatedAt: 1, ContentHash: protocol.NewContentHash(payload), SizeBytes: uint64(len(payload)), Visibility: protocol.VisibilityPublic,
		Metadata: map[string]string{}}, Payload: payload}
	objectID, err := object.ObjectID()
	if err != nil {
		t.Fatal(err)
	}
	body := registryResearchBody("Off-chain isolation")
	body.ObjectRefs = []protocol.ObjectID{objectID}
	tx := registryProposalTx(t, scenario, governance.ResearchAdmission, body, 100)
	state = executeRegistryProposal(t, state, scenario, tx, 1)
	if len(state.Research) != 1 {
		t.Fatal("object availability unexpectedly blocked canonical admission")
	}
	before, _ := state.Hash()
	store, err := objects.Open(t.TempDir(), objects.DefaultQuotaBytes)
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := store.Put(context.Background(), object); err != nil {
		t.Fatal(err)
	}
	after, _ := state.Hash()
	if before.String() != after.String() {
		t.Fatal("local object availability changed registry StateHash")
	}
}

func TestRegistryMapOrderAllocationAndReplayDeterminism(t *testing.T) {
	scenario := setupGovernanceState(t)
	state, _ := BootstrapRegistries(scenario.state)
	for position, name := range []string{"Alice research", "Bob research", "Charlie research"} {
		tx := registryProposalTx(t, scenario, governance.ResearchAdmission, registryResearchBody(name), protocol.ProtocolTimestamp(100+position))
		state = executeRegistryProposal(t, state, scenario, tx, int64(1+position*20))
	}
	first, err := state.CanonicalBytes()
	if err != nil {
		t.Fatal(err)
	}
	reordered := cloneState(state)
	reordered.Research = make(map[string]StoredResearch, len(state.Research))
	keys := make([]string, 0, len(state.Research))
	for key := range state.Research {
		keys = append(keys, key)
	}
	for position := len(keys) - 1; position >= 0; position-- {
		reordered.Research[keys[position]] = cloneStoredResearch(state.Research[keys[position]])
	}
	second, _ := reordered.CanonicalBytes()
	if !bytes.Equal(first, second) {
		t.Fatal("registry map insertion order changed canonical state")
	}
	firstHash, _ := state.Hash()
	secondHash, _ := reordered.Hash()
	if firstHash.String() != secondHash.String() {
		t.Fatal("registry map insertion order changed StateHash")
	}
	for run := 0; run < 100; run++ {
		canonical, _ := reordered.CanonicalBytes()
		if !bytes.Equal(first, canonical) {
			t.Fatalf("replay %d changed canonical bytes", run)
		}
	}
}

func mustSnapshot(t testing.TB, state State) Snapshot {
	t.Helper()
	snapshot, err := state.Snapshot()
	if err != nil {
		t.Fatal(err)
	}
	return snapshot
}
