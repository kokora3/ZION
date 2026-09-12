package p2p

import (
	"bytes"
	"context"
	"fmt"
	"slices"
	"sort"
	"time"

	"github.com/kokora3/zion/internal/protocol"
	libnetwork "github.com/libp2p/go-libp2p/core/network"
	libpeer "github.com/libp2p/go-libp2p/core/peer"
	ma "github.com/multiformats/go-multiaddr"
)

type PEXRequest struct {
	SchemaVersion uint64 `cbor:"1,keyasint" json:"schema_version"`
}

type PeerRecord struct {
	PeerID    string   `cbor:"1,keyasint" json:"peer_id"`
	Addresses []string `cbor:"2,keyasint" json:"addresses"`
	Roles     []Role   `cbor:"3,keyasint" json:"roles"`
}

type PEXResponse struct {
	SchemaVersion uint64       `cbor:"1,keyasint" json:"schema_version"`
	Peers         []PeerRecord `cbor:"2,keyasint" json:"peers"`
}

func (n *Node) handlePEX(stream libnetwork.Stream) {
	defer stream.Close()
	if !n.IsUsable(stream.Conn().RemotePeer()) {
		_ = stream.Reset()
		return
	}
	_ = stream.SetDeadline(time.Now().Add(n.cfg.Limits.PEXTimeout))
	requestBytes, err := readFrame(stream, MaxPEXRequestSize)
	if err != nil {
		_ = stream.Reset()
		return
	}
	if _, err := decodePEXRequest(requestBytes); err != nil {
		_ = stream.Reset()
		return
	}
	response := PEXResponse{SchemaVersion: WireSchema, Peers: n.pexRecords(stream.Conn().RemotePeer())}
	encoded, err := EncodePEX(response)
	if err != nil || writeFrame(stream, encoded, MaxPEXResponseSize) != nil {
		_ = stream.Reset()
	}
}

func (n *Node) pexRecords(requester libpeer.ID) []PeerRecord {
	peers := n.UsablePeers()
	records := make([]PeerRecord, 0, len(peers))
	for _, remote := range peers {
		if remote.PeerID == requester || remote.PeerID == n.PeerID() || len(remote.Addresses) == 0 {
			continue
		}
		records = append(records, PeerRecord{PeerID: remote.PeerID.String(),
			Addresses: sortedAddressStrings(remote.Addresses, n.cfg.Limits.MaxAddressesPerPeer),
			Roles:     append([]Role(nil), remote.Roles...)})
	}
	sort.Slice(records, func(i, j int) bool { return records[i].PeerID < records[j].PeerID })
	if len(records) > n.cfg.Limits.MaxPEXPeersPerResponse {
		records = records[:n.cfg.Limits.MaxPEXPeersPerResponse]
	}
	return records
}

func (n *Node) RequestPeers(ctx context.Context, peerID libpeer.ID) ([]PeerRecord, error) {
	if peerID == n.PeerID() || !n.IsUsable(peerID) {
		return nil, fmt.Errorf("PEX requires a usable remote peer")
	}
	requestCtx, cancel := context.WithTimeout(ctx, n.cfg.Limits.PEXTimeout)
	defer cancel()
	stream, err := n.host.NewStream(requestCtx, peerID, PEXProtocolID)
	if err != nil {
		return nil, err
	}
	defer stream.Close()
	_ = stream.SetDeadline(time.Now().Add(n.cfg.Limits.PEXTimeout))
	request, _ := encodePEXRequest(PEXRequest{SchemaVersion: WireSchema})
	if err := writeFrame(stream, request, MaxPEXRequestSize); err != nil {
		return nil, err
	}
	data, err := readFrame(stream, MaxPEXResponseSize)
	if err != nil {
		return nil, err
	}
	response, err := DecodePEX(data)
	if err != nil {
		return nil, err
	}
	if err := validatePEX(response, n.PeerID(), n.cfg.Limits.MaxPEXPeersPerResponse, n.cfg.Limits.MaxAddressesPerPeer); err != nil {
		return nil, err
	}
	return response.Peers, nil
}

func encodePEXRequest(request PEXRequest) ([]byte, error) {
	if request.SchemaVersion != WireSchema {
		return nil, fmt.Errorf("unsupported PEX request schema")
	}
	return protocol.CanonicalEncode(request)
}

func decodePEXRequest(data []byte) (PEXRequest, error) {
	if len(data) == 0 || len(data) > MaxPEXRequestSize {
		return PEXRequest{}, fmt.Errorf("invalid PEX request size")
	}
	var request PEXRequest
	if err := protocol.CanonicalDecode(data, &request); err != nil {
		return PEXRequest{}, err
	}
	encoded, _ := protocol.CanonicalEncode(request)
	if request.SchemaVersion != WireSchema || !bytes.Equal(data, encoded) {
		return PEXRequest{}, fmt.Errorf("invalid or non-canonical PEX request")
	}
	return request, nil
}

func EncodePEX(response PEXResponse) ([]byte, error) {
	if err := validatePEX(response, "", HardMaxPEXPeersPerResponse, HardMaxAddressesPerPeer); err != nil {
		return nil, err
	}
	data, err := protocol.CanonicalEncode(response)
	if err != nil {
		return nil, err
	}
	if len(data) > MaxPEXResponseSize {
		return nil, fmt.Errorf("PEX response exceeds %d bytes", MaxPEXResponseSize)
	}
	return data, nil
}

func DecodePEX(data []byte) (PEXResponse, error) {
	if len(data) == 0 || len(data) > MaxPEXResponseSize {
		return PEXResponse{}, fmt.Errorf("invalid PEX response size %d", len(data))
	}
	var response PEXResponse
	if err := protocol.CanonicalDecode(data, &response); err != nil {
		return PEXResponse{}, fmt.Errorf("decode PEX response: %w", err)
	}
	if err := validatePEX(response, "", HardMaxPEXPeersPerResponse, HardMaxAddressesPerPeer); err != nil {
		return PEXResponse{}, err
	}
	encoded, err := protocol.CanonicalEncode(response)
	if err != nil || !bytes.Equal(data, encoded) {
		return PEXResponse{}, fmt.Errorf("PEX response is not canonical CBOR")
	}
	return response, nil
}

func validatePEX(response PEXResponse, self libpeer.ID, maxPeers, maxAddresses int) error {
	if response.SchemaVersion != WireSchema {
		return fmt.Errorf("unsupported PEX schema %d", response.SchemaVersion)
	}
	if len(response.Peers) > maxPeers {
		return fmt.Errorf("too many PEX peers")
	}
	seen := make(map[libpeer.ID]struct{}, len(response.Peers))
	previous := ""
	for _, record := range response.Peers {
		peerID, err := libpeer.Decode(record.PeerID)
		if err != nil {
			return fmt.Errorf("invalid PEX PeerID: %w", err)
		}
		if peerID == self {
			return fmt.Errorf("PEX contains self PeerID")
		}
		if _, exists := seen[peerID]; exists {
			return fmt.Errorf("duplicate PEX PeerID")
		}
		if previous != "" && record.PeerID <= previous {
			return fmt.Errorf("PEX records are not in canonical order")
		}
		previous = record.PeerID
		seen[peerID] = struct{}{}
		if len(record.Addresses) == 0 || len(record.Addresses) > maxAddresses {
			return fmt.Errorf("invalid PEX address count")
		}
		if err := validateAddressStrings(record.Addresses, maxAddresses); err != nil {
			return err
		}
		if !sort.StringsAreSorted(record.Addresses) {
			return fmt.Errorf("PEX addresses are not in canonical order")
		}
		roles, err := normalizeRoles(record.Roles)
		if err != nil {
			return err
		}
		if !slices.Equal(roles, record.Roles) {
			return fmt.Errorf("PEX roles are not in canonical order")
		}
		for _, addressText := range record.Addresses {
			address, _ := ma.NewMultiaddr(addressText)
			if value, err := address.ValueForProtocol(ma.P_P2P); err == nil && value != record.PeerID {
				return fmt.Errorf("PEX address PeerID mismatch")
			}
		}
	}
	return nil
}
