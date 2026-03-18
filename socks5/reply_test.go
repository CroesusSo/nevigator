package socks5

import (
	"testing"
)

func TestEncodeReplySuccess(t *testing.T) {
	got := EncodeReply(REPSuccess)
	want := []byte{0x05, 0x00, 0x00, 0x01, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00}
	if len(got) != 10 {
		t.Fatalf("expected 10 bytes, got %d", len(got))
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("byte[%d] = 0x%02x, want 0x%02x", i, got[i], want[i])
		}
	}
}

func TestEncodeReplyGeneralFailure(t *testing.T) {
	got := EncodeReply(REPGeneralFailure)
	if got[0] != Version {
		t.Errorf("VER = 0x%02x, want 0x%02x", got[0], Version)
	}
	if got[1] != REPGeneralFailure {
		t.Errorf("REP = 0x%02x, want 0x%02x", got[1], REPGeneralFailure)
	}
	if got[2] != 0x00 {
		t.Errorf("RSV = 0x%02x, want 0x00", got[2])
	}
	if got[3] != ATYPIPv4 {
		t.Errorf("ATYP = 0x%02x, want 0x%02x", got[3], ATYPIPv4)
	}
}

func TestEncodeReplyAllCodes(t *testing.T) {
	codes := []byte{
		REPSuccess, REPGeneralFailure, REPConnectionNotAllowed,
		REPNetworkUnreachable, REPHostUnreachable, REPConnectionRefused,
		REPTTLExpired, REPCommandNotSupported, REPAddrTypeNotSupported,
	}
	for _, code := range codes {
		reply := EncodeReply(code)
		if reply[1] != code {
			t.Errorf("EncodeReply(0x%02x): REP byte = 0x%02x", code, reply[1])
		}
	}
}
