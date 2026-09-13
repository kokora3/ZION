package api

import (
	"net/http"
	"testing"

	"github.com/kokora3/zion/internal/protocol"
	"github.com/kokora3/zion/internal/research"
	"github.com/kokora3/zion/internal/resources"
)

func TestRegistryReadAPIRoutesAndStrictIDs(t *testing.T) {
	researchID := research.ID{HashDigest: protocol.HashBytes([]byte("research"))}
	resourceID := resources.ID{HashDigest: protocol.HashBytes([]byte("resource"))}
	backend := &objectBackend{registryValue: map[string]string{"ok": "yes"}, registryFound: true}
	server := newObjectServer(t, backend)
	for _, path := range []string{"/v1/research", "/v1/research/search?q=zion", "/v1/research/" + researchID.String(),
		"/v1/resources", "/v1/resources/search?q=data", "/v1/resources/" + resourceID.String()} {
		if response := objectRequest(t, server, http.MethodGet, path, nil); response.Code != http.StatusOK {
			t.Fatalf("GET %s returned %d: %s", path, response.Code, response.Body.String())
		}
	}
	for _, path := range []string{"/v1/research/not-an-id", "/v1/resources/not-an-id"} {
		if response := objectRequest(t, server, http.MethodGet, path, nil); response.Code != http.StatusBadRequest {
			t.Fatalf("malformed registry ID status=%d", response.Code)
		}
	}
	if response := objectRequest(t, server, http.MethodPost, "/v1/research", []byte("{}")); response.Code != http.StatusMethodNotAllowed {
		t.Fatalf("direct registry mutation endpoint enabled: %d", response.Code)
	}
}
