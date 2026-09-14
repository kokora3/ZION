package node

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/hex"
	"log/slog"
	"net"
	"net/http"
	"os"
	"strings"
	"testing"

	"github.com/kokora3/zion/internal/api"
)

func TestMetricsAreBoundedAuthenticatedAndNonCanonical(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	cfg := testRuntimeConfig(t, "metrics", true, false)
	cfg.API.MetricsEnabled = true
	runtime, err := New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	before, _ := cfg.InitialState.Hash()
	if err := runtime.Start(ctx); err != nil {
		t.Fatal(err)
	}
	defer runtime.Stop(context.Background())
	response := apiRequest(t, http.MethodGet, "http://"+runtime.APIAddress()+"/metrics", nil)
	if response.StatusCode != http.StatusOK {
		t.Fatalf("metrics returned %d: %s", response.StatusCode, response.Body)
	}
	body := string(response.Body)
	for _, required := range []string{"zion_node_info", "zion_node_ready", "zion_p2p_connected_peers",
		"zion_state_height", "zion_object_store_bytes", "zion_api_requests_total"} {
		if !strings.Contains(body, required) {
			t.Fatalf("metrics omit %s", required)
		}
	}
	after := runtimeHash(t, runtime)
	if before.String() != after {
		t.Fatal("enabling metrics changed canonical StateHash")
	}
	for _, forbidden := range []string{runtime.P2P().PeerID().String(), before.String(), cfg.StatePath, "tx_id", "object_id", "identity_id"} {
		if forbidden != "" && strings.Contains(body, forbidden) {
			t.Fatalf("metrics contain high-cardinality or local value %q", forbidden)
		}
	}
}

func TestMetricsDisabledAndBearerProtected(t *testing.T) {
	ctx := context.Background()
	cfg := testRuntimeConfig(t, "metrics-disabled", true, false)
	cfg.API.MetricsEnabled = false
	runtime, err := New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if err := runtime.Start(ctx); err != nil {
		t.Fatal(err)
	}
	response := apiRequest(t, http.MethodGet, "http://"+runtime.APIAddress()+"/metrics", nil)
	if response.StatusCode != http.StatusNotFound {
		t.Fatalf("disabled metrics returned %d", response.StatusCode)
	}
	_ = runtime.Stop(ctx)

	cfg = testRuntimeConfig(t, "metrics-auth", true, false)
	cfg.API.Listen = "0.0.0.0:0"
	cfg.API.BearerToken = "phase13-test-bearer-secret"
	cfg.API.MetricsEnabled = true
	runtime, err = New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if err := runtime.Start(ctx); err != nil {
		t.Fatal(err)
	}
	defer runtime.Stop(ctx)
	_, port, splitErr := net.SplitHostPort(runtime.APIAddress())
	if splitErr != nil {
		t.Fatal(splitErr)
	}
	request, _ := http.NewRequest(http.MethodGet, "http://127.0.0.1:"+port+"/metrics", nil)
	result, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	defer result.Body.Close()
	if result.StatusCode != http.StatusUnauthorized {
		t.Fatalf("unauthenticated metrics returned %d", result.StatusCode)
	}
}

func TestStructuredLogsExcludeConfiguredSecrets(t *testing.T) {
	var output bytes.Buffer
	secret := "phase13-bearer-secret-never-log"
	privateMarker := "phase13-private-key-never-log"
	cfg := testRuntimeConfig(t, "logging", true, false)
	cfg.API.BearerToken = secret
	cfg.ValidatorPublicKey = []byte(privateMarker)
	cfg.Logger = slog.New(slog.NewJSONHandler(&output, &slog.HandlerOptions{Level: slog.LevelDebug}))
	runtime, err := New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if err := runtime.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	peerSecret, err := os.ReadFile(cfg.P2P.KeyPath)
	if err != nil {
		t.Fatal(err)
	}
	runtime.cfg.Logger.Info("public test event", "operation", "secret-exclusion")
	if err := runtime.Stop(context.Background()); err != nil {
		t.Fatal(err)
	}
	logs := output.String()
	if !strings.Contains(logs, `"msg":"node runtime started"`) || !strings.Contains(logs, `"level":"INFO"`) {
		t.Fatalf("structured lifecycle logging missing: %s", logs)
	}
	for _, forbidden := range []string{secret, privateMarker, hex.EncodeToString(peerSecret),
		base64.StdEncoding.EncodeToString(peerSecret), cfg.P2P.KeyPath, cfg.StatePath} {
		if strings.Contains(logs, forbidden) {
			t.Fatalf("structured logs leaked secret or local path %q", forbidden)
		}
	}
}

var _ api.Backend = (*Runtime)(nil)
