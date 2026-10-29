//go:build audit
// +build audit

package socks5

import (
	"io"
	"net"
	"testing"
	"time"
)

// authDowngradeUpstream accepts a SOCKS5 greeting, records the offered
// methods, and selects the given method.
type authDowngradeUpstream struct {
	addr    string
	methods chan []byte
}

func startAuthDowngradeUpstream(t *testing.T, chosen byte) *authDowngradeUpstream {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { ln.Close() })
	u := &authDowngradeUpstream{addr: ln.Addr().String(), methods: make(chan []byte, 4)}
	go func() {
		for {
			c, err := ln.Accept()
			if err != nil {
				return
			}
			go u.handle(c)
		}
	}()
	return u
}

func (u *authDowngradeUpstream) handle(c net.Conn) {
	defer c.Close()
	hdr := make([]byte, 2)
	if _, err := io.ReadFull(c, hdr); err != nil {
		return
	}
	methods := make([]byte, hdr[1])
	if _, err := io.ReadFull(c, methods); err != nil {
		return
	}
	u.methods <- methods
	c.Write([]byte{Version, AuthNone})
	time.Sleep(200 * time.Millisecond)
}

// TestAuditClientDoesNotDowngradeToNoAuth proves that a client configured
// with credentials does not accept an upstream that selects "no
// authentication".
//
// socks5.Handshake offers {AuthNone, AuthUserPass} whenever credentials are
// configured, and treats a selected AuthNone as success. A hostile or
// impersonated upstream can therefore pick method 0x00 and the client
// proceeds with no authentication at all, believing it authenticated.
//
// Expected: with credentials configured, the client offers only user/pass
// and fails when the upstream selects no-auth.
// Actual: the client offers no-auth and connects successfully.
func TestAuditClientDoesNotDowngradeToNoAuth(t *testing.T) {
	up := startAuthDowngradeUpstream(t, AuthNone)

	conn, err := DialWithAuth("tcp", up.addr,
		&Auth{Username: "alice", Password: "s3cret"}, 3*time.Second)
	if err == nil {
		conn.Close()
		t.Fatal("BUG: the client accepted an upstream that selected no-auth " +
			"although credentials were configured; the tunnel is unauthenticated")
	}
	t.Logf("returned as expected: %v", err)

	select {
	case methods := <-up.methods:
		for _, m := range methods {
			if m == AuthNone {
				t.Fatalf("BUG: the client offered the no-auth method (0x00) "+
					"although credentials were configured: methods=%v", methods)
			}
		}
	case <-time.After(2 * time.Second):
		t.Fatal("the upstream never received a greeting")
	}
}
