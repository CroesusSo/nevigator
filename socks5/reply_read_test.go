package socks5

import (
	"bytes"
	"errors"
	"io"
	"net"
	"testing"
	"time"
)

// fakeConn feeds a canned byte stream and records what was consumed.
type fakeConn struct {
	r *bytes.Reader
}

func (f *fakeConn) Read(b []byte) (int, error) { return f.r.Read(b) }
func (f *fakeConn) Write(b []byte) (int, error) {
	return len(b), nil
}
func (f *fakeConn) Close() error                       { return nil }
func (f *fakeConn) LocalAddr() net.Addr                { return nil }
func (f *fakeConn) RemoteAddr() net.Addr               { return nil }
func (f *fakeConn) SetDeadline(t time.Time) error      { return nil }
func (f *fakeConn) SetReadDeadline(t time.Time) error  { return nil }
func (f *fakeConn) SetWriteDeadline(t time.Time) error { return nil }

func TestReadReplyConsumesFullIPv6(t *testing.T) {
	// VER, REP=0, RSV, ATYP=IPv6, 16 bytes addr, 2 bytes port, then app payload
	raw := []byte{
		0x05, 0x00, 0x00, 0x04,
		0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 1,
		0x1F, 0x90,
	}
	conn := &fakeConn{r: bytes.NewReader(append(raw, []byte("PAYLOAD")...))}

	rep, err := ReadReply(conn)
	if err != nil {
		t.Fatalf("ReadReply: %v", err)
	}
	if rep != REPSuccess {
		t.Errorf("rep = %d, want %d", rep, REPSuccess)
	}
	rest, _ := io.ReadAll(conn)
	if string(rest) != "PAYLOAD" {
		t.Errorf("leftover stream = %q, want %q (reply under-read)", rest, "PAYLOAD")
	}
}

func TestReadReplyConsumesFullDomain(t *testing.T) {
	raw := []byte{0x05, 0x00, 0x00, 0x03, 0x0B}
	raw = append(raw, []byte("example.com")...)
	raw = append(raw, 0x00, 0x50)
	conn := &fakeConn{r: bytes.NewReader(append(raw, []byte("PAYLOAD")...))}

	if _, err := ReadReply(conn); err != nil {
		t.Fatalf("ReadReply: %v", err)
	}
	rest, _ := io.ReadAll(conn)
	if string(rest) != "PAYLOAD" {
		t.Errorf("leftover stream = %q, want %q", rest, "PAYLOAD")
	}
}

func TestReadReplyIPv4(t *testing.T) {
	raw := []byte{0x05, 0x00, 0x00, 0x01, 127, 0, 0, 1, 0x1F, 0x90}
	conn := &fakeConn{r: bytes.NewReader(append(raw, []byte("PAYLOAD")...))}
	if _, err := ReadReply(conn); err != nil {
		t.Fatalf("ReadReply: %v", err)
	}
	rest, _ := io.ReadAll(conn)
	if string(rest) != "PAYLOAD" {
		t.Errorf("leftover stream = %q, want %q", rest, "PAYLOAD")
	}
}

func TestReadReplyReportsErrorCode(t *testing.T) {
	raw := []byte{0x05, 0x05, 0x00, 0x01, 0, 0, 0, 0, 0, 0} // REP=refused
	conn := &fakeConn{r: bytes.NewReader(raw)}
	rep, err := ReadReply(conn)
	if err != nil {
		t.Fatalf("ReadReply: %v", err)
	}
	if rep != REPConnectionRefused {
		t.Errorf("rep = %d, want %d", rep, REPConnectionRefused)
	}
}

func TestReadReplyRejectsBadVersion(t *testing.T) {
	conn := &fakeConn{r: bytes.NewReader([]byte{0x04, 0x00, 0x00, 0x01, 0, 0, 0, 0, 0, 0})}
	if _, err := ReadReply(conn); err == nil {
		t.Error("expected error for non-SOCKS5 reply version")
	}
}

func TestReadReplyRejectsUnknownATYP(t *testing.T) {
	conn := &fakeConn{r: bytes.NewReader([]byte{0x05, 0x00, 0x00, 0x09, 0, 0, 0, 0, 0, 0})}
	_, err := ReadReply(conn)
	if err == nil {
		t.Fatal("expected error for unknown ATYP")
	}
	if !errors.Is(err, ErrUnknownAddrType) {
		t.Errorf("expected ErrUnknownAddrType, got %v", err)
	}
}
