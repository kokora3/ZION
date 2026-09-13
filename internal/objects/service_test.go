package objects

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/kokora3/zion/internal/chain"
	"github.com/kokora3/zion/internal/p2p"
	"github.com/kokora3/zion/internal/protocol"
	libnetwork "github.com/libp2p/go-libp2p/core/network"
	libpeer "github.com/libp2p/go-libp2p/core/peer"
)

type objectPeer struct {
	node    *p2p.Node
	store   *Store
	service *Service
}

func objectPeerConfig(t testing.TB, name string, listen bool, fingerprint protocol.HashDigest) p2p.Config {
	t.Helper()
	dir := t.TempDir()
	cfg := p2p.DefaultConfig(dir, fingerprint)
	cfg.KeyPath = filepath.Join(dir, name, "peer.key")
	cfg.PeerCachePath = filepath.Join(dir, name, "peers.json")
	if listen {
		cfg.ListenAddresses = []string{"/ip4/127.0.0.1/udp/0/quic-v1"}
	} else {
		cfg.ListenAddresses = nil
	}
	cfg.Limits.DialTimeout = 2 * time.Second
	cfg.Limits.HandshakeTimeout = 2 * time.Second
	cfg.Limits.PEXTimeout = 2 * time.Second
	return cfg
}

func newObjectPeer(t testing.TB, ctx context.Context, name string, listen bool, fingerprint protocol.HashDigest, quota uint64, withService bool, roles ...p2p.Role) *objectPeer {
	t.Helper()
	cfg := objectPeerConfig(t, name, listen, fingerprint)
	if len(roles) > 0 {
		cfg.Roles = append([]p2p.Role(nil), roles...)
	}
	node, err := p2p.NewNode(ctx, cfg)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = node.Close() })
	store, err := Open(filepath.Join(t.TempDir(), "objects"), quota)
	if err != nil {
		t.Fatal(err)
	}
	peer := &objectPeer{node: node, store: store}
	if withService {
		service, err := NewService(ctx, node, store, protocol.Alpha1NetworkID, DefaultFetchConfig())
		if err != nil {
			t.Fatal(err)
		}
		peer.service = service
		t.Cleanup(service.Close)
	}
	return peer
}

func fullAddress(t testing.TB, node *p2p.Node) string {
	t.Helper()
	addresses := node.FullAddresses()
	if len(addresses) == 0 {
		t.Fatal("peer has no full address")
	}
	return addresses[0].String()
}

func connectObjectPeers(t testing.TB, ctx context.Context, from, to *p2p.Node) {
	t.Helper()
	if err := from.Dial(ctx, fullAddress(t, to), p2p.SourceManual); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if from.IsUsable(to.PeerID()) && to.IsUsable(from.PeerID()) {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatalf("peers %s and %s not mutually usable", from.PeerID(), to.PeerID())
}

func TestDirectOutboundFetchAndStateHashIsolation(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	fingerprint := protocol.HashBytes([]byte("phase-9b-direct"))
	a := newObjectPeer(t, ctx, "a-outbound", false, fingerprint, 0, true)
	b := newObjectPeer(t, ctx, "b", true, fingerprint, 0, true)
	if len(a.node.ListenAddresses()) != 0 {
		t.Fatal("outbound-only requester is listening")
	}
	connectObjectPeers(t, ctx, a.node, b.node)
	object := testObject([]byte("direct outbound content"), "direct")
	id, _, err := b.store.Put(ctx, object)
	if err != nil {
		t.Fatal(err)
	}
	state := chain.Genesis(protocol.Alpha1NetworkID)
	before, _ := state.Hash()
	got, err := a.service.FetchObject(ctx, id)
	if err != nil || !bytes.Equal(got.Payload, object.Payload) {
		t.Fatalf("FetchObject = %q, %v", got.Payload, err)
	}
	stored, err := a.store.Get(ctx, id)
	if err != nil || !bytes.Equal(stored.Payload, object.Payload) {
		t.Fatalf("fetched object not stored: %v", err)
	}
	after, _ := state.Hash()
	if before.String() != after.String() {
		t.Fatalf("off-chain fetch changed StateHash: %s != %s", before.String(), after.String())
	}
}

func installResponseHandler(node *p2p.Node, networkID protocol.NetworkID, responder func(ObjectGetRequest) ObjectGetResponse) {
	node.Host().SetStreamHandler(ObjectProtocolID, func(stream libnetwork.Stream) {
		defer stream.Close()
		data, err := readObjectFrame(stream, MaxObjectRequestFrame)
		if err != nil {
			_ = stream.Reset()
			return
		}
		request, err := DecodeObjectGetRequest(data, networkID)
		if err != nil {
			_ = stream.Reset()
			return
		}
		response := responder(request)
		// Deliberately use canonical encoding directly so security tests can
		// construct semantically invalid but canonically encoded responses.
		encoded, err := protocol.CanonicalEncode(response)
		if err != nil || writeObjectFrame(stream, encoded, MaxObjectResponseFrame) != nil {
			_ = stream.Reset()
		}
	})
}

func TestNotFoundAndAuthenticatedMaliciousPeerFallback(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 12*time.Second)
	defer cancel()
	fingerprint := protocol.HashBytes([]byte("phase-9b-malicious"))
	a := newObjectPeer(t, ctx, "requester", false, fingerprint, 0, false)
	b := newObjectPeer(t, ctx, "bootstrap-not-found", true, fingerprint, 0, true)
	c := newObjectPeer(t, ctx, "validator-bootstrap-malicious", true, fingerprint, 0, false,
		p2p.RoleNormal, p2p.RoleValidator, p2p.RoleBootstrap)
	d := newObjectPeer(t, ctx, "correct", true, fingerprint, 0, true)

	target := testObject([]byte("correct target"), "target")
	targetID, _, err := d.store.Put(ctx, target)
	if err != nil {
		t.Fatal(err)
	}
	wrong := testObject([]byte("authenticated peer lied"), "wrong")
	wrongBytes, _ := wrong.CanonicalBytes()
	installResponseHandler(c.node, protocol.Alpha1NetworkID, func(request ObjectGetRequest) ObjectGetResponse {
		return ObjectGetResponse{SchemaVersion: ObjectWireSchema, NetworkID: protocol.Alpha1NetworkID,
			Status: ObjectFound, ObjectID: request.ObjectID, ObjectBytes: wrongBytes}
	})
	for _, remote := range []*objectPeer{b, c, d} {
		connectObjectPeers(t, ctx, a.node, remote.node)
	}
	cfg := DefaultFetchConfig()
	service, err := NewService(ctx, a.node, a.store, protocol.Alpha1NetworkID, cfg)
	if err != nil {
		t.Fatal(err)
	}
	service.candidatePeers = func() []libpeer.ID {
		return []libpeer.ID{b.node.PeerID(), c.node.PeerID(), c.node.PeerID(), d.node.PeerID()}
	}
	defer service.Close()
	got, err := service.FetchObject(ctx, targetID)
	if err != nil || !bytes.Equal(got.Payload, target.Payload) {
		t.Fatalf("fallback failed: %q %v", got.Payload, err)
	}
	wrongID, _ := wrong.ObjectID()
	hasWrong, err := a.store.Has(ctx, wrongID)
	if err != nil || hasWrong {
		t.Fatalf("malicious unrelated object was stored: %v %v", hasWrong, err)
	}
}

func TestFetchSingleflightDifferentIDsCancellationAndQuota(t *testing.T) {
	t.Run("singleflight", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		fingerprint := protocol.HashBytes([]byte("phase-9b-singleflight"))
		a := newObjectPeer(t, ctx, "a", false, fingerprint, 0, false)
		b := newObjectPeer(t, ctx, "b", true, fingerprint, 0, false)
		connectObjectPeers(t, ctx, a.node, b.node)
		object := testObject([]byte("single network transfer"), "singleflight")
		id, _ := object.ObjectID()
		objectBytes, _ := object.CanonicalBytes()
		var requests atomic.Int32
		release := make(chan struct{})
		installResponseHandler(b.node, protocol.Alpha1NetworkID, func(request ObjectGetRequest) ObjectGetResponse {
			requests.Add(1)
			<-release
			return ObjectGetResponse{1, protocol.Alpha1NetworkID, ObjectFound, request.ObjectID, objectBytes}
		})
		cfg := DefaultFetchConfig()
		service, err := NewService(ctx, a.node, a.store, protocol.Alpha1NetworkID, cfg)
		if err != nil {
			t.Fatal(err)
		}
		service.candidatePeers = func() []libpeer.ID { return []libpeer.ID{b.node.PeerID()} }
		defer service.Close()
		const callers = 100
		errs := make(chan error, callers)
		var wg sync.WaitGroup
		for i := 0; i < callers; i++ {
			wg.Add(1)
			go func() { defer wg.Done(); _, err := service.FetchObject(ctx, id); errs <- err }()
		}
		deadline := time.Now().Add(2 * time.Second)
		for requests.Load() == 0 && time.Now().Before(deadline) {
			time.Sleep(time.Millisecond)
		}
		close(release)
		wg.Wait()
		close(errs)
		for err := range errs {
			if err != nil {
				t.Fatal(err)
			}
		}
		if requests.Load() != 1 {
			t.Fatalf("network requests = %d, want 1", requests.Load())
		}
	})

	t.Run("different IDs", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		fingerprint := protocol.HashBytes([]byte("phase-9b-independent"))
		a := newObjectPeer(t, ctx, "a", false, fingerprint, 0, false)
		b := newObjectPeer(t, ctx, "b", true, fingerprint, 0, false)
		connectObjectPeers(t, ctx, a.node, b.node)
		objectsByID := map[string][]byte{}
		ids := make([]protocol.ObjectID, 0, 2)
		for _, value := range []string{"x", "y"} {
			object := testObject([]byte(value), "independent-"+value)
			id, _ := object.ObjectID()
			raw, _ := object.CanonicalBytes()
			ids = append(ids, id)
			objectsByID[id.String()] = raw
		}
		var active, maximum atomic.Int32
		installResponseHandler(b.node, protocol.Alpha1NetworkID, func(request ObjectGetRequest) ObjectGetResponse {
			current := active.Add(1)
			for {
				previous := maximum.Load()
				if current <= previous || maximum.CompareAndSwap(previous, current) {
					break
				}
			}
			time.Sleep(30 * time.Millisecond)
			active.Add(-1)
			return ObjectGetResponse{1, protocol.Alpha1NetworkID, ObjectFound, request.ObjectID, objectsByID[request.ObjectID]}
		})
		cfg := DefaultFetchConfig()
		service, err := NewService(ctx, a.node, a.store, protocol.Alpha1NetworkID, cfg)
		if err != nil {
			t.Fatal(err)
		}
		service.candidatePeers = func() []libpeer.ID { return []libpeer.ID{b.node.PeerID()} }
		defer service.Close()
		errs := make(chan error, 2)
		for _, id := range ids {
			id := id
			go func() { _, err := service.FetchObject(ctx, id); errs <- err }()
		}
		for range ids {
			if err := <-errs; err != nil {
				t.Fatal(err)
			}
		}
		if maximum.Load() < 2 {
			t.Fatalf("different ObjectIDs were unnecessarily serialized, max=%d", maximum.Load())
		}
	})

	t.Run("cancellation", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		fingerprint := protocol.HashBytes([]byte("phase-9b-cancel"))
		a := newObjectPeer(t, ctx, "a", false, fingerprint, 0, false)
		b := newObjectPeer(t, ctx, "b", true, fingerprint, 0, false)
		connectObjectPeers(t, ctx, a.node, b.node)
		object := testObject([]byte("cancel"), "cancel")
		id, _ := object.ObjectID()
		installResponseHandler(b.node, protocol.Alpha1NetworkID, func(request ObjectGetRequest) ObjectGetResponse {
			time.Sleep(200 * time.Millisecond)
			return ObjectGetResponse{1, protocol.Alpha1NetworkID, ObjectNotFound, request.ObjectID, []byte{}}
		})
		cfg := DefaultFetchConfig()
		cfg.Timeout = 300 * time.Millisecond
		cfg.PeerTimeout = 250 * time.Millisecond
		service, err := NewService(ctx, a.node, a.store, protocol.Alpha1NetworkID, cfg)
		if err != nil {
			t.Fatal(err)
		}
		service.candidatePeers = func() []libpeer.ID { return []libpeer.ID{b.node.PeerID()} }
		defer service.Close()
		callerCtx, stop := context.WithTimeout(ctx, 20*time.Millisecond)
		defer stop()
		if _, err := service.FetchObject(callerCtx, id); !errors.Is(err, context.DeadlineExceeded) {
			t.Fatalf("cancellation error = %v", err)
		}
		has, err := a.store.Has(ctx, id)
		if err != nil || has {
			t.Fatalf("cancelled fetch promoted object: %v %v", has, err)
		}
	})

	t.Run("quota", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		fingerprint := protocol.HashBytes([]byte("phase-9b-quota"))
		a := newObjectPeer(t, ctx, "a", false, fingerprint, 1, true)
		b := newObjectPeer(t, ctx, "b", true, fingerprint, 0, true)
		connectObjectPeers(t, ctx, a.node, b.node)
		object := testObject([]byte("larger than one byte"), "quota-fetch")
		id, _, err := b.store.Put(ctx, object)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := a.service.FetchObject(ctx, id); !errors.Is(err, ErrStoreFull) {
			t.Fatalf("quota fetch error = %v", err)
		}
	})
}

func TestTimedOutPeerFallsBackToNextPeer(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel()
	fingerprint := protocol.HashBytes([]byte("phase-9b-timeout-fallback"))
	a := newObjectPeer(t, ctx, "a", false, fingerprint, 0, false)
	slow := newObjectPeer(t, ctx, "slow", true, fingerprint, 0, false)
	good := newObjectPeer(t, ctx, "good", true, fingerprint, 0, true)
	connectObjectPeers(t, ctx, a.node, slow.node)
	connectObjectPeers(t, ctx, a.node, good.node)
	object := testObject([]byte("fallback after timeout"), "timeout")
	id, _, err := good.store.Put(ctx, object)
	if err != nil {
		t.Fatal(err)
	}
	installResponseHandler(slow.node, protocol.Alpha1NetworkID, func(request ObjectGetRequest) ObjectGetResponse {
		time.Sleep(200 * time.Millisecond)
		return ObjectGetResponse{1, protocol.Alpha1NetworkID, ObjectNotFound, request.ObjectID, []byte{}}
	})
	cfg := DefaultFetchConfig()
	cfg.Timeout = time.Second
	cfg.PeerTimeout = 40 * time.Millisecond
	service, err := NewService(ctx, a.node, a.store, protocol.Alpha1NetworkID, cfg)
	if err != nil {
		t.Fatal(err)
	}
	service.candidatePeers = func() []libpeer.ID { return []libpeer.ID{slow.node.PeerID(), good.node.PeerID()} }
	defer service.Close()
	got, err := service.FetchObject(ctx, id)
	if err != nil || !bytes.Equal(got.Payload, object.Payload) {
		t.Fatalf("timeout fallback = %q, %v", got.Payload, err)
	}
}

func TestBootstrapDisappearanceDoesNotBlockDirectObjectFetch(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	fingerprint := protocol.HashBytes([]byte("phase-9b-bootstrap-loss"))
	bootstrapCfg := objectPeerConfig(t, "bootstrap", true, fingerprint)
	bootstrapCfg.Roles = []p2p.Role{p2p.RoleBootstrap, p2p.RoleNormal}
	bootstrapNode, err := p2p.NewNode(ctx, bootstrapCfg)
	if err != nil {
		t.Fatal(err)
	}
	defer bootstrapNode.Close()
	c := newObjectPeer(t, ctx, "c", true, fingerprint, 0, true)
	connectObjectPeers(t, ctx, c.node, bootstrapNode)

	aCfg := objectPeerConfig(t, "a", false, fingerprint)
	aCfg.BootstrapAddresses = []string{fullAddress(t, bootstrapNode)}
	aCfg.Limits.TargetOutboundPeers = 2
	aNode, err := p2p.NewNode(ctx, aCfg)
	if err != nil {
		t.Fatal(err)
	}
	defer aNode.Close()
	aStore := openTestStore(t, filepath.Join(t.TempDir(), "objects"), 0)
	aService, err := NewService(ctx, aNode, aStore, protocol.Alpha1NetworkID, DefaultFetchConfig())
	if err != nil {
		t.Fatal(err)
	}
	defer aService.Close()
	if err := aNode.Discover(ctx); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(3 * time.Second)
	for !aNode.IsUsable(c.node.PeerID()) && time.Now().Before(deadline) {
		time.Sleep(5 * time.Millisecond)
	}
	if !aNode.IsUsable(c.node.PeerID()) {
		t.Fatal("A did not discover C through bootstrap")
	}
	if err := bootstrapNode.Close(); err != nil {
		t.Fatal(err)
	}

	object := testObject([]byte("bootstrap is not the data path"), "bootstrap-loss")
	id, _, err := c.store.Put(ctx, object)
	if err != nil {
		t.Fatal(err)
	}
	got, err := aService.FetchObject(ctx, id)
	if err != nil || !bytes.Equal(got.Payload, object.Payload) {
		t.Fatalf("fetch after bootstrap loss: %q %v", got.Payload, err)
	}
}

func TestObjectServiceRejectsNoPeersAndInvalidID(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	fingerprint := protocol.HashBytes([]byte("phase-9b-errors"))
	a := newObjectPeer(t, ctx, "a", false, fingerprint, 0, true)
	id := protocol.ObjectID{HashDigest: protocol.HashBytes([]byte("absent"))}
	if _, err := a.service.FetchObject(ctx, id); !errors.Is(err, ErrNoAvailablePeers) {
		t.Fatalf("no peers error = %v", err)
	}
	invalid := protocol.ObjectID{HashDigest: protocol.HashDigest{Algorithm: "sha512", Digest: make([]byte, 32)}}
	if _, err := a.service.FetchObject(ctx, invalid); err == nil {
		t.Fatal("invalid ObjectID accepted")
	}
}

func TestFetchFailsClosedOnLocalCorruption(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	fingerprint := protocol.HashBytes([]byte("phase-9b-local-corruption"))
	a := newObjectPeer(t, ctx, "a", false, fingerprint, 0, true)
	object := testObject([]byte("corrupt locally"), "local-corrupt")
	id, _, err := a.store.Put(ctx, object)
	if err != nil {
		t.Fatal(err)
	}
	path, _ := a.store.pathForID(id)
	if err := os.WriteFile(path, []byte{0xa0}, 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := a.service.FetchObject(ctx, id); !errors.Is(err, ErrObjectCorrupt) {
		t.Fatalf("local corruption error = %v", err)
	}
}

func TestServiceCloseCancelsInFlightFetch(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	fingerprint := protocol.HashBytes([]byte("phase-9b-close"))
	a := newObjectPeer(t, ctx, "a", false, fingerprint, 0, false)
	b := newObjectPeer(t, ctx, "b", true, fingerprint, 0, false)
	connectObjectPeers(t, ctx, a.node, b.node)
	started := make(chan struct{})
	b.node.Host().SetStreamHandler(ObjectProtocolID, func(stream libnetwork.Stream) {
		defer stream.Close()
		if _, err := readObjectFrame(stream, MaxObjectRequestFrame); err != nil {
			return
		}
		close(started)
		var one [1]byte
		_, _ = stream.Read(one[:])
	})
	cfg := DefaultFetchConfig()
	service, err := NewService(ctx, a.node, a.store, protocol.Alpha1NetworkID, cfg)
	if err != nil {
		t.Fatal(err)
	}
	service.candidatePeers = func() []libpeer.ID { return []libpeer.ID{b.node.PeerID()} }
	id := protocol.ObjectID{HashDigest: protocol.HashBytes([]byte("missing-close"))}
	done := make(chan error, 1)
	go func() { _, err := service.FetchObject(ctx, id); done <- err }()
	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("fetch did not start")
	}
	closed := make(chan struct{})
	go func() { service.Close(); close(closed) }()
	select {
	case <-closed:
	case <-time.After(time.Second):
		t.Fatal("service Close hung with active fetch")
	}
	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("fetch close error = %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("fetch caller did not complete after Close")
	}
}
