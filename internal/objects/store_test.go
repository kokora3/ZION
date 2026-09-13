package objects

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"sync"
	"testing"

	"github.com/kokora3/zion/internal/chain"
	"github.com/kokora3/zion/internal/protocol"
)

func testObject(payload []byte, marker string) Object {
	copyPayload := append([]byte(nil), payload...)
	return Object{
		Core: protocol.UnsignedObjectCore{
			ObjectType:    "note",
			SchemaVersion: protocol.UnsignedObjectSchemaV1,
			CreatedAt:     1710000000123,
			ContentHash:   protocol.NewContentHash(copyPayload),
			SizeBytes:     uint64(len(copyPayload)),
			Visibility:    protocol.VisibilityPublic,
			Metadata:      map[string]string{"marker": marker},
		},
		Payload: copyPayload,
	}
}

func openTestStore(t *testing.T, root string, quota uint64) *Store {
	t.Helper()
	store, err := Open(root, quota)
	if err != nil {
		t.Fatal(err)
	}
	return store
}

func TestPutGetHasStatDuplicateAndRestart(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	store := openTestStore(t, root, 0)
	object := testObject([]byte("phase-nine-object"), "basic")

	id, result, err := store.Put(ctx, object)
	if err != nil || result != PutStored {
		t.Fatalf("first Put = %q, %v", result, err)
	}
	wantID, err := object.Core.ObjectID()
	if err != nil || id.String() != wantID.String() {
		t.Fatalf("frozen ObjectID not reused: %s, %v", id.String(), err)
	}
	got, err := store.Get(ctx, id)
	if err != nil || !bytes.Equal(got.Payload, object.Payload) {
		t.Fatalf("Get = %q, %v", got.Payload, err)
	}
	has, err := store.Has(ctx, id)
	if err != nil || !has {
		t.Fatalf("Has = %v, %v", has, err)
	}
	metadata, err := store.Stat(ctx, id)
	canonical, _ := object.CanonicalBytes()
	if err != nil || metadata.ObjectID.String() != id.String() || metadata.Size != uint64(len(canonical)) || metadata.StoredAt.IsZero() {
		t.Fatalf("Stat = %+v, %v", metadata, err)
	}

	for i := 0; i < 100; i++ {
		duplicateID, duplicateResult, err := store.Put(ctx, object)
		if err != nil || duplicateResult != PutAlreadyExists || duplicateID.String() != id.String() {
			t.Fatalf("duplicate %d = %s %q %v", i, duplicateID.String(), duplicateResult, err)
		}
	}
	count, used, _ := store.Usage()
	if count != 1 || used != uint64(len(canonical)) {
		t.Fatalf("duplicate changed usage: count=%d bytes=%d", count, used)
	}

	reopened := openTestStore(t, root, 0)
	got, err = reopened.Get(ctx, id)
	if err != nil || !bytes.Equal(got.Payload, object.Payload) {
		t.Fatalf("restart Get = %q, %v", got.Payload, err)
	}
}

func TestConcurrentDuplicatePut(t *testing.T) {
	store := openTestStore(t, t.TempDir(), 0)
	object := testObject([]byte("same immutable bytes"), "concurrent")
	const workers = 32
	var wg sync.WaitGroup
	results := make(chan PutResult, workers)
	errs := make(chan error, workers)
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, result, err := store.Put(context.Background(), object)
			results <- result
			errs <- err
		}()
	}
	wg.Wait()
	close(results)
	close(errs)
	stored := 0
	for err := range errs {
		if err != nil {
			t.Fatal(err)
		}
	}
	for result := range results {
		if result == PutStored {
			stored++
		} else if result != PutAlreadyExists {
			t.Fatalf("unexpected result %q", result)
		}
	}
	if stored != 1 {
		t.Fatalf("stored outcomes = %d, want 1", stored)
	}
	id, _ := object.ObjectID()
	got, err := store.Get(context.Background(), id)
	if err != nil || !bytes.Equal(got.Payload, object.Payload) {
		t.Fatalf("final object invalid: %v", err)
	}
}

func TestDistinctObjectsAndValidationLimits(t *testing.T) {
	a := testObject([]byte("a"), "distinct")
	b := testObject([]byte("b"), "distinct")
	aID, _ := a.ObjectID()
	bID, _ := b.ObjectID()
	if aID.String() == bID.String() {
		t.Fatal("distinct content produced the same ObjectID")
	}

	oversized := testObject(bytes.Repeat([]byte{'x'}, MaxObjectBytes+1), "oversized")
	if _, err := oversized.CanonicalBytes(); !errors.Is(err, ErrObjectTooLarge) {
		t.Fatalf("oversized error = %v", err)
	}
	badSize := testObject([]byte("data"), "bad-size")
	badSize.Core.SizeBytes++
	if _, err := badSize.CanonicalBytes(); !errors.Is(err, ErrInvalidObject) {
		t.Fatalf("bad size error = %v", err)
	}
	badHash := testObject([]byte("data"), "bad-hash")
	badHash.Core.ContentHash = protocol.NewContentHash([]byte("other"))
	if _, err := badHash.CanonicalBytes(); !errors.Is(err, ErrInvalidObject) {
		t.Fatalf("bad hash error = %v", err)
	}
	badCore := testObject(nil, "bad-core")
	badCore.Core.ObjectType = ""
	if _, _, err := openTestStore(t, t.TempDir(), 0).Put(context.Background(), badCore); !errors.Is(err, ErrInvalidObject) {
		t.Fatalf("malformed Put error = %v", err)
	}
}

func TestValidatedDigestPathsPreventTraversal(t *testing.T) {
	store := openTestStore(t, t.TempDir(), 0)
	for _, raw := range []string{
		"../../secret", "../", `zion:obj:sha256:..\\secret`,
		"zion:obj:sha256:aa/bb", "zion:obj:sha256:aa\\bb",
		"zion:obj:sha256:00", "zion:obj:sha256:GG", "zion:obj:sha512:" + string(bytes.Repeat([]byte{'0'}, 64)),
	} {
		if _, err := protocol.ParseObjectID(raw); err == nil {
			t.Errorf("ParseObjectID accepted traversal-like value %q", raw)
		}
	}
	invalid := protocol.ObjectID{HashDigest: protocol.HashDigest{Algorithm: protocol.HashAlgorithm("../sha256"), Digest: make([]byte, 32)}}
	if _, err := store.pathForID(invalid); err == nil {
		t.Fatal("pathForID accepted invalid algorithm")
	}
	valid := testObject([]byte("path"), "safe")
	id, _ := valid.ObjectID()
	path, err := store.pathForID(id)
	if err != nil {
		t.Fatal(err)
	}
	rel, err := filepath.Rel(store.root, path)
	if err != nil || rel == ".." || filepath.IsAbs(rel) || len(rel) < 7 {
		t.Fatalf("unsafe derived path %q (%v)", rel, err)
	}
}

func TestCorruptionAndTruncationFailClosed(t *testing.T) {
	for _, test := range []struct {
		name   string
		mutate func([]byte) []byte
	}{
		{name: "corrupt", mutate: func(raw []byte) []byte { raw[len(raw)-1] ^= 0xff; return raw }},
		{name: "truncated", mutate: func(raw []byte) []byte { return raw[:len(raw)/2] }},
	} {
		t.Run(test.name, func(t *testing.T) {
			store := openTestStore(t, t.TempDir(), 0)
			object := testObject([]byte("integrity protected"), test.name)
			id, _, err := store.Put(context.Background(), object)
			if err != nil {
				t.Fatal(err)
			}
			path, _ := store.pathForID(id)
			raw, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(path, test.mutate(raw), 0o600); err != nil {
				t.Fatal(err)
			}
			if _, err := store.Get(context.Background(), id); !errors.Is(err, ErrObjectCorrupt) {
				t.Fatalf("Get corrupt error = %v", err)
			}
			if _, _, err := store.Put(context.Background(), object); !errors.Is(err, ErrObjectCorrupt) {
				t.Fatalf("Put over corrupt error = %v", err)
			}
		})
	}
}

func TestQuotaAndConcurrentNearQuota(t *testing.T) {
	a := testObject([]byte("quota-a"), "quota")
	b := testObject([]byte("quota-b"), "quota")
	aBytes, _ := a.CanonicalBytes()
	bBytes, _ := b.CanonicalBytes()
	quota := uint64(len(aBytes))
	if len(bBytes) > len(aBytes) {
		quota = uint64(len(bBytes))
	}
	store := openTestStore(t, t.TempDir(), quota)
	type outcome struct {
		id  protocol.ObjectID
		err error
	}
	start := make(chan struct{})
	outcomes := make(chan outcome, 2)
	for _, object := range []Object{a, b} {
		object := object
		go func() {
			<-start
			id, _, err := store.Put(context.Background(), object)
			outcomes <- outcome{id: id, err: err}
		}()
	}
	close(start)
	var storedID protocol.ObjectID
	stored, full := 0, 0
	for i := 0; i < 2; i++ {
		result := <-outcomes
		if result.err == nil {
			stored++
			storedID = result.id
		} else if errors.Is(result.err, ErrStoreFull) {
			full++
		} else {
			t.Fatal(result.err)
		}
	}
	if stored != 1 || full != 1 {
		t.Fatalf("stored=%d full=%d", stored, full)
	}
	if _, err := store.Get(context.Background(), storedID); err != nil {
		t.Fatalf("existing read when full: %v", err)
	}
	count, used, gotQuota := store.Usage()
	if count != 1 || used > gotQuota {
		t.Fatalf("quota accounting count=%d used=%d quota=%d", count, used, gotQuota)
	}
}

func TestStaleTemporaryFileIsIgnored(t *testing.T) {
	root := t.TempDir()
	staleDir := filepath.Join(root, "sha256", "aa", "bb")
	if err := os.MkdirAll(staleDir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(staleDir, ".put-stale.tmp"), []byte("not an object"), 0o600); err != nil {
		t.Fatal(err)
	}
	store := openTestStore(t, root, 0)
	count, used, _ := store.Usage()
	if count != 0 || used != 0 {
		t.Fatalf("stale temp counted as object: count=%d bytes=%d", count, used)
	}
	missing := protocol.ObjectID{HashDigest: protocol.HashBytes([]byte("missing"))}
	has, err := store.Has(context.Background(), missing)
	if err != nil || has {
		t.Fatalf("stale temp became valid: has=%v err=%v", has, err)
	}
}

func TestObjectStorageDoesNotAlterStateHash(t *testing.T) {
	state := chain.Genesis(protocol.Alpha1NetworkID)
	before, err := state.Hash()
	if err != nil {
		t.Fatal(err)
	}
	store := openTestStore(t, t.TempDir(), 0)
	for i, payload := range [][]byte{[]byte("one"), []byte("two"), []byte("three")} {
		if _, _, err := store.Put(context.Background(), testObject(payload, string(rune('a'+i)))); err != nil {
			t.Fatal(err)
		}
	}
	after, err := state.Hash()
	if err != nil {
		t.Fatal(err)
	}
	if before.String() != after.String() {
		t.Fatalf("object availability changed StateHash: %s != %s", before.String(), after.String())
	}
}
