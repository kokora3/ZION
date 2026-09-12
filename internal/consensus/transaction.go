package consensus

import (
	"bytes"
	"errors"
	"fmt"

	"github.com/kokora3/zion/internal/chain"
	"github.com/kokora3/zion/internal/protocol"
)

type DecodeErrorKind string

const (
	DecodeOversized    DecodeErrorKind = "OVERSIZED"
	DecodeMalformed    DecodeErrorKind = "MALFORMED"
	DecodeNonCanonical DecodeErrorKind = "NON_CANONICAL"
	DecodeSchema       DecodeErrorKind = "UNSUPPORTED_SCHEMA"
	DecodeType         DecodeErrorKind = "UNSUPPORTED_TYPE"
	DecodeNetwork      DecodeErrorKind = "WRONG_NETWORK"
)

// DecodeError classifies hostile wire input without exposing variable parser
// error text to deterministic ABCI results.
type DecodeError struct {
	Kind  DecodeErrorKind
	cause error
}

func (e *DecodeError) Error() string {
	if e.cause == nil {
		return "transaction decode: " + string(e.Kind)
	}
	return fmt.Sprintf("transaction decode: %s: %v", e.Kind, e.cause)
}

func (e *DecodeError) Unwrap() error { return e.cause }

func IsDecodeError(err error, kind DecodeErrorKind) bool {
	var target *DecodeError
	return errors.As(err, &target) && target.Kind == kind
}

// DecodeTransaction is the only raw consensus-byte entry point. It accepts
// exactly the canonical Phase 4 CBOR representation.
func DecodeTransaction(raw []byte, network protocol.NetworkID) (chain.Transaction, error) {
	if len(raw) > chain.MaxTransactionBytes {
		return chain.Transaction{}, &DecodeError{Kind: DecodeOversized}
	}
	var tx chain.Transaction
	if err := protocol.CanonicalDecode(raw, &tx); err != nil {
		return chain.Transaction{}, &DecodeError{Kind: DecodeMalformed, cause: err}
	}
	canonical, err := tx.CanonicalBytes()
	if err != nil {
		return chain.Transaction{}, &DecodeError{Kind: DecodeMalformed, cause: err}
	}
	if !bytes.Equal(raw, canonical) {
		return chain.Transaction{}, &DecodeError{Kind: DecodeNonCanonical}
	}
	if tx.SchemaVersion != chain.TransactionSchema {
		return chain.Transaction{}, &DecodeError{Kind: DecodeSchema}
	}
	if tx.NetworkID != network {
		return chain.Transaction{}, &DecodeError{Kind: DecodeNetwork}
	}
	switch tx.Type {
	case chain.IdentityCreate:
		if tx.IdentityCreate == nil || tx.KeyRotation != nil {
			return chain.Transaction{}, &DecodeError{Kind: DecodeMalformed}
		}
	case chain.KeyRotation:
		if tx.KeyRotation == nil || tx.IdentityCreate != nil {
			return chain.Transaction{}, &DecodeError{Kind: DecodeMalformed}
		}
	default:
		return chain.Transaction{}, &DecodeError{Kind: DecodeType}
	}
	return tx, nil
}
