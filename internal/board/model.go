package board

import (
	"bytes"
	"crypto/ed25519"
	"errors"
	"fmt"
	"unicode/utf8"

	"github.com/kokora3/zion/internal/chain"
	"github.com/kokora3/zion/internal/identity"
	"github.com/kokora3/zion/internal/membership"
	"github.com/kokora3/zion/internal/objects"
	"github.com/kokora3/zion/internal/protocol"
	"github.com/kokora3/zion/internal/research"
	"github.com/kokora3/zion/internal/resources"
)

const (
	ContentSchema  protocol.SchemaVersion = 1
	EventSchema    protocol.SchemaVersion = 1
	ContentType                           = "zion.board/content/v1"
	EventType                             = "zion.board/event/v1"
	SigningPurpose                        = "zion.board.event/v1"

	MaxBoardContentBodyBytes = 64 * 1024
	MaxBoardTitleBytes       = 256
	MaxBoardEventBytes       = 32 * 1024
	MaxBoardEventObjectBytes = MaxBoardEventBytes + 2048
	MaxBoardReferences       = 32
	MaxRelationBytes         = 96
	MaxTargetKindBytes       = 64
	MaxTargetIDBytes         = 256
)

var (
	ErrInvalidContent      = errors.New("invalid board content")
	ErrInvalidEvent        = errors.New("invalid board event")
	ErrPublicationDenied   = errors.New("board publication not authorized")
	ErrParentNotFound      = errors.New("board parent post not found")
	ErrBoardContentMissing = errors.New("board content object missing")
	ErrReferenceNotFound   = errors.New("board canonical reference target not found")
)

type ContentFormat string

const (
	FormatText     ContentFormat = "TEXT"
	FormatMarkdown ContentFormat = "MARKDOWN"
)

type Content struct {
	SchemaVersion protocol.SchemaVersion `cbor:"1,keyasint"`
	Format        ContentFormat          `cbor:"2,keyasint"`
	Title         string                 `cbor:"3,keyasint,omitempty"`
	Body          string                 `cbor:"4,keyasint"`
}

func (c Content) Validate() error {
	if c.SchemaVersion != ContentSchema || (c.Format != FormatText && c.Format != FormatMarkdown) || c.Body == "" ||
		!utf8.ValidString(c.Title) || !utf8.ValidString(c.Body) || len([]byte(c.Title)) > MaxBoardTitleBytes ||
		len([]byte(c.Body)) > MaxBoardContentBodyBytes {
		return ErrInvalidContent
	}
	return nil
}

func (c Content) CanonicalBytes() ([]byte, error) {
	if err := c.Validate(); err != nil {
		return nil, err
	}
	return protocol.CanonicalEncode(c)
}

func DecodeContent(data []byte) (Content, error) {
	if len(data) > MaxBoardContentBodyBytes+MaxBoardTitleBytes+1024 {
		return Content{}, ErrInvalidContent
	}
	var content Content
	if err := protocol.CanonicalDecode(data, &content); err != nil || content.Validate() != nil {
		return Content{}, ErrInvalidContent
	}
	canonical, _ := content.CanonicalBytes()
	if !bytes.Equal(data, canonical) {
		return Content{}, ErrInvalidContent
	}
	return content, nil
}

type EventKind string

const (
	KindPost  EventKind = "POST"
	KindReply EventKind = "REPLY"
)

type Reference struct {
	Relation   string `cbor:"1,keyasint"`
	TargetKind string `cbor:"2,keyasint"`
	TargetID   string `cbor:"3,keyasint"`
}

func (r Reference) Validate() error {
	if r.Relation == "" || r.TargetKind == "" || r.TargetID == "" || !utf8.ValidString(r.Relation) ||
		!utf8.ValidString(r.TargetKind) || !utf8.ValidString(r.TargetID) || len([]byte(r.Relation)) > MaxRelationBytes ||
		len([]byte(r.TargetKind)) > MaxTargetKindBytes || len([]byte(r.TargetID)) > MaxTargetIDBytes {
		return ErrInvalidEvent
	}
	switch r.TargetKind {
	case "OBJECT":
		if _, err := protocol.ParseObjectID(r.TargetID); err != nil {
			return ErrInvalidEvent
		}
	case "RESEARCH":
		if _, err := research.ParseID(r.TargetID); err != nil {
			return ErrInvalidEvent
		}
	case "RESOURCE":
		if _, err := resources.ParseID(r.TargetID); err != nil {
			return ErrInvalidEvent
		}
	default:
		return ErrInvalidEvent
	}
	return nil
}

type EventBody struct {
	SchemaVersion  protocol.SchemaVersion     `cbor:"1,keyasint"`
	NetworkID      protocol.NetworkID         `cbor:"2,keyasint"`
	Kind           EventKind                  `cbor:"3,keyasint"`
	AuthorIdentity identity.IdentityID        `cbor:"4,keyasint"`
	AuthorKeyID    identity.KeyID             `cbor:"5,keyasint"`
	CreatedAt      protocol.ProtocolTimestamp `cbor:"6,keyasint"`
	ParentPost     string                     `cbor:"7,keyasint,omitempty"`
	ContentObject  string                     `cbor:"8,keyasint"`
	References     []Reference                `cbor:"9,keyasint"`
}

func (b EventBody) Validate() error {
	if b.SchemaVersion != EventSchema || b.NetworkID == "" || b.AuthorIdentity.Validate() != nil || b.AuthorKeyID.Validate() != nil ||
		b.CreatedAt < 0 || len(b.References) > MaxBoardReferences {
		return ErrInvalidEvent
	}
	if _, err := protocol.ParseObjectID(b.ContentObject); err != nil {
		return ErrInvalidEvent
	}
	switch b.Kind {
	case KindPost:
		if b.ParentPost != "" {
			return ErrInvalidEvent
		}
	case KindReply:
		if _, err := protocol.ParseObjectID(b.ParentPost); err != nil {
			return ErrInvalidEvent
		}
	default:
		return ErrInvalidEvent
	}
	for _, reference := range b.References {
		if reference.Validate() != nil {
			return ErrInvalidEvent
		}
	}
	return nil
}

type SignedEvent struct {
	Body      EventBody          `cbor:"1,keyasint"`
	Signature identity.Signature `cbor:"2,keyasint"`
}

func SignEvent(body EventBody, key ed25519.PrivateKey) (SignedEvent, error) {
	if err := body.Validate(); err != nil {
		return SignedEvent{}, err
	}
	signature, err := identity.SignPurpose(body.NetworkID, SigningPurpose, body, key)
	if err != nil {
		return SignedEvent{}, err
	}
	if signature.KeyID.String() != body.AuthorKeyID.String() {
		return SignedEvent{}, ErrInvalidEvent
	}
	return SignedEvent{Body: body, Signature: signature}, nil
}

func (e SignedEvent) CanonicalBytes() ([]byte, error) {
	if err := e.Body.Validate(); err != nil || e.Signature.Validate() != nil || e.Signature.KeyID.String() != e.Body.AuthorKeyID.String() {
		return nil, ErrInvalidEvent
	}
	data, err := protocol.CanonicalEncode(e)
	if err != nil || len(data) > MaxBoardEventBytes {
		return nil, ErrInvalidEvent
	}
	return data, nil
}

func DecodeEvent(data []byte) (SignedEvent, error) {
	if len(data) > MaxBoardEventBytes {
		return SignedEvent{}, ErrInvalidEvent
	}
	var event SignedEvent
	if err := protocol.CanonicalDecode(data, &event); err != nil {
		return SignedEvent{}, ErrInvalidEvent
	}
	canonical, err := event.CanonicalBytes()
	if err != nil || !bytes.Equal(data, canonical) {
		return SignedEvent{}, ErrInvalidEvent
	}
	return event, nil
}

func DecodeContentObject(object objects.Object) (Content, error) {
	if object.Core.ObjectType != ContentType || object.Core.Visibility != protocol.VisibilityPublic {
		return Content{}, ErrInvalidContent
	}
	if _, err := object.CanonicalBytes(); err != nil {
		return Content{}, ErrInvalidContent
	}
	return DecodeContent(object.Payload)
}

func DecodeEventObject(object objects.Object) (SignedEvent, error) {
	if object.Core.ObjectType != EventType || object.Core.Visibility != protocol.VisibilityPublic {
		return SignedEvent{}, ErrInvalidEvent
	}
	canonical, err := object.CanonicalBytes()
	if err != nil || len(canonical) > MaxBoardEventObjectBytes {
		return SignedEvent{}, ErrInvalidEvent
	}
	event, err := DecodeEvent(object.Payload)
	if err != nil || object.Core.CreatedAt != event.Body.CreatedAt {
		return SignedEvent{}, ErrInvalidEvent
	}
	return event, nil
}

func PostID(eventObject objects.Object) (protocol.ObjectID, error) {
	if _, err := DecodeEventObject(eventObject); err != nil {
		return protocol.ObjectID{}, err
	}
	return eventObject.ObjectID()
}

func VerifySignature(event SignedEvent, publicKey identity.PublicKey) error {
	if _, err := event.CanonicalBytes(); err != nil {
		return err
	}
	return identity.VerifyPurpose(event.Body.NetworkID, SigningPurpose, event.Body, publicKey, event.Signature)
}

type AuthorizationStatus string

const (
	CurrentlyAuthorized     AuthorizationStatus = "CURRENTLY_AUTHORIZED"
	HistoricalOrUnconfirmed AuthorizationStatus = "HISTORICAL_OR_UNCONFIRMED"
)

func VerifyAndClassify(event SignedEvent, state chain.State) (AuthorizationStatus, membership.Status, error) {
	if event.Body.NetworkID != state.NetworkID {
		return "", "", ErrPublicationDenied
	}
	stored, ok := state.Identities[event.Body.AuthorIdentity.String()]
	if !ok {
		return "", "", ErrPublicationDenied
	}
	publicKey, ok := stored.Keys[event.Body.AuthorKeyID.String()]
	if !ok {
		return "", "", ErrPublicationDenied
	}
	if err := identity.VerifyPurpose(event.Body.NetworkID, SigningPurpose, event.Body, publicKey, event.Signature); err != nil {
		return "", "", fmt.Errorf("%w: signature", ErrInvalidEvent)
	}
	status := state.Memberships[event.Body.AuthorIdentity.String()]
	if status == membership.Active && stored.Active[event.Body.AuthorKeyID.String()] && !stored.Revoked {
		return CurrentlyAuthorized, status, nil
	}
	return HistoricalOrUnconfirmed, status, nil
}

func BuildContentObject(content Content, createdAt protocol.ProtocolTimestamp) (objects.Object, error) {
	payload, err := content.CanonicalBytes()
	if err != nil {
		return objects.Object{}, err
	}
	return newObject(ContentType, createdAt, payload), nil
}

func BuildEventObject(event SignedEvent) (objects.Object, error) {
	payload, err := event.CanonicalBytes()
	if err != nil {
		return objects.Object{}, err
	}
	object := newObject(EventType, event.Body.CreatedAt, payload)
	canonical, err := object.CanonicalBytes()
	if err != nil || len(canonical) > MaxBoardEventObjectBytes {
		return objects.Object{}, ErrInvalidEvent
	}
	return object, nil
}

func newObject(objectType protocol.ObjectType, createdAt protocol.ProtocolTimestamp, payload []byte) objects.Object {
	return objects.Object{Core: protocol.UnsignedObjectCore{ObjectType: objectType, SchemaVersion: protocol.UnsignedObjectSchemaV1,
		CreatedAt: createdAt, ContentHash: protocol.NewContentHash(payload), SizeBytes: uint64(len(payload)),
		Visibility: protocol.VisibilityPublic, Metadata: map[string]string{}}, Payload: append([]byte(nil), payload...)}
}
