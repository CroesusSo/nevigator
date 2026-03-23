package router

import (
	"net"
	"strings"
	"testing"
	"time"

	"github.com/yourorg/proxy-pac/pac"
	"github.com/yourorg/proxy-pac/status"
)

// mockManager is a test double for TunnelManager interface.
type mockManager struct {
	dialCalled   bool
	dialErr      error
	returnedConn net.Conn
}

func (m *mockManager) Dial(id, dest string, timeout time.Duration) (net.Conn, error) {
	m.dialCalled = true
	return m.returnedConn, m.dialErr
}

func (m *mockManager) UpdateProxies(proxies []pac.ExternalProxy) {}
func (m *mockManager) Close() error                              { return nil }
func (m *mockManager) GetTunnelsStatus() map[string]status.SSHTunnelStatus {
	return map[string]status.SSHTunnelStatus{}
}

func TestRouter_SetProxyRegistry(t *testing.T) {
	r := &Router{}
	proxies := []pac.ExternalProxy{
		{ID: "proxy1", Address: "127.0.0.1:1081"},
		{ID: "proxy2", Address: "127.0.0.1:1082", Username: "user", Password: "pass"},
	}
	r.SetProxyRegistry(proxies)

	// Check lookup
	p1 := r.proxyRegistry["proxy1"]
	if p1.Address != "127.0.0.1:1081" {
		t.Errorf("expected proxy1 address 127.0.0.1:1081, got %s", p1.Address)
	}
	p2 := r.proxyRegistry["proxy2"]
	if p2.Username != "user" || p2.Password != "pass" {
		t.Errorf("expected proxy2 credentials user/pass, got %s/%s", p2.Username, p2.Password)
	}
	// Non-existent
	_, ok := r.proxyRegistry["nonexistent"]
	if ok {
		t.Error("should not find nonexistent proxy")
	}
}

func TestRouter_SetDefaultProxyID(t *testing.T) {
	r := &Router{}
	r.SetDefaultProxyID("my-default")
	if r.defaultProxyID != "my-default" {
		t.Errorf("expected defaultProxyID my-default, got %s", r.defaultProxyID)
	}
}

// TestRouter_ExternalRoute_NotFound tests that EXTERNAL route with unknown ID returns error.
func TestRouter_ExternalRoute_NotFound(t *testing.T) {
	r := &Router{}
	r.SetProxyRegistry([]pac.ExternalProxy{
		{ID: "known", Address: "127.0.0.1:1080"},
	})
	route := pac.Route{Strategy: "EXTERNAL", ExternalID: "missing"}
	// Call dialWithRoute directly, not Route (which falls back to DIRECT)
	_, err := r.dialWithRoute("8.8.8.8:53", route)
	if err == nil || !strings.Contains(err.Error(), "external proxy not found") {
		t.Errorf("expected error for unknown proxy, got %v", err)
	}
}

// TestRouter_DefaultRoute_NoDefault tests that DEFAULT without a default configured returns error.
func TestRouter_DefaultRoute_NoDefault(t *testing.T) {
	r := &Router{}
	route := pac.Route{Strategy: "DEFAULT"}
	_, err := r.dialWithRoute("8.8.8.8:53", route)
	if err == nil || !strings.Contains(err.Error(), "no default external proxy configured") {
		t.Errorf("expected error about no default, got %v", err)
	}
}

// TestRouter_DefaultRoute_DefaultNotFound tests that DEFAULT with missing default ID returns error.
func TestRouter_DefaultRoute_DefaultNotFound(t *testing.T) {
	r := &Router{}
	r.SetProxyRegistry([]pac.ExternalProxy{
		{ID: "other", Address: "127.0.0.1:1080"},
	})
	r.SetDefaultProxyID("nonexistent")
	route := pac.Route{Strategy: "DEFAULT"}
	_, err := r.dialWithRoute("8.8.8.8:53", route)
	if err == nil || !strings.Contains(err.Error(), "default external proxy not found") {
		t.Errorf("expected error about default proxy not found, got %v", err)
	}
}

// TestRouter_ExternalRoute_SSH tests that EXTERNAL with Kind=ssh uses tunnel manager.
func TestRouter_ExternalRoute_SSH(t *testing.T) {
	r := &Router{dialTimeout: 30 * time.Second}
	mockMgr := &mockManager{}
	r.tunnelMgr = mockMgr
	r.SetProxyRegistry([]pac.ExternalProxy{
		{ID: "ssh-proxy", Kind: "ssh", Address: "host:22"},
	})
	route := pac.Route{Strategy: "EXTERNAL", ExternalID: "ssh-proxy"}
	_, err := r.dialWithRoute("example.com:80", route)
	if err != nil {
		t.Fatalf("expected no error from mock manager, got %v", err)
	}
	if !mockMgr.dialCalled {
		t.Error("expected tunnel manager Dial to be called")
	}
}

// TestRouter_ExternalRoute_SSH_NoManager tests that EXTERNAL with Kind=ssh fails if tunnelMgr is nil.
func TestRouter_ExternalRoute_SSH_NoManager(t *testing.T) {
	r := &Router{dialTimeout: 30 * time.Second}
	// tunnelMgr is nil
	r.SetProxyRegistry([]pac.ExternalProxy{
		{ID: "ssh-proxy", Kind: "ssh", Address: "host:22"},
	})
	route := pac.Route{Strategy: "EXTERNAL", ExternalID: "ssh-proxy"}
	_, err := r.dialWithRoute("example.com:80", route)
	if err == nil || !strings.Contains(err.Error(), "SSH tunnel manager not configured") {
		t.Errorf("expected error about missing tunnel manager, got %v", err)
	}
}
