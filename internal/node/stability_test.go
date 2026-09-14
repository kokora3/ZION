package node

import (
	"context"
	"os"
	"runtime"
	"strconv"
	"testing"
	"time"
)

func TestIdleRuntimeRemainsBoundedAndQuiet(t *testing.T) {
	baseline := runtime.NumGoroutine()
	cfg := testRuntimeConfig(t, "idle-stability", true, false)
	cfg.SyncInterval = 250 * time.Millisecond
	node, err := New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if err := node.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	time.Sleep(300 * time.Millisecond)
	settled := runtime.NumGoroutine()
	peersBefore := len(node.P2P().UsablePeers())
	time.Sleep(700 * time.Millisecond)
	if peersAfter := len(node.P2P().UsablePeers()); peersAfter != peersBefore {
		t.Fatalf("idle peer set changed without input: %d -> %d", peersBefore, peersAfter)
	}
	if growth := runtime.NumGoroutine() - settled; growth > 4 {
		t.Fatalf("idle runtime goroutines grew unexpectedly by %d", growth)
	}
	if err := node.Stop(context.Background()); err != nil {
		t.Fatal(err)
	}
	time.Sleep(150 * time.Millisecond)
	if remaining := runtime.NumGoroutine(); remaining > baseline+8 {
		t.Fatalf("runtime shutdown left goroutines: baseline=%d remaining=%d", baseline, remaining)
	}
}

// TestOptionalSoak is intentionally skipped during normal CI. Operators run it
// explicitly with ZION_SOAK_SECONDS (60-180 for CI smoke, longer manually).
func TestOptionalSoak(t *testing.T) {
	raw := os.Getenv("ZION_SOAK_SECONDS")
	if raw == "" {
		t.Skip("set ZION_SOAK_SECONDS to run the explicit soak test")
	}
	seconds, err := strconv.Atoi(raw)
	if err != nil || seconds < 1 || seconds > 86400 {
		t.Fatal("ZION_SOAK_SECONDS must be between 1 and 86400")
	}
	baseline := runtime.NumGoroutine()
	cfg := testRuntimeConfig(t, "soak", true, false)
	node, err := New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(seconds+10)*time.Second)
	defer cancel()
	if err := node.Start(ctx); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(time.Duration(seconds) * time.Second)
	peak := runtime.NumGoroutine()
	for time.Now().Before(deadline) {
		current := runtime.NumGoroutine()
		if current > peak {
			peak = current
		}
		if len(node.P2P().UsablePeers()) > cfg.P2P.Limits.MaxConnectedPeers {
			t.Fatal("peer count exceeded configured maximum")
		}
		time.Sleep(250 * time.Millisecond)
	}
	if err := node.Stop(context.Background()); err != nil {
		t.Fatal(err)
	}
	if peak > baseline+64 {
		t.Fatalf("unexpected soak goroutine peak: baseline=%d peak=%d", baseline, peak)
	}
}
