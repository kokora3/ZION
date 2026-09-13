package node

import (
	"bytes"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"io"
	"strings"

	"github.com/kokora3/zion/internal/chain"
	"github.com/kokora3/zion/internal/consensus"
	"github.com/kokora3/zion/internal/protocol"
	libprotocol "github.com/libp2p/go-libp2p/core/protocol"
)

const (
	RuntimeWireSchema = uint64(1)
	TxProtocolID      = libprotocol.ID("/zion/tx/0.1.0")
	StateProtocolID   = libprotocol.ID("/zion/state/0.1.0")
	MaxStateFrame     = MaxSnapshotBytes + 2048
	MaxTxFrame        = chain.MaxTransactionBytes + 256
)

type TxRelayMessage struct {
	SchemaVersion uint64 `cbor:"1,keyasint" json:"schema_version"`
	Transaction   []byte `cbor:"2,keyasint" json:"transaction"`
}

type RelayStatus string

const (
	RelayAccepted RelayStatus = "ACCEPTED_FOR_SUBMISSION"
	RelayKnown    RelayStatus = "KNOWN_ALREADY"
	RelayRejected RelayStatus = "REJECTED_LOCALLY"
)

type TxRelayResponse struct {
	SchemaVersion uint64      `cbor:"1,keyasint"`
	Status        RelayStatus `cbor:"2,keyasint"`
	TxID          string      `cbor:"3,keyasint,omitempty"`
}

type StateSyncRequest struct {
	SchemaVersion uint64 `cbor:"1,keyasint"`
}

type StateSnapshotOffer struct {
	SchemaVersion      uint64              `cbor:"1,keyasint" json:"schema_version"`
	NetworkID          protocol.NetworkID  `cbor:"2,keyasint" json:"network_id"`
	NetworkFingerprint protocol.HashDigest `cbor:"3,keyasint" json:"network_fingerprint"`
	Height             int64               `cbor:"4,keyasint" json:"height"`
	StateHash          chain.StateHash     `cbor:"5,keyasint" json:"state_hash"`
	CanonicalState     []byte              `cbor:"6,keyasint" json:"canonical_state"`
	Finalized          []FinalityNotice    `cbor:"7,keyasint,omitempty" json:"finalized,omitempty"`
}

type FinalityNotice struct {
	TxID       string     `cbor:"1,keyasint" json:"tx_id"`
	Height     int64      `cbor:"2,keyasint" json:"height"`
	ResultCode chain.Code `cbor:"3,keyasint" json:"result_code"`
	StateHash  string     `cbor:"4,keyasint" json:"state_hash"`
}

func EncodeTxRelay(message TxRelayMessage, network protocol.NetworkID) ([]byte, error) {
	if message.SchemaVersion != RuntimeWireSchema || len(message.Transaction) == 0 || len(message.Transaction) > chain.MaxTransactionBytes {
		return nil, fmt.Errorf("invalid transaction relay message")
	}
	if _, err := consensus.DecodeTransaction(message.Transaction, network); err != nil {
		return nil, err
	}
	return encodeBounded(message, MaxTxFrame)
}

func DecodeTxRelay(data []byte, network protocol.NetworkID) (TxRelayMessage, error) {
	var message TxRelayMessage
	if err := decodeCanonicalBounded(data, MaxTxFrame, &message); err != nil {
		return TxRelayMessage{}, err
	}
	if message.SchemaVersion != RuntimeWireSchema || len(message.Transaction) == 0 || len(message.Transaction) > chain.MaxTransactionBytes {
		return TxRelayMessage{}, fmt.Errorf("invalid transaction relay message")
	}
	if _, err := consensus.DecodeTransaction(message.Transaction, network); err != nil {
		return TxRelayMessage{}, err
	}
	return message, nil
}

func EncodeStateOffer(offer StateSnapshotOffer) ([]byte, error) {
	if err := validateStateOfferShape(offer); err != nil {
		return nil, err
	}
	return encodeBounded(offer, MaxStateFrame)
}

func DecodeStateOffer(data []byte) (StateSnapshotOffer, error) {
	var offer StateSnapshotOffer
	if err := decodeCanonicalBounded(data, MaxStateFrame, &offer); err != nil {
		return StateSnapshotOffer{}, err
	}
	if err := validateStateOfferShape(offer); err != nil {
		return StateSnapshotOffer{}, err
	}
	return offer, nil
}

func validateStateOfferShape(offer StateSnapshotOffer) error {
	if offer.SchemaVersion != RuntimeWireSchema || offer.NetworkID == "" || offer.Height < 0 ||
		offer.NetworkFingerprint.Validate() != nil || offer.StateHash.Validate() != nil ||
		len(offer.CanonicalState) == 0 || len(offer.CanonicalState) > MaxSnapshotBytes {
		return fmt.Errorf("invalid state snapshot offer")
	}
	if len(offer.Finalized) > 256 {
		return fmt.Errorf("too many finality notices")
	}
	seen := make(map[string]struct{}, len(offer.Finalized))
	for _, notice := range offer.Finalized {
		if notice.Height < 1 || notice.Height > offer.Height || !validResultCode(notice.ResultCode) || !validStateHashString(notice.StateHash) {
			return fmt.Errorf("invalid finality notice")
		}
		if _, err := chain.ParseTxID(notice.TxID); err != nil {
			return fmt.Errorf("invalid finality TxID")
		}
		if _, exists := seen[notice.TxID]; exists {
			return fmt.Errorf("duplicate finality notice")
		}
		seen[notice.TxID] = struct{}{}
	}
	return nil
}

func validResultCode(code chain.Code) bool {
	switch code {
	case chain.OK, chain.WrongNetwork, chain.Unsupported, chain.Invalid, chain.TooLarge, chain.Exists, chain.NotFound,
		chain.KeyNotActive, chain.Sequence, chain.GovernanceUnavailable, chain.NotEligible, chain.ProposalExists,
		chain.ProposalNotFound, chain.ProposalClosed, chain.DuplicateVote, chain.NotReady, chain.Stale, chain.AlreadyExecuted:
		return true
	default:
		return false
	}
}

func validStateHashString(value string) bool {
	const prefix = "zion:state:sha256:"
	if !strings.HasPrefix(value, prefix) || len(value) != len(prefix)+64 {
		return false
	}
	digest := strings.TrimPrefix(value, prefix)
	decoded, err := hex.DecodeString(digest)
	return err == nil && len(decoded) == 32 && hex.EncodeToString(decoded) == digest
}

func encodeBounded(value any, maximum int) ([]byte, error) {
	data, err := protocol.CanonicalEncode(value)
	if err != nil {
		return nil, err
	}
	if len(data) == 0 || len(data) > maximum {
		return nil, fmt.Errorf("canonical message exceeds limit")
	}
	return data, nil
}

func decodeCanonicalBounded(data []byte, maximum int, value any) error {
	if len(data) == 0 || len(data) > maximum {
		return fmt.Errorf("invalid message size")
	}
	if err := protocol.CanonicalDecode(data, value); err != nil {
		return err
	}
	canonical, err := protocol.CanonicalEncode(value)
	if err != nil || !bytes.Equal(data, canonical) {
		return fmt.Errorf("message is not canonical CBOR")
	}
	return nil
}

func writeRuntimeFrame(writer io.Writer, data []byte, maximum int) error {
	if len(data) == 0 || len(data) > maximum {
		return fmt.Errorf("invalid runtime frame size")
	}
	var prefix [4]byte
	binary.BigEndian.PutUint32(prefix[:], uint32(len(data)))
	if _, err := writer.Write(prefix[:]); err != nil {
		return err
	}
	_, err := writer.Write(data)
	return err
}

func readRuntimeFrame(reader io.Reader, maximum int) ([]byte, error) {
	var prefix [4]byte
	if _, err := io.ReadFull(reader, prefix[:]); err != nil {
		return nil, err
	}
	size := int(binary.BigEndian.Uint32(prefix[:]))
	if size < 1 || size > maximum {
		return nil, fmt.Errorf("frame exceeds limit")
	}
	data := make([]byte, size)
	if _, err := io.ReadFull(reader, data); err != nil {
		return nil, err
	}
	return data, nil
}
