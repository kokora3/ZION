package node

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"path/filepath"
	"sort"
	"testing"
	"time"

	cmted25519 "github.com/cometbft/cometbft/crypto/ed25519"
	"github.com/kokora3/zion/internal/board"
	"github.com/kokora3/zion/internal/consensus"
	"github.com/kokora3/zion/internal/governance"
	"github.com/kokora3/zion/internal/identity"
	"github.com/kokora3/zion/internal/index"
	"github.com/kokora3/zion/internal/p2p"
	"github.com/kokora3/zion/internal/protocol"
	"github.com/libp2p/go-libp2p/core/peer"
)

// TestPhase13MultiNodeProductAcceptance composes the real four-validator
// runtime/CometBFT harness with bootstrap discovery, two NORMAL runtimes,
// state sync, object retrieval, Board propagation, restart, and state export.
// Detailed quorum-loss and registry-governance paths remain covered by the
// focused real-CometBFT tests in internal/consensus.
func TestPhase13MultiNodeProductAcceptance(t *testing.T) {
	if testing.Short() {
		t.Skip("final seven-node loopback acceptance test")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	privateKeys := make([]cmted25519.PrivKey, consensus.DefaultValidatorCount)
	validators := make([]consensus.Validator, consensus.DefaultValidatorCount)
	for i := range privateKeys {
		privateKeys[i] = cmted25519.GenPrivKeyFromSecret([]byte("TEST ONLY PUBLIC PHASE 13 VALIDATOR " + string(rune('A'+i))))
		validators[i] = consensus.Validator{Name: "phase13-validator-" + string(rune('a'+i)),
			PublicKey: privateKeys[i].PubKey().Bytes(), Power: consensus.DefaultValidatorPower}
	}
	state := registryRegistryState(t)
	authors := phase13RegistryAuthors(t, 3)
	state.Governance.Validators = make(map[string]governance.Validator, len(validators))
	for i, validator := range validators {
		entry := governance.Validator{Operator: authors[i%len(authors)].id,
			PublicKey: append([]byte(nil), validator.PublicKey...), Power: governance.ValidatorPower}
		state.Governance.Validators[governance.ValidatorKey(entry.PublicKey)] = entry
	}
	genesis, err := consensus.NewGenesisWithState(protocol.Alpha1NetworkID, validators, state)
	if err != nil {
		t.Fatal(err)
	}

	bootstrapCfg := testRuntimeConfig(t, "phase13-bootstrap", true, true)
	bootstrapCfg.GenesisID, bootstrapCfg.P2P.NetworkFingerprint = genesis.GenesisID, genesis.GenesisID
	bootstrapCfg.InitialState = state
	bootstrapCfg.P2P.Roles = []p2p.Role{p2p.RoleNormal, p2p.RoleBootstrap}
	bootstrap, err := New(bootstrapCfg)
	if err != nil {
		t.Fatal(err)
	}
	if err := bootstrap.Start(ctx); err != nil {
		t.Fatal(err)
	}
	defer bootstrap.Stop(context.Background())

	var validatorsRuntime []validatorRuntimeHarness
	for attempt := 1; attempt <= runtimePortBindAttempts; attempt++ {
		candidate, ports := newValidatorRuntimeHarness(t, genesis, privateKeys, validators)
		started, startErr := startValidatorRuntimeHarness(ctx, candidate, ports)
		if startErr == nil {
			validatorsRuntime = candidate
			break
		}
		stopValidatorRuntimeHarness(candidate, started)
		if !isRuntimeAddressInUse(startErr) || attempt == runtimePortBindAttempts {
			t.Fatal(startErr)
		}
	}
	defer stopValidatorRuntimeHarness(validatorsRuntime, len(validatorsRuntime))
	bootstrapAddress := bootstrap.P2P().FullAddresses()[0].String()
	for i := range validatorsRuntime {
		if err := validatorsRuntime[i].runtime.P2P().Dial(ctx, bootstrapAddress, p2p.SourceBootstrap); err != nil {
			t.Fatalf("validator %d general-P2P bootstrap: %v", i, err)
		}
		if i > 0 {
			address := validatorsRuntime[0].runtime.P2P().FullAddresses()[0].String()
			if err := validatorsRuntime[i].runtime.P2P().Dial(ctx, address, p2p.SourceManual); err != nil {
				t.Fatalf("validator %d direct general-P2P: %v", i, err)
			}
		}
	}

	raw := fixtureTransaction(t)
	wrapper, _ := json.Marshal(map[string]string{"encoding": "base64", "transaction": base64.StdEncoding.EncodeToString(raw)})
	response := apiRequest(t, http.MethodPost, "http://"+validatorsRuntime[0].runtime.APIAddress()+"/v1/transactions", wrapper)
	if response.StatusCode != http.StatusAccepted {
		t.Fatalf("consensus submission = %d %s", response.StatusCode, response.Body)
	}
	waitAllRuntimeHeight(t, validatorsRuntime, 1, 20*time.Second)
	canonicalHash := runtimeHash(t, validatorsRuntime[0].runtime)
	for i := range validatorsRuntime {
		if runtimeHash(t, validatorsRuntime[i].runtime) != canonicalHash {
			t.Fatalf("validator %d StateHash differs", i)
		}
		status, statusErr := validatorsRuntime[i].app.Status()
		if statusErr != nil || "zion:state:sha256:"+bytesToHex(status.AppHash) != canonicalHash {
			t.Fatalf("validator %d AppHash differs: %v", i, statusErr)
		}
	}

	n1Cfg := testRuntimeConfig(t, "phase13-normal-1", false, true)
	n1Cfg.GenesisID, n1Cfg.P2P.NetworkFingerprint = genesis.GenesisID, genesis.GenesisID
	n1Cfg.P2P.BootstrapAddresses = []string{bootstrapAddress}
	n1Cfg.P2P.Limits.TargetOutboundPeers = 4
	n2Cfg := testRuntimeConfig(t, "phase13-normal-2", false, false)
	n2Cfg.GenesisID, n2Cfg.P2P.NetworkFingerprint = genesis.GenesisID, genesis.GenesisID
	n2Cfg.P2P.BootstrapAddresses = []string{bootstrapAddress}
	n2Cfg.P2P.Limits.TargetOutboundPeers = 4
	n2Cfg.ObjectFetch.MaxPeers = 16
	n1, err := New(n1Cfg)
	if err != nil {
		t.Fatal(err)
	}
	n2, err := New(n2Cfg)
	if err != nil {
		t.Fatal(err)
	}
	if err := n1.Start(ctx); err != nil {
		t.Fatal(err)
	}
	defer n1.Stop(context.Background())
	if err := n2.Start(ctx); err != nil {
		t.Fatal(err)
	}
	defer n2.Stop(context.Background())
	if len(n2.P2P().ListenAddresses()) != 0 {
		t.Fatal("N2 opened an inbound P2P listener")
	}
	for _, normal := range []*Runtime{n1, n2} {
		connectPEXRecord(t, ctx, normal, bootstrap.P2P().PeerID(), validatorsRuntime[0].runtime.P2P().PeerID().String())
		connectPEXRecord(t, ctx, normal, bootstrap.P2P().PeerID(), validatorsRuntime[1].runtime.P2P().PeerID().String())
		if err := normal.Synchronize(ctx); err != nil {
			t.Fatal("normal state sync:", err)
		}
	}
	waitRuntime(t, n1, func() bool { return runtimeHash(t, n1) == canonicalHash && len(n1.P2P().UsablePeers()) >= 2 })
	waitRuntime(t, n2, func() bool { return runtimeHash(t, n2) == canonicalHash && len(n2.P2P().UsablePeers()) >= 2 })

	wrongCfg := testRuntimeConfig(t, "phase13-wrong-genesis", true, false)
	wrongCfg.GenesisID = protocol.HashBytes([]byte("incompatible-phase13-genesis"))
	wrongCfg.P2P.NetworkFingerprint = wrongCfg.GenesisID
	wrong, err := New(wrongCfg)
	if err != nil {
		t.Fatal(err)
	}
	if err := wrong.Start(ctx); err != nil {
		t.Fatal(err)
	}
	if err := wrong.P2P().Dial(ctx, bootstrapAddress, p2p.SourceManual); err == nil || wrong.P2P().IsUsable(bootstrap.P2P().PeerID()) {
		t.Fatal("wrong-genesis node joined the usable peer set")
	}
	_ = wrong.Stop(context.Background())

	contentID := storeBoardContent(t, validatorsRuntime[0].runtime, "Phase 13", "final product acceptance", 1730000000000)
	postRaw := signedBoardObject(t, authors[0], board.KindPost, 1730000000000, contentID, "", nil)
	post := submitBoardAPI(t, validatorsRuntime[0].runtime, postRaw)
	postID := post["post_id"].(string)
	waitRuntime(t, validatorsRuntime[1].runtime, func() bool { _, found, _ := validatorsRuntime[1].runtime.BoardPost(postID); return found })
	waitRuntime(t, n1, func() bool { _, found, _ := n1.BoardPost(postID); return found })
	waitRuntime(t, n2, func() bool { _, found, _ := n2.BoardPost(postID); return found })
	replyContent := storeBoardContent(t, validatorsRuntime[1].runtime, "", "independent member reply", 1730000000001)
	replyRaw := signedBoardObject(t, authors[1], board.KindReply, 1730000000001, replyContent, postID, nil)
	submitBoardAPI(t, validatorsRuntime[1].runtime, replyRaw)
	waitRuntime(t, n2, func() bool {
		values, err := n2.BoardReplies(postID, 0, 10)
		return err == nil && len(values.([]index.BoardEntry)) == 1
	})

	registryIDs := make([]string, 0, len(state.Research)+len(state.Resources))
	for id := range state.Research {
		registryIDs = append(registryIDs, id)
	}
	for id := range state.Resources {
		registryIDs = append(registryIDs, id)
	}
	sort.Strings(registryIDs)
	if len(registryIDs) != 2 {
		t.Fatal("expected admitted Research and Resource entries")
	}
	referenceContent := storeBoardContent(t, validatorsRuntime[0].runtime, "Canonical references", "Research and Resource", 1730000000002)
	references := []board.Reference{
		{Relation: "zion.community/discusses/v1", TargetKind: "RESEARCH", TargetID: firstIDWithPrefix(registryIDs, "zion:research:")},
		{Relation: "zion.community/discusses/v1", TargetKind: "RESOURCE", TargetID: firstIDWithPrefix(registryIDs, "zion:resource:")},
	}
	referenceRaw := signedBoardObject(t, authors[0], board.KindPost, 1730000000002, referenceContent, "", references)
	referencePost := submitBoardAPI(t, validatorsRuntime[0].runtime, referenceRaw)
	waitRuntime(t, n1, func() bool { _, found, _ := n1.BoardPost(referencePost["post_id"].(string)); return found })

	standalone, err := board.BuildContentObject(board.Content{SchemaVersion: board.ContentSchema, Format: board.FormatText,
		Body: "explicit object retrieval"}, 1730000000003)
	if err != nil {
		t.Fatal(err)
	}
	standaloneID, _, err := validatorsRuntime[0].runtime.PutObject(ctx, standalone)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := n2.FetchObject(ctx, standaloneID); err != nil {
		t.Fatal("outbound-only object fetch:", err)
	}
	if _, err := n2.GetObject(ctx, standaloneID); err != nil {
		t.Fatal("fetched object was not stored:", err)
	}

	peerIDBefore := n2.P2P().PeerID()
	if err := bootstrap.Stop(context.Background()); err != nil {
		t.Fatal(err)
	}
	if !n1.P2P().IsUsable(validatorsRuntime[0].runtime.P2P().PeerID()) || !n2.P2P().IsUsable(validatorsRuntime[0].runtime.P2P().PeerID()) {
		t.Fatal("bootstrap loss removed established direct peers")
	}
	if err := n2.Stop(context.Background()); err != nil {
		t.Fatal(err)
	}
	n2Cfg.P2P.BootstrapAddresses = nil
	n2Restarted, err := New(n2Cfg)
	if err != nil {
		t.Fatal(err)
	}
	if err := n2Restarted.Start(ctx); err != nil {
		t.Fatal(err)
	}
	defer n2Restarted.Stop(context.Background())
	if n2Restarted.P2P().PeerID() != peerIDBefore {
		t.Fatal("outbound-only PeerID changed across restart")
	}
	waitRuntime(t, n2Restarted, func() bool {
		return runtimeHash(t, n2Restarted) == canonicalHash && n2Restarted.P2P().IsUsable(validatorsRuntime[0].runtime.P2P().PeerID())
	})

	exportPath := filepath.Join(t.TempDir(), "canonical-export.json")
	exported, err := ExportState(n1Cfg.StatePath, exportPath, n1Cfg.NetworkID, n1Cfg.GenesisID)
	if err != nil {
		t.Fatal(err)
	}
	importPath := filepath.Join(t.TempDir(), "fresh-normal", "application.snapshot")
	imported, err := ImportState(exportPath, importPath, n1Cfg.NetworkID, n1Cfg.GenesisID)
	if err != nil || imported.StateHash != exported.StateHash || imported.StateHash != canonicalHash {
		t.Fatalf("final export/import mismatch: export=%+v import=%+v err=%v", exported, imported, err)
	}
	for _, runtime := range append([]*Runtime{n1, n2Restarted}, validatorRuntimePointers(validatorsRuntime)...) {
		if runtimeHash(t, runtime) != canonicalHash {
			t.Fatal("off-chain product operation changed canonical StateHash")
		}
	}
}

func phase13RegistryAuthors(t testing.TB, count int) []boardAuthor {
	t.Helper()
	authors := make([]boardAuthor, count)
	for i := range authors {
		key := ed25519.NewKeyFromSeed(bytes.Repeat([]byte{byte(0x70 + i)}, ed25519.SeedSize))
		publicKey := identity.PublicFromPrivate(key)
		body := identity.IdentityGenesisBody{SchemaVersion: identity.GenesisSchema, InitialPublicKey: publicKey,
			CreatedAt: protocol.ProtocolTimestamp(i + 1)}
		id, err := identity.DeriveIdentityID(body)
		if err != nil {
			t.Fatal(err)
		}
		keyID, err := identity.DeriveKeyID(publicKey)
		if err != nil {
			t.Fatal(err)
		}
		authors[i] = boardAuthor{key: key, id: id, keyID: keyID}
	}
	return authors
}

func waitAllRuntimeHeight(t testing.TB, nodes []validatorRuntimeHarness, height int64, timeout time.Duration) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		ready := true
		for i := range nodes {
			nodes[i].runtime.mu.RLock()
			current := nodes[i].runtime.height
			nodes[i].runtime.mu.RUnlock()
			if current < height {
				ready = false
				break
			}
		}
		if ready {
			return
		}
		time.Sleep(25 * time.Millisecond)
	}
	t.Fatal("validator runtime height timeout")
}

func firstIDWithPrefix(values []string, prefix string) string {
	for _, value := range values {
		if len(value) >= len(prefix) && value[:len(prefix)] == prefix {
			return value
		}
	}
	return ""
}

func validatorRuntimePointers(values []validatorRuntimeHarness) []*Runtime {
	result := make([]*Runtime, len(values))
	for i := range values {
		result[i] = values[i].runtime
	}
	return result
}

func connectPEXRecord(t testing.TB, ctx context.Context, normal *Runtime, bootstrapID peer.ID, targetID string) {
	t.Helper()
	waitRuntime(t, normal, func() bool { return normal.P2P().IsUsable(bootstrapID) })
	records, err := normal.P2P().RequestPeers(ctx, bootstrapID)
	if err != nil {
		t.Fatal("PEX request:", err)
	}
	for _, record := range records {
		if record.PeerID != targetID || len(record.Addresses) == 0 {
			continue
		}
		address := record.Addresses[0] + "/p2p/" + record.PeerID
		if err := normal.P2P().Dial(ctx, address, p2p.SourcePEX); err != nil {
			t.Fatal("dial PEX candidate:", err)
		}
		return
	}
	t.Fatal("bootstrap PEX did not advertise the target validator")
}
