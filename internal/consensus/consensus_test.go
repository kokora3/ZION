package consensus

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"encoding/hex"
	"encoding/json"
	"os"
	"reflect"
	"testing"

	abci "github.com/cometbft/cometbft/abci/types"
	cmted25519 "github.com/cometbft/cometbft/crypto/ed25519"
	"github.com/kokora3/zion/internal/chain"
	"github.com/kokora3/zion/internal/identity"
	"github.com/kokora3/zion/internal/membership"
	"github.com/kokora3/zion/internal/protocol"
)

const testKeyNotice = "TEST ONLY - PUBLIC FIXTURE - NOT SECRET - NEVER USE IN PRODUCTION"

type chainFixture struct {
	Canonical string `json:"canonical_transaction_cbor_hex"`
}

func fixtureBytes(t testing.TB, name string) []byte {
	t.Helper()
	raw, err := os.ReadFile("../chain/testdata/" + name)
	if err != nil {
		t.Fatal(err)
	}
	var fixture chainFixture
	if err := json.Unmarshal(raw, &fixture); err != nil {
		t.Fatal(err)
	}
	decoded, err := hex.DecodeString(fixture.Canonical)
	if err != nil {
		t.Fatal(err)
	}
	return decoded
}

func testValidators() ([]Validator, []cmted25519.PrivKey) {
	validators := make([]Validator, DefaultValidatorCount)
	privateKeys := make([]cmted25519.PrivKey, DefaultValidatorCount)
	for i := range validators {
		privateKeys[i] = cmted25519.GenPrivKeyFromSecret([]byte(testKeyNotice + string(rune('A'+i))))
		validators[i] = Validator{Name: "validator-" + string(rune('A'+i)),
			PublicKey: append([]byte(nil), privateKeys[i].PubKey().Bytes()...), Power: DefaultValidatorPower}
	}
	return validators, privateKeys
}

func testGenesis(t testing.TB) Genesis {
	t.Helper()
	validators, _ := testValidators()
	genesis, err := NewGenesis(protocol.Alpha1NetworkID, validators)
	if err != nil {
		t.Fatal(err)
	}
	return genesis
}

func validatorUpdates(genesis Genesis) []abci.ValidatorUpdate {
	updates := make([]abci.ValidatorUpdate, len(genesis.Validators))
	for i, validator := range genesis.Validators {
		updates[i] = abci.ValidatorUpdate{Power: validator.Power,
			PubKeyBytes: append([]byte(nil), validator.PublicKey...), PubKeyType: "ed25519"}
	}
	return updates
}

func TestGenesisBindingAndValidatorValidation(t *testing.T) {
	validators, _ := testValidators()
	genesisA, err := NewGenesis(protocol.Alpha1NetworkID, validators)
	if err != nil {
		t.Fatal(err)
	}
	reversed := append([]Validator(nil), validators...)
	for left, right := 0, len(reversed)-1; left < right; left, right = left+1, right-1 {
		reversed[left], reversed[right] = reversed[right], reversed[left]
	}
	genesisB, err := NewGenesis(protocol.Alpha1NetworkID, reversed)
	if err != nil {
		t.Fatal(err)
	}
	if genesisA.ChainID != genesisB.ChainID || !bytes.Equal(genesisA.AppState, genesisB.AppState) {
		t.Fatal("genesis depends on validator insertion order")
	}

	duplicate := append([]Validator(nil), validators...)
	duplicate[3].PublicKey = append([]byte(nil), duplicate[0].PublicKey...)
	if _, err := NewGenesis(protocol.Alpha1NetworkID, duplicate); err == nil {
		t.Fatal("duplicate validator consensus key accepted")
	}
	wrongPower := append([]Validator(nil), validators...)
	wrongPower[0].Power = 2
	if _, err := NewGenesis(protocol.Alpha1NetworkID, wrongPower); err == nil {
		t.Fatal("unequal voting power accepted")
	}
	other, err := NewGenesis("other-network", validators)
	if err != nil {
		t.Fatal(err)
	}
	if other.ChainID == genesisA.ChainID || bytes.Equal(other.AppState, genesisA.AppState) {
		t.Fatal("network did not bind genesis identity")
	}
}

func TestDecodeTransactionCanonicalBoundary(t *testing.T) {
	valid := fixtureBytes(t, "identity_create_golden.json")
	tx, err := DecodeTransaction(valid, protocol.Alpha1NetworkID)
	if err != nil || tx.Type != chain.IdentityCreate {
		t.Fatalf("decode golden: tx=%+v err=%v", tx, err)
	}

	tests := []struct {
		name string
		raw  []byte
		kind DecodeErrorKind
	}{
		{"random", []byte{0xff, 0x00}, DecodeMalformed},
		{"truncated", valid[:len(valid)-1], DecodeMalformed},
		{"oversized", make([]byte, chain.MaxTransactionBytes+1), DecodeOversized},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if _, err := DecodeTransaction(test.raw, protocol.Alpha1NetworkID); !IsDecodeError(err, test.kind) {
				t.Fatalf("error=%v, want kind %s", err, test.kind)
			}
		})
	}

	var decoded chain.Transaction
	if err := protocol.CanonicalDecode(valid, &decoded); err != nil {
		t.Fatal(err)
	}
	decoded.SchemaVersion++
	unsupportedSchema, _ := decoded.CanonicalBytes()
	if _, err := DecodeTransaction(unsupportedSchema, protocol.Alpha1NetworkID); !IsDecodeError(err, DecodeSchema) {
		t.Fatalf("unsupported schema error=%v", err)
	}
	decoded.SchemaVersion = chain.TransactionSchema
	decoded.Type = "Mystery"
	decoded.IdentityCreate = nil
	unknownType, _ := decoded.CanonicalBytes()
	if _, err := DecodeTransaction(unknownType, protocol.Alpha1NetworkID); !IsDecodeError(err, DecodeType) {
		t.Fatalf("unknown type error=%v", err)
	}
	if _, err := DecodeTransaction(valid, "wrong"); !IsDecodeError(err, DecodeNetwork) {
		t.Fatalf("wrong network error=%v", err)
	}

	// Integer 1 encoded in its longer CBOR form. A strict decoder may reject it
	// while parsing or the byte-for-byte canonical check rejects it afterward.
	nonCanonical := append([]byte{valid[0], valid[1], 0x18, 0x01}, valid[3:]...)
	if _, err := DecodeTransaction(nonCanonical, protocol.Alpha1NetworkID); err == nil {
		t.Fatal("non-canonical CBOR accepted")
	}
}

func TestApplicationLifecycleDeterminismAndRevalidation(t *testing.T) {
	genesis := testGenesis(t)
	apps := make([]*Application, DefaultValidatorCount)
	for i := range apps {
		var err error
		apps[i], err = NewApplication(genesis)
		if err != nil {
			t.Fatal(err)
		}
	}

	createRaw := fixtureBytes(t, "identity_create_golden.json")
	rotationRaw := fixtureBytes(t, "key_rotation_golden.json")
	createTx, err := DecodeTransaction(createRaw, protocol.Alpha1NetworkID)
	if err != nil {
		t.Fatal(err)
	}
	conflictTx := makeRotationTx(t, *createTx.IdentityCreate, deterministicMemberKey(0), deterministicMemberKey(64), 1)
	conflictRaw, err := conflictTx.CanonicalBytes()
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	var expectedResults []*abci.ExecTxResult
	for appIndex, app := range apps {
		before, _ := app.Status()
		for _, raw := range [][]byte{createRaw, []byte{0xff}} {
			response, err := app.CheckTx(ctx, &abci.CheckTxRequest{Tx: raw})
			if err != nil {
				t.Fatal(err)
			}
			if raw[0] == 0xff && response.Code != CodeMalformed {
				t.Fatalf("malformed CheckTx code=%d", response.Code)
			}
			if raw[0] != 0xff && response.Code != CodeOK {
				t.Fatalf("valid CheckTx code=%d", response.Code)
			}
		}
		after, _ := app.Status()
		if before.Height != after.Height || !bytes.Equal(before.AppHash, after.AppHash) || !reflect.DeepEqual(before.Snapshot, after.Snapshot) {
			t.Fatal("CheckTx mutated committed state")
		}

		proposal, err := app.ProcessProposal(ctx, &abci.ProcessProposalRequest{Txs: [][]byte{createRaw, rotationRaw}})
		if err != nil || proposal.Status != abci.PROCESS_PROPOSAL_STATUS_ACCEPT {
			t.Fatalf("ordered proposal rejected: %+v %v", proposal, err)
		}
		prepared, err := app.PrepareProposal(ctx, &abci.PrepareProposalRequest{MaxTxBytes: 1 << 20,
			Txs: [][]byte{[]byte{0xff}, createRaw, rotationRaw}})
		if err != nil || len(prepared.Txs) != 2 || !bytes.Equal(prepared.Txs[0], createRaw) || !bytes.Equal(prepared.Txs[1], rotationRaw) {
			t.Fatalf("proposal preparation changed valid order: %+v %v", prepared, err)
		}
		finalized, err := app.FinalizeBlock(ctx, &abci.FinalizeBlockRequest{Height: 1, Txs: [][]byte{createRaw}})
		if err != nil {
			t.Fatal(err)
		}
		if len(finalized.TxResults) != 1 || finalized.TxResults[0].Code != CodeOK {
			t.Fatalf("results=%+v", finalized.TxResults)
		}
		if _, err := app.Commit(ctx, &abci.CommitRequest{}); err != nil {
			t.Fatal(err)
		}

		// Both rotations are valid against the same committed state and may
		// independently pass mempool checking.
		for _, raw := range [][]byte{rotationRaw, conflictRaw} {
			response, err := app.CheckTx(ctx, &abci.CheckTxRequest{Tx: raw})
			if err != nil || response.Code != CodeOK {
				t.Fatalf("initial rotation CheckTx: response=%+v err=%v", response, err)
			}
		}
		beforeRotations, _ := app.Status()
		afterChecks, _ := app.Status()
		if !bytes.Equal(beforeRotations.AppHash, afterChecks.AppHash) {
			t.Fatal("rotation CheckTx mutated state")
		}

		// Ordered proposal validation rejects the conflict because the first
		// rotation retires the key/consumes sequence 1 on temporary state.
		proposal, err = app.ProcessProposal(ctx, &abci.ProcessProposalRequest{Txs: [][]byte{rotationRaw, conflictRaw}})
		if err != nil || proposal.Status != abci.PROCESS_PROPOSAL_STATUS_REJECT {
			t.Fatalf("conflicting proposal not rejected: %+v %v", proposal, err)
		}

		// Final execution still revalidates independently if invalid input ever
		// reaches this boundary: first succeeds, conflict fails atomically.
		finalized, err = app.FinalizeBlock(ctx, &abci.FinalizeBlockRequest{Height: 2, Txs: [][]byte{rotationRaw, conflictRaw}})
		if err != nil {
			t.Fatal(err)
		}
		if len(finalized.TxResults) != 2 || finalized.TxResults[0].Code != CodeOK || finalized.TxResults[1].Code != CodeRejected {
			t.Fatalf("revalidation results=%+v", finalized.TxResults)
		}
		if appIndex == 0 {
			expectedResults = finalized.TxResults
		} else if !reflect.DeepEqual(finalized.TxResults, expectedResults) {
			t.Fatalf("application %d produced different deterministic results", appIndex)
		}
		if _, err := app.Commit(ctx, &abci.CommitRequest{}); err != nil {
			t.Fatal(err)
		}
	}

	wantHash, _ := apps[0].Status()
	for i, app := range apps {
		status, err := app.Status()
		if err != nil {
			t.Fatal(err)
		}
		if !bytes.Equal(status.AppHash, wantHash.AppHash) || status.Height != 2 {
			t.Fatalf("validator %d diverged", i)
		}
		if len(status.Snapshot.Identities) != 1 || status.Snapshot.Identities[0].Sequence != 1 ||
			status.Snapshot.Memberships[0].Status != membership.Pending {
			t.Fatalf("unexpected committed state: %+v", status.Snapshot)
		}
		active := 0
		for _, key := range status.Snapshot.Identities[0].Keys {
			if key.Active {
				active++
			}
		}
		if active != 1 {
			t.Fatalf("active key count=%d", active)
		}
	}

	// A later malformed/replayed finalized transaction has a stable result and
	// leaves the already committed canonical state unchanged.
	app := apps[0]
	response, _ := app.CheckTx(ctx, &abci.CheckTxRequest{Tx: rotationRaw})
	if response.Code != CodeRejected {
		t.Fatalf("rotation replay CheckTx code=%d", response.Code)
	}
	before, _ := app.Status()
	finalized, err := app.FinalizeBlock(ctx, &abci.FinalizeBlockRequest{Height: 3, Txs: [][]byte{rotationRaw, []byte{0xff}}})
	if err != nil {
		t.Fatal(err)
	}
	if finalized.TxResults[0].Code != CodeRejected || finalized.TxResults[1].Code != CodeMalformed {
		t.Fatalf("invalid block results=%+v", finalized.TxResults)
	}
	if _, err := app.Commit(ctx, &abci.CommitRequest{}); err != nil {
		t.Fatal(err)
	}
	after, _ := app.Status()
	if !bytes.Equal(before.AppHash, after.AppHash) {
		t.Fatal("invalid finalized transactions mutated state")
	}
}

func TestInitChainRejectsWrongBinding(t *testing.T) {
	genesis := testGenesis(t)
	newApp := func() *Application {
		app, err := NewApplication(genesis)
		if err != nil {
			t.Fatal(err)
		}
		return app
	}
	request := func() *abci.InitChainRequest {
		return &abci.InitChainRequest{ChainId: genesis.ChainID, AppStateBytes: append([]byte(nil), genesis.AppState...),
			Validators: validatorUpdates(genesis)}
	}
	if _, err := newApp().InitChain(context.Background(), request()); err != nil {
		t.Fatalf("valid genesis rejected: %v", err)
	}
	wrongNetwork := request()
	wrongNetwork.ChainId = "wrong"
	if _, err := newApp().InitChain(context.Background(), wrongNetwork); err == nil {
		t.Fatal("wrong consensus chain ID accepted")
	}
	wrongGenesis := request()
	wrongGenesis.AppStateBytes[0] ^= 1
	if _, err := newApp().InitChain(context.Background(), wrongGenesis); err == nil {
		t.Fatal("wrong application genesis accepted")
	}
	wrongValidators := request()
	wrongValidators.Validators[0].PubKeyBytes[0] ^= 1
	if _, err := newApp().InitChain(context.Background(), wrongValidators); err == nil {
		t.Fatal("wrong validator configuration accepted")
	}
}

func FuzzDecodeTransaction(f *testing.F) {
	f.Add([]byte{})
	f.Add([]byte{0xff})
	f.Add(fixtureBytes(f, "identity_create_golden.json"))
	f.Add(fixtureBytes(f, "key_rotation_golden.json"))
	f.Fuzz(func(t *testing.T, raw []byte) {
		_, _ = DecodeTransaction(raw, protocol.Alpha1NetworkID)
	})
}

func deterministicMemberKey(start byte) ed25519.PrivateKey {
	seed := make([]byte, ed25519.SeedSize)
	for i := range seed {
		seed[i] = start + byte(i)
	}
	return ed25519.NewKeyFromSeed(seed)
}

func makeIdentityTx(t testing.TB, start byte, createdAt protocol.ProtocolTimestamp) (chain.Transaction, identity.IdentityGenesisProof) {
	t.Helper()
	key := deterministicMemberKey(start)
	body := identity.IdentityGenesisBody{SchemaVersion: identity.GenesisSchema,
		InitialPublicKey: identity.PublicFromPrivate(key), CreatedAt: createdAt}
	proof, err := identity.CreateIdentity(body, key)
	if err != nil {
		t.Fatal(err)
	}
	return chain.Transaction{SchemaVersion: chain.TransactionSchema, NetworkID: protocol.Alpha1NetworkID,
		Type: chain.IdentityCreate, IdentityCreate: &proof}, proof
}

func makeRotationTx(t testing.TB, genesis identity.IdentityGenesisProof, oldKey, newKey ed25519.PrivateKey, sequence uint64) chain.Transaction {
	t.Helper()
	oldKeyID, err := identity.DeriveKeyID(identity.PublicFromPrivate(oldKey))
	if err != nil {
		t.Fatal(err)
	}
	request := identity.RotationRequest{SchemaVersion: identity.RotationSchema, NetworkID: protocol.Alpha1NetworkID,
		IdentityID: genesis.IdentityID, Sequence: sequence, OldKeyID: oldKeyID,
		NewPublicKey: identity.PublicFromPrivate(newKey), CreatedAt: protocol.ProtocolTimestamp(200 + sequence)}
	proof, err := identity.CreateRotation(request, oldKey, newKey)
	if err != nil {
		t.Fatal(err)
	}
	return chain.Transaction{SchemaVersion: chain.TransactionSchema, NetworkID: protocol.Alpha1NetworkID,
		Type: chain.KeyRotation, KeyRotation: &proof}
}
