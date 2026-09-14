// Package main runs a real, loopback-only ZION Runtime for browser acceptance
// tests. Every deterministic key is TEST ONLY — PUBLIC FIXTURE — NEVER USE IN PRODUCTION.
package main

import (
	"context"
	"crypto/ed25519"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"os"
	"os/signal"
	"sort"
	"sync"
	"syscall"
	"time"

	"github.com/kokora3/zion/internal/board"
	"github.com/kokora3/zion/internal/chain"
	"github.com/kokora3/zion/internal/consensus"
	"github.com/kokora3/zion/internal/governance"
	"github.com/kokora3/zion/internal/identity"
	"github.com/kokora3/zion/internal/membership"
	"github.com/kokora3/zion/internal/node"
	"github.com/kokora3/zion/internal/objects"
	"github.com/kokora3/zion/internal/protocol"
	"github.com/kokora3/zion/internal/registry"
	"github.com/kokora3/zion/internal/research"
	"github.com/kokora3/zion/internal/resources"
)

type fixtureConsensus struct {
	mu       sync.Mutex
	active   bool
	state    chain.State
	height   int64
	observer func(chain.State, int64, [][]byte) error
}

func (c *fixtureConsensus) Start(context.Context) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.active = true
	return nil
}

func (c *fixtureConsensus) Stop(context.Context) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.active = false
	return nil
}

func (c *fixtureConsensus) Active() bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.active
}

func (c *fixtureConsensus) SetCommitObserver(observer func(chain.State, int64, [][]byte) error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.observer = observer
}

func (c *fixtureConsensus) Submit(_ context.Context, raw []byte) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	tx, err := consensus.DecodeTransaction(raw, protocol.Alpha1NetworkID)
	if err != nil {
		return err
	}
	nextHeight := c.height + 1
	next, _, err := chain.ApplyWithContext(c.state, tx, chain.ExecutionContext{Height: nextHeight})
	if err != nil {
		return err
	}
	// Three YES votes complete the fixture electorate. Advancing over the
	// intervening empty blocks keeps the real Phase 6 voting-height rule intact.
	if tx.Type == chain.GovernanceVote {
		record := next.Governance.Proposals[tx.GovernanceVote.Body.ProposalID.String()]
		if len(record.Votes) == len(record.Electorate) && nextHeight < record.EndHeight-1 {
			nextHeight = record.EndHeight - 1
		}
	}
	if c.observer == nil {
		return fmt.Errorf("fixture consensus commit observer is not installed")
	}
	if err := c.observer(next, nextHeight, [][]byte{raw}); err != nil {
		return err
	}
	c.state, c.height = next, nextHeight
	return nil
}

type fixture struct {
	Transaction        string   `json:"transaction"`
	BoardEvent         string   `json:"board_event"`
	PostID             string   `json:"post_id"`
	ResearchID         string   `json:"research_id"`
	ResourceID         string   `json:"resource_id"`
	MissingObjectID    string   `json:"missing_object_id"`
	ResearchTxs        []string `json:"research_transactions"`
	ResourceTxs        []string `json:"resource_transactions"`
	AdmittedResearchID string   `json:"admitted_research_id"`
	AdmittedResourceID string   `json:"admitted_resource_id"`
}
type identityFixture struct {
	key   ed25519.PrivateKey
	id    identity.IdentityID
	keyID identity.KeyID
}

func key(marker byte) ed25519.PrivateKey {
	seed := make([]byte, ed25519.SeedSize)
	for index := range seed {
		seed[index] = marker + byte(index)
	}
	return ed25519.NewKeyFromSeed(seed)
}

func addIdentity(state *chain.State, marker byte, created protocol.ProtocolTimestamp) identityFixture {
	private := key(marker)
	public := identity.PublicFromPrivate(private)
	body := identity.IdentityGenesisBody{SchemaVersion: identity.GenesisSchema, InitialPublicKey: public, CreatedAt: created}
	id, _ := identity.DeriveIdentityID(body)
	keyID, _ := identity.DeriveKeyID(public)
	state.Identities[id.String()] = chain.StoredIdentity{ID: id, Keys: map[string]identity.PublicKey{keyID.String(): public}, Active: map[string]bool{keyID.String(): true}}
	state.Memberships[id.String()] = membership.Pending
	return identityFixture{private, id, keyID}
}

func votes(members []identityFixture) map[string]governance.VoteRecord {
	result := make(map[string]governance.VoteRecord, len(members))
	for _, member := range members {
		result[member.id.String()] = governance.VoteRecord{Voter: member.id, Choice: governance.Yes, KeyID: member.keyID}
	}
	return result
}
func proposal(body governance.ProposalBody, status governance.ProposalStatus, members []identityFixture, start int64) governance.ProposalRecord {
	id, _ := body.ID()
	electorate := make([]identity.IdentityID, len(members))
	for index := range members {
		electorate[index] = members[index].id
	}
	recordVotes := map[string]governance.VoteRecord{}
	if status != governance.Open {
		recordVotes = votes(members)
	}
	return governance.ProposalRecord{ID: id, Body: body, Status: status, StartHeight: start, EndHeight: start + 10, Electorate: electorate, Votes: recordVotes}
}

func buildState() (chain.State, []identityFixture, protocol.ObjectID, protocol.ObjectID, research.ID, resources.ID) {
	state := chain.Genesis(protocol.Alpha1NetworkID)
	members := []identityFixture{addIdentity(&state, 0x10, 1700000000000), addIdentity(&state, 0x30, 1700000000001), addIdentity(&state, 0x50, 1700000000002)}
	validators := make([]governance.Validator, 3)
	for index, member := range members {
		validators[index] = governance.Validator{Operator: member.id, PublicKey: identity.PublicFromPrivate(key(byte(0x90 + index*8))).Key, Power: governance.ValidatorPower}
	}
	state, _ = chain.BootstrapGovernance(state, []identity.IdentityID{members[0].id, members[1].id, members[2].id}, validators)
	state, _ = chain.BootstrapRegistries(state)
	present := objects.Object{Core: protocol.UnsignedObjectCore{ObjectType: "zion.test/document/v1", SchemaVersion: 1, CreatedAt: 1700000000100, Visibility: protocol.VisibilityPublic, Metadata: map[string]string{}}, Payload: []byte("ZION browser acceptance object")}
	present.Core.ContentHash = protocol.NewContentHash(present.Payload)
	present.Core.SizeBytes = uint64(len(present.Payload))
	presentID, _ := present.ObjectID()
	missing := present
	missing.Payload = []byte("missing peer object")
	missing.Core.CreatedAt++
	missing.Core.ContentHash = protocol.NewContentHash(missing.Payload)
	missing.Core.SizeBytes = uint64(len(missing.Payload))
	missingID, _ := missing.ObjectID()
	objectRefs := []protocol.ObjectID{presentID, missingID}
	sort.Slice(objectRefs, func(i, j int) bool { return objectRefs[i].String() < objectRefs[j].String() })
	researchBody := research.EntryBody{SchemaVersion: research.Schema, Title: "Deterministic AI security evaluation", Summary: "A canonical alpha entry used by the real-node browser acceptance slice.", Contributors: []research.Contributor{{DisplayName: "ZION Research Collective"}}, ExternalIdentifiers: []research.ExternalIdentifier{{Scheme: research.DOI, Value: "10.0000/zion.alpha"}}, ExternalURI: "https://example.org/zion-research", ObjectRefs: objectRefs, CanonicalRefs: []registry.CanonicalReference{}}
	researchProposalBody := governance.ProposalBody{SchemaVersion: governance.Schema, NetworkID: protocol.Alpha1NetworkID, Kind: governance.ResearchAdmission, Proposer: members[0].id, CreatedAt: 1700000000200, Research: &researchBody}
	researchRecord := proposal(researchProposalBody, governance.Executed, members, 1)
	state.Governance.Proposals[researchRecord.ID.String()] = researchRecord
	researchID, _ := researchBody.ID()
	state.Research[researchID.String()] = chain.StoredResearch{ID: researchID, Body: researchBody, ProposalID: researchRecord.ID, AdmittedHeight: 14}
	resourceBody := resources.EntryBody{SchemaVersion: resources.Schema, Kind: resources.Tool, Name: "Bounded Analysis Toolkit", Summary: "Governance-admitted metadata for a tool that the web client never executes.", ExternalURI: "https://example.org/zion-tool", License: "Apache-2.0", Maintainers: []resources.Maintainer{{DisplayName: "ZION Maintainers"}}, ObjectRefs: []protocol.ObjectID{presentID}, CanonicalRefs: []registry.CanonicalReference{{Relation: registry.RelationImplements, TargetKind: registry.TargetResearch, TargetID: researchID.String()}}}
	resourceProposalBody := governance.ProposalBody{SchemaVersion: governance.Schema, NetworkID: protocol.Alpha1NetworkID, Kind: governance.ResourceAdmission, Proposer: members[1].id, CreatedAt: 1700000000300, Resource: &resourceBody}
	resourceRecord := proposal(resourceProposalBody, governance.Executed, members, 20)
	state.Governance.Proposals[resourceRecord.ID.String()] = resourceRecord
	resourceID, _ := resourceBody.ID()
	state.Resources[resourceID.String()] = chain.StoredResource{ID: resourceID, Body: resourceBody, ProposalID: resourceRecord.ID, AdmittedHeight: 33}
	upgrade := governance.ProtocolUpgradePayload{TargetVersion: "0.2", ActivationHeight: 500, MigrationIdentifier: "future-alpha-migration"}
	approvedBody := governance.ProposalBody{SchemaVersion: governance.Schema, NetworkID: protocol.Alpha1NetworkID, Kind: governance.ProtocolUpgrade, Proposer: members[2].id, CreatedAt: 1700000000400, Upgrade: &upgrade}
	approved := proposal(approvedBody, governance.Approved, members, 40)
	state.Governance.Proposals[approved.ID.String()] = approved
	membershipPayload := governance.MembershipChangePayload{Target: members[0].id, Expected: membership.Active, Requested: membership.Suspended}
	openBody := governance.ProposalBody{SchemaVersion: governance.Schema, NetworkID: protocol.Alpha1NetworkID, Kind: governance.MembershipChange, Proposer: members[0].id, CreatedAt: 1700000000500, Membership: &membershipPayload}
	open := proposal(openBody, governance.Open, members, 60)
	state.Governance.Proposals[open.ID.String()] = open
	return state, members, presentID, missingID, researchID, resourceID
}

func boardObjects(author identityFixture, researchID research.ID, resourceID resources.ID) (objects.Object, objects.Object, objects.Object, objects.Object) {
	postContent, _ := board.BuildContentObject(board.Content{SchemaVersion: board.ContentSchema, Format: board.FormatMarkdown, Title: "Welcome to the ZION alpha Board", Body: "Signed community text stays plain and safe in the browser."}, 1700000000600)
	postContentID, _ := postContent.ObjectID()
	postEvent, _ := board.SignEvent(board.EventBody{SchemaVersion: board.EventSchema, NetworkID: protocol.Alpha1NetworkID, Kind: board.KindPost, AuthorIdentity: author.id, AuthorKeyID: author.keyID, CreatedAt: 1700000000600, ContentObject: postContentID.String(), References: []board.Reference{{Relation: "zion.community/discusses/v1", TargetKind: "RESEARCH", TargetID: researchID.String()}, {Relation: "zion.community/discusses/v1", TargetKind: "RESOURCE", TargetID: resourceID.String()}}}, author.key)
	postObject, _ := board.BuildEventObject(postEvent)
	postID, _ := postObject.ObjectID()
	replyContent, _ := board.BuildContentObject(board.Content{SchemaVersion: board.ContentSchema, Format: board.FormatText, Body: "A bounded signed reply from another ACTIVE member."}, 1700000000700)
	replyContentID, _ := replyContent.ObjectID()
	replyEvent, _ := board.SignEvent(board.EventBody{SchemaVersion: board.EventSchema, NetworkID: protocol.Alpha1NetworkID, Kind: board.KindReply, AuthorIdentity: author.id, AuthorKeyID: author.keyID, CreatedAt: 1700000000700, ParentPost: postID.String(), ContentObject: replyContentID.String(), References: []board.Reference{}}, author.key)
	replyObject, _ := board.BuildEventObject(replyEvent)
	return postContent, postObject, replyContent, replyObject
}

func signedTransaction() string {
	private := key(0x70)
	body := identity.IdentityGenesisBody{SchemaVersion: identity.GenesisSchema, InitialPublicKey: identity.PublicFromPrivate(private), CreatedAt: 1700000000800}
	proof, _ := identity.CreateIdentity(body, private)
	tx := chain.Transaction{SchemaVersion: chain.TransactionSchema, NetworkID: protocol.Alpha1NetworkID, Type: chain.IdentityCreate, IdentityCreate: &proof}
	raw, _ := tx.CanonicalBytes()
	return base64.StdEncoding.EncodeToString(raw)
}

func encodeTransaction(tx chain.Transaction) string {
	raw, err := tx.CanonicalBytes()
	if err != nil {
		panic(err)
	}
	return base64.StdEncoding.EncodeToString(raw)
}

func signedAdmissionTransactions(kind governance.ProposalKind, researchBody *research.EntryBody, resourceBody *resources.EntryBody, members []identityFixture, created protocol.ProtocolTimestamp) []string {
	body := governance.ProposalBody{SchemaVersion: governance.Schema, NetworkID: protocol.Alpha1NetworkID, Kind: kind, Proposer: members[0].id, CreatedAt: created, Research: researchBody, Resource: resourceBody}
	proposal, err := governance.CreateProposal(body, members[0].key)
	if err != nil {
		panic(err)
	}
	transactions := []chain.Transaction{{SchemaVersion: chain.TransactionSchema, NetworkID: protocol.Alpha1NetworkID, Type: chain.GovernanceProposal, GovernanceProposal: &proposal}}
	for _, member := range members {
		vote, voteErr := governance.CreateVote(governance.VoteBody{SchemaVersion: governance.Schema, NetworkID: protocol.Alpha1NetworkID, ProposalID: proposal.ProposalID, Voter: member.id, Choice: governance.Yes}, member.key)
		if voteErr != nil {
			panic(voteErr)
		}
		transactions = append(transactions, chain.Transaction{SchemaVersion: chain.TransactionSchema, NetworkID: protocol.Alpha1NetworkID, Type: chain.GovernanceVote, GovernanceVote: &vote})
	}
	actionBody := governance.ActionBody{SchemaVersion: governance.Schema, NetworkID: protocol.Alpha1NetworkID, ProposalID: proposal.ProposalID, Submitter: members[0].id}
	finalize, err := governance.CreateFinalize(actionBody, members[0].key)
	if err != nil {
		panic(err)
	}
	execute, err := governance.CreateExecute(actionBody, members[0].key)
	if err != nil {
		panic(err)
	}
	transactions = append(transactions,
		chain.Transaction{SchemaVersion: chain.TransactionSchema, NetworkID: protocol.Alpha1NetworkID, Type: chain.GovernanceFinalize, GovernanceFinalize: &finalize},
		chain.Transaction{SchemaVersion: chain.TransactionSchema, NetworkID: protocol.Alpha1NetworkID, Type: chain.GovernanceExecute, GovernanceExecute: &execute},
	)
	encoded := make([]string, len(transactions))
	for index := range transactions {
		encoded[index] = encodeTransaction(transactions[index])
	}
	return encoded
}

func main() {
	state, members, presentID, missingID, researchID, resourceID := buildState()
	admittedResearch := research.EntryBody{SchemaVersion: research.Schema, Title: "Web-imported canonical research", Summary: "Admitted by deterministic signed proposal and vote transactions submitted through ZION Web.", Contributors: []research.Contributor{{DisplayName: "Browser acceptance member"}}, ExternalIdentifiers: []research.ExternalIdentifier{}, ObjectRefs: []protocol.ObjectID{}, CanonicalRefs: []registry.CanonicalReference{}}
	admittedResearchID, _ := admittedResearch.ID()
	admittedResource := resources.EntryBody{SchemaVersion: resources.Schema, Kind: resources.Dataset, Name: "Web-imported canonical resource", Summary: "Admitted through the same canonical governance path.", License: "CC0-1.0", Maintainers: []resources.Maintainer{{DisplayName: "Browser acceptance member"}}, ObjectRefs: []protocol.ObjectID{}, CanonicalRefs: []registry.CanonicalReference{{Relation: registry.RelationRelated, TargetKind: registry.TargetResearch, TargetID: admittedResearchID.String()}}}
	admittedResourceID, _ := admittedResource.ID()
	researchTransactions := signedAdmissionTransactions(governance.ResearchAdmission, &admittedResearch, nil, members, 1700000000900)
	resourceTransactions := signedAdmissionTransactions(governance.ResourceAdmission, nil, &admittedResource, members, 1700000001000)
	postContent, postObject, replyContent, replyObject := boardObjects(members[0], researchID, resourceID)
	postRaw, _ := postObject.CanonicalBytes()
	postID, _ := postObject.ObjectID()
	if len(os.Args) > 1 && os.Args[1] == "--fixture" {
		_ = json.NewEncoder(os.Stdout).Encode(fixture{Transaction: signedTransaction(), BoardEvent: base64.StdEncoding.EncodeToString(postRaw), PostID: postID.String(), ResearchID: researchID.String(), ResourceID: resourceID.String(), MissingObjectID: missingID.String(), ResearchTxs: researchTransactions, ResourceTxs: resourceTransactions, AdmittedResearchID: admittedResearchID.String(), AdmittedResourceID: admittedResourceID.String()})
		return
	}
	dataDir, err := os.MkdirTemp("", "zion-web-e2e-")
	if err != nil {
		panic(err)
	}
	defer os.RemoveAll(dataDir)
	fingerprint := protocol.HashBytes([]byte("ZION PHASE 12 REAL NODE TEST GENESIS"))
	cfg := node.DefaultConfig(dataDir, fingerprint, state)
	cfg.InitialHeight = 75
	cfg.FreshStateIsAuthoritative = true
	cfg.P2P.ListenAddresses = []string{"/ip4/127.0.0.1/udp/0/quic-v1"}
	cfg.API.Listen = "127.0.0.1:42001"
	cfg.API.AllowedOrigins = []string{"http://127.0.0.1:3000"}
	cfg.Consensus = &fixtureConsensus{state: state, height: cfg.InitialHeight}
	runtime, err := node.New(cfg)
	if err != nil {
		panic(err)
	}
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()
	if err = runtime.Start(ctx); err != nil {
		panic(err)
	}
	present := objects.Object{Core: protocol.UnsignedObjectCore{ObjectType: "zion.test/document/v1", SchemaVersion: 1, CreatedAt: 1700000000100, Visibility: protocol.VisibilityPublic, Metadata: map[string]string{}}, Payload: []byte("ZION browser acceptance object")}
	present.Core.ContentHash = protocol.NewContentHash(present.Payload)
	present.Core.SizeBytes = uint64(len(present.Payload))
	if actual, _, putErr := runtime.PutObject(ctx, present); putErr != nil || actual.String() != presentID.String() {
		panic("store fixture object")
	}
	for _, object := range []objects.Object{postContent, replyContent} {
		if _, _, err = runtime.PutObject(ctx, object); err != nil {
			panic(err)
		}
	}
	for _, object := range []objects.Object{postObject, replyObject} {
		raw, _ := object.CanonicalBytes()
		if _, err = runtime.SubmitBoardEvent(ctx, raw); err != nil {
			panic(err)
		}
	}
	fmt.Println("ZION_WEB_E2E_READY")
	<-ctx.Done()
	stop, done := context.WithTimeout(context.Background(), 5*time.Second)
	defer done()
	_ = runtime.Stop(stop)
}
