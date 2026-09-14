package node

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/kokora3/zion/internal/governance"
	"github.com/kokora3/zion/internal/research"
)

func TestGovernanceProposalViewUsesStableClientIDsAndAuthoritativeTally(t *testing.T) {
	state := registryRegistryState(t)
	snapshot, err := state.Governance.Snapshot()
	if err != nil {
		t.Fatal(err)
	}
	var view GovernanceProposalView
	for _, proposal := range snapshot.Proposals {
		if proposal.Body.Kind == governance.ResearchAdmission {
			view = governanceProposalView(proposal)
			break
		}
	}
	if view.ProposalID == "" || !strings.HasPrefix(view.ProposalID, "zion:proposal:sha256:") ||
		view.Proposer == "" || !strings.HasPrefix(view.Proposer, "zion:id:sha256:") {
		t.Fatalf("typed identifiers were not projected as protocol strings: %+v", view)
	}
	if view.Yes != 3 || view.No != 0 || view.Abstain != 0 || view.Participants != 3 || view.ElectorateCount != 3 {
		t.Fatalf("unexpected authoritative proposal counts: %+v", view)
	}
	if _, ok := view.Payload.(*research.EntryBody); !ok {
		t.Fatalf("research payload type = %T", view.Payload)
	}
	raw, err := json.Marshal(view)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(raw), "HashDigest") || strings.Contains(string(raw), "Digest") || strings.Contains(string(raw), "Algorithm") {
		t.Fatalf("Go digest internals leaked into client JSON: %s", raw)
	}
}
