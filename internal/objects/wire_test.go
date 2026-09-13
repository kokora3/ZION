package objects

import (
	"bytes"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/kokora3/zion/internal/protocol"
)

type requestFixture struct {
	SchemaVersion uint64 `json:"schema_version"`
	NetworkID     string `json:"network_id"`
	ObjectID      string `json:"object_id"`
	Canonical     string `json:"canonical_cbor_hex"`
}

type responseFixture struct {
	SchemaVersion uint64 `json:"schema_version"`
	NetworkID     string `json:"network_id"`
	Status        string `json:"status"`
	ObjectID      string `json:"object_id"`
	ObjectCBOR    string `json:"object_cbor_hex"`
	Canonical     string `json:"canonical_cbor_hex"`
}

func loadJSONFixture(t testing.TB, name string, value any) {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("testdata", name))
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(data, value); err != nil {
		t.Fatal(err)
	}
}

func goldenRequest(t testing.TB) (ObjectGetRequest, []byte) {
	t.Helper()
	var fixture requestFixture
	loadJSONFixture(t, "object_get_request_golden.json", &fixture)
	raw, err := hex.DecodeString(fixture.Canonical)
	if err != nil {
		t.Fatal(err)
	}
	return ObjectGetRequest{SchemaVersion: fixture.SchemaVersion, NetworkID: protocol.NetworkID(fixture.NetworkID), ObjectID: fixture.ObjectID}, raw
}

func goldenResponse(t testing.TB) (ObjectGetResponse, []byte) {
	t.Helper()
	var fixture responseFixture
	loadJSONFixture(t, "object_get_response_golden.json", &fixture)
	objectBytes, err := hex.DecodeString(fixture.ObjectCBOR)
	if err != nil {
		t.Fatal(err)
	}
	raw, err := hex.DecodeString(fixture.Canonical)
	if err != nil {
		t.Fatal(err)
	}
	return ObjectGetResponse{SchemaVersion: fixture.SchemaVersion, NetworkID: protocol.NetworkID(fixture.NetworkID), Status: ObjectResponseStatus(fixture.Status), ObjectID: fixture.ObjectID, ObjectBytes: objectBytes}, raw
}

func TestObjectWireGoldenFixtures(t *testing.T) {
	request, requestRaw := goldenRequest(t)
	encoded, err := EncodeObjectGetRequest(request)
	if err != nil || !bytes.Equal(encoded, requestRaw) {
		t.Fatalf("request golden changed: %x %v", encoded, err)
	}
	decodedRequest, err := DecodeObjectGetRequest(requestRaw, protocol.Alpha1NetworkID)
	if err != nil || decodedRequest != request {
		t.Fatalf("request decode: %+v %v", decodedRequest, err)
	}

	response, responseRaw := goldenResponse(t)
	encoded, err = EncodeObjectGetResponse(response)
	if err != nil || !bytes.Equal(encoded, responseRaw) {
		t.Fatalf("response golden changed: %x %v", encoded, err)
	}
	decodedResponse, err := DecodeObjectGetResponse(responseRaw, protocol.Alpha1NetworkID, response.ObjectID)
	if err != nil || decodedResponse.ObjectID != response.ObjectID || decodedResponse.Status != ObjectFound || !bytes.Equal(decodedResponse.ObjectBytes, response.ObjectBytes) {
		t.Fatalf("response decode: %+v %v", decodedResponse, err)
	}
}

func nonCanonicalRequest(t testing.TB, request ObjectGetRequest) []byte {
	t.Helper()
	schema, _ := protocol.CanonicalEncode(request.SchemaVersion)
	network, _ := protocol.CanonicalEncode(request.NetworkID)
	id, _ := protocol.CanonicalEncode(request.ObjectID)
	return bytes.Join([][]byte{{0xa3, 0x03}, id, {0x02}, network, {0x01}, schema}, nil)
}

func nonCanonicalResponse(t testing.TB, response ObjectGetResponse) []byte {
	t.Helper()
	schema, _ := protocol.CanonicalEncode(response.SchemaVersion)
	network, _ := protocol.CanonicalEncode(response.NetworkID)
	status, _ := protocol.CanonicalEncode(response.Status)
	id, _ := protocol.CanonicalEncode(response.ObjectID)
	object, _ := protocol.CanonicalEncode(response.ObjectBytes)
	return bytes.Join([][]byte{{0xa5, 0x05}, object, {0x04}, id, {0x03}, status, {0x02}, network, {0x01}, schema}, nil)
}

func TestObjectWireRejectsNonCanonicalWrongNetworkAndMalformed(t *testing.T) {
	request, _ := goldenRequest(t)
	if _, err := DecodeObjectGetRequest(nonCanonicalRequest(t, request), protocol.Alpha1NetworkID); err == nil {
		t.Fatal("non-canonical request accepted")
	}
	wrongNetwork := request
	wrongNetwork.NetworkID = "another-network"
	raw, _ := EncodeObjectGetRequest(wrongNetwork)
	if _, err := DecodeObjectGetRequest(raw, protocol.Alpha1NetworkID); err == nil {
		t.Fatal("wrong request network accepted")
	}
	for _, value := range []ObjectGetRequest{
		{SchemaVersion: 2, NetworkID: protocol.Alpha1NetworkID, ObjectID: request.ObjectID},
		{SchemaVersion: 1, NetworkID: protocol.Alpha1NetworkID, ObjectID: "../../bad"},
		{SchemaVersion: 1, NetworkID: protocol.Alpha1NetworkID, ObjectID: "zion:obj:sha512:" + string(bytes.Repeat([]byte{'0'}, 64))},
	} {
		candidate, _ := protocol.CanonicalEncode(value)
		if _, err := DecodeObjectGetRequest(candidate, protocol.Alpha1NetworkID); err == nil {
			t.Fatalf("malformed request accepted: %+v", value)
		}
	}
	for _, raw := range [][]byte{{}, {0xff}, {0xa3, 0x01}, bytes.Repeat([]byte{0}, MaxObjectRequestFrame+1)} {
		if _, err := DecodeObjectGetRequest(raw, protocol.Alpha1NetworkID); err == nil {
			t.Fatalf("bad request bytes accepted: %x", raw[:min(len(raw), 8)])
		}
	}
}

func TestObjectResponseValidationMatrix(t *testing.T) {
	response, _ := goldenResponse(t)
	if _, err := DecodeObjectGetResponse(nonCanonicalResponse(t, response), protocol.Alpha1NetworkID, response.ObjectID); err == nil {
		t.Fatal("non-canonical response accepted")
	}

	other := testObject([]byte("other"), "wire-other")
	otherID, _ := other.ObjectID()
	otherBytes, _ := other.CanonicalBytes()
	cases := []struct {
		name      string
		value     ObjectGetResponse
		requested string
	}{
		{"wrong-network", ObjectGetResponse{1, "other", ObjectFound, response.ObjectID, response.ObjectBytes}, response.ObjectID},
		{"unknown-status", ObjectGetResponse{1, protocol.Alpha1NetworkID, "UNKNOWN", response.ObjectID, []byte{}}, response.ObjectID},
		{"malformed-id", ObjectGetResponse{1, protocol.Alpha1NetworkID, ObjectNotFound, "bad", []byte{}}, response.ObjectID},
		{"wrong-response-id", ObjectGetResponse{1, protocol.Alpha1NetworkID, ObjectFound, otherID.String(), otherBytes}, response.ObjectID},
		{"wrong-object-bytes", ObjectGetResponse{1, protocol.Alpha1NetworkID, ObjectFound, response.ObjectID, otherBytes}, response.ObjectID},
		{"found-empty", ObjectGetResponse{1, protocol.Alpha1NetworkID, ObjectFound, response.ObjectID, []byte{}}, response.ObjectID},
		{"not-found-with-bytes", ObjectGetResponse{1, protocol.Alpha1NetworkID, ObjectNotFound, response.ObjectID, otherBytes}, response.ObjectID},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			raw, err := protocol.CanonicalEncode(test.value)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := DecodeObjectGetResponse(raw, protocol.Alpha1NetworkID, test.requested); err == nil {
				t.Fatal("invalid response accepted")
			}
		})
	}
	if _, err := DecodeObjectGetResponse(bytes.Repeat([]byte{0}, MaxObjectResponseFrame+1), protocol.Alpha1NetworkID, response.ObjectID); err == nil {
		t.Fatal("oversized response accepted")
	}
	notFound := ObjectGetResponse{1, protocol.Alpha1NetworkID, ObjectNotFound, response.ObjectID, []byte{}}
	raw, err := EncodeObjectGetResponse(notFound)
	if err != nil {
		t.Fatal(err)
	}
	got, err := DecodeObjectGetResponse(raw, protocol.Alpha1NetworkID, response.ObjectID)
	if err != nil || got.Status != ObjectNotFound {
		t.Fatalf("NOT_FOUND: %+v %v", got, err)
	}
}

func FuzzDecodeObjectGetRequest(f *testing.F) {
	_, raw := goldenRequest(f)
	f.Add(raw)
	f.Add([]byte{})
	f.Fuzz(func(t *testing.T, data []byte) { _, _ = DecodeObjectGetRequest(data, protocol.Alpha1NetworkID) })
}

func FuzzDecodeObjectGetResponse(f *testing.F) {
	response, raw := goldenResponse(f)
	f.Add(raw)
	f.Add([]byte{})
	f.Fuzz(func(t *testing.T, data []byte) {
		_, _ = DecodeObjectGetResponse(data, protocol.Alpha1NetworkID, response.ObjectID)
	})
}

func TestObjectWireEncodeRejectsInvalid(t *testing.T) {
	request, _ := goldenRequest(t)
	request.SchemaVersion = 99
	if _, err := EncodeObjectGetRequest(request); err == nil {
		t.Fatal("invalid request encoded")
	}
	response, _ := goldenResponse(t)
	response.Status = ObjectNotFound
	if _, err := EncodeObjectGetResponse(response); err == nil || !errors.Is(err, ErrInvalidObject) && err.Error() == "" {
		t.Fatal("invalid response encoded")
	}
}
