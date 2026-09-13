package node

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/http"
	"os"
	"testing"
	"time"

	"github.com/kokora3/zion/internal/board"
	"github.com/kokora3/zion/internal/chain"
	"github.com/kokora3/zion/internal/identity"
	"github.com/kokora3/zion/internal/index"
	"github.com/kokora3/zion/internal/membership"
	"github.com/kokora3/zion/internal/objects"
	"github.com/kokora3/zion/internal/p2p"
	"github.com/kokora3/zion/internal/protocol"
	"github.com/kokora3/zion/internal/research"
	"github.com/kokora3/zion/internal/resources"
)

type boardAuthor struct {
	key   ed25519.PrivateKey
	id    identity.IdentityID
	keyID identity.KeyID
}

func runtimeBoardState(t testing.TB, authors int) (chain.State, []boardAuthor) {
	t.Helper()
	state := chain.Genesis(protocol.Alpha1NetworkID)
	result := make([]boardAuthor, authors)
	for position := range result {
		seed := bytes.Repeat([]byte{byte(0x30 + position)}, ed25519.SeedSize)
		key := ed25519.NewKeyFromSeed(seed)
		publicKey := identity.PublicFromPrivate(key)
		genesis := identity.IdentityGenesisBody{SchemaVersion: identity.GenesisSchema, InitialPublicKey: publicKey,
			CreatedAt: protocol.ProtocolTimestamp(1700000000000 + position)}
		id, _ := identity.DeriveIdentityID(genesis)
		keyID, _ := identity.DeriveKeyID(publicKey)
		state.Identities[id.String()] = chain.StoredIdentity{ID: id, Keys: map[string]identity.PublicKey{keyID.String(): publicKey},
			Active: map[string]bool{keyID.String(): true}}
		state.Memberships[id.String()] = membership.Active
		result[position] = boardAuthor{key: key, id: id, keyID: keyID}
	}
	return state, result
}

func storeBoardContent(t testing.TB, runtime *Runtime, title, body string, createdAt protocol.ProtocolTimestamp) protocol.ObjectID {
	t.Helper()
	object, err := board.BuildContentObject(board.Content{SchemaVersion: board.ContentSchema, Format: board.FormatMarkdown,
		Title: title, Body: body}, createdAt)
	if err != nil {
		t.Fatal(err)
	}
	id, _, err := runtime.ObjectStore().Put(context.Background(), object)
	if err != nil {
		t.Fatal(err)
	}
	return id
}

func signedBoardObject(t testing.TB, author boardAuthor, kind board.EventKind, createdAt protocol.ProtocolTimestamp,
	contentID protocol.ObjectID, parent string, references []board.Reference) []byte {
	t.Helper()
	event, err := board.SignEvent(board.EventBody{SchemaVersion: board.EventSchema, NetworkID: protocol.Alpha1NetworkID,
		Kind: kind, AuthorIdentity: author.id, AuthorKeyID: author.keyID, CreatedAt: createdAt, ParentPost: parent,
		ContentObject: contentID.String(), References: references}, author.key)
	if err != nil {
		t.Fatal(err)
	}
	object, err := board.BuildEventObject(event)
	if err != nil {
		t.Fatal(err)
	}
	raw, err := object.CanonicalBytes()
	if err != nil {
		t.Fatal(err)
	}
	return raw
}

func submitBoardAPI(t testing.TB, runtime *Runtime, raw []byte) map[string]any {
	t.Helper()
	wrapper, _ := json.Marshal(map[string]string{"encoding": "base64", "event": base64.StdEncoding.EncodeToString(raw)})
	response := apiRequest(t, http.MethodPost, "http://"+runtime.APIAddress()+"/v1/board/events", wrapper)
	if response.StatusCode != http.StatusCreated {
		t.Fatalf("Board submit = %d %s", response.StatusCode, response.Body)
	}
	var result map[string]any
	if err := json.Unmarshal(response.Body, &result); err != nil {
		t.Fatal(err)
	}
	return result
}

func TestRuntimeBoardAPIReplySearchHideRebuildAndIsolation(t *testing.T) {
	state, authors := runtimeBoardState(t, 2)
	cfg := testRuntimeConfig(t, "board-local", true, false)
	cfg.InitialState = state
	runtime, err := New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := runtime.Start(ctx); err != nil {
		t.Fatal(err)
	}
	before, _ := state.Hash()
	postContent := storeBoardContent(t, runtime, "ZION community", "สวัสดี decentralized world", 1710000000001)
	postRaw := signedBoardObject(t, authors[0], board.KindPost, 1710000000001, postContent, "", []board.Reference{{
		Relation: "zion.community/discusses/v1", TargetKind: "OBJECT", TargetID: postContent.String()}})
	post := submitBoardAPI(t, runtime, postRaw)
	postID := post["post_id"].(string)
	replyContent := storeBoardContent(t, runtime, "", "Thai reply ตอบกลับ", 1710000000002)
	orphanRaw := signedBoardObject(t, authors[1], board.KindReply, 1710000000002, replyContent,
		postContent.String(), nil)
	if _, err := runtime.SubmitBoardEvent(ctx, orphanRaw); !errors.Is(err, board.ErrParentNotFound) {
		t.Fatalf("local orphan reply = %v", err)
	}
	replyRaw := signedBoardObject(t, authors[1], board.KindReply, 1710000000002, replyContent, postID, nil)
	submitBoardAPI(t, runtime, replyRaw)

	base := "http://" + runtime.APIAddress()
	for _, endpoint := range []string{"/v1/board/posts", "/v1/board/posts/" + postID,
		"/v1/board/posts/" + postID + "/replies", "/v1/board/search?q=%E0%B8%95%E0%B8%AD%E0%B8%9A%E0%B8%81%E0%B8%A5%E0%B8%B1%E0%B8%9A"} {
		response := apiRequest(t, http.MethodGet, base+endpoint, nil)
		if response.StatusCode != http.StatusOK || len(response.Body) < 3 {
			t.Fatalf("GET %s = %d %s", endpoint, response.StatusCode, response.Body)
		}
	}
	hide := apiRequest(t, http.MethodPost, base+"/v1/board/posts/"+postID+"/hide", nil)
	if hide.StatusCode != http.StatusOK {
		t.Fatalf("hide = %d %s", hide.StatusCode, hide.Body)
	}
	feed := apiRequest(t, http.MethodGet, base+"/v1/board/posts", nil)
	if bytes.Contains(feed.Body, []byte(postID)) {
		t.Fatal("locally hidden post remained in default feed")
	}
	storedPostID, _ := protocol.ParseObjectID(postID)
	if _, err := runtime.ObjectStore().Get(ctx, storedPostID); err != nil {
		t.Fatalf("local hide removed immutable Board event: %v", err)
	}
	unhide := apiRequest(t, http.MethodPost, base+"/v1/board/posts/"+postID+"/unhide", nil)
	if unhide.StatusCode != http.StatusOK {
		t.Fatalf("unhide = %d", unhide.StatusCode)
	}
	runtime.mu.RLock()
	after, _ := runtime.state.Hash()
	canonicalState, _ := runtime.state.CanonicalBytes()
	runtime.mu.RUnlock()
	if before.String() != after.String() {
		t.Fatal("Board activity changed StateHash/AppHash")
	}
	if bytes.Contains(canonicalState, []byte("ZION community")) || bytes.Contains(canonicalState, []byte("decentralized world")) ||
		bytes.Contains(canonicalState, []byte(postID)) {
		t.Fatal("Board payload or PostID entered canonical chain state")
	}
	peerID := runtime.P2P().PeerID()
	if err := runtime.Stop(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(cfg.BoardIndexPath, []byte("{"), 0o600); err != nil {
		t.Fatal(err)
	}
	restarted, err := New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if err := restarted.Start(ctx); err != nil {
		t.Fatal(err)
	}
	defer restarted.Stop(context.Background())
	if restarted.P2P().PeerID() != peerID {
		t.Fatal("PeerID changed during Board restart")
	}
	feed = apiRequest(t, http.MethodGet, "http://"+restarted.APIAddress()+"/v1/board/posts", nil)
	if !bytes.Contains(feed.Body, []byte(postID)) {
		t.Fatal("Board index did not rebuild from immutable objects")
	}
}

func TestBoardRegistryReferencesRequireLocalTargetsButRetainRemoteUnresolved(t *testing.T) {
	state, authors := runtimeBoardState(t, 1)
	cfg := testRuntimeConfig(t, "board-registry-refs", true, false)
	cfg.InitialState = state
	runtime, err := New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := runtime.Start(ctx); err != nil {
		t.Fatal(err)
	}
	defer runtime.Stop(context.Background())
	researchID := research.ID{HashDigest: protocol.HashBytes([]byte("admitted"))}
	missingResource := resources.ID{HashDigest: protocol.HashBytes([]byte("missing"))}
	runtime.mu.Lock()
	runtime.state.Research = map[string]chain.StoredResearch{researchID.String(): {ID: researchID}}
	runtime.state.Resources = map[string]chain.StoredResource{}
	runtime.mu.Unlock()
	contentID := storeBoardContent(t, runtime, "Registry link", "Community reference", 1710000000300)
	validRaw := signedBoardObject(t, authors[0], board.KindPost, 1710000000300, contentID, "", []board.Reference{{Relation: "zion.community/discusses/v1", TargetKind: "RESEARCH", TargetID: researchID.String()}})
	if _, err := runtime.SubmitBoardEvent(ctx, validRaw); err != nil {
		t.Fatalf("admitted ResearchID local reference rejected: %v", err)
	}
	missingRaw := signedBoardObject(t, authors[0], board.KindPost, 1710000000301, contentID, "", []board.Reference{{Relation: "zion.community/discusses/v1", TargetKind: "RESOURCE", TargetID: missingResource.String()}})
	if _, err := runtime.SubmitBoardEvent(ctx, missingRaw); !errors.Is(err, board.ErrReferenceNotFound) {
		t.Fatalf("missing local registry target = %v", err)
	}
	if err := runtime.AdmitRemoteBoardEvent(ctx, missingRaw); err != nil {
		t.Fatalf("remote unresolved event rejected: %v", err)
	}
	object, _ := objects.Decode(missingRaw)
	postID, _ := board.PostID(object)
	value, found, err := runtime.BoardPost(postID.String())
	if err != nil || !found || len(value.(index.BoardEntry).UnresolvedReferences) != 1 {
		t.Fatal("remote unresolved reference was not retained and marked")
	}
}

func TestRuntimeBoardAuthorizationRotationAndSuspension(t *testing.T) {
	state, authors := runtimeBoardState(t, 1)
	cfg := testRuntimeConfig(t, "board-auth", true, false)
	cfg.InitialState = state
	runtime, _ := New(cfg)
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel()
	if err := runtime.Start(ctx); err != nil {
		t.Fatal(err)
	}
	defer runtime.Stop(context.Background())
	contentID := storeBoardContent(t, runtime, "auth", "authorization", 1710000000100)
	first := signedBoardObject(t, authors[0], board.KindPost, 1710000000100, contentID, "", nil)
	if _, err := runtime.SubmitBoardEvent(ctx, first); err != nil {
		t.Fatal(err)
	}
	runtime.mu.Lock()
	stored := runtime.state.Identities[authors[0].id.String()]
	stored.Active[authors[0].keyID.String()] = false
	runtime.state.Identities[authors[0].id.String()] = stored
	runtime.mu.Unlock()
	retired := signedBoardObject(t, authors[0], board.KindPost, 1710000000101, contentID, "", nil)
	if _, err := runtime.SubmitBoardEvent(ctx, retired); !errors.Is(err, board.ErrPublicationDenied) {
		t.Fatalf("retired key publication = %v", err)
	}
	newKey := ed25519.NewKeyFromSeed(bytes.Repeat([]byte{0x72}, ed25519.SeedSize))
	newPublic := identity.PublicFromPrivate(newKey)
	newKeyID, _ := identity.DeriveKeyID(newPublic)
	runtime.mu.Lock()
	stored.Keys[newKeyID.String()] = newPublic
	stored.Active[newKeyID.String()] = true
	runtime.state.Identities[authors[0].id.String()] = stored
	runtime.mu.Unlock()
	rotatedAuthor := boardAuthor{key: newKey, id: authors[0].id, keyID: newKeyID}
	rotated := signedBoardObject(t, rotatedAuthor, board.KindPost, 1710000000102, contentID, "", nil)
	if _, err := runtime.SubmitBoardEvent(ctx, rotated); err != nil {
		t.Fatalf("current rotated key publication = %v", err)
	}
	runtime.mu.Lock()
	runtime.state.Memberships[authors[0].id.String()] = membership.Suspended
	runtime.mu.Unlock()
	suspended := signedBoardObject(t, rotatedAuthor, board.KindPost, 1710000000103, contentID, "", nil)
	if _, err := runtime.SubmitBoardEvent(ctx, suspended); !errors.Is(err, board.ErrPublicationDenied) {
		t.Fatalf("suspended publication = %v", err)
	}
	for offset, status := range []membership.Status{membership.Pending, membership.Revoked} {
		runtime.mu.Lock()
		runtime.state.Memberships[authors[0].id.String()] = status
		runtime.mu.Unlock()
		raw := signedBoardObject(t, rotatedAuthor, board.KindPost, protocol.ProtocolTimestamp(1710000000104+offset), contentID, "", nil)
		if _, err := runtime.SubmitBoardEvent(ctx, raw); !errors.Is(err, board.ErrPublicationDenied) {
			t.Fatalf("%s publication = %v", status, err)
		}
	}
	missingContent, _ := board.BuildContentObject(board.Content{SchemaVersion: board.ContentSchema, Format: board.FormatText,
		Body: "not stored locally"}, 1710000000200)
	missingID, _ := missingContent.ObjectID()
	remoteHistorical := signedBoardObject(t, rotatedAuthor, board.KindPost, 1710000000200, missingID, "", nil)
	if err := runtime.AdmitRemoteBoardEvent(ctx, remoteHistorical); err != nil {
		t.Fatalf("historical remote event with missing content = %v", err)
	}
	remoteObject, _ := objects.Decode(remoteHistorical)
	remoteID, _ := remoteObject.ObjectID()
	remoteEntry, found, _ := runtime.BoardPost(remoteID.String())
	if !found || remoteEntry.(index.BoardEntry).ContentPresent ||
		remoteEntry.(index.BoardEntry).AuthorizationStatus != board.HistoricalOrUnconfirmed {
		t.Fatalf("remote historical classification = %+v", remoteEntry)
	}
}

func TestRuntimeBoardMultiNodeAnnounceOfflineSyncOutboundOnly(t *testing.T) {
	state, authors := runtimeBoardState(t, 1)
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	aCfg := testRuntimeConfig(t, "board-a", true, true)
	aCfg.InitialState = state
	aCfg.Board.SyncInterval = time.Hour
	bCfg := testRuntimeConfig(t, "board-b", true, false)
	bCfg.InitialState = state
	bCfg.GenesisID, bCfg.P2P.NetworkFingerprint = aCfg.GenesisID, aCfg.GenesisID
	bCfg.Board.SyncInterval = time.Hour
	a, _ := New(aCfg)
	b, _ := New(bCfg)
	if err := a.Start(ctx); err != nil {
		t.Fatal(err)
	}
	if err := b.Start(ctx); err != nil {
		t.Fatal(err)
	}
	if len(b.P2P().ListenAddresses()) != 0 {
		t.Fatal("Board client is not outbound-only")
	}
	if err := b.P2P().Dial(ctx, a.P2P().FullAddresses()[0].String(), p2p.SourceManual); err != nil {
		t.Fatal(err)
	}
	contentID := storeBoardContent(t, a, "announce", "direct Board propagation", 1710000000200)
	raw := signedBoardObject(t, authors[0], board.KindPost, 1710000000200, contentID, "", nil)
	result := submitBoardAPI(t, a, raw)
	postID := result["post_id"].(string)
	waitRuntime(t, b, func() bool { _, found, _ := b.BoardPost(postID); return found })
	remote, found, _ := b.BoardPost(postID)
	if !found || !remote.(index.BoardEntry).ContentPresent {
		t.Fatal("announced Board event/content not available on outbound-only peer")
	}
	if err := b.Stop(context.Background()); err != nil {
		t.Fatal(err)
	}
	offlineContent := storeBoardContent(t, a, "offline", "anti entropy recovery", 1710000000201)
	offlineRaw := signedBoardObject(t, authors[0], board.KindPost, 1710000000201, offlineContent, "", nil)
	offline := submitBoardAPI(t, a, offlineRaw)
	offlineID := offline["post_id"].(string)
	b, _ = New(bCfg)
	if err := b.Start(ctx); err != nil {
		t.Fatal(err)
	}
	defer b.Stop(context.Background())
	defer a.Stop(context.Background())
	if err := b.P2P().Dial(ctx, a.P2P().FullAddresses()[0].String(), p2p.SourceManual); err != nil {
		t.Fatal(err)
	}
	if err := b.SyncBoard(ctx); err != nil && !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
	entry, found, _ := b.BoardPost(offlineID)
	if !found || !entry.(index.BoardEntry).ContentPresent {
		t.Fatal("offline anti-entropy did not recover Board event/content")
	}
}

func TestRuntimeBoardContinuesAfterBootstrapLoss(t *testing.T) {
	state, authors := runtimeBoardState(t, 1)
	ctx, cancel := context.WithTimeout(context.Background(), 12*time.Second)
	defer cancel()
	bootstrapCfg := testRuntimeConfig(t, "board-bootstrap", true, true)
	bootstrapCfg.InitialState = state
	bootstrapCfg.P2P.Roles = []p2p.Role{p2p.RoleNormal, p2p.RoleBootstrap}
	aCfg := testRuntimeConfig(t, "board-after-bootstrap-a", true, true)
	aCfg.InitialState, aCfg.GenesisID, aCfg.P2P.NetworkFingerprint = state, bootstrapCfg.GenesisID, bootstrapCfg.GenesisID
	cCfg := testRuntimeConfig(t, "board-after-bootstrap-c", true, true)
	cCfg.InitialState, cCfg.GenesisID, cCfg.P2P.NetworkFingerprint = state, bootstrapCfg.GenesisID, bootstrapCfg.GenesisID
	bootstrap, _ := New(bootstrapCfg)
	a, _ := New(aCfg)
	c, _ := New(cCfg)
	if err := bootstrap.Start(ctx); err != nil {
		t.Fatal(err)
	}
	if err := a.Start(ctx); err != nil {
		_ = bootstrap.Stop(context.Background())
		t.Fatal(err)
	}
	defer a.Stop(context.Background())
	if err := c.Start(ctx); err != nil {
		_ = bootstrap.Stop(context.Background())
		t.Fatal(err)
	}
	defer c.Stop(context.Background())
	bootstrapAddress := bootstrap.P2P().FullAddresses()[0].String()
	if err := a.P2P().Dial(ctx, bootstrapAddress, p2p.SourceBootstrap); err != nil {
		t.Fatal(err)
	}
	if err := c.P2P().Dial(ctx, bootstrapAddress, p2p.SourceBootstrap); err != nil {
		t.Fatal(err)
	}
	if err := a.P2P().Dial(ctx, c.P2P().FullAddresses()[0].String(), p2p.SourcePEX); err != nil {
		t.Fatal(err)
	}
	beforeA, _ := state.Hash()
	beforeC, _ := state.Hash()
	if err := bootstrap.Stop(context.Background()); err != nil {
		t.Fatal(err)
	}
	contentID := storeBoardContent(t, a, "bootstrap gone", "ordinary peers continue directly", 1710000000300)
	raw := signedBoardObject(t, authors[0], board.KindPost, 1710000000300, contentID, "", nil)
	post := submitBoardAPI(t, a, raw)
	postID := post["post_id"].(string)
	waitRuntime(t, c, func() bool {
		entry, found, _ := c.BoardPost(postID)
		return found && entry.(index.BoardEntry).ContentPresent
	})
	a.mu.RLock()
	afterA, _ := a.state.Hash()
	a.mu.RUnlock()
	c.mu.RLock()
	afterC, _ := c.state.Hash()
	c.mu.RUnlock()
	if beforeA.String() != afterA.String() || beforeC.String() != afterC.String() {
		t.Fatal("bootstrap loss or Board propagation changed canonical state")
	}
}
