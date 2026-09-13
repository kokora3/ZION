package governance

import (
	"crypto/ed25519"
	"strings"
	"testing"

	"github.com/kokora3/zion/internal/identity"
	"github.com/kokora3/zion/internal/membership"
	"github.com/kokora3/zion/internal/protocol"
	"github.com/kokora3/zion/internal/registry"
	"github.com/kokora3/zion/internal/research"
)

func testPrivateKey(start byte) ed25519.PrivateKey {
	seed := make([]byte, ed25519.SeedSize)
	for i := range seed {
		seed[i] = start + byte(i)
	}
	return ed25519.NewKeyFromSeed(seed)
}

func TestResearchAdmissionRetainsExistingProposalPayloadLimit(t *testing.T) {
	contributors := make([]research.Contributor, research.MaxContributors)
	for position := range contributors {
		contributors[position] = research.Contributor{DisplayName: strings.Repeat("x", research.MaxContributorNameBytes)}
	}
	body := ProposalBody{SchemaVersion: Schema, NetworkID: protocol.Alpha1NetworkID, Kind: ResearchAdmission,
		Proposer: identity.IdentityID{HashDigest: protocol.HashBytes([]byte("proposer"))}, CreatedAt: 1,
		Research: &research.EntryBody{SchemaVersion: research.Schema, Title: "bounded", Summary: "bounded", Contributors: contributors,
			ExternalIdentifiers: []research.ExternalIdentifier{}, ObjectRefs: []protocol.ObjectID{}, CanonicalRefs: []registry.CanonicalReference{}}}
	if body.Research.Validate() != nil {
		t.Fatal("field-valid research candidate unexpectedly invalid")
	}
	if body.Validate() == nil {
		t.Fatal("research proposal bypassed the existing 4096-byte governance payload limit")
	}
}

func testIdentity(t testing.TB, key ed25519.PrivateKey, createdAt protocol.ProtocolTimestamp) identity.IdentityID {
	t.Helper()
	body := identity.IdentityGenesisBody{SchemaVersion: identity.GenesisSchema,
		InitialPublicKey: identity.PublicFromPrivate(key), CreatedAt: createdAt}
	id, err := identity.DeriveIdentityID(body)
	if err != nil {
		t.Fatal(err)
	}
	return id
}

func testProposalBody(t testing.TB, proposer identity.IdentityID) ProposalBody {
	t.Helper()
	target := testIdentity(t, testPrivateKey(96), 4)
	return ProposalBody{SchemaVersion: Schema, NetworkID: protocol.Alpha1NetworkID, Kind: MembershipChange,
		Proposer: proposer, CreatedAt: 100, Membership: &MembershipChangePayload{
			Target: target, Expected: membership.Pending, Requested: membership.Active,
		}}
}

func TestPolicyIntegerThresholdAndAbstain(t *testing.T) {
	policy := AlphaPolicy()
	tests := []struct {
		name  string
		tally Tally
		want  ProposalStatus
	}{
		{"three yes", Tally{Yes: 3, Participants: 3}, Approved},
		{"two yes one no is exactly two thirds", Tally{Yes: 2, No: 1, Participants: 3}, Rejected},
		{"insufficient participation", Tally{Yes: 2, Participants: 2}, Expired},
		{"abstain participates but is not counted", Tally{Yes: 2, Abstain: 1, Participants: 3}, Approved},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := policy.Decision(test.tally); got != test.want {
				t.Fatalf("decision=%s, want %s", got, test.want)
			}
		})
	}
}

func TestGovernanceSignatureDomains(t *testing.T) {
	privateKey := testPrivateKey(0)
	publicKey := identity.PublicFromPrivate(privateKey)
	proposer := testIdentity(t, privateKey, 1)
	body := testProposalBody(t, proposer)
	proposal, err := CreateProposal(body, privateKey)
	if err != nil || VerifyProposal(proposal, publicKey) != nil {
		t.Fatalf("valid proposal signature failed: %v", err)
	}

	tampered := proposal
	tampered.Body.CreatedAt++
	if VerifyProposal(tampered, publicKey) == nil {
		t.Fatal("tampered proposal signature accepted")
	}
	wrongDomain := proposal
	wrongDomain.Signature, err = sign(voteDomain, body.NetworkID, body, privateKey)
	if err != nil || VerifyProposal(wrongDomain, publicKey) == nil {
		t.Fatal("wrong proposal signature domain accepted")
	}

	voteBody := VoteBody{SchemaVersion: Schema, NetworkID: protocol.Alpha1NetworkID,
		ProposalID: proposal.ProposalID, Voter: proposer, Choice: Yes}
	vote, err := CreateVote(voteBody, privateKey)
	if err != nil || VerifyVote(vote, publicKey) != nil {
		t.Fatalf("valid vote signature failed: %v", err)
	}
	tamperedVote := vote
	tamperedVote.Body.Choice = No
	if VerifyVote(tamperedVote, publicKey) == nil {
		t.Fatal("tampered vote signature accepted")
	}
	reusedVote := vote
	reusedVote.Signature = proposal.Signature
	if VerifyVote(reusedVote, publicKey) == nil {
		t.Fatal("proposal signature reused as vote signature")
	}
	reusedProposal := proposal
	reusedProposal.Signature = vote.Signature
	if VerifyProposal(reusedProposal, publicKey) == nil {
		t.Fatal("vote signature reused as proposal signature")
	}

	wrongNetwork := proposal
	wrongNetwork.Signature, err = sign(proposalDomain, "other-network", body, privateKey)
	if err != nil || VerifyProposal(wrongNetwork, publicKey) == nil {
		t.Fatal("proposal signature from another network accepted")
	}
}

func TestGovernanceResourceBounds(t *testing.T) {
	proposer := testIdentity(t, testPrivateKey(0), 1)
	body := ProposalBody{SchemaVersion: Schema, NetworkID: protocol.Alpha1NetworkID, Kind: Migration,
		Proposer: proposer, CreatedAt: 100, Migration: &MigrationPayload{TargetNetwork: "zion-alpha-2",
			TargetVersion: "0.2", MigrationIdentifier: string(make([]byte, MaxProposalPayloadBytes+1))}}
	if _, err := CreateProposal(body, testPrivateKey(0)); err == nil {
		t.Fatal("oversized governance proposal payload accepted")
	}
	state, err := NewState([]Validator{
		{Operator: proposer, PublicKey: make([]byte, ed25519.PublicKeySize), Power: ValidatorPower},
		{Operator: proposer, PublicKey: append([]byte{1}, make([]byte, ed25519.PublicKeySize-1)...), Power: ValidatorPower},
		{Operator: proposer, PublicKey: append([]byte{2}, make([]byte, ed25519.PublicKeySize-1)...), Power: ValidatorPower},
	})
	if err != nil {
		t.Fatal(err)
	}
	validBody := testProposalBody(t, proposer)
	proposalID, err := validBody.ID()
	if err != nil {
		t.Fatal(err)
	}
	record := ProposalRecord{ID: proposalID, Body: validBody, Status: Open,
		StartHeight: 1, EndHeight: 2, Electorate: make([]identity.IdentityID, MaxElectorateSize+1), Votes: map[string]VoteRecord{}}
	state.Proposals[record.ID.String()] = record
	if _, err := state.Snapshot(); err == nil {
		t.Fatal("oversized electorate accepted by canonical snapshot")
	}
}

func TestProposalIDStrictAndStable(t *testing.T) {
	proposer := testIdentity(t, testPrivateKey(0), 1)
	body := testProposalBody(t, proposer)
	first, err := body.ID()
	if err != nil {
		t.Fatal(err)
	}
	second, err := body.ID()
	if err != nil || first.String() != second.String() {
		t.Fatal("ProposalID is not stable")
	}
	parsed, err := ParseProposalID(first.String())
	if err != nil || parsed.String() != first.String() {
		t.Fatal("ProposalID round trip failed")
	}
	for _, invalid := range []string{"", "zion:tx:sha256:" + first.String(), "ZION:proposal:sha256:00", first.String() + "00"} {
		if _, err := ParseProposalID(invalid); err == nil {
			t.Fatalf("invalid ProposalID accepted: %q", invalid)
		}
	}
}

func TestFinalizeAndExecuteSignatureDomains(t *testing.T) {
	privateKey := testPrivateKey(0)
	publicKey := identity.PublicFromPrivate(privateKey)
	proposer := testIdentity(t, privateKey, 1)
	proposalID, err := testProposalBody(t, proposer).ID()
	if err != nil {
		t.Fatal(err)
	}
	body := ActionBody{SchemaVersion: Schema, NetworkID: protocol.Alpha1NetworkID,
		ProposalID: proposalID, Submitter: proposer}
	finalize, err := CreateFinalize(body, privateKey)
	if err != nil || VerifyFinalize(finalize, publicKey) != nil {
		t.Fatal("valid finalization authorization failed")
	}
	execute, err := CreateExecute(body, privateKey)
	if err != nil || VerifyExecute(execute, publicKey) != nil {
		t.Fatal("valid execution authorization failed")
	}
	crossFinalize := finalize
	crossFinalize.Signature = execute.Signature
	if VerifyFinalize(crossFinalize, publicKey) == nil {
		t.Fatal("execution signature reused for finalization")
	}
	crossExecute := execute
	crossExecute.Signature = finalize.Signature
	if VerifyExecute(crossExecute, publicKey) == nil {
		t.Fatal("finalization signature reused for execution")
	}
}

func FuzzParseProposalID(f *testing.F) {
	f.Add("")
	f.Add("zion:proposal:sha256:00")
	f.Fuzz(func(t *testing.T, value string) {
		_, _ = ParseProposalID(value)
	})
}
