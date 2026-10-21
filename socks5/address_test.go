package socks5

import (
	"bytes"
	"errors"
	"net"
	"strings"
	"testing"
)

func TestSerializeAddrIPv4(t *testing.T) {
	got, err := SerializeAddr("192.168.1.1", 443)
	if err != nil {
		t.Fatalf("SerializeAddr: %v", err)
	}
	want := []byte{0x01, 0xC0, 0xA8, 0x01, 0x01, 0x01, 0xBB}
	if !bytes.Equal(got, want) {
		t.Errorf("SerializeAddr(192.168.1.1, 443) = %v, want %v", got, want)
	}
}

func TestSerializeAddrDomain(t *testing.T) {
	got, err := SerializeAddr("example.com", 80)
	if err != nil {
		t.Fatalf("SerializeAddr: %v", err)
	}
	want := []byte{0x03, 0x0B, 'e', 'x', 'a', 'm', 'p', 'l', 'e', '.', 'c', 'o', 'm', 0x00, 0x50}
	if !bytes.Equal(got, want) {
		t.Errorf("SerializeAddr(example.com, 80) = %v, want %v", got, want)
	}
}

func TestSerializeAddrIPv6(t *testing.T) {
	got, err := SerializeAddr("::1", 8080)
	if err != nil {
		t.Fatalf("SerializeAddr: %v", err)
	}
	if got[0] != ATYPIPv6 {
		t.Errorf("expected ATYP IPv6 (0x04), got 0x%02x", got[0])
	}
	if len(got) != 1+16+2 {
		t.Errorf("expected 19 bytes, got %d", len(got))
	}
	// Port 8080 = 0x1F90
	if got[17] != 0x1F || got[18] != 0x90 {
		t.Errorf("port bytes = [0x%02x, 0x%02x], want [0x1F, 0x90]", got[17], got[18])
	}
}

func TestDeserializeAddrIPv4(t *testing.T) {
	buf := []byte{0x00, 0x01, 0xC0, 0xA8, 0x01, 0x01, 0x01, 0xBB}
	host, port, offset, err := DeserializeAddr(buf, 1)
	if err != nil {
		t.Fatal(err)
	}
	if host != "192.168.1.1" {
		t.Errorf("host = %s, want 192.168.1.1", host)
	}
	if port != 443 {
		t.Errorf("port = %d, want 443", port)
	}
	if offset != 8 {
		t.Errorf("offset = %d, want 8", offset)
	}
}

func TestDeserializeAddrDomain(t *testing.T) {
	buf := []byte{0x00, 0x03, 0x0B, 'e', 'x', 'a', 'm', 'p', 'l', 'e', '.', 'c', 'o', 'm', 0x00, 0x50}
	host, port, offset, err := DeserializeAddr(buf, 1)
	if err != nil {
		t.Fatal(err)
	}
	if host != "example.com" {
		t.Errorf("host = %s, want example.com", host)
	}
	if port != 80 {
		t.Errorf("port = %d, want 80", port)
	}
	if offset != 16 {
		t.Errorf("offset = %d, want 16", offset)
	}
}

func TestDeserializeAddrIPv6(t *testing.T) {
	buf := make([]byte, 1+1+16+2)
	buf[1] = ATYPIPv6
	// IPv6 ::1: 16 bytes, last byte = 0x01 (at buf[1+16])
	buf[1+16] = 0x01
	// Port 8080 = 0x1F90 at buf[1+17], buf[1+18]
	buf[1+17] = 0x1F
	buf[1+18] = 0x90
	host, port, offset, err := DeserializeAddr(buf, 1)
	if err != nil {
		t.Fatal(err)
	}
	if host != "::1" {
		t.Errorf("host = %s, want ::1", host)
	}
	if port != 8080 {
		t.Errorf("port = %d, want 8080", port)
	}
	if offset != 1+1+16+2 {
		t.Errorf("offset = %d, want %d", offset, 1+1+16+2)
	}
}

func TestDeserializeAddrRoundTrip(t *testing.T) {
	tests := []struct {
		host string
		port uint16
	}{
		{"10.0.0.1", 8080},
		{"example.com", 443},
		{"::1", 22},
	}
	for _, tt := range tests {
		serialized, err := SerializeAddr(tt.host, tt.port)
		if err != nil {
			t.Fatalf("SerializeAddr(%s, %d): %v", tt.host, tt.port, err)
		}
		host, port, _, err := DeserializeAddr(serialized, 0)
		if err != nil {
			t.Fatalf("DeserializeAddr failed for %s:%d: %v", tt.host, tt.port, err)
		}
		// Compare IPs, not strings (IPv6 formatting varies)
		if net.ParseIP(tt.host) != nil {
			if !net.ParseIP(tt.host).Equal(net.ParseIP(host)) {
				t.Errorf("round-trip IP mismatch: %s -> %s", tt.host, host)
			}
		} else {
			if host != tt.host {
				t.Errorf("round-trip domain mismatch: %s -> %s", tt.host, host)
			}
		}
		if port != tt.port {
			t.Errorf("round-trip port mismatch: %d -> %d", tt.port, port)
		}
	}
}

func TestSerializeAddrDomainTooLong(t *testing.T) {
	long := strings.Repeat("a", 300)
	_, err := SerializeAddr(long, 80)
	if err == nil {
		t.Fatal("expected error for domain longer than 255 bytes")
	}
	if !errors.Is(err, ErrDomainTooLong) {
		t.Errorf("expected ErrDomainTooLong, got: %v", err)
	}
}

func TestSerializeAddrDomainAtLimit(t *testing.T) {
	max := strings.Repeat("a", 255)
	got, err := SerializeAddr(max, 80)
	if err != nil {
		t.Fatalf("255-byte domain should encode: %v", err)
	}
	if int(got[1]) != 255 {
		t.Errorf("length byte = %d, want 255", got[1])
	}
	host, _, _, err := DeserializeAddr(got, 0)
	if err != nil {
		t.Fatalf("DeserializeAddr: %v", err)
	}
	if host != max {
		t.Errorf("round-trip domain length = %d, want 255", len(host))
	}
}

func TestSerializeAddrEmptyDomain(t *testing.T) {
	if _, err := SerializeAddr("", 80); err == nil {
		t.Error("expected error for empty domain")
	}
}

func TestDeserializeAddrUnknownATYP(t *testing.T) {
	buf := []byte{0x99, 0x00}
	_, _, _, err := DeserializeAddr(buf, 0)
	if err == nil {
		t.Fatal("expected error for unknown ATYP")
	}
	if !errors.Is(err, ErrUnknownAddrType) {
		t.Errorf("expected ErrUnknownAddrType, got: %v", err)
	}
}

func TestDeserializeAddrTruncated(t *testing.T) {
	tests := []struct {
		name string
		buf  []byte
	}{
		{"empty", []byte{}},
		{"IPv4 too short", []byte{ATYPIPv4, 0x01, 0x02}},
		{"domain length only", []byte{ATYPDomain, 0x05}},
		{"IPv6 too short", []byte{ATYPIPv6, 0x01, 0x02, 0x03}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, _, _, err := DeserializeAddr(tt.buf, 0)
			if err == nil {
				t.Fatal("expected error for truncated buffer")
			}
			if !errors.Is(err, ErrBufferTooShort) {
				t.Errorf("expected ErrBufferTooShort, got: %v", err)
			}
		})
	}
}
