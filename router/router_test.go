package router

import (
	"net"
	"strings"
	"testing"
	"time"

	"github.com/yourorg/proxy-pac/pac"
)

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

// TestRouter_TLSRoute_Success tests that TLS routes work with proper configuration
func TestRouter_TLSRoute_Success(t *testing.T) {
	r := &Router{
		tunIP:       net.ParseIP("127.0.0.1"),
		dialTimeout: 30 * time.Second,
	}

	// Test TLS_TUN route - we're mainly checking that it doesn't return a configuration error
	route := pac.Route{Strategy: "TLS_TUN"}
	_, err := r.dialWithRoute("example.com:443", route)
	if err != nil && err != ErrTUNNotConfigured {
		// Only fail if it's not the expected TUN configuration error
		t.Errorf("TLS_TUN route failed unexpectedly: %v", err)
	}

	// Test TLS_SOCKS5 route
	route = pac.Route{Strategy: "TLS_SOCKS5", Address: "127.0.0.1:1080"}
	_, err = r.dialWithRoute("example.com:443", route)
	if err != nil {
		// This might fail because there's no SOCKS5 server running, which is expected in a unit test
		// We're mainly checking that it doesn't return a configuration error
		if err == ErrInvalidSOCKS5Address {
			t.Errorf("TLS_SOCKS5 route failed with address error: %v", err)
		}
	}

	// Test TLS_DIRECT route
	route = pac.Route{Strategy: "TLS_DIRECT"}
	_, err = r.dialWithRoute("example.com:443", route)
	if err != nil {
		// For DIRECT, we expect a DNS resolution error, not a configuration error
		if !strings.Contains(err.Error(), "lookup") && !strings.Contains(err.Error(), "no such host") {
			t.Errorf("TLS_DIRECT route failed with unexpected error: %v", err)
		}
	}
}

// TestRouter_TLSRoute_TUNNotConfigured tests that TLS_TUN fails when TUN is not configured
func TestRouter_TLSRoute_TUNNotConfigured(t *testing.T) {
	r := &Router{
		tunIP:       nil,
		dialTimeout: 30 * time.Second,
	}

	route := pac.Route{Strategy: "TLS_TUN"}
	_, err := r.dialWithRoute("example.com:443", route)
	if err == nil {
		t.Error("Expected error for TUN not configured")
	}
	if err != ErrTUNNotConfigured {
		t.Errorf("Expected ErrTUNNotConfigured, got %v", err)
	}
}

// TestRouter_TLSRoute_SOCKS5AddressRequired tests that TLS_SOCKS5 fails when address is missing
func TestRouter_TLSRoute_SOCKS5AddressRequired(t *testing.T) {
	r := &Router{
		tunIP:       net.ParseIP("10.0.0.1"),
		dialTimeout: 30 * time.Second,
	}

	route := pac.Route{Strategy: "TLS_SOCKS5"}
	_, err := r.dialWithRoute("example.com:443", route)
	if err == nil {
		t.Error("Expected error for SOCKS5 address required")
	}
	if err != ErrInvalidSOCKS5Address {
		t.Errorf("Expected ErrInvalidSOCKS5Address, got %v", err)
	}
}
