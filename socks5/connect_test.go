package socks5

import (
	"context"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"
)

// connectRequest records the destination a SOCKS5 client asked for.
type connectRequest struct {
	host string
	port uint16
}

// recordingSocks5Server is a SOCKS5 server that records the CONNECT target
// and then relays to a fixed destination.
//
// The destination is dialed as given, so a domain target is resolved by the
// server, not by the client. That is what lets a client behind a censored
// network reach a host whose name only the far side can resolve.
type recordingSocks5Server struct {
	addr     string
	requests chan connectRequest
	upstream string
	wg       sync.WaitGroup
}

func startRecordingSocks5Server(t *testing.T, upstream string) *recordingSocks5Server {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	s := &recordingSocks5Server{
		addr:     ln.Addr().String(),
		requests: make(chan connectRequest, 4),
		upstream: upstream,
	}
	s.wg.Add(1)
	go func() {
		defer s.wg.Done()
		for {
			c, err := ln.Accept()
			if err != nil {
				return
			}
			go s.handle(c)
		}
	}()
	t.Cleanup(func() {
		ln.Close()
		s.wg.Wait()
	})
	return s
}

func (s *recordingSocks5Server) handle(c net.Conn) {
	defer c.Close()

	// Greeting: VER, NMETHODS, METHODS
	hdr := make([]byte, 2)
	if _, err := io.ReadFull(c, hdr); err != nil {
		return
	}
	methods := make([]byte, hdr[1])
	if _, err := io.ReadFull(c, methods); err != nil {
		return
	}
	chosen := byte(AuthNone)
	for _, m := range methods {
		if m == AuthUserPass {
			chosen = AuthUserPass
			break
		}
	}
	if _, err := c.Write([]byte{Version, chosen}); err != nil {
		return
	}
	if chosen == AuthUserPass {
		ver := make([]byte, 1)
		if _, err := io.ReadFull(c, ver); err != nil {
			return
		}
		ulenB := make([]byte, 1)
		if _, err := io.ReadFull(c, ulenB); err != nil {
			return
		}
		uname := make([]byte, ulenB[0])
		if _, err := io.ReadFull(c, uname); err != nil {
			return
		}
		plenB := make([]byte, 1)
		if _, err := io.ReadFull(c, plenB); err != nil {
			return
		}
		pass := make([]byte, plenB[0])
		if _, err := io.ReadFull(c, pass); err != nil {
			return
		}
		c.Write([]byte{AuthUserPassVersion, REPSuccess})
	}

	// Request: VER, CMD, RSV, ATYP, ADDR, PORT
	req := make([]byte, 262)
	if _, err := io.ReadFull(c, req[:4]); err != nil {
		return
	}
	if req[0] != Version || req[1] != CMDConnect {
		return
	}
	addrLen := 0
	switch req[3] {
	case ATYPIPv4:
		addrLen = 4
	case ATYPDomain:
		l := make([]byte, 1)
		if _, err := io.ReadFull(c, l); err != nil {
			return
		}
		req[4] = l[0]
		addrLen = int(l[0])
	case ATYPIPv6:
		addrLen = 16
	default:
		return
	}
	rest := make([]byte, addrLen+2)
	if _, err := io.ReadFull(c, rest); err != nil {
		return
	}
	host, port, _, err := DeserializeAddr(append(req[:4:4], append([]byte{req[4]}, rest...)...), 3)
	if err != nil {
		return
	}
	s.requests <- connectRequest{host: host, port: port}

	// Relay to the fixed upstream, ignoring the requested destination.
	out, err := net.Dial("tcp", s.upstream)
	if err != nil {
		c.Write(EncodeReply(REPGeneralFailure))
		return
	}
	defer out.Close()
	c.Write(EncodeReply(REPSuccess))
	go io.Copy(out, c)
	io.Copy(c, out)
}

// TestConnectSendsDomainUnresolved proves that a domain target reaches the
// SOCKS5 server as a name, so the server resolves it.
//
// If the client resolved the name itself, a censored network could not
// reach the host at all, and the proxy's own routing rules would be applied
// to an address instead of to the host.
func TestConnectSendsDomainUnresolved(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("ok"))
	}))
	defer upstream.Close()

	srv := startRecordingSocks5Server(t, strings.TrimPrefix(upstream.URL, "http://"))

	conn, err := net.Dial("tcp", srv.addr)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	conn.SetDeadline(time.Now().Add(5 * time.Second))

	if err := Handshake(conn, nil); err != nil {
		t.Fatalf("Handshake: %v", err)
	}
	if _, err := Connect(conn, "raw.githubusercontent.com", 443); err != nil {
		t.Fatalf("Connect: %v", err)
	}

	select {
	case req := <-srv.requests:
		if req.host != "raw.githubusercontent.com" {
			t.Errorf("host = %q, want %q", req.host, "raw.githubusercontent.com")
		}
		if req.port != 443 {
			t.Errorf("port = %d, want 443", req.port)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("the server never received a CONNECT request")
	}
}

// TestHTTPTransportTunnelsThroughServer proves that the HTTP transport sends
// the request through the SOCKS5 server instead of connecting directly.
func TestHTTPTransportTunnelsThroughServer(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("through-the-tunnel"))
	}))
	defer upstream.Close()

	srv := startRecordingSocks5Server(t, strings.TrimPrefix(upstream.URL, "http://"))

	client := &http.Client{
		Transport: NewHTTPTransport(HTTPTransportOptions{ProxyAddr: srv.addr, Timeout: 5 * time.Second}),
		Timeout:   10 * time.Second,
	}
	resp, err := client.Get("http://raw.githubusercontent.com/gfwlist.txt")
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatal(err)
	}
	if string(body) != "through-the-tunnel" {
		t.Errorf("body = %q, want %q", body, "through-the-tunnel")
	}

	select {
	case req := <-srv.requests:
		if req.host != "raw.githubusercontent.com" {
			t.Errorf("host = %q, want %q", req.host, "raw.githubusercontent.com")
		}
	case <-time.After(3 * time.Second):
		t.Fatal("the server never received a CONNECT request")
	}
}

// TestHTTPTransportDoesNotFallBackToDirect proves that a failed tunnel is
// reported as an error instead of being retried directly.
//
// A direct retry would send the request outside the proxy, which is exactly
// what the tunnel is there to prevent.
func TestHTTPTransportDoesNotFallBackToDirect(t *testing.T) {
	// A port that nothing listens on.
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	dead := ln.Addr().String()
	ln.Close()

	// A server that would answer if the client connected to it directly.
	direct := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("direct"))
	}))
	defer direct.Close()

	client := &http.Client{
		Transport: NewHTTPTransport(HTTPTransportOptions{ProxyAddr: dead, Timeout: 2 * time.Second}),
		Timeout:   5 * time.Second,
	}
	resp, err := client.Get(direct.URL)
	if err == nil {
		resp.Body.Close()
		t.Fatal("the request succeeded, so it bypassed the failed tunnel")
	}
}

// TestDialConnectUsesAuthentication proves that the tunnel uses the
// configured credentials against a server that requires them.
func TestDialConnectUsesAuthentication(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("ok"))
	}))
	defer upstream.Close()

	srv := startRecordingSocks5Server(t, strings.TrimPrefix(upstream.URL, "http://"))

	conn, err := DialConnect(context.Background(), srv.addr, "example.com:443",
		&Auth{Username: "u", Password: "p"}, 5*time.Second)
	if err != nil {
		t.Fatalf("DialConnect: %v", err)
	}
	defer conn.Close()

	select {
	case req := <-srv.requests:
		if req.host != "example.com" {
			t.Errorf("host = %q, want %q", req.host, "example.com")
		}
	case <-time.After(3 * time.Second):
		t.Fatal("the server never received a CONNECT request")
	}
}
