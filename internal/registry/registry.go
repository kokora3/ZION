// Package registry contains the small shared value types used by the two
// canonical Phase 11 registries. It owns no chain state or authorization.
package registry

import (
	"encoding/hex"
	"fmt"
	"net/url"
	"strings"
	"unicode/utf8"

	"github.com/kokora3/zion/internal/protocol"
	"golang.org/x/text/unicode/norm"
)

const (
	MaxExternalURIBytes = 2048
	MaxCanonicalRefs    = 32
	MaxRelationBytes    = 64
	MaxTargetIDBytes    = 96
)

type TargetKind string

const (
	TargetResearch TargetKind = "RESEARCH"
	TargetResource TargetKind = "RESOURCE"
	TargetObject   TargetKind = "OBJECT"
)

type CanonicalReference struct {
	Relation   string     `cbor:"1,keyasint" json:"relation"`
	TargetKind TargetKind `cbor:"2,keyasint" json:"target_kind"`
	TargetID   string     `cbor:"3,keyasint" json:"target_id"`
}

const (
	RelationCites      = "zion.canonical/cites/v1"
	RelationUses       = "zion.canonical/uses/v1"
	RelationImplements = "zion.canonical/implements/v1"
	RelationRelated    = "zion.canonical/related/v1"
)

func (reference CanonicalReference) Validate() error {
	if err := ValidateText(reference.Relation, MaxRelationBytes, false); err != nil ||
		len(reference.TargetID) == 0 || len(reference.TargetID) > MaxTargetIDBytes {
		return fmt.Errorf("invalid canonical reference")
	}
	switch reference.Relation {
	case RelationCites, RelationUses, RelationImplements, RelationRelated:
	default:
		return fmt.Errorf("unsupported canonical reference relation")
	}
	switch reference.TargetKind {
	case TargetResearch:
		_, err := ParseID(reference.TargetID, "research")
		return err
	case TargetResource:
		_, err := ParseID(reference.TargetID, "resource")
		return err
	case TargetObject:
		_, err := protocol.ParseObjectID(reference.TargetID)
		return err
	default:
		return fmt.Errorf("invalid canonical reference target kind")
	}
}

func ReferenceKey(reference CanonicalReference) string {
	return reference.Relation + "\x00" + string(reference.TargetKind) + "\x00" + reference.TargetID
}

func ValidateReferences(references []CanonicalReference) error {
	if len(references) > MaxCanonicalRefs {
		return fmt.Errorf("too many canonical references")
	}
	previous := ""
	for position, reference := range references {
		if err := reference.Validate(); err != nil {
			return err
		}
		key := ReferenceKey(reference)
		if position > 0 && key <= previous {
			return fmt.Errorf("canonical references must be sorted and unique")
		}
		previous = key
	}
	return nil
}

func ValidateText(value string, maximum int, optional bool) error {
	if (!optional && value == "") || !utf8.ValidString(value) || len([]byte(value)) > maximum || !norm.NFC.IsNormalString(value) {
		return fmt.Errorf("invalid canonical text")
	}
	return nil
}

func ValidateHTTPSURI(value string) error {
	if value == "" {
		return nil
	}
	if err := ValidateText(value, MaxExternalURIBytes, false); err != nil {
		return err
	}
	parsed, err := url.Parse(value)
	if err != nil || parsed.Scheme != "https" || parsed.Host == "" || parsed.User != nil || parsed.Fragment != "" || !parsed.IsAbs() {
		return fmt.Errorf("external URI must be an absolute HTTPS URI without credentials or fragment")
	}
	return nil
}

func ParseID(value, namespace string) (protocol.HashDigest, error) {
	parts := strings.Split(value, ":")
	if len(parts) != 4 || parts[0] != "zion" || parts[1] != namespace || parts[2] != "sha256" || strings.ToLower(parts[3]) != parts[3] {
		return protocol.HashDigest{}, fmt.Errorf("invalid %s ID", namespace)
	}
	digest, err := hex.DecodeString(parts[3])
	if err != nil {
		return protocol.HashDigest{}, fmt.Errorf("decode %s ID: %w", namespace, err)
	}
	hash := protocol.HashDigest{Algorithm: protocol.HashAlgorithmSHA256, Digest: digest}
	return hash, hash.Validate()
}

func CloneHash(hash protocol.HashDigest) protocol.HashDigest {
	return protocol.HashDigest{Algorithm: hash.Algorithm, Digest: append([]byte(nil), hash.Digest...)}
}

func CloneObjectID(id protocol.ObjectID) protocol.ObjectID {
	return protocol.ObjectID{HashDigest: CloneHash(id.HashDigest)}
}

func ValidateObjectRefs(refs []protocol.ObjectID, maximum int) error {
	if len(refs) > maximum {
		return fmt.Errorf("too many object references")
	}
	previous := ""
	for position, id := range refs {
		value := id.String()
		if value == "" || (position > 0 && value <= previous) {
			return fmt.Errorf("object references must be valid, sorted and unique")
		}
		previous = value
	}
	return nil
}
