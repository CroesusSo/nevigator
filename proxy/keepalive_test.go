package proxy

import (
	"net"
	"os"
	"syscall"
	"testing"
	"time"

	"github.com/yourorg/proxy-pac/gfw"
	"github.com/yourorg/proxy-pac/pac"
	"github.com/yourorg/proxy-pac/router"
	"github.com/yourorg/proxy-pac/socks5"
)

func keepAliveServer(t *testing.T, ka pac.TCPKeepAliveConfig) *Server {
	t.Helper()
	dir := t.TempDir()
	pacPath := dir + "/test.pac"
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
		SOCKS5HandshakeTimeout: 2 * time.Second,
		TCPKeepAlive:           ka,
	}, ev, rt)
}

func tcpOpts(t *testing.T, c *net.TCPConn) (idle, interval, count int) {
	t.Helper()
	raw, err := c.SyscallConn()
	if err != nil {
		t.Fatalf("SyscallConn: %v", err)
	}
	if err := raw.Control(func(fd uintptr) {
		idle, _ = syscall.GetsockoptInt(int(fd), syscall.IPPROTO_TCP, 4)
		interval, _ = syscall.GetsockoptInt(int(fd), syscall.IPPROTO_TCP, 5)
		count, _ = syscall.GetsockoptInt(int(fd), syscall.IPPROTO_TCP, 6)
	}); err != nil {
		t.Fatalf("Control: %v", err)
	}
	return idle, interval, count
}

// acceptedSOCKS5Conn starts a listener whose accepted conn is handed to
// handleSOCKS5, completes a SOCKS5 greeting so the handler has run
// ApplyKeepAlive, and returns both sides of the connection.
func acceptedSOCKS5Conn(t *testing.T, s *Server) *net.TCPConn {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { ln.Close() })

	serverConns := make(chan *net.TCPConn, 1)
	go func() {
		c, err := ln.Accept()
		if err != nil {
			return
		}
		go s.handleSOCKS5(c)
		serverConns <- c.(*net.TCPConn)
	}()

	client, err := net.Dial("tcp", ln.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { client.Close() })
	if _, err := client.Write([]byte{socks5.Version, 0x01, socks5.AuthNone}); err != nil {
		t.Fatal(err)
	}
	sc := <-serverConns
	t.Cleanup(func() { sc.Close() })
	return sc
}

// A real TCP client talking to handleSOCKS5 must end up with tuned probes.
func TestSOCKS5InboundConnGetsTunedKeepalive(t *testing.T) {
	s := keepAliveServer(t, pac.TCPKeepAliveConfig{
		Enabled: true, Idle: 30 * time.Second, Interval: 10 * time.Second, Count: 3,
	})
	sc := acceptedSOCKS5Conn(t, s)

	// The socket must be tuned by the time the handler has processed the greeting.
	deadline := time.Now().Add(2 * time.Second)
	var idle, interval, count int
	for time.Now().Before(deadline) {
		idle, interval, count = tcpOpts(t, sc)
		if idle == 30 && interval == 10 && count == 3 {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}

	t.Logf("inbound SOCKS5 socket: idle=%d intvl=%d cnt=%d", idle, interval, count)
	if idle != 30 || interval != 10 || count != 3 {
		t.Errorf("inbound conn not tuned: idle=%d intvl=%d cnt=%d (want 30/10/3)", idle, interval, count)
	}
}

// With keepalive disabled the socket must keep Go's defaults.
func TestSOCKS5InboundKeepaliveDisabled(t *testing.T) {
	s := keepAliveServer(t, pac.TCPKeepAliveConfig{Enabled: false})
	sc := acceptedSOCKS5Conn(t, s)
	time.Sleep(100 * time.Millisecond)

	idle, interval, count := tcpOpts(t, sc)
	t.Logf("disabled: idle=%d intvl=%d cnt=%d", idle, interval, count)
	if idle == 30 && interval == 10 && count == 3 {
		t.Error("disabled config still applied tuned values")
	}
}
