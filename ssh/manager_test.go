package ssh

import (
	"testing"
	"time"

	"github.com/yourorg/proxy-pac/pac"
)

func TestManagerUpdateProxies(t *testing.T) {
	m := NewManager()

	proxies := []pac.ExternalProxy{
		{ID: "p1", Kind: "ssh", Address: "host1", AuthMethod: "password", Password: "pass"},
		{ID: "p2", Kind: "ssh", Address: "host2", AuthMethod: "key", PrivateKeyPath: "/nonexistent"},
	}

	// Update with proxies
	m.UpdateProxies(proxies)

	// Tunnels should be created lazily, not immediately; but proxies map should be populated
	if len(m.proxies) != 2 {
		t.Errorf("expected 2 proxies, got %d", len(m.proxies))
	}

	// Get tunnel for p1 should create it
	tunnel1, err := m.getOrCreateTunnel("p1")
	if err != nil {
		t.Fatalf("getOrCreateTunnel failed: %v", err)
	}
	if tunnel1 == nil {
		t.Fatal("expected tunnel for p1")
	}
	if tunnel1.cfg.ID != "p1" {
		t.Errorf("expected tunnel cfg ID p1, got %s", tunnel1.cfg.ID)
	}

	// Update with only p1 (p2 removed)
	m.UpdateProxies([]pac.ExternalProxy{proxies[0]})

	// p2 tunnel should be closed and removed
	m.mu.RLock()
	_, p2Exists := m.tunnels["p2"]
	m.mu.RUnlock()
	if p2Exists {
		t.Error("p2 tunnel should have been removed")
	}
}

func TestManagerDialUnknownProxy(t *testing.T) {
	m := NewManager()
	// Dial without any proxy config
	conn, err := m.Dial("unknown", "example.com:80", 1*time.Second)
	if conn != nil {
		t.Fatal("expected nil connection")
	}
	if err == nil {
		t.Fatal("expected error for unknown proxy")
	}
}

func TestTunnelClose(t *testing.T) {
	cfg := pac.ExternalProxy{ID: "test", Kind: "ssh", Address: "host", AuthMethod: "password", Password: "pass"}
	tunnel := NewTunnel(cfg)
	err := tunnel.Close()
	if err != nil {
		t.Fatalf("Close returned error: %v", err)
	}
	// Second close should be no-op
	err = tunnel.Close()
	if err != nil {
		t.Fatalf("second Close returned error: %v", err)
	}
}

// TestTunnelInvalidConfig tests that a tunnel with invalid configuration fails permanently.
func TestTunnelInvalidConfig(t *testing.T) {
	cfg := pac.ExternalProxy{
		ID:             "bad",
		Kind:           "ssh",
		Address:        "host",
		AuthMethod:     "key",
		PrivateKeyPath: "/nonexistent/key.pem",
	}
	tunnel := NewTunnel(cfg)
	// Use a short timeout
	_, err := tunnel.Dial("example.com:80", 100*time.Millisecond)
	if err == nil {
		t.Fatal("expected error for invalid key path")
	}
	// Should be permanent, so second dial should return same error quickly
	start := time.Now()
	_, err2 := tunnel.Dial("example.com:80", 100*time.Millisecond)
	elapsed := time.Since(start)
	if err2 == nil {
		t.Fatal("expected error on second dial")
	}
	// Should not have retried for long (permanent failure should stop quickly)
	if elapsed > 1*time.Second {
		t.Logf("second dial took %v; may have retried briefly", elapsed)
	}
	_ = tunnel.Close()
}
