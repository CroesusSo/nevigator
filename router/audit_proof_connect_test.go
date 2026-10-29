//go:build audit
// +build audit

package router

import (
	"io"
	"net"
	"testing"
	"time"

	"github.com/yourorg/proxy-pac/pac"
)

// stallingSocks5Upstream completes the SOCKS5 greeting and then never answers
// the CONNECT request.
func startStallingSocks5Upstream(t *testing.T) string {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { ln.Close() })
	go func() {
		for {
			c, err := ln.Accept()
			if err != nil {
				return
			}
			go func(c net.Conn) {
				defer c.Close()
				hdr := make([]byte, 2)
				if _, err := io.ReadFull(c, hdr); err != nil {
					return
				}
				methods := make([]byte, hdr[1])
				if _, err := io.ReadFull(c, methods); err != nil {
					return
				}
				c.Write([]byte{0x05, 0x00})
				// Read the CONNECT request, then stay silent forever.
				buf := make([]byte, 512)
				for {
					if _, err := c.Read(buf); err != nil {
						return
					}
				}
			}(c)
		}
	}()
	return ln.Addr().String()
}

// TestAuditUpstreamConnectPhaseHasTimeout proves that the upstream CONNECT
// exchange is bounded by the dial timeout.
//
// This is a different stall from the greeting one: the upstream completes
// the handshake and then never answers the CONNECT request.
//
// Expected: Route gives up at the dial timeout and returns an error.
// Actual: Route never returns.
func TestAuditUpstreamConnectPhaseHasTimeout(t *testing.T) {
	addr := startStallingSocks5Upstream(t)

	rt, err := NewRouter(pac.RouterConfig{DialTimeout: 500 * time.Millisecond})
	if err != nil {
		t.Fatal(err)
	}

	done := make(chan error, 1)
	go func() {
		_, err := rt.Route("127.0.0.1:9", pac.Route{Strategy: "SOCKS5", Address: addr})
		done <- err
	}()

	select {
	case err := <-done:
		if err == nil {
			t.Fatal("expected an error, got a live connection")
		}
		t.Logf("returned as expected: %v", err)
	case <-time.After(3 * time.Second):
		t.Fatal("BUG: Route blocked forever on an upstream that completed the greeting " +
			"and then never answered the CONNECT request")
	}
}
