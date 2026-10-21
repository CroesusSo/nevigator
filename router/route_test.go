package router

import (
	"io"
	"net"
	"sync/atomic"
	"testing"
	"time"

	"github.com/yourorg/proxy-pac/pac"
)

// fakeSocks5Upstream answers the handshake then a CONNECT with the given reply.
func fakeSocks5Upstream(t *testing.T, reply []byte, increment *int32) string {
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
			if increment != nil {
				atomic.AddInt32(increment, 1)
			}
			go func(c net.Conn) {
				defer c.Close()
				hdr := make([]byte, 2)
				if _, err := io.ReadFull(c, hdr); err != nil {
					return
				}
				m := make([]byte, hdr[1])
				if _, err := io.ReadFull(c, m); err != nil {
					return
				}
				c.Write([]byte{0x05, 0x00})
				h := make([]byte, 4)
				if _, err := io.ReadFull(c, h); err != nil {
					return
				}
				rest := 0
				switch h[3] {
				case 0x01:
					rest = 4 + 2
				case 0x04:
					rest = 16 + 2
				case 0x03:
					l := make([]byte, 1)
					if _, err := io.ReadFull(c, l); err != nil {
						return
					}
					rest = int(l[0]) + 2
				}
				if _, err := io.ReadFull(c, make([]byte, rest)); err != nil {
					return
				}
				if reply != nil {
					c.Write(reply)
				}
				io.Copy(c, c)
			}(c)
		}
	}()
	return ln.Addr().String()
}

// An IPv6 BND.ADDR reply must be fully consumed before the tunnel is handed on.
func TestDialSOCKS5ConsumesIPv6Reply(t *testing.T) {
	reply := make([]byte, 22)
	reply[0], reply[1], reply[3] = 0x05, 0x00, 0x04
	addr := fakeSocks5Upstream(t, reply, nil)

	rt, err := NewRouter(pac.RouterConfig{DialTimeout: 2 * time.Second})
	if err != nil {
		t.Fatal(err)
	}
	conn, err := rt.Route("example.com:80", pac.Route{Strategy: "SOCKS5", Address: addr})
	if err != nil {
		t.Fatalf("Route: %v", err)
	}
	defer conn.Close()

	// Send a known payload; it must arrive intact with no reply bytes interleaved.
	conn.SetDeadline(time.Now().Add(2 * time.Second))
	if _, err := conn.Write([]byte("HELLO")); err != nil {
		t.Fatal(err)
	}
	buf := make([]byte, 5)
	if _, err := io.ReadFull(conn, buf); err != nil {
		t.Fatalf("read: %v", err)
	}
	if string(buf) != "HELLO" {
		t.Errorf("got %q, want HELLO (BND.ADDR bytes leaked into stream)", buf)
	}
}

func TestDialSOCKS5ConsumesDomainReply(t *testing.T) {
	reply := []byte{0x05, 0x00, 0x00, 0x03, 0x0B}
	reply = append(reply, []byte("example.com")...)
	reply = append(reply, 0x00, 0x50)
	addr := fakeSocks5Upstream(t, reply, nil)

	rt, err := NewRouter(pac.RouterConfig{DialTimeout: 2 * time.Second})
	if err != nil {
		t.Fatal(err)
	}
	conn, err := rt.Route("example.com:80", pac.Route{Strategy: "SOCKS5", Address: addr})
	if err != nil {
		t.Fatalf("Route: %v", err)
	}
	defer conn.Close()
	conn.SetDeadline(time.Now().Add(2 * time.Second))
	conn.Write([]byte("HELLO"))
	buf := make([]byte, 5)
	if _, err := io.ReadFull(conn, buf); err != nil {
		t.Fatal(err)
	}
	if string(buf) != "HELLO" {
		t.Errorf("got %q, want HELLO", buf)
	}
}

// A deterministic failure must not be retried three times.
func TestRouteDoesNotRetryDeterministicFailure(t *testing.T) {
	var attempts int32
	addr := fakeSocks5Upstream(t, []byte{0x05, 0x02, 0x00, 0x01, 0, 0, 0, 0, 0, 0}, &attempts) // not allowed

	rt, err := NewRouter(pac.RouterConfig{DialTimeout: time.Second})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := rt.Route("example.com:80", pac.Route{Strategy: "SOCKS5", Address: addr}); err == nil {
		t.Fatal("expected error")
	}
	if n := atomic.LoadInt32(&attempts); n != 1 {
		t.Errorf("BUG: upstream dialed %d times for a deterministic rejection, want 1", n)
	}
}

func TestRouteUnknownStrategyNotRetried(t *testing.T) {
	rt, err := NewRouter(pac.RouterConfig{DialTimeout: time.Second})
	if err != nil {
		t.Fatal(err)
	}
	start := time.Now()
	_, err = rt.Route("example.com:80", pac.Route{Strategy: "NONSENSE"})
	if err == nil {
		t.Fatal("expected error")
	}
	if d := time.Since(start); d > 500*time.Millisecond {
		t.Errorf("took %v; deterministic error should return immediately", d)
	}
}

// A transport failure through SOCKS5 must fall back to DIRECT.
func TestRouteFallsBackToDirect(t *testing.T) {
	// Upstream that dies mid-handshake => transport error, fallback expected.
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
			c.Close()
		}
	}()

	echo, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer echo.Close()
	go func() {
		for {
			c, err := echo.Accept()
			if err != nil {
				return
			}
			go func(c net.Conn) { defer c.Close(); io.Copy(c, c) }(c)
		}
	}()

	rt, err := NewRouter(pac.RouterConfig{DialTimeout: time.Second})
	if err != nil {
		t.Fatal(err)
	}
	conn, err := rt.Route(echo.Addr().String(), pac.Route{Strategy: "SOCKS5", Address: ln.Addr().String()})
	if err != nil {
		t.Fatalf("expected DIRECT fallback to succeed: %v", err)
	}
	defer conn.Close()
	conn.SetDeadline(time.Now().Add(2 * time.Second))
	conn.Write([]byte("PING"))
	buf := make([]byte, 4)
	if _, err := io.ReadFull(conn, buf); err != nil {
		t.Fatalf("read: %v", err)
	}
	if string(buf) != "PING" {
		t.Errorf("got %q", buf)
	}
}

// A DIRECT route that fails must NOT recurse into fallback.
func TestDirectRouteFailureReturnsPromptly(t *testing.T) {
	rt, err := NewRouter(pac.RouterConfig{DialTimeout: time.Second})
	if err != nil {
		t.Fatal(err)
	}
	// Port 1 on localhost: refused.
	start := time.Now()
	if _, err := rt.Route("127.0.0.1:1", pac.Route{Strategy: "DIRECT"}); err == nil {
		t.Fatal("expected error")
	}
	if d := time.Since(start); d > 2*time.Second {
		t.Errorf("DIRECT failure took %v; fallback loop suspected", d)
	}
}
