package research

import (
	"bytes"
	"encoding/hex"
	"fmt"
	"sort"

	"github.com/kokora3/zion/internal/identity"
	"github.com/kokora3/zion/internal/protocol"
	"github.com/kokora3/zion/internal/registry"
)

const (
	Schema                   protocol.SchemaVersion = 1
	MaxTitleBytes                                   = 256
	MaxSummaryBytes                                 = 1024
	MaxContributors                                 = 32
	MaxContributorNameBytes                         = 128
	MaxExternalIdentifiers                          = 16
	MaxIdentifierSchemeBytes                        = 16
	MaxIdentifierValueBytes                         = 256
	MaxObjectRefs                                   = 16
)

type ID struct{ protocol.HashDigest }

func (id ID) Validate() error { return id.HashDigest.Validate() }
func (id ID) String() string {
	if id.Validate() != nil {
		return ""
	}
	return "zion:research:sha256:" + hex.EncodeToString(id.Digest)
}
func ParseID(value string) (ID, error) {
	hash, err := registry.ParseID(value, "research")
	return ID{HashDigest: hash}, err
}

type Contributor struct {
	DisplayName string               `cbor:"1,keyasint" json:"display_name"`
	IdentityID  *identity.IdentityID `cbor:"2,keyasint,omitempty" json:"identity_id,omitempty"`
}

type IdentifierScheme string

const (
	DOI   IdentifierScheme = "DOI"
	ArXiv IdentifierScheme = "ARXIV"
	Other IdentifierScheme = "OTHER"
)

type ExternalIdentifier struct {
	Scheme IdentifierScheme `cbor:"1,keyasint" json:"scheme"`
	Value  string           `cbor:"2,keyasint" json:"value"`
}

type EntryBody struct {
	SchemaVersion       protocol.SchemaVersion        `cbor:"1,keyasint" json:"schema_version"`
	Title               string                        `cbor:"2,keyasint" json:"title"`
	Summary             string                        `cbor:"3,keyasint" json:"summary"`
	Contributors        []Contributor                 `cbor:"4,keyasint" json:"contributors"`
	ExternalIdentifiers []ExternalIdentifier          `cbor:"5,keyasint" json:"external_identifiers"`
	ExternalURI         string                        `cbor:"6,keyasint,omitempty" json:"external_uri,omitempty"`
	ObjectRefs          []protocol.ObjectID           `cbor:"7,keyasint" json:"object_refs"`
	CanonicalRefs       []registry.CanonicalReference `cbor:"8,keyasint" json:"canonical_refs"`
}

func (body EntryBody) Validate() error {
	if body.SchemaVersion != Schema || registry.ValidateText(body.Title, MaxTitleBytes, false) != nil ||
		registry.ValidateText(body.Summary, MaxSummaryBytes, false) != nil || len(body.Contributors) == 0 ||
		len(body.Contributors) > MaxContributors || len(body.ExternalIdentifiers) > MaxExternalIdentifiers ||
		registry.ValidateHTTPSURI(body.ExternalURI) != nil || registry.ValidateObjectRefs(body.ObjectRefs, MaxObjectRefs) != nil ||
		registry.ValidateReferences(body.CanonicalRefs) != nil {
		return fmt.Errorf("invalid research entry")
	}
	for _, contributor := range body.Contributors {
		if registry.ValidateText(contributor.DisplayName, MaxContributorNameBytes, false) != nil ||
			(contributor.IdentityID != nil && contributor.IdentityID.Validate() != nil) {
			return fmt.Errorf("invalid research contributor")
		}
	}
	previous := ""
	for position, identifier := range body.ExternalIdentifiers {
		if identifier.Scheme != DOI && identifier.Scheme != ArXiv && identifier.Scheme != Other ||
			registry.ValidateText(string(identifier.Scheme), MaxIdentifierSchemeBytes, false) != nil ||
			registry.ValidateText(identifier.Value, MaxIdentifierValueBytes, false) != nil {
			return fmt.Errorf("invalid external research identifier")
		}
		key := string(identifier.Scheme) + "\x00" + identifier.Value
		if position > 0 && key <= previous {
			return fmt.Errorf("external identifiers must be sorted and unique")
		}
		previous = key
	}
	return nil
}

func (body EntryBody) CanonicalBytes() ([]byte, error) {
	if err := body.Validate(); err != nil {
		return nil, err
	}
	return protocol.CanonicalEncode(body)
}

func Decode(data []byte) (EntryBody, error) {
	var body EntryBody
	if len(data) > 4096 || protocol.CanonicalDecode(data, &body) != nil || body.Validate() != nil {
		return EntryBody{}, fmt.Errorf("invalid research entry")
	}
	canonical, _ := body.CanonicalBytes()
	if !bytes.Equal(data, canonical) {
		return EntryBody{}, fmt.Errorf("non-canonical research entry")
	}
	return body, nil
}

func (body EntryBody) ID() (ID, error) {
	canonical, err := body.CanonicalBytes()
	if err != nil {
		return ID{}, err
	}
	return ID{HashDigest: protocol.HashBytes(canonical)}, nil
}

func CloneBody(body EntryBody) EntryBody {
	clone := body
	clone.Contributors = make([]Contributor, len(body.Contributors))
	for position, contributor := range body.Contributors {
		clone.Contributors[position] = contributor
		if contributor.IdentityID != nil {
			id := identity.IdentityID{HashDigest: registry.CloneHash(contributor.IdentityID.HashDigest)}
			clone.Contributors[position].IdentityID = &id
		}
	}
	if body.ExternalIdentifiers != nil {
		clone.ExternalIdentifiers = make([]ExternalIdentifier, len(body.ExternalIdentifiers))
		copy(clone.ExternalIdentifiers, body.ExternalIdentifiers)
	}
	clone.ObjectRefs = make([]protocol.ObjectID, len(body.ObjectRefs))
	for position, id := range body.ObjectRefs {
		clone.ObjectRefs[position] = registry.CloneObjectID(id)
	}
	if body.CanonicalRefs != nil {
		clone.CanonicalRefs = make([]registry.CanonicalReference, len(body.CanonicalRefs))
		copy(clone.CanonicalRefs, body.CanonicalRefs)
	}
	return clone
}

func SortExternalIdentifiers(values []ExternalIdentifier) {
	sort.Slice(values, func(i, j int) bool {
		left, right := string(values[i].Scheme)+"\x00"+values[i].Value, string(values[j].Scheme)+"\x00"+values[j].Value
		return left < right
	})
}
