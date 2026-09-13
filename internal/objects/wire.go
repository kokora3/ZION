package objects

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"io"

	"github.com/kokora3/zion/internal/protocol"
	libprotocol "github.com/libp2p/go-libp2p/core/protocol"
)

const (
	ObjectProtocolID       libprotocol.ID = "/zion/object/0.1.0"
	ObjectWireSchema                      = uint64(1)
	MaxObjectRequestFrame                 = 512
	MaxObjectResponseFrame                = MaxObjectBytes + 2048
)

type ObjectResponseStatus string

const (
	ObjectFound          ObjectResponseStatus = "FOUND"
	ObjectNotFound       ObjectResponseStatus = "NOT_FOUND"
	ObjectInvalidRequest ObjectResponseStatus = "INVALID_REQUEST"
	ObjectTooLarge       ObjectResponseStatus = "TOO_LARGE"
	ObjectStoreError     ObjectResponseStatus = "STORE_ERROR"
)

type ObjectGetRequest struct {
	SchemaVersion uint64             `cbor:"1,keyasint" json:"schema_version"`
	NetworkID     protocol.NetworkID `cbor:"2,keyasint" json:"network_id"`
	ObjectID      string             `cbor:"3,keyasint" json:"object_id"`
}

type ObjectGetResponse struct {
	SchemaVersion uint64               `cbor:"1,keyasint" json:"schema_version"`
	NetworkID     protocol.NetworkID   `cbor:"2,keyasint" json:"network_id"`
	Status        ObjectResponseStatus `cbor:"3,keyasint" json:"status"`
	ObjectID      string               `cbor:"4,keyasint" json:"object_id"`
	ObjectBytes   []byte               `cbor:"5,keyasint" json:"object_bytes"`
}

func EncodeObjectGetRequest(request ObjectGetRequest) ([]byte, error) {
	if err := validateObjectGetRequest(request, ""); err != nil {
		return nil, err
	}
	return encodeObjectMessage(request, MaxObjectRequestFrame)
}

func DecodeObjectGetRequest(data []byte, expectedNetwork protocol.NetworkID) (ObjectGetRequest, error) {
	var request ObjectGetRequest
	if err := decodeObjectMessage(data, MaxObjectRequestFrame, &request); err != nil {
		return request, err
	}
	if err := validateObjectGetRequest(request, expectedNetwork); err != nil {
		return request, err
	}
	return request, nil
}

func validateObjectGetRequest(request ObjectGetRequest, expectedNetwork protocol.NetworkID) error {
	if request.SchemaVersion != ObjectWireSchema {
		return fmt.Errorf("unsupported object request schema %d", request.SchemaVersion)
	}
	if request.NetworkID == "" {
		return fmt.Errorf("empty object request network")
	}
	if expectedNetwork != "" && request.NetworkID != expectedNetwork {
		return fmt.Errorf("wrong object request network")
	}
	id, err := protocol.ParseObjectID(request.ObjectID)
	if err != nil || id.Algorithm != protocol.HashAlgorithmSHA256 {
		return fmt.Errorf("invalid object request ID")
	}
	return nil
}

func EncodeObjectGetResponse(response ObjectGetResponse) ([]byte, error) {
	response.ObjectBytes = normalizeEmptyBytes(response.ObjectBytes)
	if err := validateObjectGetResponse(response, "", ""); err != nil {
		return nil, err
	}
	return encodeObjectMessage(response, MaxObjectResponseFrame)
}

func DecodeObjectGetResponse(data []byte, expectedNetwork protocol.NetworkID, requestedID string) (ObjectGetResponse, error) {
	var response ObjectGetResponse
	if err := decodeObjectMessage(data, MaxObjectResponseFrame, &response); err != nil {
		return response, err
	}
	if response.ObjectBytes == nil {
		return response, fmt.Errorf("object response bytes must use a byte string")
	}
	response.ObjectBytes = normalizeEmptyBytes(response.ObjectBytes)
	if err := validateObjectGetResponse(response, expectedNetwork, requestedID); err != nil {
		return response, err
	}
	return response, nil
}

func validateObjectGetResponse(response ObjectGetResponse, expectedNetwork protocol.NetworkID, requestedID string) error {
	if response.SchemaVersion != ObjectWireSchema {
		return fmt.Errorf("unsupported object response schema %d", response.SchemaVersion)
	}
	if response.NetworkID == "" {
		return fmt.Errorf("empty object response network")
	}
	if expectedNetwork != "" && response.NetworkID != expectedNetwork {
		return fmt.Errorf("wrong object response network")
	}
	id, err := protocol.ParseObjectID(response.ObjectID)
	if err != nil || id.Algorithm != protocol.HashAlgorithmSHA256 {
		return fmt.Errorf("invalid object response ID")
	}
	if requestedID != "" && response.ObjectID != requestedID {
		return fmt.Errorf("object response ID does not answer request")
	}
	switch response.Status {
	case ObjectFound:
		if len(response.ObjectBytes) == 0 {
			return fmt.Errorf("FOUND response has no object")
		}
		object, err := Decode(response.ObjectBytes)
		if err != nil {
			return fmt.Errorf("invalid returned object: %w", err)
		}
		actual, err := object.ObjectID()
		if err != nil || actual.String() != response.ObjectID {
			return fmt.Errorf("returned object does not match response ID")
		}
	case ObjectNotFound, ObjectInvalidRequest, ObjectTooLarge, ObjectStoreError:
		if len(response.ObjectBytes) != 0 {
			return fmt.Errorf("%s response contains object bytes", response.Status)
		}
	default:
		return fmt.Errorf("unknown object response status %q", response.Status)
	}
	return nil
}

func encodeObjectMessage(value any, limit int) ([]byte, error) {
	data, err := protocol.CanonicalEncode(value)
	if err != nil {
		return nil, err
	}
	if len(data) > limit {
		return nil, fmt.Errorf("object protocol message exceeds %d bytes", limit)
	}
	return data, nil
}

func decodeObjectMessage(data []byte, limit int, value any) error {
	if len(data) == 0 || len(data) > limit {
		return fmt.Errorf("invalid object protocol message size %d", len(data))
	}
	if err := protocol.CanonicalDecode(data, value); err != nil {
		return err
	}
	canonical, err := protocol.CanonicalEncode(value)
	if err != nil {
		return err
	}
	if !bytes.Equal(data, canonical) {
		return fmt.Errorf("non-canonical object protocol message")
	}
	return nil
}

func normalizeEmptyBytes(data []byte) []byte {
	if len(data) == 0 {
		return []byte{}
	}
	return data
}

func writeObjectFrame(writer io.Writer, data []byte, limit int) error {
	if len(data) == 0 || len(data) > limit {
		return fmt.Errorf("invalid object frame size %d", len(data))
	}
	var header [4]byte
	binary.BigEndian.PutUint32(header[:], uint32(len(data)))
	if _, err := writer.Write(header[:]); err != nil {
		return err
	}
	_, err := writer.Write(data)
	return err
}

func readObjectFrame(reader io.Reader, limit int) ([]byte, error) {
	var header [4]byte
	if _, err := io.ReadFull(reader, header[:]); err != nil {
		return nil, err
	}
	size := binary.BigEndian.Uint32(header[:])
	if size == 0 || uint64(size) > uint64(limit) {
		return nil, fmt.Errorf("invalid object frame size %d", size)
	}
	data := make([]byte, int(size))
	if _, err := io.ReadFull(reader, data); err != nil {
		return nil, err
	}
	return data, nil
}
