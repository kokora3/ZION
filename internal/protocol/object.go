package protocol

import (
	"encoding/hex"
	"fmt"
	"strings"

	"golang.org/x/text/unicode/norm"
)

type ObjectType string
type Visibility string

const (
	VisibilityPublic       Visibility    = "PUBLIC"
	VisibilityLocal        Visibility    = "LOCAL"
	UnsignedObjectSchemaV1 SchemaVersion = 1
)

func (v Visibility) Validate() error {
	if v != VisibilityPublic && v != VisibilityLocal {
		return fmt.Errorf("invalid visibility %q", v)
	}
	return nil
}

// ContentHash hashes opaque payload bytes; it is distinct from ObjectID.
type ContentHash struct{ HashDigest }

func NewContentHash(payload []byte) ContentHash { return ContentHash{HashBytes(payload)} }

// ObjectID hashes canonical object-body bytes, never payload bytes alone.
type ObjectID struct{ HashDigest }

func (id ObjectID) Validate() error { return id.HashDigest.Validate() }
func (id ObjectID) String() string {
	if id.Validate() != nil {
		return ""
	}
	return "zion:obj:" + string(id.Algorithm) + ":" + hex.EncodeToString(id.Digest)
}
func ParseObjectID(s string) (ObjectID, error) {
	p := strings.Split(s, ":")
	if len(p) != 4 || p[0] != "zion" || p[1] != "obj" || p[2] != "sha256" || strings.ToLower(p[3]) != p[3] {
		return ObjectID{}, fmt.Errorf("invalid object ID syntax")
	}
	d, err := hex.DecodeString(p[3])
	if err != nil {
		return ObjectID{}, fmt.Errorf("decode object ID: %w", err)
	}
	id := ObjectID{HashDigest{HashAlgorithm(p[2]), d}}
	if err := id.Validate(); err != nil {
		return ObjectID{}, err
	}
	return id, nil
}

// DeriveObjectID receives only canonical body data, never an envelope carrying
// the resulting ID; this prevents a recursive self-hashing cycle.
func DeriveObjectID(body any) (ObjectID, error) {
	h, err := HashCanonical(body)
	if err != nil {
		return ObjectID{}, err
	}
	return ObjectID{h}, nil
}

// UnsignedObjectCore is a Phase 2 body, not a complete valid object: identity
// and signature fields are intentionally absent.
type UnsignedObjectCore struct {
	ObjectType    ObjectType        `cbor:"1,keyasint"`
	SchemaVersion SchemaVersion     `cbor:"2,keyasint"`
	CreatedAt     ProtocolTimestamp `cbor:"3,keyasint"`
	ContentHash   ContentHash       `cbor:"4,keyasint"`
	SizeBytes     uint64            `cbor:"5,keyasint"`
	Visibility    Visibility        `cbor:"6,keyasint"`
	Metadata      map[string]string `cbor:"7,keyasint"`
}

func (c UnsignedObjectCore) canonicalBody() (UnsignedObjectCore, error) {
	if c.ObjectType == "" || !isASCII(string(c.ObjectType)) {
		return c, fmt.Errorf("object type must be nonempty ASCII")
	}
	if c.SchemaVersion != UnsignedObjectSchemaV1 {
		return c, fmt.Errorf("unsupported object schema version %d", c.SchemaVersion)
	}
	if err := c.ContentHash.Validate(); err != nil {
		return c, err
	}
	if err := c.Visibility.Validate(); err != nil {
		return c, err
	}
	metadata := c.Metadata
	c.Metadata = make(map[string]string, len(metadata))
	for k, v := range metadata {
		if k == "" {
			return c, fmt.Errorf("empty metadata key")
		}
		c.Metadata[norm.NFC.String(k)] = norm.NFC.String(v)
	}
	return c, nil
}
func (c UnsignedObjectCore) CanonicalBytes() ([]byte, error) {
	b, err := c.canonicalBody()
	if err != nil {
		return nil, err
	}
	return CanonicalEncode(b)
}
func (c UnsignedObjectCore) ObjectID() (ObjectID, error) {
	b, err := c.canonicalBody()
	if err != nil {
		return ObjectID{}, err
	}
	return DeriveObjectID(b)
}
func DecodeUnsignedObjectCore(data []byte) (UnsignedObjectCore, error) {
	var c UnsignedObjectCore
	if err := CanonicalDecode(data, &c); err != nil {
		return c, err
	}
	return c.canonicalBody()
}
func isASCII(s string) bool {
	for _, r := range s {
		if r > 0x7f {
			return false
		}
	}
	return true
}
