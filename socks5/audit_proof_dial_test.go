//go:build audit
// +build audit

package socks5

import (
	"context"
	"io"
	"net"
	"testing"
	"time"
)

// portRecordingUpstream answers the handshake and records the CONNECT target
// port.
type portRecordingUpstream struct {
	addr  string
	ports chan uint16
}

func startPortRecordingUpstream(t *testing.T) *portRecordingUpstream {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { ln.Close() })
	u := &portRecordingUpstream{addr: ln.Addr().String(), ports: make(chan uint16, 4)}
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

func (u *portRecordingUpstream) handle(c net.Conn) {
	defer c.Close()
	hdr := make([]byte, 2)
	if _, err := io.ReadFull(c, hdr); err != nil {
		return
	}
	methods := make([]byte, hdr[1])
	if _, err := io.ReadFull(c, methods); err != nil {
		return
	}
	c.Write([]byte{Version, AuthNone})
	req := make([]byte, 4)
	if _, err := io.ReadFull(c, req); err != nil {
		return
	}
	var port uint16
	switch req[3] {
	case ATYPIPv4:
		rest := make([]byte, 6)
		io.ReadFull(c, rest)
		port = uint16(rest[4])<<8 | uint16(rest[5])
	case ATYPDomain:
		l := make([]byte, 1)
		io.ReadFull(c, l)
		rest := make([]byte, int(l[0])+2)
		io.ReadFull(c, rest)
		port = uint16(rest[len(rest)-2])<<8 | uint16(rest[len(rest)-1])
	}
	u.ports <- port
	c.Write(EncodeReply(REPSuccess))
	time.Sleep(300 * time.Millisecond)
}

// TestAuditDialConnectRejectsOutOfRangePort proves that a destination port
// outside 0-65535 is rejected instead of being silently truncated.
//
// socks5.DialConnect parses the port with strconv.Atoi and casts it to
// uint16 without a range check, so "host:70000" becomes port 4464 and the
// tunnel is established to the wrong port.
//
// Expected: DialConnect returns an error for an out-of-range port.
// Actual: the client connects to port 4464.
func TestAuditDialConnectRejectsOutOfRangePort(t *testing.T) {
	up := startPortRecordingUpstream(t)

	conn, err := DialConnect(context.Background(), up.addr, "example.com:70000", nil, 3*time.Second)
	if err == nil {
		conn.Close()
		select {
		case p := <-up.ports:
			t.Fatalf("BUG: the client dialled port %d for the destination port 70000 "+
				"(the port was truncated modulo 65536)", p)
		case <-time.After(time.Second):
			t.Fatal("no CONNECT request was observed")
		}
	}
	t.Logf("returned as expected: %v", err)
}

// TestAuditDialConnectHonoursContextCancellation proves that cancelling the
// context aborts a blocked SOCKS5 handshake.
//
// DialConnect only consults ctx.Deadline(); a cancelled context with no
// deadline leaves the handshake blocked forever, so the caller's cancellation
// is silently ignored and the goroutine and socket leak.
//
// Expected: DialConnect returns an error once the context is cancelled.
// Actual: it blocks indefinitely.
func TestAuditDialConnectHonoursContextCancellation(t *testing.T) {
	// Upstream that accepts and never answers.
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
				buf := make([]byte, 64)
				for {
					if _, err := c.Read(buf); err != nil {
						return
					}
				}
			}(c)
		}
	}()

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() {
		_, err := DialConnect(ctx, ln.Addr().String(), "example.com:443", nil, 0)
		done <- err
	}()

	time.Sleep(200 * time.Millisecond)
	cancel()

	select {
	case err := <-done:
		t.Logf("returned as expected: %v", err)
	case <-time.After(3 * time.Second):
		t.Fatal("BUG: cancelling the context did not abort the SOCKS5 handshake; " +
			"DialConnect blocked forever and leaked the goroutine and socket")
	}
}
