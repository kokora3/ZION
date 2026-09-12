package p2p

import (
	"fmt"
	"path/filepath"

	"github.com/kokora3/zion/internal/protocol"
)

type Config struct {
	Enabled            bool
	NetworkID          protocol.NetworkID
	NetworkFingerprint protocol.HashDigest
	KeyPath            string
	PeerCachePath      string
	ListenAddresses    []string
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
	c.BootstrapAddresses = append([]string(nil), c.BootstrapAddresses...)
	c.FallbackAddresses = append([]string(nil), c.FallbackAddresses...)
	c.ManualPeers = append([]string(nil), c.ManualPeers...)
	if err := validateAddressStrings(c.ListenAddresses, HardMaxCachedPeers); err != nil {
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

func errLimit(name string, value int) error {
	return fmt.Errorf("invalid P2P %s limit %d", name, value)
}
