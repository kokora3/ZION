package objects

import (
	"bytes"
	"errors"
	"fmt"

	"github.com/kokora3/zion/internal/protocol"
)

// MaxObjectBytes is the zion-alpha-1 hard limit for the complete canonical
// stored representation. SPEC.md requires a limit but does not freeze a
// numeric value, so Phase 9 freezes the conservative 1 MiB alpha limit.
const MaxObjectBytes = 1 << 20

var (
	ErrInvalidObject  = errors.New("invalid object")
	ErrObjectTooLarge = errors.New("object exceeds hard size limit")
)

// Object couples the frozen Phase 2 identity body with its opaque content.
// Its ObjectID is derived only from Core using protocol.UnsignedObjectCore's
// existing ObjectID method. Payload integrity is bound by Core.ContentHash and
// Core.SizeBytes; the derived ID is never embedded in its own input.
type Object struct {
	Core    protocol.UnsignedObjectCore `cbor:"1,keyasint"`
	Payload []byte                      `cbor:"2,keyasint"`
}

// CanonicalBytes validates and returns the immutable bytes stored on disk.
func (o Object) CanonicalBytes() ([]byte, error) {
	if len(o.Payload) > MaxObjectBytes {
		return nil, ErrObjectTooLarge
	}
	coreBytes, err := o.Core.CanonicalBytes()
	if err != nil {
		return nil, fmt.Errorf("%w: core: %v", ErrInvalidObject, err)
	}
	core, err := protocol.DecodeUnsignedObjectCore(coreBytes)
	if err != nil {
		return nil, fmt.Errorf("%w: canonical core: %v", ErrInvalidObject, err)
	}
	if core.SizeBytes != uint64(len(o.Payload)) {
		return nil, fmt.Errorf("%w: payload size is %d, core declares %d", ErrInvalidObject, len(o.Payload), core.SizeBytes)
	}
	want := protocol.NewContentHash(o.Payload)
	if core.ContentHash.Algorithm != want.Algorithm || !bytes.Equal(core.ContentHash.Digest, want.Digest) {
		return nil, fmt.Errorf("%w: payload content hash mismatch", ErrInvalidObject)
	}
	canonical, err := protocol.CanonicalEncode(Object{Core: core, Payload: append([]byte(nil), o.Payload...)})
	if err != nil {
		return nil, fmt.Errorf("%w: encode: %v", ErrInvalidObject, err)
	}
	if len(canonical) > MaxObjectBytes {
		return nil, ErrObjectTooLarge
	}
	return canonical, nil
}

// ObjectID reuses the frozen Phase 2 ObjectID derivation exactly.
func (o Object) ObjectID() (protocol.ObjectID, error) {
	if _, err := o.CanonicalBytes(); err != nil {
		return protocol.ObjectID{}, err
	}
	return o.Core.ObjectID()
}

// Decode parses only canonical, bounded representations.
func Decode(data []byte) (Object, error) {
	if len(data) > MaxObjectBytes {
		return Object{}, ErrObjectTooLarge
	}
	var o Object
	if err := protocol.CanonicalDecode(data, &o); err != nil {
		return Object{}, fmt.Errorf("%w: decode: %v", ErrInvalidObject, err)
	}
	canonical, err := o.CanonicalBytes()
	if err != nil {
		return Object{}, err
	}
	if !bytes.Equal(data, canonical) {
		return Object{}, fmt.Errorf("%w: non-canonical representation", ErrInvalidObject)
	}
	o.Payload = append([]byte(nil), o.Payload...)
	return o, nil
}
