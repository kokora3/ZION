package p2p

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"io"
	"slices"
	"sort"

	"github.com/kokora3/zion/internal/protocol"
	libpeer "github.com/libp2p/go-libp2p/core/peer"
	ma "github.com/multiformats/go-multiaddr"
)

type Hello struct {
	SchemaVersion       uint64              `cbor:"1,keyasint" json:"schema_version"`
	NetworkID           protocol.NetworkID  `cbor:"2,keyasint" json:"network_id"`
	NetworkFingerprint  protocol.HashDigest `cbor:"3,keyasint" json:"network_fingerprint"`
	SupportedVersions   []Version           `cbor:"4,keyasint" json:"supported_versions"`
	Roles               []Role              `cbor:"5,keyasint" json:"roles"`
	AuthenticatedPeerID string              `cbor:"6,keyasint" json:"authenticated_peer_id"`
	AdvertisedAddresses []string            `cbor:"7,keyasint" json:"advertised_addresses"`
}

func EncodeHello(hello Hello) ([]byte, error) {
	if err := validateHello(hello, ""); err != nil {
		return nil, err
	}
	data, err := protocol.CanonicalEncode(hello)
	if err != nil {
		return nil, err
	}
	if len(data) > MaxHelloSize {
		return nil, fmt.Errorf("hello exceeds %d bytes", MaxHelloSize)
	}
	return data, nil
}

func DecodeHello(data []byte) (Hello, error) {
	if len(data) == 0 || len(data) > MaxHelloSize {
		return Hello{}, fmt.Errorf("invalid hello size %d", len(data))
	}
	var hello Hello
	if err := protocol.CanonicalDecode(data, &hello); err != nil {
		return Hello{}, fmt.Errorf("decode hello: %w", err)
	}
	if err := validateHello(hello, ""); err != nil {
		return Hello{}, err
	}
	encoded, err := protocol.CanonicalEncode(hello)
	if err != nil || !bytes.Equal(data, encoded) {
		return Hello{}, fmt.Errorf("hello is not canonical CBOR")
	}
	return hello, nil
}

func validateHello(hello Hello, authenticated libpeer.ID) error {
	if hello.SchemaVersion != WireSchema {
		return fmt.Errorf("unsupported hello schema %d", hello.SchemaVersion)
	}
	if hello.NetworkID == "" {
		return fmt.Errorf("empty hello network ID")
	}
	if err := hello.NetworkFingerprint.Validate(); err != nil {
		return fmt.Errorf("invalid hello network fingerprint: %w", err)
	}
	versions, err := normalizeVersions(hello.SupportedVersions)
	if err != nil {
		return err
	}
	if !slices.Equal(versions, hello.SupportedVersions) {
		return fmt.Errorf("protocol versions are not in canonical order")
	}
	roles, err := normalizeRoles(hello.Roles)
	if err != nil {
		return err
	}
	if !slices.Equal(roles, hello.Roles) {
		return fmt.Errorf("roles are not in canonical order")
	}
	reported, err := libpeer.Decode(hello.AuthenticatedPeerID)
	if err != nil {
		return fmt.Errorf("invalid reported PeerID: %w", err)
	}
	if authenticated != "" && reported != authenticated {
		return fmt.Errorf("reported PeerID does not match authenticated remote PeerID")
	}
	if len(hello.AdvertisedAddresses) > MaxAdvertisedAddresses {
		return fmt.Errorf("too many advertised addresses")
	}
	if err := validateAddressStrings(hello.AdvertisedAddresses, MaxAdvertisedAddresses); err != nil {
		return err
	}
	if !sort.StringsAreSorted(hello.AdvertisedAddresses) {
		return fmt.Errorf("advertised addresses are not in canonical order")
	}
	for _, addressText := range hello.AdvertisedAddresses {
		address, _ := ma.NewMultiaddr(addressText)
		if value, err := address.ValueForProtocol(ma.P_P2P); err == nil && value != reported.String() {
			return fmt.Errorf("advertised address PeerID mismatch")
		}
	}
	return nil
}

func validateAddressStrings(addresses []string, maximum int) error {
	if len(addresses) > maximum {
		return fmt.Errorf("too many multiaddrs: %d", len(addresses))
	}
	seen := make(map[string]struct{}, len(addresses))
	for _, value := range addresses {
		if len(value) == 0 || len(value) > MaxMultiaddrSize {
			return fmt.Errorf("invalid multiaddr size %d", len(value))
		}
		address, err := ma.NewMultiaddr(value)
		if err != nil {
			return fmt.Errorf("invalid multiaddr: %w", err)
		}
		if _, err := address.ValueForProtocol(ma.P_CIRCUIT); err == nil {
			return fmt.Errorf("relay addresses are not supported in Phase 7")
		}
		if _, exists := seen[address.String()]; exists {
			return fmt.Errorf("duplicate multiaddr")
		}
		seen[address.String()] = struct{}{}
	}
	return nil
}

func writeFrame(w io.Writer, data []byte, maximum int) error {
	if len(data) == 0 || len(data) > maximum {
		return fmt.Errorf("frame size %d exceeds bound %d", len(data), maximum)
	}
	var prefix [4]byte
	binary.BigEndian.PutUint32(prefix[:], uint32(len(data)))
	if _, err := w.Write(prefix[:]); err != nil {
		return err
	}
	_, err := w.Write(data)
	return err
}

func readFrame(r io.Reader, maximum int) ([]byte, error) {
	var prefix [4]byte
	if _, err := io.ReadFull(r, prefix[:]); err != nil {
		return nil, err
	}
	size := int(binary.BigEndian.Uint32(prefix[:]))
	if size < 1 || size > maximum {
		return nil, fmt.Errorf("frame size %d exceeds bound %d", size, maximum)
	}
	data := make([]byte, size)
	if _, err := io.ReadFull(r, data); err != nil {
		return nil, err
	}
	return data, nil
}

func sortedAddressStrings(addresses []ma.Multiaddr, maximum int) []string {
	seen := make(map[string]struct{}, len(addresses))
	result := make([]string, 0, len(addresses))
	for _, address := range addresses {
		value := address.String()
		if len(value) > MaxMultiaddrSize {
			continue
		}
		if _, exists := seen[value]; exists {
			continue
		}
		seen[value] = struct{}{}
		result = append(result, value)
	}
	sort.Strings(result)
	if len(result) > maximum {
		result = result[:maximum]
	}
	return result
}
