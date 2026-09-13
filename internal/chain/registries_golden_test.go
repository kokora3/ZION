package chain

import (
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/kokora3/zion/internal/governance"
	"github.com/kokora3/zion/internal/protocol"
	"github.com/kokora3/zion/internal/registry"
)

type admissionGolden struct {
	Kind       governance.ProposalKind `json:"kind"`
	ProposalID string                  `json:"proposal_id"`
	Canonical  string                  `json:"canonical_payload_cbor_hex"`
}

func phase11Fixture[T any](t testing.TB, name string) T {
	t.Helper()
	var value T
	data, err := os.ReadFile(filepath.Join("testdata", name))
	if err != nil {
		t.Fatal(err)
	}
	if json.Unmarshal(data, &value) != nil {
		t.Fatal("decode fixture")
	}
	return value
}

func TestRegistryGoldenVectors(t *testing.T) {
	scenario := setupGovernanceState(t)
	state, _ := BootstrapRegistries(scenario.state)
	researchBody := registryResearchBody("ZION Research")
	researchTx := registryProposalTx(t, scenario, governance.ResearchAdmission, researchBody, 100)
	researchFixture := phase11Fixture[admissionGolden](t, "research_admission_payload_golden.json")
	researchCanonical, _ := protocol.CanonicalEncode(researchTx.GovernanceProposal.Body)
	if researchTx.GovernanceProposal.ProposalID.String() != researchFixture.ProposalID || hex.EncodeToString(researchCanonical) != researchFixture.Canonical || researchFixture.Kind != governance.ResearchAdmission {
		t.Fatal("research admission golden changed")
	}
	state = executeRegistryProposal(t, state, scenario, researchTx, 1)
	researchID, _ := researchBody.ID()
	resourceBody := registryResourceBody("ZION Dataset")
	resourceBody.CanonicalRefs = []registry.CanonicalReference{{Relation: registry.RelationRelated, TargetKind: registry.TargetResearch, TargetID: researchID.String()}}
	resourceTx := registryProposalTx(t, scenario, governance.ResourceAdmission, resourceBody, 200)
	resourceFixture := phase11Fixture[admissionGolden](t, "resource_admission_payload_golden.json")
	resourceCanonical, _ := protocol.CanonicalEncode(resourceTx.GovernanceProposal.Body)
	if resourceTx.GovernanceProposal.ProposalID.String() != resourceFixture.ProposalID || hex.EncodeToString(resourceCanonical) != resourceFixture.Canonical || resourceFixture.Kind != governance.ResourceAdmission {
		t.Fatal("resource admission golden changed")
	}
	state = executeRegistryProposal(t, state, scenario, resourceTx, 20)
	stateFixture := phase11Fixture[struct {
		Canonical string `json:"canonical_snapshot_cbor_hex"`
		Hash      string `json:"state_hash"`
		Digest    string `json:"sha256_digest_hex"`
	}](t, "registry_state_golden.json")
	canonical, _ := state.CanonicalBytes()
	hash, _ := state.Hash()
	if hex.EncodeToString(canonical) != stateFixture.Canonical || hash.String() != stateFixture.Hash || hex.EncodeToString(protocol.HashBytes(canonical).Digest) != stateFixture.Digest {
		t.Fatal("registry state golden changed")
	}
}
