package board

import (
	"bytes"
	"fmt"

	"github.com/kokora3/zion/internal/objects"
	"github.com/kokora3/zion/internal/protocol"
)

const (
	BoardWireSchema      protocol.SchemaVersion = 1
	AnnounceProtocolID                          = "/zion/board/announce/0.1.0"
	SyncProtocolID                              = "/zion/board/sync/0.1.0"
	MaxAnnounceFrame                            = MaxBoardEventBytes + 4096
	MaxSyncRequestFrame                         = 1024
	MaxSyncResponseFrame                        = 1 << 20
	MaxSyncPageEvents                           = 24
)

type Announce struct {
	SchemaVersion protocol.SchemaVersion `cbor:"1,keyasint"`
	NetworkID     protocol.NetworkID     `cbor:"2,keyasint"`
	EventObject   []byte                 `cbor:"3,keyasint"`
}

type SyncRequest struct {
	SchemaVersion protocol.SchemaVersion `cbor:"1,keyasint"`
	NetworkID     protocol.NetworkID     `cbor:"2,keyasint"`
	AfterPostID   string                 `cbor:"3,keyasint,omitempty"`
	Limit         uint16                 `cbor:"4,keyasint"`
}

type SyncResponse struct {
	SchemaVersion protocol.SchemaVersion `cbor:"1,keyasint"`
	NetworkID     protocol.NetworkID     `cbor:"2,keyasint"`
	Events        [][]byte               `cbor:"3,keyasint"`
	NextPostID    string                 `cbor:"4,keyasint,omitempty"`
	More          bool                   `cbor:"5,keyasint"`
}

func EncodeAnnounce(value Announce) ([]byte, error) {
	if value.SchemaVersion != BoardWireSchema || value.NetworkID == "" {
		return nil, fmt.Errorf("invalid board announcement")
	}
	object, err := objects.Decode(value.EventObject)
	if err != nil {
		return nil, err
	}
	if _, err := DecodeEventObject(object); err != nil {
		return nil, err
	}
	data, err := protocol.CanonicalEncode(value)
	if err != nil || len(data) > MaxAnnounceFrame {
		return nil, fmt.Errorf("board announcement exceeds limit")
	}
	return data, nil
}

func DecodeAnnounce(data []byte, network protocol.NetworkID) (Announce, error) {
	if len(data) > MaxAnnounceFrame {
		return Announce{}, fmt.Errorf("board announcement exceeds limit")
	}
	var value Announce
	if err := protocol.CanonicalDecode(data, &value); err != nil || value.SchemaVersion != BoardWireSchema || value.NetworkID != network {
		return Announce{}, fmt.Errorf("invalid board announcement")
	}
	canonical, err := EncodeAnnounce(value)
	if err != nil || !bytes.Equal(data, canonical) {
		return Announce{}, fmt.Errorf("non-canonical board announcement")
	}
	value.EventObject = append([]byte(nil), value.EventObject...)
	return value, nil
}

func EncodeSyncRequest(value SyncRequest) ([]byte, error) {
	if value.SchemaVersion != BoardWireSchema || value.NetworkID == "" || value.Limit < 1 || value.Limit > MaxSyncPageEvents {
		return nil, fmt.Errorf("invalid board sync request")
	}
	if value.AfterPostID != "" {
		if _, err := protocol.ParseObjectID(value.AfterPostID); err != nil {
			return nil, fmt.Errorf("invalid board sync cursor")
		}
	}
	data, err := protocol.CanonicalEncode(value)
	if err != nil || len(data) > MaxSyncRequestFrame {
		return nil, fmt.Errorf("board sync request exceeds limit")
	}
	return data, nil
}

func DecodeSyncRequest(data []byte, network protocol.NetworkID) (SyncRequest, error) {
	if len(data) > MaxSyncRequestFrame {
		return SyncRequest{}, fmt.Errorf("board sync request exceeds limit")
	}
	var value SyncRequest
	if err := protocol.CanonicalDecode(data, &value); err != nil || value.NetworkID != network {
		return SyncRequest{}, fmt.Errorf("invalid board sync request")
	}
	canonical, err := EncodeSyncRequest(value)
	if err != nil || !bytes.Equal(data, canonical) {
		return SyncRequest{}, fmt.Errorf("non-canonical board sync request")
	}
	return value, nil
}

func EncodeSyncResponse(value SyncResponse) ([]byte, error) {
	if value.SchemaVersion != BoardWireSchema || value.NetworkID == "" || len(value.Events) > MaxSyncPageEvents {
		return nil, fmt.Errorf("invalid board sync response")
	}
	if value.More && (len(value.Events) == 0 || value.NextPostID == "") {
		return nil, fmt.Errorf("invalid board sync continuation")
	}
	for _, raw := range value.Events {
		object, err := objects.Decode(raw)
		if err != nil {
			return nil, err
		}
		if _, err := DecodeEventObject(object); err != nil {
			return nil, err
		}
	}
	if value.NextPostID != "" {
		if _, err := protocol.ParseObjectID(value.NextPostID); err != nil {
			return nil, err
		}
	}
	data, err := protocol.CanonicalEncode(value)
	if err != nil || len(data) > MaxSyncResponseFrame {
		return nil, fmt.Errorf("board sync response exceeds limit")
	}
	return data, nil
}

func DecodeSyncResponse(data []byte, network protocol.NetworkID) (SyncResponse, error) {
	if len(data) > MaxSyncResponseFrame {
		return SyncResponse{}, fmt.Errorf("board sync response exceeds limit")
	}
	var value SyncResponse
	if err := protocol.CanonicalDecode(data, &value); err != nil || value.NetworkID != network {
		return SyncResponse{}, fmt.Errorf("invalid board sync response")
	}
	canonical, err := EncodeSyncResponse(value)
	if err != nil || !bytes.Equal(data, canonical) {
		return SyncResponse{}, fmt.Errorf("non-canonical board sync response")
	}
	for index := range value.Events {
		value.Events[index] = append([]byte(nil), value.Events[index]...)
	}
	return value, nil
}
