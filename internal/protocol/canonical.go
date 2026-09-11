package protocol

import (
	"crypto/sha256"
	"fmt"
	"time"

	"github.com/fxamacker/cbor/v2"
)

var canonicalEncMode cbor.EncMode
var canonicalDecMode cbor.DecMode

func init() {
	var err error
	canonicalEncMode, err = cbor.CoreDetEncOptions().EncMode()
	if err != nil {
		panic(fmt.Sprintf("create canonical CBOR encoder: %v", err))
	}
	canonicalDecMode, err = (cbor.DecOptions{DupMapKey: cbor.DupMapKeyEnforcedAPF, IndefLength: cbor.IndefLengthForbidden, TagsMd: cbor.TagsForbidden, UTF8: cbor.UTF8RejectInvalid, MaxNestedLevels: 16, MaxArrayElements: 1024, MaxMapPairs: 128, ExtraReturnErrors: cbor.ExtraDecErrorUnknownField}).DecMode()
	if err != nil {
		panic(fmt.Sprintf("create canonical CBOR decoder: %v", err))
	}
}

// CanonicalEncode uses RFC 8949 Core Deterministic Encoding. Object-specific
// text normalization is done by protocol objects, never opaque bytes.
func CanonicalEncode(v any) ([]byte, error) { return canonicalEncMode.Marshal(v) }
func CanonicalDecode(data []byte, v any) error {
	if len(data) == 0 {
		return fmt.Errorf("canonical CBOR: empty input")
	}
	return canonicalDecMode.Unmarshal(data, v)
}

type ProtocolTimestamp int64

func TimestampFromTime(t time.Time) ProtocolTimestamp { return ProtocolTimestamp(t.UTC().UnixMilli()) }
func (t ProtocolTimestamp) Time() time.Time           { return time.UnixMilli(int64(t)).UTC() }

type HashAlgorithm string

const HashAlgorithmSHA256 HashAlgorithm = "sha256"

type HashDigest struct {
	Algorithm HashAlgorithm `cbor:"1,keyasint"`
	Digest    []byte        `cbor:"2,keyasint"`
}

func (h HashDigest) Validate() error {
	if h.Algorithm != HashAlgorithmSHA256 {
		return fmt.Errorf("unknown hash algorithm %q", h.Algorithm)
	}
	if len(h.Digest) != sha256.Size {
		return fmt.Errorf("sha256 digest length %d", len(h.Digest))
	}
	return nil
}
func HashBytes(data []byte) HashDigest {
	sum := sha256.Sum256(data)
	return HashDigest{HashAlgorithmSHA256, sum[:]}
}
func HashCanonical(v any) (HashDigest, error) {
	b, err := CanonicalEncode(v)
	if err != nil {
		return HashDigest{}, err
	}
	return HashBytes(b), nil
}
