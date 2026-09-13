package resources

import (
	"bytes"
	"encoding/hex"
	"fmt"

	"github.com/kokora3/zion/internal/identity"
	"github.com/kokora3/zion/internal/protocol"
	"github.com/kokora3/zion/internal/registry"
)

const (
	Schema                 protocol.SchemaVersion = 1
	MaxNameBytes                                  = 256
	MaxSummaryBytes                               = 1024
	MaxLicenseBytes                               = 128
	MaxMaintainers                                = 16
	MaxMaintainerNameBytes                        = 128
	MaxObjectRefs                                 = 16
)

type ID struct{ protocol.HashDigest }

func (id ID) Validate() error { return id.HashDigest.Validate() }
func (id ID) String() string {
	if id.Validate() != nil {
		return ""
	}
	return "zion:resource:sha256:" + hex.EncodeToString(id.Digest)
}
func ParseID(value string) (ID, error) {
	hash, err := registry.ParseID(value, "resource")
	return ID{HashDigest: hash}, err
}

type Kind string

const (
	Repository Kind = "REPOSITORY"
	Dataset    Kind = "DATASET"
	Document   Kind = "DOCUMENT"
	Tool       Kind = "TOOL"
	Website    Kind = "WEBSITE"
	Model      Kind = "MODEL"
	Other      Kind = "OTHER"
)

func (kind Kind) Validate() error {
	switch kind {
	case Repository, Dataset, Document, Tool, Website, Model, Other:
		return nil
	default:
		return fmt.Errorf("invalid resource kind")
	}
}

type Maintainer struct {
	DisplayName string               `cbor:"1,keyasint" json:"display_name"`
	IdentityID  *identity.IdentityID `cbor:"2,keyasint,omitempty" json:"identity_id,omitempty"`
}

type EntryBody struct {
	SchemaVersion protocol.SchemaVersion        `cbor:"1,keyasint" json:"schema_version"`
	Kind          Kind                          `cbor:"2,keyasint" json:"kind"`
	Name          string                        `cbor:"3,keyasint" json:"name"`
	Summary       string                        `cbor:"4,keyasint" json:"summary"`
	ExternalURI   string                        `cbor:"5,keyasint,omitempty" json:"external_uri,omitempty"`
	License       string                        `cbor:"6,keyasint,omitempty" json:"license,omitempty"`
	Maintainers   []Maintainer                  `cbor:"7,keyasint" json:"maintainers"`
	ObjectRefs    []protocol.ObjectID           `cbor:"8,keyasint" json:"object_refs"`
	CanonicalRefs []registry.CanonicalReference `cbor:"9,keyasint" json:"canonical_refs"`
}

func (body EntryBody) Validate() error {
	if body.SchemaVersion != Schema || body.Kind.Validate() != nil || registry.ValidateText(body.Name, MaxNameBytes, false) != nil ||
		registry.ValidateText(body.Summary, MaxSummaryBytes, false) != nil || registry.ValidateHTTPSURI(body.ExternalURI) != nil ||
		registry.ValidateText(body.License, MaxLicenseBytes, true) != nil || len(body.Maintainers) > MaxMaintainers ||
		registry.ValidateObjectRefs(body.ObjectRefs, MaxObjectRefs) != nil || registry.ValidateReferences(body.CanonicalRefs) != nil {
		return fmt.Errorf("invalid resource entry")
	}
	for _, maintainer := range body.Maintainers {
		if registry.ValidateText(maintainer.DisplayName, MaxMaintainerNameBytes, false) != nil ||
			(maintainer.IdentityID != nil && maintainer.IdentityID.Validate() != nil) {
			return fmt.Errorf("invalid resource maintainer")
		}
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
		return EntryBody{}, fmt.Errorf("invalid resource entry")
	}
	canonical, _ := body.CanonicalBytes()
	if !bytes.Equal(data, canonical) {
		return EntryBody{}, fmt.Errorf("non-canonical resource entry")
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
	clone.Maintainers = make([]Maintainer, len(body.Maintainers))
	for position, maintainer := range body.Maintainers {
		clone.Maintainers[position] = maintainer
		if maintainer.IdentityID != nil {
			id := identity.IdentityID{HashDigest: registry.CloneHash(maintainer.IdentityID.HashDigest)}
			clone.Maintainers[position].IdentityID = &id
		}
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
