package socks5

import (
	"errors"
	"fmt"
	"net"
)

var (
	ErrUnknownAddrType = errors.New("unknown SOCKS5 address type")
	ErrBufferTooShort  = errors.New("buffer too short for address")
	ErrDomainTooLong   = errors.New("SOCKS5 domain name exceeds 255 bytes")
	ErrEmptyDomain     = errors.New("empty SOCKS5 domain name")
)

// MaxDomainLen is the largest domain name representable in a SOCKS5 address,
// since the length is encoded in a single byte (RFC 1928 section 5).
const MaxDomainLen = 255

// SerializeAddr encodes a host:port pair into SOCKS5 wire format.
// The returned bytes contain ATYP + address + port (big-endian).
// An error is returned if the host cannot be represented on the wire.
func SerializeAddr(host string, port uint16) ([]byte, error) {
	portBytes := []byte{byte(port >> 8), byte(port)}

	ip := net.ParseIP(host)
	if ip != nil {
		if ip4 := ip.To4(); ip4 != nil {
			buf := make([]byte, 1+4+2)
			buf[0] = ATYPIPv4
			copy(buf[1:], ip4)
			copy(buf[5:], portBytes)
			return buf, nil
		}
		buf := make([]byte, 1+16+2)
		buf[0] = ATYPIPv6
		copy(buf[1:], ip.To16())
		copy(buf[17:], portBytes)
		return buf, nil
	}

	// Domain name
	domain := []byte(host)
	if len(domain) == 0 {
		return nil, ErrEmptyDomain
	}
	if len(domain) > MaxDomainLen {
		return nil, fmt.Errorf("%w: %d bytes", ErrDomainTooLong, len(domain))
	}
	buf := make([]byte, 1+1+len(domain)+2)
	buf[0] = ATYPDomain
	buf[1] = byte(len(domain))
	copy(buf[2:], domain)
	copy(buf[2+len(domain):], portBytes)
	return buf, nil
}

// DeserializeAddr decodes a SOCKS5 address from buf starting at offset.
// Returns the host, port, new offset after the address, and any error.
func DeserializeAddr(buf []byte, offset int) (string, uint16, int, error) {
	if offset >= len(buf) {
		return "", 0, 0, ErrBufferTooShort
	}

	atyp := buf[offset]
	offset++

	switch atyp {
	case ATYPIPv4:
		if offset+4+2 > len(buf) {
			return "", 0, 0, ErrBufferTooShort
		}
		host := net.IP(buf[offset : offset+4]).String()
		offset += 4
		port := uint16(buf[offset])<<8 | uint16(buf[offset+1])
		offset += 2
		return host, port, offset, nil

	case ATYPIPv6:
		if offset+16+2 > len(buf) {
			return "", 0, 0, ErrBufferTooShort
		}
		host := net.IP(buf[offset : offset+16]).String()
		offset += 16
		port := uint16(buf[offset])<<8 | uint16(buf[offset+1])
		offset += 2
		return host, port, offset, nil

	case ATYPDomain:
		if offset >= len(buf) {
			return "", 0, 0, ErrBufferTooShort
		}
		domainLen := int(buf[offset])
		offset++
		if offset+domainLen+2 > len(buf) {
			return "", 0, 0, ErrBufferTooShort
		}
		host := string(buf[offset : offset+domainLen])
		offset += domainLen
		port := uint16(buf[offset])<<8 | uint16(buf[offset+1])
		offset += 2
		return host, port, offset, nil

	default:
		return "", 0, 0, fmt.Errorf("%w: 0x%02x", ErrUnknownAddrType, atyp)
	}
}
