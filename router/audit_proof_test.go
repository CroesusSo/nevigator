//go:build audit
// +build audit

package router

import (
	"net"
	"testing"
	"time"

	"github.com/yourorg/proxy-pac/pac"
)

// TestAuditUpstreamHandshakeHasNoTimeout proves that the upstream SOCKS5
// handshake is bounded by the dial timeout.
//
// The dialer timeout covers only the TCP connect. Once the connection is
// established, the greeting write and the method-selection read block
// forever if the upstream accepts but never answers, so the goroutine and
// the upstream socket leak.
//
// Expected: Route gives up at the dial timeout and returns an error.
// Actual: Route never returns.
func TestAuditUpstreamHandshakeHasNoTimeout(t *testing.T) {
	// Upstream that accepts the TCP connection and then says nothing.
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	go func() {
		for {
			c, err := ln.Accept()
			if err != nil {
				return
			}
			// Deliberately never reply to the greeting.
			go func(c net.Conn) {
				buf := make([]byte, 64)
				for {
					if _, err := c.Read(buf); err != nil {
						return
					}
				}
			}(c)
		}
	}()

	rt, err := NewRouter(pac.RouterConfig{DialTimeout: 500 * time.Millisecond})
	if err != nil {
		t.Fatal(err)
	}

	done := make(chan error, 1)
	go func() {
		_, err := rt.Route("127.0.0.1:9", pac.Route{Strategy: "SOCKS5", Address: ln.Addr().String()})
		done <- err
	}()

	select {
	case err := <-done:
		if err == nil {
			t.Fatal("expected an error, got a live connection")
		}
		t.Logf("returned as expected: %v", err)
	case <-time.After(3 * time.Second):
		t.Fatal("BUG: Route blocked forever on an unresponsive upstream SOCKS5 server; " +
			"the dial timeout only covers the TCP connect, not the handshake")
	}
}
