//go:build audit
// +build audit

package socks5

import (
	"errors"
	"io"
	"net"
	"testing"
	"time"
)

// Proof 6: the client silently truncates a username or password longer than
// 255 bytes.
//
// RFC 1929 encodes ULEN and PLEN as a single byte, so a credential longer
// than 255 bytes cannot be represented. The client builds the frame with
// byte(len(u)), which wraps modulo 256, and the frame it sends is malformed.
//
// ErrUserPassTooLong is declared in proto.go for exactly this case but is
// never returned anywhere in the package.
//
// Expected: the client refuses to dial with an over-long credential.
// Actual: it dials and sends a corrupt credential frame.

// TestAuditOverlongUsernameIsRejected proves the client refuses a username
// longer than 255 bytes instead of sending a malformed frame.
func TestAuditOverlongUsernameIsRejected(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()

	frameCh := make(chan []byte, 1)
	go func() {
		c, err := ln.Accept()
		if err != nil {
			return
		}
		defer c.Close()
		hdr := make([]byte, 2)
		if _, err := io.ReadFull(c, hdr); err != nil {
			return
		}
		methods := make([]byte, hdr[1])
		if _, err := io.ReadFull(c, methods); err != nil {
			return
		}
		c.Write([]byte{Version, AuthUserPass})
		frame := make([]byte, 0, 1024)
		buf := make([]byte, 1024)
		c.SetReadDeadline(time.Now().Add(300 * time.Millisecond))
		for {
			n, err := c.Read(buf)
			if n > 0 {
				frame = append(frame, buf[:n]...)
			}
			if err != nil {
				break
			}
		}
		frameCh <- frame
		c.Write([]byte{AuthUserPassVersion, 0x01})
	}()

	longUser := make([]byte, 300)
	for i := range longUser {
		longUser[i] = 'A'
	}
	_, err = DialWithAuth("tcp", ln.Addr().String(), &Auth{Username: string(longUser), Password: "p"}, 2*time.Second)

	// The client must refuse before dialling.
	if err != nil && errors.Is(err, ErrUserPassTooLong) {
		t.Logf("returned as expected: %v", err)
		return
	}

	select {
	case frame := <-frameCh:
		if len(frame) < 2 {
			t.Fatalf("BUG: client dialled and sent a malformed credential frame (%d bytes): %v", len(frame), frame)
		}
		ulen := int(frame[1])
		if ulen != len(longUser) {
			t.Fatalf("BUG: client sent a credential frame advertising ULEN=%d for a %d-byte username "+
				"(the length byte wrapped); the frame is malformed: %v", ulen, len(longUser), frame[:min(len(frame), 40)])
		}
	case <-time.After(2 * time.Second):
		t.Fatal("client neither returned ErrUserPassTooLong nor dialled")
	}
	t.Fatalf("BUG: over-long username was accepted; ErrUserPassTooLong was never returned")
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
