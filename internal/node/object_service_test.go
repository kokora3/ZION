package node

import (
	"bytes"
	"context"
	"testing"
	"time"

	"github.com/kokora3/zion/internal/objects"
	"github.com/kokora3/zion/internal/p2p"
	"github.com/kokora3/zion/internal/protocol"
)

func runtimeTestObject(payload []byte) objects.Object {
	return objects.Object{Core: protocol.UnsignedObjectCore{
		ObjectType: "note", SchemaVersion: protocol.UnsignedObjectSchemaV1, CreatedAt: 1710000000123,
		ContentHash: protocol.NewContentHash(payload), SizeBytes: uint64(len(payload)),
		Visibility: protocol.VisibilityPublic, Metadata: map[string]string{"phase": "9b"},
	}, Payload: append([]byte(nil), payload...)}
}

func TestRuntimeInternalObjectFetchAndCleanShutdown(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 12*time.Second)
	defer cancel()
	serverCfg := testRuntimeConfig(t, "object-server", true, true)
	clientCfg := testRuntimeConfig(t, "object-client", true, false)
	clientCfg.GenesisID = serverCfg.GenesisID
	clientCfg.P2P.NetworkFingerprint = serverCfg.GenesisID
	server, err := New(serverCfg)
	if err != nil {
		t.Fatal(err)
	}
	client, err := New(clientCfg)
	if err != nil {
		t.Fatal(err)
	}
	if err := server.Start(ctx); err != nil {
		t.Fatal(err)
	}
	if err := client.Start(ctx); err != nil {
		t.Fatal(err)
	}
	defer server.Stop(context.Background())
	defer client.Stop(context.Background())
	if len(client.P2P().ListenAddresses()) != 0 {
		t.Fatal("runtime client is not outbound-only")
	}
	if err := client.P2P().Dial(ctx, server.P2P().FullAddresses()[0].String(), p2p.SourceManual); err != nil {
		t.Fatal(err)
	}

	object := runtimeTestObject([]byte("runtime internal object retrieval"))
	id, _, err := server.ObjectStore().Put(ctx, object)
	if err != nil {
		t.Fatal(err)
	}
	client.mu.RLock()
	before, _ := client.state.Hash()
	client.mu.RUnlock()
	got, err := client.FetchObject(ctx, id)
	if err != nil || !bytes.Equal(got.Payload, object.Payload) {
		t.Fatalf("runtime FetchObject = %q, %v", got.Payload, err)
	}
	client.mu.RLock()
	after, _ := client.state.Hash()
	client.mu.RUnlock()
	if before.String() != after.String() {
		t.Fatal("runtime object fetch changed canonical StateHash/AppHash")
	}
	if err := client.Stop(context.Background()); err != nil {
		t.Fatal(err)
	}
	if _, err := client.FetchObject(ctx, id); err == nil {
		t.Fatal("stopped runtime retained active object fetch service")
	}
}

func TestRuntimeObjectServiceRepeatedStartStop(t *testing.T) {
	for i := 0; i < 3; i++ {
		cfg := testRuntimeConfig(t, "object-cycle", true, false)
		runtime, err := New(cfg)
		if err != nil {
			t.Fatal(err)
		}
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		if err := runtime.Start(ctx); err != nil {
			cancel()
			t.Fatal(err)
		}
		if err := runtime.Stop(context.Background()); err != nil {
			cancel()
			t.Fatal(err)
		}
		cancel()
	}
}
