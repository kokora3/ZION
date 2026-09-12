package chain

import (
	"bytes"
	"crypto/ed25519"
	"reflect"
	"strings"
	"testing"

	"github.com/kokora3/zion/internal/identity"
	"github.com/kokora3/zion/internal/membership"
	"github.com/kokora3/zion/internal/protocol"
)

func deterministicKey(first byte) ed25519.PrivateKey {
	seed := make([]byte, ed25519.SeedSize)
	for i := range seed {
		seed[i] = first + byte(i)
	}
	return ed25519.NewKeyFromSeed(seed)
}

func keyA() ed25519.PrivateKey { return deterministicKey(0) }
func keyB() ed25519.PrivateKey { return deterministicKey(32) }
func keyC() ed25519.PrivateKey { return deterministicKey(64) }

func identityTransaction(t *testing.T, key ed25519.PrivateKey, createdAt protocol.ProtocolTimestamp) (Transaction, identity.IdentityGenesisProof) {
	t.Helper()
	body := identity.IdentityGenesisBody{
		SchemaVersion:    identity.GenesisSchema,
		InitialPublicKey: identity.PublicFromPrivate(key),
		CreatedAt:        createdAt,
	}
	proof, err := identity.CreateIdentity(body, key)
	if err != nil {
		t.Fatal(err)
	}
	return Transaction{
		SchemaVersion:  TransactionSchema,
		NetworkID:      protocol.Alpha1NetworkID,
		Type:           IdentityCreate,
		IdentityCreate: &proof,
	}, proof
}

func stateWithIdentity(t *testing.T) (State, Transaction, identity.IdentityGenesisProof) {
	t.Helper()
	tx, proof := identityTransaction(t, keyA(), 1)
	state, receipt, err := Apply(Genesis(protocol.Alpha1NetworkID), tx)
	if err != nil || receipt.Code != OK {
		t.Fatalf("create identity: receipt=%+v err=%v", receipt, err)
	}
	return state, tx, proof
}

func rotationTransaction(t *testing.T, proof identity.IdentityGenesisProof, oldKey, newKey ed25519.PrivateKey, sequence uint64) Transaction {
	t.Helper()
	oldKeyID, err := identity.DeriveKeyID(identity.PublicFromPrivate(oldKey))
	if err != nil {
		t.Fatal(err)
	}
	request := identity.RotationRequest{
		SchemaVersion: identity.RotationSchema,
		NetworkID:     protocol.Alpha1NetworkID,
		IdentityID:    proof.IdentityID,
		Sequence:      sequence,
		OldKeyID:      oldKeyID,
		NewPublicKey:  identity.PublicFromPrivate(newKey),
		CreatedAt:     protocol.ProtocolTimestamp(sequence + 10),
	}
	rotation, err := identity.CreateRotation(request, oldKey, newKey)
	if err != nil {
		t.Fatal(err)
	}
	return Transaction{
		SchemaVersion: TransactionSchema,
		NetworkID:     protocol.Alpha1NetworkID,
		Type:          KeyRotation,
		KeyRotation:   &rotation,
	}
}

func requireRejectedAtomically(t *testing.T, state State, tx Transaction, expected Code) Receipt {
	t.Helper()
	beforeBytes, err := state.CanonicalBytes()
	if err != nil {
		t.Fatal(err)
	}
	beforeHash, err := state.Hash()
	if err != nil {
		t.Fatal(err)
	}

	after, receipt, applyErr := Apply(state, tx)
	if applyErr == nil {
		t.Fatalf("accepted invalid transaction, receipt=%+v", receipt)
	}
	if receipt.Code != expected {
		t.Fatalf("result code=%q, want %q (err=%v)", receipt.Code, expected, applyErr)
	}
	afterBytes, err := after.CanonicalBytes()
	if err != nil {
		t.Fatal(err)
	}
	afterHash, err := after.Hash()
	if err != nil {
		t.Fatal(err)
	}
	originalBytes, err := state.CanonicalBytes()
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(beforeBytes, afterBytes) || !bytes.Equal(beforeBytes, originalBytes) {
		t.Fatal("rejected transaction changed canonical state")
	}
	if beforeHash.String() != afterHash.String() || receipt.StateHash.String() != beforeHash.String() {
		t.Fatal("rejected transaction changed StateHash or returned the wrong receipt StateHash")
	}
	wantTxID, err := tx.ID()
	if err != nil {
		t.Fatal(err)
	}
	if receipt.TxID.String() != wantTxID.String() {
		t.Fatalf("receipt TxID=%q, want %q", receipt.TxID.String(), wantTxID.String())
	}
	return receipt
}

func TestGenesisDeterministic(t *testing.T) {
	a := Genesis(protocol.Alpha1NetworkID)
	b := Genesis(protocol.Alpha1NetworkID)
	ab, err := a.CanonicalBytes()
	if err != nil {
		t.Fatal(err)
	}
	bb, err := b.CanonicalBytes()
	if err != nil {
		t.Fatal(err)
	}
	ah, err := a.Hash()
	if err != nil {
		t.Fatal(err)
	}
	bh, err := b.Hash()
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(ab, bb) || ah.String() != bh.String() {
		t.Fatal("empty genesis state is not deterministic")
	}
}

func TestIdentityCreateAndRotationChain(t *testing.T) {
	state, _, proof := stateWithIdentity(t)
	identityID := proof.IdentityID.String()
	stored := state.Identities[identityID]
	if state.Memberships[identityID] != membership.Pending || stored.Sequence != 0 || !stored.Active[proof.InitialKeyID.String()] {
		t.Fatal("incorrect IdentityCreate state")
	}

	ab := rotationTransaction(t, proof, keyA(), keyB(), 1)
	state, receipt, err := Apply(state, ab)
	if err != nil || receipt.Code != OK {
		t.Fatalf("A to B: receipt=%+v err=%v", receipt, err)
	}
	stored = state.Identities[identityID]
	keyBID, _ := identity.DeriveKeyID(identity.PublicFromPrivate(keyB()))
	if stored.Sequence != 1 || stored.Active[proof.InitialKeyID.String()] || !stored.Active[keyBID.String()] {
		t.Fatal("incorrect A to B state")
	}
	requireRejectedAtomically(t, state, ab, Sequence)

	bc := rotationTransaction(t, proof, keyB(), keyC(), 2)
	state, receipt, err = Apply(state, bc)
	if err != nil || receipt.Code != OK {
		t.Fatalf("B to C: receipt=%+v err=%v", receipt, err)
	}
	stored = state.Identities[identityID]
	keyCID, _ := identity.DeriveKeyID(identity.PublicFromPrivate(keyC()))
	if stored.Sequence != 2 || !stored.Active[keyCID.String()] || state.Memberships[identityID] != membership.Pending {
		t.Fatal("incorrect B to C state or membership changed")
	}
	if stored.ID.String() != proof.IdentityID.String() {
		t.Fatal("rotation changed IdentityID")
	}
}

func TestRejectedTransactionsAreAtomic(t *testing.T) {
	state, createTx, proof := stateWithIdentity(t)

	t.Run("duplicate IdentityCreate", func(t *testing.T) {
		requireRejectedAtomically(t, state, createTx, Exists)
	})
	t.Run("wrong NetworkID", func(t *testing.T) {
		tx := createTx
		tx.NetworkID = "zion-other"
		requireRejectedAtomically(t, state, tx, WrongNetwork)
	})
	t.Run("unsupported transaction type", func(t *testing.T) {
		tx := Transaction{SchemaVersion: TransactionSchema, NetworkID: protocol.Alpha1NetworkID, Type: "Unknown"}
		requireRejectedAtomically(t, state, tx, Unsupported)
	})
	t.Run("reserved MembershipChange", func(t *testing.T) {
		tx := Transaction{SchemaVersion: TransactionSchema, NetworkID: protocol.Alpha1NetworkID, Type: MembershipChange}
		requireRejectedAtomically(t, state, tx, Unsupported)
	})
	for _, sequence := range []uint64{0, 2} {
		t.Run("wrong rotation sequence", func(t *testing.T) {
			tx := rotationTransaction(t, proof, keyA(), keyB(), sequence)
			requireRejectedAtomically(t, state, tx, Sequence)
		})
	}
	t.Run("unknown identity rotation", func(t *testing.T) {
		unknown := proof
		unknown.IdentityID.Digest = append([]byte(nil), proof.IdentityID.Digest...)
		unknown.IdentityID.Digest[0] ^= 1
		tx := rotationTransaction(t, unknown, keyA(), keyB(), 1)
		requireRejectedAtomically(t, state, tx, NotFound)
	})
	t.Run("wrong active old key", func(t *testing.T) {
		tx := rotationTransaction(t, proof, keyC(), keyB(), 1)
		requireRejectedAtomically(t, state, tx, KeyNotActive)
	})
	t.Run("missing new-key proof", func(t *testing.T) {
		tx := rotationTransaction(t, proof, keyA(), keyB(), 1)
		tx.KeyRotation.NewKeyProof = identity.Signature{}
		requireRejectedAtomically(t, state, tx, Invalid)
	})
	t.Run("corrupted new-key proof", func(t *testing.T) {
		tx := rotationTransaction(t, proof, keyA(), keyB(), 1)
		tx.KeyRotation.NewKeyProof.Bytes = append([]byte(nil), tx.KeyRotation.NewKeyProof.Bytes...)
		tx.KeyRotation.NewKeyProof.Bytes[0] ^= 1
		requireRejectedAtomically(t, state, tx, Invalid)
	})
	t.Run("corrupted old-key authorization", func(t *testing.T) {
		tx := rotationTransaction(t, proof, keyA(), keyB(), 1)
		tx.KeyRotation.OldAuthorization.Bytes = append([]byte(nil), tx.KeyRotation.OldAuthorization.Bytes...)
		tx.KeyRotation.OldAuthorization.Bytes[0] ^= 1
		requireRejectedAtomically(t, state, tx, Invalid)
	})
	t.Run("new-key proof signed by wrong private key", func(t *testing.T) {
		tx := rotationTransaction(t, proof, keyA(), keyB(), 1)
		wrongProof, err := identity.CreateRotation(tx.KeyRotation.Request, keyA(), keyC())
		if err != nil {
			t.Fatal(err)
		}
		tx.KeyRotation = &wrongProof
		requireRejectedAtomically(t, state, tx, Invalid)
	})
	t.Run("inner rotation network differs", func(t *testing.T) {
		tx := rotationTransaction(t, proof, keyA(), keyB(), 1)
		tx.KeyRotation.Request.NetworkID = "zion-other"
		wrongNetworkProof, err := identity.CreateRotation(tx.KeyRotation.Request, keyA(), keyB())
		if err != nil {
			t.Fatal(err)
		}
		tx.KeyRotation = &wrongNetworkProof
		requireRejectedAtomically(t, state, tx, WrongNetwork)
	})
	t.Run("oversized canonical transaction", func(t *testing.T) {
		tx, _ := identityTransaction(t, keyB(), 2)
		tx.NetworkID = protocol.NetworkID(strings.Repeat("n", MaxTransactionBytes))
		canonical, err := tx.CanonicalBytes()
		if err != nil {
			t.Fatal(err)
		}
		if len(canonical) <= MaxTransactionBytes {
			t.Fatalf("test transaction size=%d, limit=%d", len(canonical), MaxTransactionBytes)
		}
		if err := identity.VerifyIdentityGenesis(*tx.IdentityCreate); err != nil {
			t.Fatalf("embedded proof must remain valid: %v", err)
		}
		requireRejectedAtomically(t, state, tx, TooLarge)
	})

	t.Run("retired old key", func(t *testing.T) {
		ab := rotationTransaction(t, proof, keyA(), keyB(), 1)
		rotated, _, err := Apply(state, ab)
		if err != nil {
			t.Fatal(err)
		}
		retired := rotationTransaction(t, proof, keyA(), keyC(), 2)
		requireRejectedAtomically(t, rotated, retired, KeyNotActive)
		stored := rotated.Identities[proof.IdentityID.String()]
		if stored.Sequence != 1 || stored.Active[proof.InitialKeyID.String()] || rotated.Memberships[proof.IdentityID.String()] != membership.Pending {
			t.Fatal("retired-key rejection changed sequence, active keys, or membership")
		}
	})
}

func TestSnapshotOrderCopyAndReceiptDeterminism(t *testing.T) {
	createdAt := []protocol.ProtocolTimestamp{101, 102, 103}

	build := func(t *testing.T, order []int) State {
		t.Helper()
		state := Genesis(protocol.Alpha1NetworkID)
		for _, index := range order {
			tx, _ := identityTransaction(t, deterministicKey(byte(index*32)), createdAt[index])
			var err error
			state, _, err = Apply(state, tx)
			if err != nil {
				t.Fatal(err)
			}
		}
		return state
	}

	// Alice, Bob, Charlie versus Charlie, Alice, Bob. Each build creates fresh
	// proofs, records, maps, and public-key slices and uses the production Apply
	// and Snapshot path.
	stateA := build(t, []int{0, 1, 2})
	stateB := build(t, []int{2, 0, 1})
	bytesA, err := stateA.CanonicalBytes()
	if err != nil {
		t.Fatal(err)
	}
	bytesB, err := stateB.CanonicalBytes()
	if err != nil {
		t.Fatal(err)
	}
	hashA, _ := stateA.Hash()
	hashB, _ := stateB.Hash()
	if !bytes.Equal(bytesA, bytesB) || hashA.String() != hashB.String() {
		t.Fatal("map insertion order or independent allocation changed canonical state")
	}

	// Reallocate every consensus byte slice with deliberately different spare
	// capacity. Pointer identity and slice capacity are not encoded.
	stateC := cloneState(stateA)
	for idString, stored := range stateC.Identities {
		idDigest := make([]byte, len(stored.ID.Digest), len(stored.ID.Digest)+71)
		copy(idDigest, stored.ID.Digest)
		stored.ID.Digest = idDigest
		for keyID, publicKey := range stored.Keys {
			keyBytes := make([]byte, len(publicKey.Key), len(publicKey.Key)+113)
			copy(keyBytes, publicKey.Key)
			publicKey.Key = keyBytes
			stored.Keys[keyID] = publicKey
		}
		stateC.Identities[idString] = stored
	}
	bytesC, err := stateC.CanonicalBytes()
	if err != nil {
		t.Fatal(err)
	}
	hashC, _ := stateC.Hash()
	if !bytes.Equal(bytesA, bytesC) || hashA.String() != hashC.String() {
		t.Fatal("slice allocation or capacity changed canonical state")
	}

	createTx, _ := identityTransaction(t, keyA(), 1)
	genesisA := Genesis(protocol.Alpha1NetworkID)
	genesisB := cloneState(genesisA)
	_, receiptA, errA := Apply(genesisA, createTx)
	_, receiptB, errB := Apply(genesisB, createTx)
	if errA != nil || errB != nil || !reflect.DeepEqual(receiptA, receiptB) {
		t.Fatalf("equivalent inputs produced different receipts: %+v %+v", receiptA, receiptB)
	}
}

func TestApplyCopiesTransactionOwnedBytes(t *testing.T) {
	createTx, _ := identityTransaction(t, keyA(), 1)
	state, _, err := Apply(Genesis(protocol.Alpha1NetworkID), createTx)
	if err != nil {
		t.Fatal(err)
	}
	createdHash, _ := state.Hash()
	createTx.IdentityCreate.Body.InitialPublicKey.Key[0] ^= 1
	createTx.IdentityCreate.IdentityID.Digest[0] ^= 1
	afterMutation, _ := state.Hash()
	if createdHash.String() != afterMutation.String() {
		t.Fatal("State aliases IdentityCreate transaction bytes")
	}

	_, freshProof := identityTransaction(t, keyA(), 1)
	rotation := rotationTransaction(t, freshProof, keyA(), keyB(), 1)
	state, _, err = Apply(state, rotation)
	if err != nil {
		t.Fatal(err)
	}
	rotatedHash, _ := state.Hash()
	rotation.KeyRotation.Request.NewPublicKey.Key[0] ^= 1
	rotation.KeyRotation.Request.IdentityID.Digest[0] ^= 1
	afterMutation, _ = state.Hash()
	if rotatedHash.String() != afterMutation.String() {
		t.Fatal("State aliases KeyRotation transaction bytes")
	}
}

func TestTransactionOrderIsPreserved(t *testing.T) {
	createTx, proof := identityTransaction(t, keyA(), 1)
	rotationTx := rotationTransaction(t, proof, keyA(), keyB(), 1)

	createThenRotate, _, err := Apply(Genesis(protocol.Alpha1NetworkID), createTx)
	if err != nil {
		t.Fatal(err)
	}
	createThenRotate, _, err = Apply(createThenRotate, rotationTx)
	if err != nil {
		t.Fatal(err)
	}

	rotateThenCreate := Genesis(protocol.Alpha1NetworkID)
	rotateThenCreate, _, err = Apply(rotateThenCreate, rotationTx)
	if err == nil {
		t.Fatal("rotation before identity creation unexpectedly succeeded")
	}
	rotateThenCreate, _, err = Apply(rotateThenCreate, createTx)
	if err != nil {
		t.Fatal(err)
	}

	hashA, _ := createThenRotate.Hash()
	hashB, _ := rotateThenCreate.Hash()
	if hashA.String() == hashB.String() {
		t.Fatal("state machine reordered transactions")
	}
	if createThenRotate.Identities[proof.IdentityID.String()].Sequence != 1 || rotateThenCreate.Identities[proof.IdentityID.String()].Sequence != 0 {
		t.Fatal("transaction order did not determine rotation sequence")
	}
}

func TestReplayDeterminism(t *testing.T) {
	var expectedBytes []byte
	var expectedHash string
	for run := 0; run < 100; run++ {
		state, _, proof := stateWithIdentity(t)
		for _, tx := range []Transaction{
			rotationTransaction(t, proof, keyA(), keyB(), 1),
			rotationTransaction(t, proof, keyB(), keyC(), 2),
		} {
			var err error
			state, _, err = Apply(state, tx)
			if err != nil {
				t.Fatal(err)
			}
		}
		canonical, _ := state.CanonicalBytes()
		hash, _ := state.Hash()
		if run == 0 {
			expectedBytes = canonical
			expectedHash = hash.String()
		} else if !bytes.Equal(canonical, expectedBytes) || hash.String() != expectedHash {
			t.Fatalf("replay %d differs", run)
		}
	}
}

func TestTxIDDeterminismNamespaceAndParser(t *testing.T) {
	tx, _ := identityTransaction(t, keyA(), 1)
	idA, err := tx.ID()
	if err != nil {
		t.Fatal(err)
	}
	idB, err := tx.ID()
	if err != nil {
		t.Fatal(err)
	}
	if idA.String() != idB.String() || !strings.HasPrefix(idA.String(), "zion:tx:sha256:") {
		t.Fatal("TxID is nondeterministic or has the wrong namespace")
	}
	changed := tx
	changed.NetworkID = "zion-alpha-2"
	changedID, _ := changed.ID()
	if idA.String() == changedID.String() {
		t.Fatal("distinct canonical transactions have the same TxID")
	}
	parsed, err := ParseTxID(idA.String())
	if err != nil || parsed.String() != idA.String() {
		t.Fatalf("parse valid TxID: %v", err)
	}
	for _, invalid := range []string{
		"x",
		"zion:tx:sha512:00",
		"zion:tx:sha256:00",
		"zion:tx:sha256:GG",
		"zion:tx:sha256:ABC",
		"zion:tx:sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa:extra",
	} {
		if _, err := ParseTxID(invalid); err == nil {
			t.Fatalf("accepted invalid TxID %q", invalid)
		}
	}
}

func TestCanonicalStateTypesExcludePrivateKeys(t *testing.T) {
	privateKeyType := reflect.TypeOf(ed25519.PrivateKey{})
	seen := make(map[reflect.Type]bool)
	var walk func(reflect.Type)
	walk = func(typ reflect.Type) {
		if typ == privateKeyType {
			t.Fatalf("private key type appears in canonical state: %v", typ)
		}
		if seen[typ] {
			return
		}
		seen[typ] = true
		switch typ.Kind() {
		case reflect.Pointer, reflect.Slice, reflect.Array, reflect.Map:
			walk(typ.Elem())
		case reflect.Struct:
			for i := 0; i < typ.NumField(); i++ {
				walk(typ.Field(i).Type)
			}
		}
	}
	walk(reflect.TypeOf(State{}))
	walk(reflect.TypeOf(Snapshot{}))
}

func FuzzParseTxID(f *testing.F) {
	f.Add("zion:tx:sha256:0000000000000000000000000000000000000000000000000000000000000000")
	f.Fuzz(func(t *testing.T, input string) {
		_, _ = ParseTxID(input)
	})
}
