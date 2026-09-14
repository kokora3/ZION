package p2p

import (
	"fmt"
	"net"
	"path/filepath"

	"github.com/kokora3/zion/internal/protocol"
	ma "github.com/multiformats/go-multiaddr"
)

type Config struct {
	Enabled            bool
	NetworkID          protocol.NetworkID
	NetworkFingerprint protocol.HashDigest
	KeyPath            string
	PeerCachePath      string
	ListenAddresses    []string
	AdvertiseAddresses []string
	BootstrapAddresses []string
	FallbackAddresses  []string
	ManualPeers        []string
	Roles              []Role
	SupportedVersions  []Version
	Limits             Limits
}

func DefaultConfig(dataDir string, fingerprint protocol.HashDigest) Config {
	return Config{
		Enabled:            true,
		NetworkID:          protocol.Alpha1NetworkID,
		NetworkFingerprint: fingerprint,
		KeyPath:            filepath.Join(dataDir, "p2p", "peer.key"),
		PeerCachePath:      filepath.Join(dataDir, "p2p", "peers.json"),
		ListenAddresses:    []string{"/ip4/0.0.0.0/udp/42000/quic-v1"},
		Roles:              []Role{RoleNormal},
		SupportedVersions:  []Version{CurrentVersion},
		Limits:             DefaultLimits(),
	}
}

func (c *Config) normalize() error {
	if !c.Enabled {
		return fmt.Errorf("P2P is disabled")
	}
	if c.NetworkID == "" || c.KeyPath == "" || c.PeerCachePath == "" {
		return fmt.Errorf("P2P network, key path, and cache path are required")
	}
	if err := c.NetworkFingerprint.Validate(); err != nil {
		return fmt.Errorf("network fingerprint: %w", err)
	}
	c.NetworkFingerprint.Digest = append([]byte(nil), c.NetworkFingerprint.Digest...)
	roles, err := normalizeRoles(c.Roles)
	if err != nil {
		return err
	}
	c.Roles = roles
	versions, err := normalizeVersions(c.SupportedVersions)
	if err != nil {
		return err
	}
	c.SupportedVersions = versions
	if err := c.Limits.validate(); err != nil {
		return err
	}
	c.ListenAddresses = append([]string(nil), c.ListenAddresses...)
	c.AdvertiseAddresses = append([]string(nil), c.AdvertiseAddresses...)
	c.BootstrapAddresses = append([]string(nil), c.BootstrapAddresses...)
	c.FallbackAddresses = append([]string(nil), c.FallbackAddresses...)
	c.ManualPeers = append([]string(nil), c.ManualPeers...)
	if err := validateAddressStrings(c.ListenAddresses, HardMaxCachedPeers); err != nil {
		return err
	}
	if err := validateAdvertiseAddresses(c.AdvertiseAddresses); err != nil {
		return err
	}
	for _, group := range [][]string{c.BootstrapAddresses, c.FallbackAddresses, c.ManualPeers} {
		for _, address := range group {
			if _, err := parseDialAddress(address); err != nil {
				return err
			}
		}
		if len(group) > HardMaxCachedPeers {
			return fmt.Errorf("too many discovery addresses")
		}
		if err := validateAddressStrings(group, HardMaxCachedPeers); err != nil {
			return err
		}
	}
	return nil
}

// Validate checks P2P configuration without opening sockets or creating keys.
func (c Config) Validate() error { return c.normalize() }

func validateAdvertiseAddresses(addresses []string) error {
	if err := validateAddressStrings(addresses, MaxAdvertisedAddresses); err != nil {
		return err
	}
	for _, value := range addresses {
		address, _ := ma.NewMultiaddr(value)
		if _, err := address.ValueForProtocol(ma.P_P2P); err == nil {
			return fmt.Errorf("configured advertised address must not include /p2p/PeerID")
		}
		if _, err := address.ValueForProtocol(ma.P_UDP); err != nil {
			return fmt.Errorf("configured advertised address must use UDP")
		}
		if _, err := address.ValueForProtocol(ma.P_QUIC_V1); err != nil {
			return fmt.Errorf("configured advertised address must use quic-v1")
		}
		if value, err := address.ValueForProtocol(ma.P_IP4); err == nil {
			if err := validatePublicIP(value); err != nil {
				return err
			}
			continue
		}
		if value, err := address.ValueForProtocol(ma.P_IP6); err == nil {
			if err := validatePublicIP(value); err != nil {
				return err
			}
			continue
		}
		if _, err := address.ValueForProtocol(ma.P_DNS4); err == nil {
			continue
		}
		if _, err := address.ValueForProtocol(ma.P_DNS6); err == nil {
			continue
		}
		return fmt.Errorf("configured advertised address must use ip4, ip6, dns4, or dns6")
	}
	return nil
}

func validatePublicIP(value string) error {
	ip := net.ParseIP(value)
	if ip == nil || !ip.IsGlobalUnicast() || ip.IsUnspecified() || ip.IsLoopback() || ip.IsPrivate() || ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() {
		return fmt.Errorf("configured advertised IP must be publicly routable")
	}
	return nil
}

func errLimit(name string, value int) error {
	return fmt.Errorf("invalid P2P %s limit %d", name, value)
}
