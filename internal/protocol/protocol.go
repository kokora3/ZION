// Package protocol contains versioned, runtime-independent protocol identifiers
// and compatibility primitives. Canonical types and object/schema versioning are
// intentionally deferred beyond Phase 1.
package protocol

import "fmt"

const (
	// NetworkID is the frozen identifier for the first ZION alpha network.
	NetworkID = "zion-alpha-1"

	// Version is the ZION protocol version represented by this repository.
	Version = "0.1"

	// Implementation is the v0.1 reference implementation name.
	Implementation = "Go Native ZION Node"
)

// Role names v0.1 operational capabilities. Roles may overlap; validator
// authority is determined by genesis/chain state, never by local configuration.
type Role string

const (
	RoleNormal    Role = "NORMAL"
	RoleValidator Role = "VALIDATOR"
	RoleBootstrap Role = "BOOTSTRAP"
)

// VersionString returns shared command-line version metadata.
func VersionString(program string) string {
	return fmt.Sprintf("%s: ZION protocol version %s (network %s; %s)", program, Version, NetworkID, Implementation)
}
