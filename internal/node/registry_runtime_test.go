package node

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"net/http"
	"testing"
	"time"

	"github.com/kokora3/zion/internal/chain"
	"github.com/kokora3/zion/internal/governance"
	"github.com/kokora3/zion/internal/identity"
	"github.com/kokora3/zion/internal/p2p"
	"github.com/kokora3/zion/internal/protocol"
	"github.com/kokora3/zion/internal/registry"
	"github.com/kokora3/zion/internal/research"
	"github.com/kokora3/zion/internal/resources"
)

func TestRegistryRuntimeListGetSearchAndAvailabilityHint(t *testing.T) {
	cfg := testRuntimeConfig(t, "registry-api", true, false)
	runtime, err := New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	researchBody := research.EntryBody{SchemaVersion: research.Schema, Title: "Local discovery", Summary: "Research search",
		Contributors: []research.Contributor{{DisplayName: "Alice"}}, ExternalIdentifiers: []research.ExternalIdentifier{}, ObjectRefs: []protocol.ObjectID{}, CanonicalRefs: nil}
	resourceBody := resources.EntryBody{SchemaVersion: resources.Schema, Kind: resources.Tool, Name: "Discovery Tool", Summary: "Resource search",
		Maintainers: []resources.Maintainer{{DisplayName: "Bob"}}, ObjectRefs: []protocol.ObjectID{}, CanonicalRefs: nil}
	researchID, _ := researchBody.ID()
	resourceID, _ := resourceBody.ID()
	proposalID := governance.ProposalID{HashDigest: protocol.HashBytes([]byte("proposal"))}
	state := chain.State{Research: map[string]chain.StoredResearch{researchID.String(): {ID: researchID, Body: researchBody, ProposalID: proposalID, AdmittedHeight: 5}}, Resources: map[string]chain.StoredResource{resourceID.String(): {ID: resourceID, Body: resourceBody, ProposalID: proposalID, AdmittedHeight: 6}}}
	if err := runtime.registryIndex.Rebuild(state); err != nil {
		t.Fatal(err)
	}
	if values, err := runtime.ResearchList(0, 10); err != nil || len(values.([]ResearchView)) != 1 {
		t.Fatal("research list failed")
	}
	if _, found, err := runtime.Research(researchID.String()); err != nil || !found {
		t.Fatal("research get failed")
	}
	if values, err := runtime.ResearchSearch("Alice", 0, 10); err != nil || len(values.([]ResearchView)) != 1 {
		t.Fatal("research search failed")
	}
	if _, found, err := runtime.Resource(resourceID.String()); err != nil || !found {
		t.Fatal("resource get failed")
	}
	if values, err := runtime.ResourceSearch("tool", 0, 10); err != nil || len(values.([]ResourceView)) != 1 {
		t.Fatal("resource search failed")
	}
}

func registryRegistryState(t testing.TB) chain.State {
	t.Helper()
	state := chain.Genesis(protocol.Alpha1NetworkID)
	keys := make([]ed25519.PrivateKey, 4)
	ids := make([]identity.IdentityID, 4)
	for position := range keys {
		keys[position] = ed25519.NewKeyFromSeed(bytes.Repeat([]byte{byte(0x70 + position)}, ed25519.SeedSize))
		body := identity.IdentityGenesisBody{SchemaVersion: identity.GenesisSchema, InitialPublicKey: identity.PublicFromPrivate(keys[position]), CreatedAt: protocol.ProtocolTimestamp(position + 1)}
		proof, err := identity.CreateIdentity(body, keys[position])
		if err != nil {
			t.Fatal(err)
		}
		ids[position] = proof.IdentityID
		tx := chain.Transaction{SchemaVersion: chain.TransactionSchema, NetworkID: protocol.Alpha1NetworkID, Type: chain.IdentityCreate, IdentityCreate: &proof}
		state, _, err = chain.Apply(state, tx)
		if err != nil {
			t.Fatal(err)
		}
	}
	validators := make([]governance.Validator, 4)
	for position := range validators {
		validators[position] = governance.Validator{Operator: ids[position%3], PublicKey: bytes.Repeat([]byte{byte(0xc0 + position)}, ed25519.PublicKeySize), Power: governance.ValidatorPower}
	}
	var err error
	state, err = chain.BootstrapGovernance(state, ids[:3], validators)
	if err != nil {
		t.Fatal(err)
	}
	state, err = chain.BootstrapRegistries(state)
	if err != nil {
		t.Fatal(err)
	}
	applyProposal := func(kind governance.ProposalKind, researchBody *research.EntryBody, resourceBody *resources.EntryBody, created protocol.ProtocolTimestamp, height int64) {
		body := governance.ProposalBody{SchemaVersion: governance.Schema, NetworkID: protocol.Alpha1NetworkID, Kind: kind, Proposer: ids[0], CreatedAt: created, Research: researchBody, Resource: resourceBody}
		proposal, createErr := governance.CreateProposal(body, keys[0])
		if createErr != nil {
			t.Fatal(createErr)
		}
		tx := chain.Transaction{SchemaVersion: chain.TransactionSchema, NetworkID: protocol.Alpha1NetworkID, Type: chain.GovernanceProposal, GovernanceProposal: &proposal}
		state, _, err = chain.ApplyWithContext(state, tx, chain.ExecutionContext{Height: height})
		if err != nil {
			t.Fatal(err)
		}
		for position := 0; position < 3; position++ {
			vote, voteErr := governance.CreateVote(governance.VoteBody{SchemaVersion: governance.Schema, NetworkID: protocol.Alpha1NetworkID, ProposalID: proposal.ProposalID, Voter: ids[position], Choice: governance.Yes}, keys[position])
			if voteErr != nil {
				t.Fatal(voteErr)
			}
			tx = chain.Transaction{SchemaVersion: chain.TransactionSchema, NetworkID: protocol.Alpha1NetworkID, Type: chain.GovernanceVote, GovernanceVote: &vote}
			state, _, err = chain.ApplyWithContext(state, tx, chain.ExecutionContext{Height: height + int64(position) + 1})
			if err != nil {
				t.Fatal(err)
			}
		}
		actionBody := governance.ActionBody{SchemaVersion: governance.Schema, NetworkID: protocol.Alpha1NetworkID, ProposalID: proposal.ProposalID, Submitter: ids[0]}
		finalize, finalErr := governance.CreateFinalize(actionBody, keys[0])
		if finalErr != nil {
			t.Fatal(finalErr)
		}
		tx = chain.Transaction{SchemaVersion: chain.TransactionSchema, NetworkID: protocol.Alpha1NetworkID, Type: chain.GovernanceFinalize, GovernanceFinalize: &finalize}
		state, _, err = chain.ApplyWithContext(state, tx, chain.ExecutionContext{Height: height + 10})
		if err != nil {
			t.Fatal(err)
		}
		execute, executeErr := governance.CreateExecute(actionBody, keys[0])
		if executeErr != nil {
			t.Fatal(executeErr)
		}
		tx = chain.Transaction{SchemaVersion: chain.TransactionSchema, NetworkID: protocol.Alpha1NetworkID, Type: chain.GovernanceExecute, GovernanceExecute: &execute}
		state, _, err = chain.ApplyWithContext(state, tx, chain.ExecutionContext{Height: height + 11})
		if err != nil {
			t.Fatal(err)
		}
	}
	researchBody := research.EntryBody{SchemaVersion: research.Schema, Title: "Synchronized Research", Summary: "Delivered through state sync.", Contributors: []research.Contributor{{DisplayName: "Alice"}}, ExternalIdentifiers: []research.ExternalIdentifier{}, ObjectRefs: []protocol.ObjectID{}, CanonicalRefs: []registry.CanonicalReference{}}
	applyProposal(governance.ResearchAdmission, &researchBody, nil, 100, 1)
	researchID, _ := researchBody.ID()
	resourceBody := resources.EntryBody{SchemaVersion: resources.Schema, Kind: resources.Dataset, Name: "Synchronized Resource", Summary: "Delivered through state sync.", Maintainers: []resources.Maintainer{{DisplayName: "Bob"}}, ObjectRefs: []protocol.ObjectID{}, CanonicalRefs: []registry.CanonicalReference{{Relation: registry.RelationRelated, TargetKind: registry.TargetResearch, TargetID: researchID.String()}}}
	applyProposal(governance.ResourceAdmission, nil, &resourceBody, 200, 20)
	return state
}

func TestOutboundNormalStateSyncsRegistriesAndServesAPI(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	state := registryRegistryState(t)
	serverCfg := testRuntimeConfig(t, "registry-sync-server", true, true)
	serverCfg.InitialState, serverCfg.InitialHeight = state, 31
	serverCfg.P2P.Roles = []p2p.Role{p2p.RoleNormal, p2p.RoleBootstrap}
	server, err := New(serverCfg)
	if err != nil {
		t.Fatal(err)
	}
	if err := server.Start(ctx); err != nil {
		t.Fatal(err)
	}
	defer server.Stop(context.Background())

	normalCfg := testRuntimeConfig(t, "registry-sync-normal", false, false)
	normalCfg.GenesisID = serverCfg.GenesisID
	normalCfg.P2P.NetworkFingerprint = serverCfg.GenesisID
	normalCfg.P2P.BootstrapAddresses = []string{server.P2P().FullAddresses()[0].String()}
	normal, err := New(normalCfg)
	if err != nil {
		t.Fatal(err)
	}
	if err := normal.Start(ctx); err != nil {
		t.Fatal(err)
	}
	defer normal.Stop(context.Background())
	if len(normal.P2P().ListenAddresses()) != 0 {
		t.Fatal("normal registry-sync node is not outbound-only")
	}
	expected, _ := state.Hash()
	waitRuntime(t, normal, func() bool { return runtimeHash(t, normal) == expected.String() })
	for _, path := range []string{"/v1/research", "/v1/resources", "/v1/research/search?q=Synchronized", "/v1/resources/search?q=Synchronized"} {
		response := apiRequest(t, http.MethodGet, "http://"+normal.APIAddress()+path, nil)
		if response.StatusCode != http.StatusOK || !bytes.Contains(response.Body, []byte("Synchronized")) {
			t.Fatalf("synced registry API %s = %d %s", path, response.StatusCode, response.Body)
		}
	}
}
