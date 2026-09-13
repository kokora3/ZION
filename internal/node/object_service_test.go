package node

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"strings"
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

func TestRuntimeObjectAPIEndToEndRemoteFetchRestartAndStateIsolation(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	serverCfg := testRuntimeConfig(t, "object-api-server", true, true)
	clientCfg := testRuntimeConfig(t, "object-api-client", true, false)
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
	if err := client.P2P().Dial(ctx, server.P2P().FullAddresses()[0].String(), p2p.SourceManual); err != nil {
		t.Fatal(err)
	}
	object := runtimeTestObject([]byte("runtime API remote object retrieval"))
	id, _, err := server.ObjectStore().Put(ctx, object)
	if err != nil {
		t.Fatal(err)
	}
	client.mu.RLock()
	before, _ := client.state.Hash()
	client.mu.RUnlock()
	base := "http://" + client.APIAddress()
	localObject := runtimeTestObject([]byte("runtime API locally submitted object"))
	localRaw, _ := localObject.CanonicalBytes()
	localWrapper, _ := json.Marshal(map[string]string{"encoding": "base64", "object": base64.StdEncoding.EncodeToString(localRaw)})
	put := apiRequest(t, http.MethodPost, base+"/v1/objects", localWrapper)
	if put.StatusCode != http.StatusCreated || !strings.Contains(string(put.Body), `"status":"STORED"`) {
		t.Fatalf("put endpoint = %d %s", put.StatusCode, put.Body)
	}
	fetch := apiRequest(t, http.MethodPost, base+"/v1/objects/"+id.String()+"/fetch", nil)
	if fetch.StatusCode != http.StatusOK || !strings.Contains(string(fetch.Body), `"status":"FETCHED"`) {
		t.Fatalf("fetch endpoint = %d %s", fetch.StatusCode, fetch.Body)
	}
	get := apiRequest(t, http.MethodGet, base+"/v1/objects/"+id.String(), nil)
	if get.StatusCode != http.StatusOK {
		t.Fatalf("get endpoint = %d %s", get.StatusCode, get.Body)
	}
	var wrapper struct {
		Object string `json:"object"`
	}
	if err := json.Unmarshal(get.Body, &wrapper); err != nil {
		t.Fatal(err)
	}
	raw, _ := base64.StdEncoding.DecodeString(wrapper.Object)
	want, _ := object.CanonicalBytes()
	if !bytes.Equal(raw, want) {
		t.Fatal("runtime API GET changed canonical object bytes")
	}
	status := apiRequest(t, http.MethodGet, base+"/v1/status", nil)
	for _, field := range []string{"object_store_enabled", "object_count", "object_bytes", "object_quota_bytes"} {
		if !strings.Contains(string(status.Body), `"`+field+`"`) {
			t.Fatalf("status omits %s: %s", field, status.Body)
		}
	}
	if strings.Contains(string(status.Body), clientCfg.ObjectDirectory) {
		t.Fatal("status leaked local object-store directory")
	}
	client.mu.RLock()
	after, _ := client.state.Hash()
	client.mu.RUnlock()
	if before.String() != after.String() {
		t.Fatal("API fetch changed canonical StateHash/AppHash")
	}
	peerID := client.P2P().PeerID()
	if err := client.Stop(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err := server.Stop(context.Background()); err != nil {
		t.Fatal(err)
	}

	restarted, err := New(clientCfg)
	if err != nil {
		t.Fatal(err)
	}
	if err := restarted.Start(ctx); err != nil {
		t.Fatal(err)
	}
	defer restarted.Stop(context.Background())
	if restarted.P2P().PeerID() != peerID {
		t.Fatal("P2P PeerID changed across object-store restart")
	}
	get = apiRequest(t, http.MethodGet, "http://"+restarted.APIAddress()+"/v1/objects/"+id.String(), nil)
	if get.StatusCode != http.StatusOK {
		t.Fatalf("restart local GET = %d %s", get.StatusCode, get.Body)
	}
	restarted.mu.RLock()
	restartedHash, _ := restarted.state.Hash()
	restarted.mu.RUnlock()
	if restartedHash.String() != before.String() {
		t.Fatal("restart object persistence changed canonical StateHash")
	}
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
