package main

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/kokora3/zion/internal/objects"
	"github.com/kokora3/zion/internal/protocol"
)

func cliTestObject(payload string) objects.Object {
	raw := []byte(payload)
	return objects.Object{Core: protocol.UnsignedObjectCore{ObjectType: "note", SchemaVersion: protocol.UnsignedObjectSchemaV1,
		CreatedAt: 1710000000123, ContentHash: protocol.NewContentHash(raw), SizeBytes: uint64(len(raw)),
		Visibility: protocol.VisibilityPublic, Metadata: map[string]string{"suite": "cli"}}, Payload: raw}
}

func TestWriteObjectResponseExactBytesAndNoOverwrite(t *testing.T) {
	object := cliTestObject("phase-9c-cli")
	raw, _ := object.CanonicalBytes()
	id, _ := object.ObjectID()
	response, _ := json.Marshal(map[string]any{"encoding": "base64", "object": base64.StdEncoding.EncodeToString(raw),
		"object_id": id.String(), "size": len(raw)})
	path := filepath.Join(t.TempDir(), "object.cbor")
	if err := writeObjectResponse(response, path, false); err != nil {
		t.Fatal(err)
	}
	stored, err := os.ReadFile(path)
	if err != nil || !bytes.Equal(stored, raw) {
		t.Fatalf("output differs: %v", err)
	}
	if err := writeObjectResponse(response, path, false); err == nil || !strings.Contains(err.Error(), "--force") {
		t.Fatalf("overwrite without --force = %v", err)
	}
	replacement := cliTestObject("phase-9c-replacement")
	replacementRaw, _ := replacement.CanonicalBytes()
	replacementID, _ := replacement.ObjectID()
	replacementResponse, _ := json.Marshal(map[string]any{"encoding": "base64", "object": base64.StdEncoding.EncodeToString(replacementRaw),
		"object_id": replacementID.String(), "size": len(replacementRaw)})
	if err := writeObjectResponse(replacementResponse, path, true); err != nil {
		t.Fatal(err)
	}
	stored, _ = os.ReadFile(path)
	if !bytes.Equal(stored, replacementRaw) {
		t.Fatal("--force did not replace output with exact canonical bytes")
	}
}

func TestWriteObjectResponseRejectsTamperingAndReadBoundedRejectsDirectory(t *testing.T) {
	object := cliTestObject("phase-9c-cli-tamper")
	raw, _ := object.CanonicalBytes()
	response, _ := json.Marshal(map[string]any{"encoding": "base64", "object": base64.StdEncoding.EncodeToString(raw),
		"object_id": "sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", "size": len(raw)})
	if err := writeObjectResponse(response, filepath.Join(t.TempDir(), "bad.cbor"), false); err == nil {
		t.Fatal("accepted ObjectID mismatch")
	}
	if _, err := readBoundedFile(t.TempDir(), objects.MaxObjectBytes, "object"); err == nil || !strings.Contains(err.Error(), "regular file") {
		t.Fatalf("directory input = %v", err)
	}
	oversized := filepath.Join(t.TempDir(), "oversized.cbor")
	if err := os.WriteFile(oversized, bytes.Repeat([]byte{0}, objects.MaxObjectBytes+1), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := readBoundedFile(oversized, objects.MaxObjectBytes, "object"); err == nil || !strings.Contains(err.Error(), "size exceeds") {
		t.Fatalf("oversized object input = %v", err)
	}
}

func TestObjectCommandsUseVersionedAPIEndToEnd(t *testing.T) {
	object := cliTestObject("phase-9c-cli-api-e2e")
	raw, _ := object.CanonicalBytes()
	id, _ := object.ObjectID()
	seen := make(map[string]int)
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		key := request.Method + " " + request.URL.Path
		seen[key]++
		writer.Header().Set("Content-Type", "application/json")
		switch key {
		case http.MethodPost + " /v1/objects":
			var wrapper map[string]string
			_ = json.NewDecoder(request.Body).Decode(&wrapper)
			decoded, _ := base64.StdEncoding.DecodeString(wrapper["object"])
			if wrapper["encoding"] != "base64" || !bytes.Equal(decoded, raw) {
				http.Error(writer, `{"code":"INVALID_OBJECT"}`, http.StatusBadRequest)
				return
			}
			writer.WriteHeader(http.StatusCreated)
			_, _ = io.WriteString(writer, `{"object_id":"`+id.String()+`","size":`+fmt.Sprint(len(raw))+`,"status":"STORED"}`)
		case http.MethodGet + " /v1/objects/" + id.String():
			_ = json.NewEncoder(writer).Encode(map[string]any{"encoding": "base64", "object": base64.StdEncoding.EncodeToString(raw),
				"object_id": id.String(), "size": len(raw)})
		case http.MethodGet + " /v1/objects/" + id.String() + "/meta":
			_, _ = io.WriteString(writer, `{"object_id":"`+id.String()+`","size":`+fmt.Sprint(len(raw))+`,"present":true}`)
		case http.MethodPost + " /v1/objects/" + id.String() + "/fetch":
			_, _ = io.WriteString(writer, `{"object_id":"`+id.String()+`","size":`+fmt.Sprint(len(raw))+`,"status":"ALREADY_LOCAL"}`)
		default:
			http.NotFound(writer, request)
		}
	}))
	defer server.Close()
	input := filepath.Join(t.TempDir(), "input.cbor")
	if err := os.WriteFile(input, raw, 0o600); err != nil {
		t.Fatal(err)
	}
	output := filepath.Join(t.TempDir(), "output.cbor")
	for _, args := range [][]string{
		{"--api", server.URL, "object", "put", input},
		{"--api", server.URL, "object", "stat", id.String()},
		{"--api", server.URL, "object", "fetch", id.String()},
		{"--api", server.URL, "object", "get", id.String(), "--out", output},
	} {
		runSuccessfulCLI(t, args)
	}
	written, err := os.ReadFile(output)
	if err != nil || !bytes.Equal(written, raw) {
		t.Fatalf("CLI GET output differs: %v", err)
	}
	for _, key := range []string{
		http.MethodPost + " /v1/objects",
		http.MethodGet + " /v1/objects/" + id.String(),
		http.MethodGet + " /v1/objects/" + id.String() + "/meta",
		http.MethodPost + " /v1/objects/" + id.String() + "/fetch",
	} {
		if seen[key] != 1 {
			t.Fatalf("API request %q count = %d", key, seen[key])
		}
	}
}

func runSuccessfulCLI(t *testing.T, args []string) string {
	t.Helper()
	originalArgs, originalStdout := os.Args, os.Stdout
	reader, writer, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	os.Args = append([]string{"zionctl"}, args...)
	os.Stdout = writer
	defer func() {
		os.Args = originalArgs
		os.Stdout = originalStdout
	}()
	main()
	_ = writer.Close()
	data, err := io.ReadAll(reader)
	_ = reader.Close()
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}
