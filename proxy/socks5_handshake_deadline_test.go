package proxy

import (
	"io"
	"net"
	"os"
	"testing"
	"time"

	"github.com/yourorg/proxy-pac/gfw"
	"github.com/yourorg/proxy-pac/pac"
	"github.com/yourorg/proxy-pac/router"
	"github.com/yourorg/proxy-pac/socks5"
)

// deadlineTestServer builds a Server whose SOCKS5 handshake timeout is short
// enough that any leak of the deadline into relaying shows up quickly.
func deadlineTestServer(t *testing.T, handshakeTimeout time.Duration) *Server {
	t.Helper()
	pacPath := t.TempDir() + "/t.pac"
	if err := os.WriteFile(pacPath, []byte(gfw.GeneratePAC(nil, nil, gfw.FWMeta{})), 0600); err != nil {
		t.Fatal(err)
	}
	ev, err := pac.NewEvaluator(pac.EvaluatorConfig{
		PACPath: pacPath, EvalTimeout: time.Second,
		FallbackRoute: pac.Route{Strategy: "DIRECT"},
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { ev.Close() })
	rt, _ := router.NewRouter(pac.RouterConfig{DialTimeout: 2 * time.Second})
	return NewServer(pac.ProxyConfig{
		HTTPListen:             "127.0.0.1:0",
		SOCKS5HandshakeTimeout: handshakeTimeout,
	}, ev, rt)
}

// TestSOCKS5TunnelSurvivesHandshakeTimeout holds an established tunnel open
// longer than the SOCKS5 handshake timeout.
//
// Regression test: the handshake deadline must be cleared once the CONNECT
// request is handled. If it is left armed (e.g. cleared via defer, which only
// runs after relay returns) every tunnel is severed when the deadline expires.
func TestSOCKS5TunnelSurvivesHandshakeTimeout(t *testing.T) {
	// Upstream echo target
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

	s := deadlineTestServer(t, 300*time.Millisecond) // deliberately short

	ln2, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln2.Close()
	go func() {
		c, err := ln2.Accept()
		if err != nil {
			return
		}
		go s.handleSOCKS5(c)
	}()

	client, err := net.Dial("tcp", ln2.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()

	// greeting
	client.Write([]byte{socks5.Version, 0x01, socks5.AuthNone})
	m := make([]byte, 2)
	if _, err := io.ReadFull(client, m); err != nil {
		t.Fatalf("greeting: %v", err)
	}

	// CONNECT to the echo target by IP
	host, portStr, _ := net.SplitHostPort(ln.Addr().String())
	var port int
	for _, c := range portStr {
		port = port*10 + int(c-'0')
	}
	req := []byte{socks5.Version, socks5.CMDConnect, 0x00, socks5.ATYPIPv4}
	req = append(req, net.ParseIP(host).To4()...)
	req = append(req, byte(port>>8), byte(port))
	client.Write(req)

	rep := make([]byte, 10)
	if _, err := io.ReadFull(client, rep); err != nil {
		t.Fatalf("connect reply: %v", err)
	}
	if rep[1] != 0 {
		t.Fatalf("REP = %d", rep[1])
	}

	// Sleep well past the handshake deadline, then expect the tunnel to work.
	time.Sleep(800 * time.Millisecond)

	client.SetDeadline(time.Now().Add(2 * time.Second))
	if _, err := client.Write([]byte("ping")); err != nil {
		t.Fatalf("BUG: write after handshake deadline failed: %v", err)
	}
	buf := make([]byte, 4)
	if _, err := io.ReadFull(client, buf); err != nil {
		t.Fatalf("BUG: read after handshake deadline failed: %v", err)
	}
	if string(buf) != "ping" {
		t.Errorf("got %q", buf)
	}
	t.Log("tunnel still alive after handshake timeout: OK")
}
