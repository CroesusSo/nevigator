package router

import (
	"encoding/binary"
	"net"
	"sync/atomic"
	"testing"
	"time"

	"github.com/yourorg/proxy-pac/pac"
)

// fakeDNSServer answers every A query with a fixed address and counts the
// queries it received.
type fakeDNSServer struct {
	addr    string
	queries int32
}

func startFakeDNSServer(t *testing.T, ip net.IP) *fakeDNSServer {
	t.Helper()
	conn, err := net.ListenPacket("udp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { conn.Close() })

	d := &fakeDNSServer{addr: conn.LocalAddr().String()}
	go func() {
		buf := make([]byte, 512)
		for {
			n, addr, err := conn.ReadFrom(buf)
			if err != nil {
				return
			}
			atomic.AddInt32(&d.queries, 1)
			reply, err := buildDNSReply(buf[:n], ip)
			if err != nil {
				continue
			}
			conn.WriteTo(reply, addr)
		}
	}()
	return d
}

// buildDNSReply builds an A-record answer for a query, echoing the question.
func buildDNSReply(query []byte, ip net.IP) ([]byte, error) {
	if len(query) < 12 {
		return nil, errShortQuery
	}
	// The question section follows the 12-byte header and ends at the first
	// zero-length label.
	off := 12
	for off < len(query) {
		l := int(query[off])
		off++
		if l == 0 {
			break
		}
		off += l
	}
	if off+4 > len(query) {
		return nil, errShortQuery
	}
	question := query[12 : off+4]

	reply := make([]byte, 0, 12+len(question)+16)
	header := make([]byte, 12)
	copy(header, query[:2])                        // ID
	binary.BigEndian.PutUint16(header[2:], 0x8180) // response, recursion available
	binary.BigEndian.PutUint16(header[4:], 1)      // QDCOUNT
	binary.BigEndian.PutUint16(header[6:], 1)      // ANCOUNT
	reply = append(reply, header...)
	reply = append(reply, question...)

	answer := make([]byte, 16)
	answer[0] = 0xC0 // pointer to the question name
	answer[1] = 0x0C
	binary.BigEndian.PutUint16(answer[2:], 1)  // TYPE A
	binary.BigEndian.PutUint16(answer[4:], 1)  // CLASS IN
	binary.BigEndian.PutUint32(answer[6:], 60) // TTL
	binary.BigEndian.PutUint16(answer[10:], 4) // RDLENGTH
	copy(answer[12:], ip.To4())
	reply = append(reply, answer...)
	return reply, nil
}

type dnsError string

func (e dnsError) Error() string { return string(e) }

const errShortQuery = dnsError("short dns query")

// TestDirectRouteUsesCustomDNS proves that a DIRECT connection resolves the
// host name with the configured custom DNS servers.
//
// The GFW list is fetched through the proxy's own SOCKS5 listener, so a
// DIRECT route for the source host resolves the name here, in the router.
func TestDirectRouteUsesCustomDNS(t *testing.T) {
	// A server the fake DNS points at.
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
			go func(c net.Conn) {
				defer c.Close()
				buf := make([]byte, 64)
				c.Read(buf)
				c.Write(buf)
			}(c)
		}
	}()
	_, port, err := net.SplitHostPort(echo.Addr().String())
	if err != nil {
		t.Fatal(err)
	}

	dns := startFakeDNSServer(t, net.ParseIP("127.0.0.1"))

	rt, err := NewRouter(pac.RouterConfig{
		DialTimeout:      2 * time.Second,
		CustomDNSServers: []string{dns.addr},
	})
	if err != nil {
		t.Fatalf("NewRouter: %v", err)
	}

	// The name is one the system resolver would not know.
	conn, err := rt.Route("unresolvable.invalid:"+port, pac.Route{Strategy: "DIRECT"})
	if err != nil {
		t.Fatalf("Route: %v", err)
	}
	defer conn.Close()

	if got := atomic.LoadInt32(&dns.queries); got == 0 {
		t.Error("the custom DNS server received no query, so it was not consulted")
	}
}

// TestDirectRouteWithoutCustomDNSUsesSystemResolver proves that the custom
// DNS servers are only consulted when they are configured.
func TestDirectRouteWithoutCustomDNSUsesSystemResolver(t *testing.T) {
	dns := startFakeDNSServer(t, net.ParseIP("127.0.0.1"))

	rt, err := NewRouter(pac.RouterConfig{DialTimeout: 2 * time.Second})
	if err != nil {
		t.Fatalf("NewRouter: %v", err)
	}
	// A name that does not resolve: the dial fails, but the custom DNS
	// server must not have been asked.
	_, err = rt.Route("unresolvable.invalid:1", pac.Route{Strategy: "DIRECT"})
	if err == nil {
		t.Fatal("expected the dial to fail")
	}
	if got := atomic.LoadInt32(&dns.queries); got != 0 {
		t.Errorf("the custom DNS server received %d queries, want 0", got)
	}
}
