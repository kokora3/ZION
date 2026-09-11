package protocol

import (
	"strings"
	"testing"
)

func TestProtocolIdentifiers(t *testing.T) {
	if NetworkID != "zion-alpha-1" {
		t.Fatalf("NetworkID = %q, want zion-alpha-1", NetworkID)
	}
	if Version != "0.1" {
		t.Fatalf("Version = %q, want 0.1", Version)
	}
}

func TestV01Roles(t *testing.T) {
	roles := []Role{RoleNormal, RoleValidator, RoleBootstrap}
	want := []Role{"NORMAL", "VALIDATOR", "BOOTSTRAP"}
	for i, role := range roles {
		if role != want[i] {
			t.Errorf("role[%d] = %q, want %q", i, role, want[i])
		}
	}
}

func TestVersionString(t *testing.T) {
	got := VersionString("zion-node")
	for _, want := range []string{"zion-node", "ZION", "protocol version 0.1", "zion-alpha-1"} {
		if !strings.Contains(got, want) {
			t.Errorf("VersionString() = %q, missing %q", got, want)
		}
	}
}
