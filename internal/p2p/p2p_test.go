package p2p

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/kokora3/zion/internal/chain"
	"github.com/kokora3/zion/internal/protocol"
	libcrypto "github.com/libp2p/go-libp2p/core/crypto"
	libpeer "github.com/libp2p/go-libp2p/core/peer"
	ma "github.com/multiformats/go-multiaddr"
)

func fixtureFingerprint(label string) protocol.HashDigest { return protocol.HashBytes([]byte(label)) }

func writeTestKey(t testing.TB, path, label string) libpeer.ID {
	t.Helper()
	// TEST ONLY - PUBLIC FIXTURE - NEVER USE IN PRODUCTION.
	seed := sha256.Sum256([]byte("zion-phase-7-test-key:" + label))
	key, err := libcrypto.UnmarshalEd25519PrivateKey(ed25519.NewKeyFromSeed(seed[:]))
	if err != nil {
		t.Fatal(err)
	}
	encoded, err := libcrypto.MarshalPrivateKey(key)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, encoded, 0o600); err != nil {
		t.Fatal(err)
	}
	peerID, err := libpeer.IDFromPrivateKey(key)
	if err != nil {
		t.Fatal(err)
	}
	return peerID
}

func testConfig(t testing.TB, name string, listen bool) Config {
	t.Helper()
	directory := t.TempDir()
	cfg := DefaultConfig(directory, fixtureFingerprint("alpha-genesis"))
	cfg.KeyPath = filepath.Join(directory, "p2p", "peer.key")
	cfg.PeerCachePath = filepath.Join(directory, "p2p", "peers.json")
	writeTestKey(t, cfg.KeyPath, name)
	if listen {
		cfg.ListenAddresses = []string{"/ip4/127.0.0.1/udp/0/quic-v1"}
	} else {
		cfg.ListenAddresses = nil
	}
	cfg.Limits.DialTimeout = time.Second
	cfg.Limits.HandshakeTimeout = 2 * time.Second
	cfg.Limits.PEXTimeout = 2 * time.Second
	cfg.Limits.BackoffInitial = 10 * time.Millisecond
	cfg.Limits.BackoffMaximum = 100 * time.Millisecond
	return cfg
}

func TestPersistentPeerIdentityAndCanonicalIsolation(t *testing.T) {
	directory := t.TempDir()
	path := filepath.Join(directory, "p2p", "peer.key")
	first, err := LoadOrCreatePeerKey(path)
	if err != nil {
		t.Fatal(err)
	}
	second, err := LoadOrCreatePeerKey(path)
	if err != nil {
		t.Fatal(err)
	}
	firstID, _ := libpeer.IDFromPrivateKey(first)
	secondID, _ := libpeer.IDFromPrivateKey(second)
	if firstID != secondID {
		t.Fatal("PeerID changed across key reload")
	}
	state := chain.Genesis(protocol.Alpha1NetworkID)
	before, _ := state.Hash()
	otherPath := filepath.Join(directory, "other", "peer.key")
	other, err := LoadOrCreatePeerKey(otherPath)
	if err != nil {
		t.Fatal(err)
	}
	otherID, _ := libpeer.IDFromPrivateKey(other)
	if otherID == firstID {
		t.Fatal("different P2P keys produced the same PeerID")
	}
	after, _ := state.Hash()
	if before.String() != after.String() {
		t.Fatal("local P2P identity changed canonical StateHash")
	}
	if strings.HasPrefix(firstID.String(), "zion:id:") {
		t.Fatal("PeerID was conflated with IdentityID")
	}
}

func TestVersionNegotiationIsTyped(t *testing.T) {
	selected, err := negotiateVersion([]Version{{0, 1, 2}, {0, 10, 0}}, []Version{{0, 10, 0}, {0, 1, 2}})
	if err != nil || selected != (Version{0, 10, 0}) {
		t.Fatalf("typed negotiation selected %v: %v", selected, err)
	}
	if _, err := negotiateVersion([]Version{{0, 1, 0}}, []Version{{1, 0, 0}}); err == nil {
		t.Fatal("incompatible versions accepted")
	}
}

func TestHelloCompatibilityAndMalformedInputs(t *testing.T) {
	peerID := writeTestKey(t, filepath.Join(t.TempDir(), "peer.key"), "hello")
	valid := Hello{WireSchema, protocol.Alpha1NetworkID, fixtureFingerprint("alpha-genesis"),
		[]Version{CurrentVersion}, []Role{RoleBootstrap, RoleNormal}, peerID.String(),
		[]string{"/ip4/127.0.0.1/udp/42000/quic-v1"}}
	encoded, err := EncodeHello(valid)
	if err != nil {
		t.Fatal(err)
	}
	decoded, err := DecodeHello(encoded)
	if err != nil || decoded.AuthenticatedPeerID != peerID.String() {
		t.Fatalf("valid hello failed: %v", err)
	}
	wrongAuthenticated := writeTestKey(t, filepath.Join(t.TempDir(), "other.key"), "hello-other")
	if err := validateHello(valid, wrongAuthenticated); err == nil {
		t.Fatal("self-reported PeerID mismatch was accepted")
	}
	cases := [][]byte{{}, {0xff}, encoded[:len(encoded)-1], bytes.Repeat([]byte{0}, MaxHelloSize+1)}
	for i, input := range cases {
		if _, err := DecodeHello(input); err == nil {
			t.Fatalf("malformed hello %d accepted", i)
		}
	}
	mutations := []func(*Hello){
		func(h *Hello) { h.SchemaVersion++ },
		func(h *Hello) { h.AuthenticatedPeerID = "invalid" },
		func(h *Hello) { h.Roles = []Role{"STORAGE"} },
		func(h *Hello) { h.Roles = []Role{RoleNormal, RoleNormal} },
		func(h *Hello) { h.AdvertisedAddresses = []string{"not-a-multiaddr"} },
		func(h *Hello) {
			h.AdvertisedAddresses = []string{"/ip4/127.0.0.1/udp/42000/quic-v1/p2p/" + wrongAuthenticated.String()}
		},
		func(h *Hello) { h.SupportedVersions = nil },
	}
	for i, mutate := range mutations {
		candidate := valid
		mutate(&candidate)
		if _, err := EncodeHello(candidate); err == nil {
			t.Fatalf("invalid hello mutation %d accepted", i)
		}
	}
}

func TestDocumentedLocalOnlyGenesisCannotJoinSharedAlpha(t *testing.T) {
	peerID := writeTestKey(t, filepath.Join(t.TempDir(), "peer.key"), "shared-genesis-separation")
	sharedBytes, err := hex.DecodeString("72ef0c7816d64255fc7b1da266c6a5b5ca345dd89cba2423fc8cfc8b4d1a61c6")
	if err != nil {
		t.Fatal(err)
	}
	localBytes, err := hex.DecodeString("4cce10c9eb93aa5baff6ec94b13ff27662464668764a39efed6d59373a487d55")
	if err != nil {
		t.Fatal(err)
	}
	shared := protocol.HashDigest{Algorithm: protocol.HashAlgorithmSHA256, Digest: sharedBytes}
	local := protocol.HashDigest{Algorithm: protocol.HashAlgorithmSHA256, Digest: localBytes}
	node := &Node{cfg: Config{NetworkID: protocol.Alpha1NetworkID, NetworkFingerprint: shared,
		SupportedVersions: []Version{CurrentVersion}}}
	hello := Hello{SchemaVersion: WireSchema, NetworkID: protocol.Alpha1NetworkID, NetworkFingerprint: local,
		SupportedVersions: []Version{CurrentVersion}, Roles: []Role{RoleNormal}, AuthenticatedPeerID: peerID.String(),
		AdvertisedAddresses: []string{}}
	if _, _, err := node.validateRemoteHello(hello, peerID); err == nil || !strings.Contains(err.Error(), "wrong ZION network fingerprint") {
		t.Fatalf("local-only genesis was considered shared-alpha compatible: %v", err)
	}
}

func TestPEXValidationMatrix(t *testing.T) {
	peerA := writeTestKey(t, filepath.Join(t.TempDir(), "a.key"), "pex-a")
	peerB := writeTestKey(t, filepath.Join(t.TempDir(), "b.key"), "pex-b")
	ids := []string{peerA.String(), peerB.String()}
	if ids[1] < ids[0] {
		ids[0], ids[1] = ids[1], ids[0]
	}
	valid := PEXResponse{SchemaVersion: WireSchema, Peers: []PeerRecord{
		{PeerID: ids[0], Addresses: []string{"/ip4/127.0.0.1/udp/41001/quic-v1"}, Roles: []Role{RoleNormal}},
		{PeerID: ids[1], Addresses: []string{"/ip4/127.0.0.1/udp/41002/quic-v1"}, Roles: []Role{RoleBootstrap, RoleNormal}},
	}}
	data, err := EncodePEX(valid)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := DecodePEX(data); err != nil {
		t.Fatal(err)
	}
	if _, err := EncodePEX(PEXResponse{SchemaVersion: WireSchema}); err != nil {
		t.Fatalf("empty bounded PEX response rejected: %v", err)
	}
	bad := valid
	bad.Peers = append(append([]PeerRecord(nil), valid.Peers...), valid.Peers[0])
	if _, err := EncodePEX(bad); err == nil {
		t.Fatal("duplicate PEX PeerID accepted")
	}
	bad = valid
	bad.Peers = []PeerRecord{{PeerID: "bad", Addresses: []string{"/ip4/127.0.0.1/udp/1/quic-v1"}, Roles: []Role{RoleNormal}}}
	if _, err := EncodePEX(bad); err == nil {
		t.Fatal("malformed PEX PeerID accepted")
	}
	bad = valid
	bad.Peers = []PeerRecord{{PeerID: peerA.String(), Addresses: []string{"bad"}, Roles: []Role{RoleNormal}}}
	if _, err := EncodePEX(bad); err == nil {
		t.Fatal("malformed PEX multiaddr accepted")
	}
	tooMany := make([]PeerRecord, HardMaxPEXPeersPerResponse+1)
	bad = PEXResponse{SchemaVersion: WireSchema, Peers: tooMany}
	if _, err := EncodePEX(bad); err == nil {
		t.Fatal("oversized PEX peer list accepted")
	}
	if err := validatePEX(valid, peerA, HardMaxPEXPeersPerResponse, HardMaxAddressesPerPeer); err == nil {
		t.Fatal("self PeerID accepted from PEX")
	}
	addresses := make([]string, HardMaxAddressesPerPeer+1)
	for i := range addresses {
		addresses[i] = fmt.Sprintf("/ip4/127.0.0.1/udp/%d/quic-v1", 43000+i)
	}
	bad = PEXResponse{SchemaVersion: WireSchema, Peers: []PeerRecord{{PeerID: peerA.String(), Addresses: addresses, Roles: []Role{RoleNormal}}}}
	if _, err := EncodePEX(bad); err == nil {
		t.Fatal("too many PEX addresses accepted")
	}
}

func TestPeerCachePersistenceNormalizationAndCorruption(t *testing.T) {
	cfg := testConfig(t, "cache", true)
	cache, err := OpenPeerCache(cfg.PeerCachePath, cfg.Limits)
	if err != nil {
		t.Fatal(err)
	}
	peerID := writeTestKey(t, filepath.Join(t.TempDir(), "remote.key"), "cache-remote")
	address, _ := ma.NewMultiaddr("/ip4/127.0.0.1/udp/42000/quic-v1")
	if err := cache.markSuccess(peerID, []ma.Multiaddr{address, address}, SourcePEX); err != nil {
		t.Fatal(err)
	}
	reloaded, err := OpenPeerCache(cfg.PeerCachePath, cfg.Limits)
	if err != nil {
		t.Fatal(err)
	}
	candidates := reloaded.SuccessfulCandidates("")
	if len(candidates) != 1 || len(candidates[0].Addresses) != 1 {
		t.Fatalf("cache was not normalized: %+v", candidates)
	}
	if err := os.WriteFile(cfg.PeerCachePath, []byte("{broken"), 0o600); err != nil {
		t.Fatal(err)
	}
	corrupt, err := OpenPeerCache(cfg.PeerCachePath, cfg.Limits)
	if err == nil || corrupt == nil || len(corrupt.SuccessfulCandidates("")) != 0 {
		t.Fatal("corrupt cache did not fail safely to an empty cache")
	}
	if err := os.WriteFile(cfg.PeerCachePath, bytes.Repeat([]byte("x"), MaxPeerCacheFileSize+1), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := OpenPeerCache(cfg.PeerCachePath, cfg.Limits); err == nil {
		t.Fatal("oversized cache accepted")
	}
}

func TestPeerCacheFailureBackoffIsBoundedAndPersisted(t *testing.T) {
	cfg := testConfig(t, "cache-backoff", false)
	cache, err := OpenPeerCache(cfg.PeerCachePath, cfg.Limits)
	if err != nil {
		t.Fatal(err)
	}
	fixed := time.Unix(1_700_000_000, 0)
	cache.now = func() time.Time { return fixed }
	peerID := writeTestKey(t, filepath.Join(t.TempDir(), "unreachable.key"), "unreachable")
	address, _ := ma.NewMultiaddr("/ip4/127.0.0.1/udp/1/quic-v1")
	for range 40 {
		if err := cache.markFailure(peerID, []ma.Multiaddr{address}, SourceBootstrap); err != nil {
			t.Fatal(err)
		}
	}
	if !cache.backoffActive(peerID, []ma.Multiaddr{address}) {
		t.Fatal("failed peer was not placed in dial backoff")
	}
	record := cache.entries[peerID]
	if record.Successful || record.Failures != 32 {
		t.Fatalf("unexpected failed-peer cache record: %+v", record)
	}
	if got := time.UnixMilli(record.NextAttemptUnixMS).Sub(fixed); got != cfg.Limits.BackoffMaximum {
		t.Fatalf("backoff was not capped: %v", got)
	}
	reloaded, err := OpenPeerCache(cfg.PeerCachePath, cfg.Limits)
	if err != nil {
		t.Fatal(err)
	}
	reloaded.now = cache.now
	if !reloaded.backoffActive(peerID, []ma.Multiaddr{address}) || len(reloaded.SuccessfulCandidates("")) != 0 {
		t.Fatal("failed candidate was trusted or its backoff was not persisted")
	}
}

func TestNodeContinuesWithCorruptedCache(t *testing.T) {
	cfg := testConfig(t, "corrupt-cache-node", false)
	if err := os.MkdirAll(filepath.Dir(cfg.PeerCachePath), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(cfg.PeerCachePath, []byte("not-json"), 0o600); err != nil {
		t.Fatal(err)
	}
	node, err := NewNode(context.Background(), cfg)
	if err != nil {
		t.Fatal("corrupted local cache crashed node construction:", err)
	}
	defer node.Close()
	if node.CacheLoadError() == nil || len(node.cache.SuccessfulCandidates(node.PeerID())) != 0 {
		t.Fatal("corrupt cache was not isolated as empty local state")
	}
}

type helloGolden struct {
	SchemaVersion        uint64    `json:"schema_version"`
	NetworkID            string    `json:"network_id"`
	FingerprintAlgorithm string    `json:"network_fingerprint_algorithm"`
	FingerprintHex       string    `json:"network_fingerprint_hex"`
	Versions             []Version `json:"supported_versions"`
	Roles                []Role    `json:"roles"`
	PeerID               string    `json:"authenticated_peer_id"`
	Addresses            []string  `json:"advertised_addresses"`
	ProtocolID           string    `json:"protocol_id"`
	CanonicalCBORHex     string    `json:"canonical_cbor_hex"`
}

type pexGolden struct {
	SchemaVersion    uint64       `json:"schema_version"`
	ProtocolID       string       `json:"protocol_id"`
	Peers            []PeerRecord `json:"peers"`
	CanonicalCBORHex string       `json:"canonical_cbor_hex"`
}

func TestWireGoldenFixtures(t *testing.T) {
	var helloFixture helloGolden
	loadJSON(t, "testdata/hello_golden.json", &helloFixture)
	fingerprint, err := hex.DecodeString(helloFixture.FingerprintHex)
	if err != nil {
		t.Fatal(err)
	}
	hello := Hello{helloFixture.SchemaVersion, protocol.NetworkID(helloFixture.NetworkID),
		protocol.HashDigest{Algorithm: protocol.HashAlgorithmSHA256, Digest: fingerprint}, helloFixture.Versions,
		helloFixture.Roles, helloFixture.PeerID, helloFixture.Addresses}
	helloBytes, err := EncodeHello(hello)
	if err != nil {
		t.Fatal(err)
	}
	if helloFixture.FingerprintAlgorithm != string(protocol.HashAlgorithmSHA256) {
		t.Fatal("hello golden hash algorithm mismatch")
	}
	if hex.EncodeToString(helloBytes) != helloFixture.CanonicalCBORHex || helloFixture.ProtocolID != string(HelloProtocolID) {
		t.Fatal("hello golden mismatch")
	}
	if decoded, err := DecodeHello(helloBytes); err != nil || !reflect.DeepEqual(decoded, hello) {
		t.Fatalf("hello golden decode: %v", err)
	}

	var pexFixture pexGolden
	loadJSON(t, "testdata/pex_golden.json", &pexFixture)
	pex := PEXResponse{SchemaVersion: pexFixture.SchemaVersion, Peers: pexFixture.Peers}
	pexBytes, err := EncodePEX(pex)
	if err != nil {
		t.Fatal(err)
	}
	if hex.EncodeToString(pexBytes) != pexFixture.CanonicalCBORHex || pexFixture.ProtocolID != string(PEXProtocolID) {
		t.Fatal("PEX golden mismatch")
	}
	if decoded, err := DecodePEX(pexBytes); err != nil || !reflect.DeepEqual(decoded, pex) {
		t.Fatalf("PEX golden decode: %v", err)
	}
}

func loadJSON(t testing.TB, path string, target any) {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(data, target); err != nil {
		t.Fatal(err)
	}
}

func FuzzDecodeHello(f *testing.F) {
	f.Add([]byte{})
	f.Add(bytes.Repeat([]byte{0}, MaxHelloSize+1))
	var fixture helloGolden
	if data, err := os.ReadFile("testdata/hello_golden.json"); err == nil && json.Unmarshal(data, &fixture) == nil {
		if canonical, err := hex.DecodeString(fixture.CanonicalCBORHex); err == nil {
			f.Add(canonical)
		}
	}
	f.Fuzz(func(t *testing.T, data []byte) { _, _ = DecodeHello(data) })
}

func FuzzDecodePEX(f *testing.F) {
	f.Add([]byte{})
	f.Add(bytes.Repeat([]byte{0}, MaxPEXResponseSize+1))
	var fixture pexGolden
	if data, err := os.ReadFile("testdata/pex_golden.json"); err == nil && json.Unmarshal(data, &fixture) == nil {
		if canonical, err := hex.DecodeString(fixture.CanonicalCBORHex); err == nil {
			f.Add(canonical)
		}
	}
	f.Fuzz(func(t *testing.T, data []byte) { _, _ = DecodePEX(data) })
}

func TestSelfDialRejected(t *testing.T) {
	cfg := testConfig(t, "self", true)
	node, err := NewNode(context.Background(), cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer node.Close()
	if err := node.Dial(context.Background(), node.FullAddresses()[0].String(), SourceManual); err == nil {
		t.Fatal("self dial accepted")
	}
}
