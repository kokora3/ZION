package p2p

import (
	"fmt"
	"sort"
)

type Role string

const (
	RoleNormal    Role = "NORMAL"
	RoleValidator Role = "VALIDATOR"
	RoleBootstrap Role = "BOOTSTRAP"
)

func normalizeRoles(roles []Role) ([]Role, error) {
	if len(roles) == 0 || len(roles) > MaxRoles {
		return nil, fmt.Errorf("invalid role count %d", len(roles))
	}
	seen := make(map[Role]struct{}, len(roles))
	for _, role := range roles {
		switch role {
		case RoleNormal, RoleValidator, RoleBootstrap:
		default:
			return nil, fmt.Errorf("unsupported P2P role %q", role)
		}
		if _, ok := seen[role]; ok {
			return nil, fmt.Errorf("duplicate P2P role %q", role)
		}
		seen[role] = struct{}{}
	}
	out := append([]Role(nil), roles...)
	sort.Slice(out, func(i, j int) bool { return out[i] < out[j] })
	return out, nil
}
