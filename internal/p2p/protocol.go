package p2p

import (
	"fmt"
	"sort"

	libprotocol "github.com/libp2p/go-libp2p/core/protocol"
)

const (
	HelloProtocolID libprotocol.ID = "/zion/hello/0.1.0"
	PEXProtocolID   libprotocol.ID = "/zion/pex/0.1.0"
	WireSchema                     = uint64(1)
)

type Version struct {
	Major uint16 `cbor:"1,keyasint" json:"major"`
	Minor uint16 `cbor:"2,keyasint" json:"minor"`
	Patch uint16 `cbor:"3,keyasint" json:"patch"`
}

var CurrentVersion = Version{Major: 0, Minor: 1, Patch: 0}

func (v Version) String() string { return fmt.Sprintf("%d.%d.%d", v.Major, v.Minor, v.Patch) }

func normalizeVersions(versions []Version) ([]Version, error) {
	if len(versions) == 0 || len(versions) > MaxProtocolVersions {
		return nil, fmt.Errorf("invalid protocol version count %d", len(versions))
	}
	seen := make(map[Version]struct{}, len(versions))
	out := append([]Version(nil), versions...)
	for _, v := range out {
		if _, ok := seen[v]; ok {
			return nil, fmt.Errorf("duplicate protocol version %s", v)
		}
		seen[v] = struct{}{}
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Major != out[j].Major {
			return out[i].Major < out[j].Major
		}
		if out[i].Minor != out[j].Minor {
			return out[i].Minor < out[j].Minor
		}
		return out[i].Patch < out[j].Patch
	})
	return out, nil
}

func negotiateVersion(local, remote []Version) (Version, error) {
	localSet := make(map[Version]struct{}, len(local))
	for _, v := range local {
		localSet[v] = struct{}{}
	}
	var common []Version
	for _, v := range remote {
		if _, ok := localSet[v]; ok {
			common = append(common, v)
		}
	}
	if len(common) == 0 {
		return Version{}, fmt.Errorf("no common ZION P2P protocol version")
	}
	sort.Slice(common, func(i, j int) bool {
		if common[i].Major != common[j].Major {
			return common[i].Major > common[j].Major
		}
		if common[i].Minor != common[j].Minor {
			return common[i].Minor > common[j].Minor
		}
		return common[i].Patch > common[j].Patch
	})
	return common[0], nil
}
