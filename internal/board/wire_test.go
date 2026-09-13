package board

import (
	"bytes"
	"testing"

	"github.com/kokora3/zion/internal/protocol"
)

func TestBoardWireRejectsWrongNetworkBoundsAndNonCanonical(t *testing.T) {
	_, _, _, eventObjectBytes := deterministicGoldenEvent(t)
	announce, _ := EncodeAnnounce(Announce{SchemaVersion: BoardWireSchema, NetworkID: protocol.Alpha1NetworkID, EventObject: eventObjectBytes})
	if _, err := DecodeAnnounce(announce, "another-network"); err == nil {
		t.Fatal("wrong-network announcement accepted")
	}
	if _, err := DecodeAnnounce(bytes.Repeat([]byte{0}, MaxAnnounceFrame+1), protocol.Alpha1NetworkID); err == nil {
		t.Fatal("oversized announcement accepted")
	}
	if _, err := EncodeSyncRequest(SyncRequest{SchemaVersion: BoardWireSchema, NetworkID: protocol.Alpha1NetworkID, Limit: MaxSyncPageEvents + 1}); err == nil {
		t.Fatal("oversized sync page accepted")
	}
	if _, err := EncodeSyncResponse(SyncResponse{SchemaVersion: BoardWireSchema, NetworkID: protocol.Alpha1NetworkID,
		Events: make([][]byte, MaxSyncPageEvents+1)}); err == nil {
		t.Fatal("too many sync events accepted")
	}
	if _, err := DecodeSyncRequest(append([]byte{0x9f}, announce...), protocol.Alpha1NetworkID); err == nil {
		t.Fatal("malformed sync request accepted")
	}
}

func FuzzDecodeBoardAnnounce(f *testing.F) {
	_, _, _, objectBytes := deterministicGoldenEvent(f)
	seed, _ := EncodeAnnounce(Announce{SchemaVersion: BoardWireSchema, NetworkID: protocol.Alpha1NetworkID, EventObject: objectBytes})
	f.Add(seed)
	f.Fuzz(func(t *testing.T, data []byte) { _, _ = DecodeAnnounce(data, protocol.Alpha1NetworkID) })
}

func FuzzDecodeBoardSyncRequest(f *testing.F) {
	seed, _ := EncodeSyncRequest(SyncRequest{SchemaVersion: BoardWireSchema, NetworkID: protocol.Alpha1NetworkID, Limit: 1})
	f.Add(seed)
	f.Fuzz(func(t *testing.T, data []byte) { _, _ = DecodeSyncRequest(data, protocol.Alpha1NetworkID) })
}

func FuzzDecodeBoardSyncResponse(f *testing.F) {
	_, _, _, objectBytes := deterministicGoldenEvent(f)
	seed, _ := EncodeSyncResponse(SyncResponse{SchemaVersion: BoardWireSchema, NetworkID: protocol.Alpha1NetworkID, Events: [][]byte{objectBytes}})
	f.Add(seed)
	f.Fuzz(func(t *testing.T, data []byte) { _, _ = DecodeSyncResponse(data, protocol.Alpha1NetworkID) })
}
