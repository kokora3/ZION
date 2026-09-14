package node

import (
	"context"
	"fmt"
	"net"
	"testing"

	cmted25519 "github.com/cometbft/cometbft/crypto/ed25519"
	"github.com/kokora3/zion/internal/consensus"
	"github.com/kokora3/zion/internal/governance"
	"github.com/kokora3/zion/internal/protocol"
)

func TestAPIListenConflictFailsCleanlyWithoutCanonicalCorruption(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	cfg := testRuntimeConfig(t, "phase13-port-conflict", true, false)
	cfg.API.Listen = listener.Addr().String()
	before, _ := cfg.InitialState.Hash()
	runtime, err := New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if err := runtime.Start(context.Background()); err == nil {
		t.Fatal("runtime started despite occupied API port")
	}
	after := runtimeHash(t, runtime)
	if after != before.String() || runtime.P2P() != nil {
		t.Fatal("failed startup changed canonical state or left P2P running")
	}
	store := mustStateStore(t, cfg.StatePath, cfg.NetworkID, cfg.GenesisID)
	persisted, err := store.Load()
	if err != nil {
		t.Fatal("authoritative state was not left recoverable:", err)
	}
	hash, _ := persisted.State.Hash()
	if hash.String() != before.String() {
		t.Fatal("port conflict corrupted durable canonical state")
	}
}

func TestP2PListenConflictFailsCleanlyWithoutCanonicalCorruption(t *testing.T) {
	listener, err := net.ListenPacket("udp4", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	port := listener.LocalAddr().(*net.UDPAddr).Port
	cfg := testRuntimeConfig(t, "phase13-p2p-port-conflict", true, true)
	cfg.P2P.ListenAddresses = []string{fmt.Sprintf("/ip4/127.0.0.1/udp/%d/quic-v1", port)}
	before, _ := cfg.InitialState.Hash()
	runtime, err := New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if err := runtime.Start(context.Background()); err == nil {
		_ = runtime.Stop(context.Background())
		t.Fatal("runtime started despite occupied P2P port")
	}
	if runtimeHash(t, runtime) != before.String() || runtime.P2P() != nil {
		t.Fatal("P2P port conflict changed canonical state or left P2P running")
	}
}

func TestCometBFTListenConflictFailsCleanlyWithoutCanonicalCorruption(t *testing.T) {
	privateKeys := make([]cmted25519.PrivKey, consensus.DefaultValidatorCount)
	validators := make([]consensus.Validator, consensus.DefaultValidatorCount)
	state := registryRegistryState(t)
	authors := phase13RegistryAuthors(t, consensus.DefaultValidatorCount)
	state.Governance.Validators = map[string]governance.Validator{}
	for i := range privateKeys {
		privateKeys[i] = cmted25519.GenPrivKeyFromSecret([]byte(fmt.Sprintf("TEST ONLY PUBLIC PHASE 13 PORT CONFLICT VALIDATOR %d", i)))
		validators[i] = consensus.Validator{Name: fmt.Sprintf("phase13-conflict-validator-%d", i),
			PublicKey: privateKeys[i].PubKey().Bytes(), Power: consensus.DefaultValidatorPower}
		entry := governance.Validator{Operator: authors[i].id, PublicKey: append([]byte(nil), validators[i].PublicKey...), Power: governance.ValidatorPower}
		state.Governance.Validators[governance.ValidatorKey(entry.PublicKey)] = entry
	}
	genesis, err := consensus.NewGenesisWithState(protocol.Alpha1NetworkID, validators, state)
	if err != nil {
		t.Fatal(err)
	}
	nodes, reservations := newValidatorRuntimeHarness(t, genesis, privateKeys, validators)
	defer nodes[0].runtime.Stop(context.Background())
	before, _ := state.Hash()
	// Keep the harness reservation open so CometBFT cannot bind its configured
	// consensus transport address.
	if reservations[0].listener == nil {
		t.Fatal("missing consensus port reservation")
	}
	if err := nodes[0].runtime.Start(context.Background()); err == nil {
		t.Fatal("runtime started despite occupied CometBFT P2P port")
	}
	if runtimeHash(t, nodes[0].runtime) != before.String() || nodes[0].runtime.P2P() != nil {
		t.Fatal("CometBFT port conflict changed canonical state or left general P2P running")
	}
}
