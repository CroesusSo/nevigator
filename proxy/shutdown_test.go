package proxy

import (
	"io"
	"net"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/yourorg/proxy-pac/pac"
	"github.com/yourorg/proxy-pac/router"
	"github.com/yourorg/proxy-pac/socks5"
)

// shutdownServer builds a Server listening on the given SOCKS5 address.
func shutdownServer(t *testing.T, socksAddr string) *Server {
	t.Helper()
	dir := t.TempDir()
	pacPath := filepath.Join(dir, "t.pac")
	pacContent := "var __PAC_RULES__ = {\"version\":1,\"rules\":[]};\nfunction FindProxyForURL(url, host) { return \"DIRECT\"; }"
	if err := os.WriteFile(pacPath, []byte(pacContent), 0600); err != nil {
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
		HTTPListen:   "127.0.0.1:0",
		SOCKS5Listen: socksAddr,
	}, ev, rt)
}

// freeAddr returns a loopback address whose port is currently free.
func freeAddr(t *testing.T) string {
	t.Helper()
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	addr := l.Addr().String()
	l.Close()
	return addr
}

// waitListener blocks until the address accepts a connection, so tests do
// not race the listener goroutine's startup.
func waitListener(t *testing.T, addr string) {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		c, err := net.DialTimeout("tcp", addr, 200*time.Millisecond)
		if err == nil {
			c.Close()
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("listener %s never came up", addr)
}

// TestStopReleasesSOCKS5Listener covers the shutdown contract that the
// SOCKS5 port is released.
//
// Regression test: the listener is created inside listenSOCKS5() and was
// never stored on the Server, so Stop() could not close it. The accept
// loop parks in Accept() and only re-checks the stopped channel after an
// accept returns, so the port stayed bound and the proxy kept serving
// after shutdown.
func TestStopReleasesSOCKS5Listener(t *testing.T) {
	addr := freeAddr(t)
	s := shutdownServer(t, addr)

	if err := s.Start(); err != nil {
		t.Fatal(err)
	}
	waitListener(t, addr)

	s.Stop()
	time.Sleep(200 * time.Millisecond)

	probe2, err := net.Listen("tcp", addr)
	if err != nil {
		t.Fatalf("the SOCKS5 port %s is still bound after Stop(): %v", addr, err)
	}
	probe2.Close()
}

// TestStopStopsServingSOCKS5 covers the shutdown contract that no new
// client is served once Stop() has returned.
//
// Regression test: the accept loop kept running after Stop() because
// nothing closed the listener, so the proxy kept answering greetings long
// after shutdown.
func TestStopStopsServingSOCKS5(t *testing.T) {
	addr := freeAddr(t)
	s := shutdownServer(t, addr)

	if err := s.Start(); err != nil {
		t.Fatal(err)
	}
	waitListener(t, addr)

	// Complete a full SOCKS5 greeting. This proves the accept loop has
	// accepted a connection and is therefore parked in Accept() again.
	// The settle delay then lets it get back there, so Stop() cannot land
	// in the narrow window between an accept returning and the loop
	// re-checking the stopped channel.
	probe, err := net.Dial("tcp", addr)
	if err != nil {
		t.Fatal(err)
	}
	probe.SetDeadline(time.Now().Add(3 * time.Second))
	if _, err := probe.Write([]byte{socks5.Version, 0x01, socks5.AuthNone}); err != nil {
		t.Fatal(err)
	}
	reply := make([]byte, 2)
	if _, err := io.ReadFull(probe, reply); err != nil {
		t.Fatal(err)
	}
	probe.Close()
	time.Sleep(150 * time.Millisecond)

	s.Stop()
	time.Sleep(200 * time.Millisecond)

	// The server must be gone: a fresh connection must not be served.
	conn, err := net.DialTimeout("tcp", addr, time.Second)
	if err != nil {
		return // refused: correct
	}
	defer conn.Close()
	conn.SetDeadline(time.Now().Add(2 * time.Second))
	if _, err := conn.Write([]byte{socks5.Version, 0x01, socks5.AuthNone}); err != nil {
		return // connection unusable: correct
	}
	reply = make([]byte, 2)
	if _, err := io.ReadFull(conn, reply); err != nil {
		return // no greeting: correct
	}
	t.Fatalf("the SOCKS5 server is still serving after Stop() returned; "+
		"it answered a greeting with %v", reply)
}

// TestStopIsIdempotent covers the shutdown contract that Stop() can be
// called more than once.
//
// Regression test: Stop() closed s.stopped unconditionally, so a second
// call panicked with "close of closed channel". Every other Close in the
// codebase guards against a double close.
func TestStopIsIdempotent(t *testing.T) {
	s := shutdownServer(t, freeAddr(t))
	if err := s.Start(); err != nil {
		t.Fatal(err)
	}
	s.Stop()

	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("the second Stop() panicked: %v", r)
		}
	}()
	s.Stop()
}

// TestStopHasNoDataRaceOnHTTPServer covers the shutdown contract that
// Stop() does not race with the HTTPS listener goroutine.
//
// Regression test: the goroutine started by NewServer wrote
// s.httpsServer = nil when the HTTPS listener failed to bind, while
// Stop() read the same field without synchronisation.
func TestStopHasNoDataRaceOnHTTPServer(t *testing.T) {
	// Occupy a port so the HTTPS listener fails to bind and the goroutine
	// takes the s.httpsServer = nil branch.
	blocker, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer blocker.Close()

	certPath, keyPath := generateSelfSignedCert(t)
	s := NewServer(pac.ProxyConfig{
		HTTPListen:   "127.0.0.1:0",
		SOCKS5Listen: freeAddr(t),
		HTTPSListen:  blocker.Addr().String(),
		TLSConfig: pac.TLSConfig{
			CertificateFile: certPath,
			KeyFile:         keyPath,
		},
	}, nil, nil)

	// Give the goroutine time to fail the bind and write the field.
	time.Sleep(200 * time.Millisecond)
	s.Stop()
}

// TestStopDrainsInFlightTunnels covers the shutdown contract that Stop()
// does not return while a tunnel is still relaying.
//
// Regression test: Stop() closed the HTTP server and returned without
// waiting for the connection goroutines, so tunnels established before the
// shutdown were still open and relaying afterwards.
func TestStopDrainsInFlightTunnels(t *testing.T) {
	addr := freeAddr(t)
	s := shutdownServer(t, addr)
	// Use a short grace period so the test does not wait out the default.
	s.drainGrace = 300 * time.Millisecond
	if err := s.Start(); err != nil {
		t.Fatal(err)
	}
	waitListener(t, addr)

	// Establish a tunnel through the proxy to a local echo server.
	echo, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer echo.Close()
	go func() {
		c, err := echo.Accept()
		if err != nil {
			return
		}
		io.Copy(c, c)
	}()

	client, err := net.Dial("tcp", addr)
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()
	client.SetDeadline(time.Now().Add(3 * time.Second))

	if _, err := client.Write([]byte{socks5.Version, 0x01, socks5.AuthNone}); err != nil {
		t.Fatal(err)
	}
	m := make([]byte, 2)
	if _, err := io.ReadFull(client, m); err != nil {
		t.Fatal(err)
	}

	host, portStr, _ := net.SplitHostPort(echo.Addr().String())
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
		t.Fatal(err)
	}
	if rep[1] != socks5.REPSuccess {
		t.Fatalf("REP = %d", rep[1])
	}

	// Shutdown while the tunnel is live.
	if err := s.Stop(); err != nil {
		t.Fatalf("Stop: %v", err)
	}

	// After Stop() returns, the tunnel must be finished, not still relaying.
	client.SetDeadline(time.Now().Add(time.Second))
	if _, err := client.Write([]byte("ping")); err == nil {
		buf := make([]byte, 4)
		if _, err := io.ReadFull(client, buf); err == nil && string(buf) == "ping" {
			t.Fatal("Stop() returned while a tunnel was still relaying; " +
				"the shutdown does not drain in-flight connections")
		}
	}
}
