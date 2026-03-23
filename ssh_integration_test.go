package main

import (
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/yourorg/proxy-pac/pac"
	"github.com/yourorg/proxy-pac/ssh"
)

// These integration tests require a running SSH server.
// Set RUN_SSH_TESTS=1 environment variable to run them.

func TestMain(m *testing.M) {
	if os.Getenv("RUN_SSH_TESTS") != "1" {
		fmt.Println("Skipping SSH integration tests; set RUN_SSH_TESTS=1 to run")
		os.Exit(0)
	}
	m.Run()
}

// TestSSHProxyConnect tests connecting through an SSH proxy (manual setup required).
// This test expects an SSH server running on 127.0.0.1:2222 with user/pass test/testpass.
func TestSSHProxyConnect(t *testing.T) {
	proxy := pac.ExternalProxy{
		ID:         "manual-ssh",
		Kind:       "ssh",
		Address:    "127.0.0.1:2222",
		Username:   "test",
		AuthMethod: "password",
		Password:   "testpass",
	}

	mgr := ssh.NewManager()
	mgr.UpdateProxies([]pac.ExternalProxy{proxy})
	defer mgr.Close()

	// Wait for connection
	time.Sleep(1 * time.Second)

	status := mgr.GetTunnelsStatus()
	s := status[proxy.ID]
	if s.State != "connected" {
		t.Fatalf("tunnel not connected: state=%s, error=%s", s.State, s.LastError)
	}

	// Try a dummy connection (will likely fail because our test server doesn't forward)
	_, err := mgr.Dial(proxy.ID, "example.com:80", 2*time.Second)
	if err == nil {
		t.Log("dial succeeded (unexpected)")
	} else {
		t.Logf("dial error (acceptable for test server): %v", err)
	}
}

// TestSSHRouterIntegration tests that router can use an SSH proxy.
func TestSSHRouterIntegration(t *testing.T) {
	proxy := pac.ExternalProxy{
		ID:         "router-ssh",
		Kind:       "ssh",
		Address:    "127.0.0.1:2222",
		Username:   "test",
		AuthMethod: "password",
		Password:   "testpass",
	}

	mgr := ssh.NewManager()
	mgr.UpdateProxies([]pac.ExternalProxy{proxy})

	// We can't easily test full router without starting a server, but we can test that
	// the tunnel manager integrates without panics.
	if mgr.GetTunnelsStatus() == nil {
		t.Fatal("GetTunnelsStatus returned nil")
	}
}

// TestSSHTunnelManagement tests creating and removing tunnels via UpdateProxies.
func TestSSHTunnelManagement(t *testing.T) {
	// We'll use a non-existent SSH server to ensure tunnels fail gracefully
	proxy1 := pac.ExternalProxy{
		ID:         "bad-ssh",
		Kind:       "ssh",
		Address:    "127.0.0.1:9999",
		Username:   "test",
		AuthMethod: "password",
		Password:   "testpass",
	}

	mgr := ssh.NewManager()
	mgr.UpdateProxies([]pac.ExternalProxy{proxy1})

	time.Sleep(200 * time.Millisecond)
	status := mgr.GetTunnelsStatus()
	s := status[proxy1.ID]
	// Should be reconnecting or errored, not crashed
	if s.State == "" {
		t.Error("tunnel state empty")
	}

	// Remove proxy
	mgr.UpdateProxies([]pac.ExternalProxy{})
	status = mgr.GetTunnelsStatus()
	if _, ok := status[proxy1.ID]; ok {
		t.Error("tunnel should be removed")
	}
}
