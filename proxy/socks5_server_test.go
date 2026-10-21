package proxy

import (
	"io"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	"github.com/yourorg/proxy-pac/gfw"
	"github.com/yourorg/proxy-pac/pac"
	"github.com/yourorg/proxy-pac/router"
	"github.com/yourorg/proxy-pac/socks5"
)

func reviewServer(t *testing.T, auth *pac.SOCKS5AuthConfig) *Server {
	t.Helper()
	dir := t.TempDir()
	pacPath := filepath.Join(dir, "test.pac")
	if err := os.WriteFile(pacPath, []byte(gfw.GeneratePAC(nil, nil, gfw.FWMeta{})), 0600); err != nil {
		t.Fatal(err)
	}
	ev, err := pac.NewEvaluator(pac.EvaluatorConfig{
		PACPath:       pacPath,
		EvalTimeout:   time.Second,
		FallbackRoute: pac.Route{Strategy: "DIRECT"},
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { ev.Close() })
	rt, err := router.NewRouter(pac.RouterConfig{DialTimeout: 2 * time.Second})
	if err != nil {
		t.Fatal(err)
	}
	return NewServer(pac.ProxyConfig{
		HTTPListen:             "127.0.0.1:0",
		SOCKS5Auth:             auth,
		SOCKS5HandshakeTimeout: 500 * time.Millisecond,
	}, ev, rt)
}

// startEcho starts a TCP echo server and returns its address.
func startEcho(t *testing.T) string {
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
				b := make([]byte, 64)
				for {
					n, err := c.Read(b)
					if n > 0 {
						c.Write(b[:n])
					}
					if err != nil {
						return
					}
				}
			}(c)
		}
	}()
	return ln.Addr().String()
}

func greeting(t *testing.T, cl net.Conn, methods ...byte) byte {
	t.Helper()
	req := append([]byte{socks5.Version, byte(len(methods))}, methods...)
	if _, err := cl.Write(req); err != nil {
		t.Fatal(err)
	}
	resp := make([]byte, 2)
	if _, err := io.ReadFull(cl, resp); err != nil {
		t.Fatalf("method reply: %v", err)
	}
	if resp[0] != socks5.Version {
		t.Fatalf("method reply version = %d", resp[0])
	}
	return resp[1]
}

// When auth is configured the server must select user/pass, not no-auth.
func TestSOCKS5ServerSelectsUserPassWhenConfigured(t *testing.T) {
	s := reviewServer(t, &pac.SOCKS5AuthConfig{Username: "u", Password: "p"})
	cl, srv := net.Pipe()
	go s.handleSOCKS5(srv)
	defer cl.Close()
	cl.SetDeadline(time.Now().Add(2 * time.Second))

	if got := greeting(t, cl, socks5.AuthNone, socks5.AuthUserPass); got != socks5.AuthUserPass {
		t.Fatalf("chosen method = 0x%02x, want 0x02 (user/pass)", got)
	}
}

// Correct credentials must succeed and produce a working tunnel.
func TestSOCKS5ServerAcceptsValidCredentials(t *testing.T) {
	addr := startEcho(t)
	s := reviewServer(t, &pac.SOCKS5AuthConfig{Username: "u", Password: "p"})
	cl, srv := net.Pipe()
	go s.handleSOCKS5(srv)
	defer cl.Close()
	cl.SetDeadline(time.Now().Add(3 * time.Second))

	if got := greeting(t, cl, socks5.AuthUserPass); got != socks5.AuthUserPass {
		t.Fatalf("chosen method = 0x%02x", got)
	}
	cred := []byte{0x01, 0x01, 'u', 0x01, 'p'}
	cl.Write(cred)
	authResp := make([]byte, 2)
	if _, err := io.ReadFull(cl, authResp); err != nil {
		t.Fatalf("auth reply: %v", err)
	}
	if authResp[1] != 0x00 {
		t.Fatalf("auth status = %d, want 0x00", authResp[1])
	}

	dom := "localhost"
	_, portStr, err := net.SplitHostPort(addr)
	if err != nil {
		t.Fatal(err)
	}
	port, err := strconv.Atoi(portStr)
	if err != nil {
		t.Fatal(err)
	}
	req := []byte{socks5.Version, socks5.CMDConnect, 0x00, socks5.ATYPDomain, byte(len(dom))}
	req = append(req, []byte(dom)...)
	req = append(req, byte(port>>8), byte(port))
	cl.Write(req)

	rep := make([]byte, 10)
	if _, err := io.ReadFull(cl, rep); err != nil {
		t.Fatalf("connect reply: %v", err)
	}
	if rep[1] != socks5.REPSuccess {
		t.Fatalf("connect REP = %d, want 0", rep[1])
	}
	cl.Write([]byte("ping"))
	buf := make([]byte, 4)
	if _, err := io.ReadFull(cl, buf); err != nil {
		t.Fatalf("relay: %v", err)
	}
	if string(buf) != "ping" {
		t.Errorf("echoed %q", buf)
	}
}

func TestSOCKS5ServerRejectsBadCredentials(t *testing.T) {
	s := reviewServer(t, &pac.SOCKS5AuthConfig{Username: "u", Password: "p"})
	cl, srv := net.Pipe()
	go s.handleSOCKS5(srv)
	defer cl.Close()
	cl.SetDeadline(time.Now().Add(2 * time.Second))

	if got := greeting(t, cl, socks5.AuthUserPass); got != socks5.AuthUserPass {
		t.Fatalf("chosen method = 0x%02x", got)
	}
	cl.Write([]byte{0x01, 0x01, 'u', 0x01, 'x'}) // wrong password
	authResp := make([]byte, 2)
	if _, err := io.ReadFull(cl, authResp); err != nil {
		t.Fatalf("auth reply: %v", err)
	}
	if authResp[1] == 0x00 {
		t.Fatal("wrong password accepted")
	}
}

// Without configured auth the server must still accept plain no-auth.
func TestSOCKS5ServerNoAuthWhenUnconfigured(t *testing.T) {
	s := reviewServer(t, nil)
	cl, srv := net.Pipe()
	go s.handleSOCKS5(srv)
	defer cl.Close()
	cl.SetDeadline(time.Now().Add(2 * time.Second))
	if got := greeting(t, cl, socks5.AuthNone); got != socks5.AuthNone {
		t.Fatalf("chosen method = 0x%02x, want 0x00", got)
	}
}

// A client offering only user/pass when none is configured gets 0xFF.
func TestSOCKS5ServerRejectsUserPassWhenUnconfigured(t *testing.T) {
	s := reviewServer(t, nil)
	cl, srv := net.Pipe()
	go s.handleSOCKS5(srv)
	defer cl.Close()
	cl.SetDeadline(time.Now().Add(2 * time.Second))
	if got := greeting(t, cl, socks5.AuthUserPass); got != socks5.AuthNoAcceptable {
		t.Errorf("chosen method = 0x%02x, want 0xFF", got)
	}
}

// The handshake must not block forever on a silent client.
func TestSOCKS5HandshakeHasDeadline(t *testing.T) {
	s := reviewServer(t, nil)
	cl, srv := net.Pipe() // client never speaks; kept alive so the pipe stays open
	defer cl.Close()

	done := make(chan struct{})
	go func() {
		s.handleSOCKS5(srv)
		close(done)
	}()
	select {
	case <-done:
		// Expected: the 500ms handshake deadline fires and the handler unwinds.
	case <-time.After(3 * time.Second):
		t.Fatal("handshake did not time out within 3s (configured 500ms)")
	}
}
