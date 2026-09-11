// Package protocol contains runtime-independent ZION protocol identifiers,
// canonical encoding, and content-derived object identity primitives.
package protocol

import "fmt"

type ProtocolVersion string
type NetworkID string
type SchemaVersion uint16

const (
	CurrentProtocolVersion ProtocolVersion = "0.1"
	Alpha1NetworkID        NetworkID       = "zion-alpha-1"
	Implementation                         = "Go Native ZION Node"
)

const (
	Version = string(CurrentProtocolVersion)
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
	return fmt.Sprintf("%s: ZION protocol version %s (network %s; %s)", program, CurrentProtocolVersion, Alpha1NetworkID, Implementation)
}
