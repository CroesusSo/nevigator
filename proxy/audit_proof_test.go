//go:build audit
// +build audit

package proxy

import (
	"io"
	"net"
	"testing"
	"time"

	"github.com/yourorg/proxy-pac/socks5"
)

// halfCloseEcho accepts a connection, reads the request, and then sends a
// response. The client half-closes its write side, and the response must
// still be delivered over the read side.
func halfCloseEcho(t *testing.T) string {
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
				buf := make([]byte, 64)
				if _, err := c.Read(buf); err != nil {
					return
				}
				// The client half-closed its write side. The server's reply
				// must still be delivered over the read side.
				time.Sleep(50 * time.Millisecond)
				c.Write([]byte("pong"))
				time.Sleep(300 * time.Millisecond)
				c.Close()
			}(c)
		}
	}()
	return ln.Addr().String()
}

// startSOCKS5Listener runs the server's SOCKS5 handler on a real TCP
// listener and returns its address.
func startSOCKS5Listener(t *testing.T, s *Server) string {
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
			go s.handleSOCKS5(c)
		}
	}()
	return ln.Addr().String()
}

// TestAuditRelayHalfClosePreservesResponse proves that a client
// half-close does not truncate the server's response.
//
// relay() closes both sockets as soon as either io.Copy returns, so a
// client that half-closes loses the response, and the upstream never
// sees the half-close either because io.Copy does not propagate the FIN.
func TestAuditRelayHalfClosePreservesResponse(t *testing.T) {
	addr := halfCloseEcho(t)
	s := reviewServer(t, nil)
	proxyAddr := startSOCKS5Listener(t, s)

	cl, err := net.Dial("tcp", proxyAddr)
	if err != nil {
		t.Fatal(err)
	}
	defer cl.Close()
	cl.SetDeadline(time.Now().Add(5 * time.Second))

	if got := greeting(t, cl, socks5.AuthNone); got != socks5.AuthNone {
		t.Fatalf("chosen method = 0x%02x", got)
	}

	host, portStr, _ := net.SplitHostPort(addr)
	var port int
	for _, c := range portStr {
		port = port*10 + int(c-'0')
	}
	req := []byte{socks5.Version, socks5.CMDConnect, 0x00, socks5.ATYPIPv4}
	req = append(req, net.ParseIP(host).To4()...)
	req = append(req, byte(port>>8), byte(port))
	cl.Write(req)

	rep := make([]byte, 10)
	if _, err := io.ReadFull(cl, rep); err != nil {
		t.Fatalf("connect reply: %v", err)
	}
	if rep[1] != socks5.REPSuccess {
		t.Fatalf("REP = %d", rep[1])
	}

	// Send the request, then half-close the write side.
	cl.Write([]byte("ping"))
	cl.(*net.TCPConn).CloseWrite()

	// The server's response must still arrive.
	got := make([]byte, 4)
	if _, err := io.ReadFull(cl, got); err != nil {
		t.Fatalf("BUG: half-close by the client truncated the server's response: %v", err)
	}
	if string(got) != "pong" {
		t.Errorf("got %q, want %q", got, "pong")
	}
}
