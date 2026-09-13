package api

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/kokora3/zion/internal/objects"
	"github.com/kokora3/zion/internal/protocol"
)

type objectBackend struct {
	store       *objects.Store
	fetchObject objects.Object
	fetchErr    error
	getErr      error
	putErr      error
}

func (b *objectBackend) Health() any       { return map[string]bool{"live": true} }
func (b *objectBackend) Status() any       { return map[string]any{} }
func (b *objectBackend) Peers() any        { return []any{} }
func (b *objectBackend) StateSummary() any { return map[string]any{} }
func (b *objectBackend) Identity(string) (any, bool, error) {
	return nil, false, nil
}
func (b *objectBackend) Membership(string) (any, bool, error) {
	return nil, false, nil
}
func (b *objectBackend) Proposals(int, int) (any, error) { return []any{}, nil }
func (b *objectBackend) Proposal(string) (any, bool, error) {
	return nil, false, nil
}
func (b *objectBackend) SubmitTransaction(context.Context, []byte) (Submission, error) {
	return Submission{}, errors.New("unused")
}
func (b *objectBackend) Transaction(string) (any, bool, error) {
	return nil, false, nil
}
func (b *objectBackend) PutObject(ctx context.Context, object objects.Object) (protocol.ObjectID, objects.PutResult, error) {
	if b.putErr != nil {
		return protocol.ObjectID{}, "", b.putErr
	}
	return b.store.Put(ctx, object)
}
func (b *objectBackend) GetObject(ctx context.Context, id protocol.ObjectID) (objects.Object, error) {
	if b.getErr != nil {
		return objects.Object{}, b.getErr
	}
	return b.store.Get(ctx, id)
}
func (b *objectBackend) StatObject(ctx context.Context, id protocol.ObjectID) (objects.Metadata, error) {
	if b.getErr != nil {
		return objects.Metadata{}, b.getErr
	}
	return b.store.Stat(ctx, id)
}
func (b *objectBackend) FetchObject(context.Context, protocol.ObjectID) (objects.Object, error) {
	return b.fetchObject, b.fetchErr
}

func apiTestObject(payload string) objects.Object {
	data := []byte(payload)
	return objects.Object{Core: protocol.UnsignedObjectCore{ObjectType: "note", SchemaVersion: protocol.UnsignedObjectSchemaV1,
		CreatedAt: 1710000000123, ContentHash: protocol.NewContentHash(data), SizeBytes: uint64(len(data)),
		Visibility: protocol.VisibilityPublic, Metadata: map[string]string{"suite": "api"}}, Payload: data}
}

func newObjectServer(t *testing.T, backend Backend) *Server {
	t.Helper()
	cfg := DefaultConfig()
	server, err := NewServer(cfg, backend)
	if err != nil {
		t.Fatal(err)
	}
	return server
}

func objectRequest(t *testing.T, server *Server, method, path string, body []byte) *httptest.ResponseRecorder {
	t.Helper()
	request := httptest.NewRequest(method, path, bytes.NewReader(body))
	request.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()
	server.ServeHTTP(response, request)
	return response
}

func TestObjectAPIStoreGetMetaDuplicateAndFetch(t *testing.T) {
	store, err := objects.Open(t.TempDir(), 0)
	if err != nil {
		t.Fatal(err)
	}
	backend := &objectBackend{store: store}
	server := newObjectServer(t, backend)
	object := apiTestObject("phase-9c-local-object")
	raw, _ := object.CanonicalBytes()
	wrapper, _ := json.Marshal(map[string]string{"encoding": "base64", "object": base64.StdEncoding.EncodeToString(raw)})

	response := objectRequest(t, server, http.MethodPost, BasePath+"/objects", wrapper)
	if response.Code != http.StatusCreated || !strings.Contains(response.Body.String(), `"status":"STORED"`) {
		t.Fatalf("first put = %d %s", response.Code, response.Body.String())
	}
	id, _ := object.ObjectID()
	response = objectRequest(t, server, http.MethodPost, BasePath+"/objects", wrapper)
	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), `"status":"ALREADY_EXISTS"`) {
		t.Fatalf("duplicate put = %d %s", response.Code, response.Body.String())
	}
	response = objectRequest(t, server, http.MethodGet, BasePath+"/objects/"+id.String(), nil)
	if response.Code != http.StatusOK {
		t.Fatalf("get = %d %s", response.Code, response.Body.String())
	}
	var got struct {
		Encoding string `json:"encoding"`
		Object   string `json:"object"`
		ObjectID string `json:"object_id"`
		Size     int    `json:"size"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &got); err != nil || got.Encoding != "base64" || got.ObjectID != id.String() || got.Size != len(raw) {
		t.Fatalf("get wrapper = %+v, %v", got, err)
	}
	decoded, _ := base64.StdEncoding.DecodeString(got.Object)
	if !bytes.Equal(decoded, raw) {
		t.Fatal("GET did not return exact canonical object bytes")
	}
	response = objectRequest(t, server, http.MethodGet, BasePath+"/objects/"+id.String()+"/meta", nil)
	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), `"present":true`) || strings.Contains(response.Body.String(), "path") {
		t.Fatalf("meta = %d %s", response.Code, response.Body.String())
	}
	response = objectRequest(t, server, http.MethodPost, BasePath+"/objects/"+id.String()+"/fetch", nil)
	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), `"status":"ALREADY_LOCAL"`) {
		t.Fatalf("local fetch = %d %s", response.Code, response.Body.String())
	}

	remote := apiTestObject("phase-9c-remote-object")
	remoteID, _ := remote.ObjectID()
	backend.fetchObject = remote
	response = objectRequest(t, server, http.MethodPost, BasePath+"/objects/"+remoteID.String()+"/fetch", nil)
	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), `"status":"FETCHED"`) {
		t.Fatalf("remote fetch = %d %s", response.Code, response.Body.String())
	}
}

func TestObjectAPIRejectsUnsafeMalformedAndBoundedInputs(t *testing.T) {
	store, _ := objects.Open(t.TempDir(), 0)
	backend := &objectBackend{store: store}
	server := newObjectServer(t, backend)
	tests := []struct {
		name   string
		method string
		path   string
		body   []byte
		status int
		code   string
	}{
		{"unknown path field", http.MethodPost, BasePath + "/objects", []byte(`{"encoding":"base64","object":"AA==","destination_path":"C:\\escape"}`), http.StatusBadRequest, "INVALID_OBJECT"},
		{"invalid base64", http.MethodPost, BasePath + "/objects", []byte(`{"encoding":"base64","object":"%%%"}`), http.StatusBadRequest, "INVALID_OBJECT"},
		{"noncanonical object", http.MethodPost, BasePath + "/objects", []byte(`{"encoding":"base64","object":"AA=="}`), http.StatusUnprocessableEntity, "INVALID_OBJECT"},
		{"traversal", http.MethodGet, BasePath + "/objects/..%2F..%2Fsecret", nil, http.StatusBadRequest, "INVALID_OBJECT_ID"},
		{"missing", http.MethodGet, BasePath + "/objects/zion:obj:sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", nil, http.StatusNotFound, "OBJECT_NOT_FOUND"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			response := objectRequest(t, server, test.method, test.path, test.body)
			if response.Code != test.status || !strings.Contains(response.Body.String(), test.code) {
				t.Fatalf("got %d %s", response.Code, response.Body.String())
			}
		})
	}
	response := objectRequest(t, server, http.MethodPost, BasePath+"/objects", bytes.Repeat([]byte("x"), MaxObjectBody+1))
	if response.Code != http.StatusRequestEntityTooLarge || !strings.Contains(response.Body.String(), "BODY_TOO_LARGE") {
		t.Fatalf("oversized body = %d %s", response.Code, response.Body.String())
	}

	object := apiTestObject("error mapping")
	raw, _ := object.CanonicalBytes()
	wrapper, _ := json.Marshal(map[string]string{"encoding": "base64", "object": base64.StdEncoding.EncodeToString(raw)})
	backend.putErr = objects.ErrStoreFull
	response = objectRequest(t, server, http.MethodPost, BasePath+"/objects", wrapper)
	if response.Code != http.StatusInsufficientStorage || !strings.Contains(response.Body.String(), "OBJECT_STORE_FULL") {
		t.Fatalf("quota = %d %s", response.Code, response.Body.String())
	}
	backend.putErr = nil
	id, _ := object.ObjectID()
	backend.getErr = objects.ErrObjectCorrupt
	response = objectRequest(t, server, http.MethodGet, BasePath+"/objects/"+id.String(), nil)
	if response.Code != http.StatusInternalServerError || !strings.Contains(response.Body.String(), "OBJECT_CORRUPT") {
		t.Fatalf("corruption = %d %s", response.Code, response.Body.String())
	}
}

func TestObjectAPIAuthAndCORSRemainEnforced(t *testing.T) {
	store, _ := objects.Open(t.TempDir(), 0)
	cfg := DefaultConfig()
	cfg.BearerToken = "0123456789abcdef"
	cfg.AllowedOrigins = []string{"https://local.example"}
	server, err := NewServer(cfg, &objectBackend{store: store})
	if err != nil {
		t.Fatal(err)
	}
	request := httptest.NewRequest(http.MethodGet, BasePath+"/objects/zion:obj:sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", nil)
	response := httptest.NewRecorder()
	server.ServeHTTP(response, request)
	if response.Code != http.StatusUnauthorized {
		t.Fatalf("unauthorized = %d", response.Code)
	}
	request = httptest.NewRequest(http.MethodGet, BasePath+"/objects/zion:obj:sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", nil)
	request.Header.Set("Authorization", "Bearer 0123456789abcdef")
	request.Header.Set("Origin", "https://evil.example")
	response = httptest.NewRecorder()
	server.ServeHTTP(response, request)
	if response.Code != http.StatusForbidden {
		t.Fatalf("forbidden origin = %d", response.Code)
	}
}

func TestObjectAPIConfigurationRetainsLoopbackAndCORSBoundaries(t *testing.T) {
	store, _ := objects.Open(t.TempDir(), 0)
	backend := &objectBackend{store: store}
	if DefaultConfig().Listen != "127.0.0.1:42001" {
		t.Fatalf("default API listen is not loopback: %s", DefaultConfig().Listen)
	}
	cfg := DefaultConfig()
	cfg.Listen = "0.0.0.0:42001"
	if _, err := NewServer(cfg, backend); err == nil {
		t.Fatal("non-loopback API without bearer token was accepted")
	}
	cfg.BearerToken = "0123456789abcdef"
	if _, err := NewServer(cfg, backend); err != nil {
		t.Fatalf("authenticated non-loopback API rejected: %v", err)
	}
	cfg.Listen = DefaultListen
	cfg.AllowedOrigins = []string{"*"}
	if _, err := NewServer(cfg, backend); err == nil {
		t.Fatal("wildcard CORS origin was accepted")
	}
}

func TestObjectAPIErrorMessagesContainNoSensitiveInternals(t *testing.T) {
	store, _ := objects.Open(t.TempDir(), 0)
	backend := &objectBackend{store: store, getErr: errors.New(`C:\\secret\\objects\\private-key`)}
	server := newObjectServer(t, backend)
	response := objectRequest(t, server, http.MethodGet,
		BasePath+"/objects/zion:obj:sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", nil)
	if response.Code != http.StatusInternalServerError || strings.Contains(response.Body.String(), "secret") || strings.Contains(response.Body.String(), "private-key") {
		t.Fatalf("sensitive error leaked: %s", response.Body.String())
	}
}

func TestObjectAPIFetchErrorContractAndWrongContent(t *testing.T) {
	store, _ := objects.Open(t.TempDir(), 0)
	wanted := apiTestObject("wanted")
	wantedID, _ := wanted.ObjectID()
	backend := &objectBackend{store: store}
	server := newObjectServer(t, backend)
	cases := []struct {
		err    error
		status int
		code   string
	}{
		{objects.ErrObjectNotFound, http.StatusNotFound, "OBJECT_NOT_FOUND"},
		{objects.ErrObjectFetchTimeout, http.StatusGatewayTimeout, "OBJECT_FETCH_TIMEOUT"},
		{objects.ErrStoreFull, http.StatusInsufficientStorage, "OBJECT_STORE_FULL"},
		{objects.ErrInvalidRemoteResponse, http.StatusBadGateway, "INVALID_REMOTE_OBJECT"},
		{objects.ErrNoAvailablePeers, http.StatusServiceUnavailable, "NO_OBJECT_PEERS"},
	}
	for _, test := range cases {
		backend.fetchErr = test.err
		response := objectRequest(t, server, http.MethodPost, BasePath+"/objects/"+wantedID.String()+"/fetch", nil)
		if response.Code != test.status || !strings.Contains(response.Body.String(), test.code) {
			t.Fatalf("fetch %v = %d %s", test.err, response.Code, response.Body.String())
		}
	}
	backend.fetchErr = nil
	backend.fetchObject = apiTestObject("wrong-content")
	response := objectRequest(t, server, http.MethodPost, BasePath+"/objects/"+wantedID.String()+"/fetch", nil)
	if response.Code != http.StatusBadGateway || !strings.Contains(response.Body.String(), "INVALID_REMOTE_OBJECT") {
		t.Fatalf("wrong-content fetch = %d %s", response.Code, response.Body.String())
	}
}

func TestObjectMetadataTimestampIsUTCAndStableShape(t *testing.T) {
	store, _ := objects.Open(t.TempDir(), 0)
	object := apiTestObject("metadata")
	id, _, err := store.Put(context.Background(), object)
	if err != nil {
		t.Fatal(err)
	}
	server := newObjectServer(t, &objectBackend{store: store})
	response := objectRequest(t, server, http.MethodGet, BasePath+"/objects/"+id.String()+"/meta", nil)
	var result struct {
		StoredAt string `json:"stored_at"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	when, err := time.Parse(time.RFC3339Nano, result.StoredAt)
	if err != nil || when.Location() != time.UTC {
		t.Fatalf("stored_at = %q, %v", result.StoredAt, err)
	}
}
