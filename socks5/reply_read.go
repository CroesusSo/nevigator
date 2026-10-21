package socks5

import (
	"fmt"
	"net"
)

// ReadReply reads a complete SOCKS5 reply from conn and returns its REP code.
//
// The reply header (VER, REP, RSV, ATYP) is read first, then the bound address
// is consumed according to ATYP. The full reply is variable length: 10 bytes
// for IPv4, 22 for IPv6, and 7+len for a domain, so a fixed-size read would
// leave trailing bytes in the stream and corrupt the tunnelled payload.
func ReadReply(conn net.Conn) (byte, error) {
	head := make([]byte, 4)
	if err := ReadFull(conn, head); err != nil {
		return 0, fmt.Errorf("read reply header: %w", err)
	}
	if head[0] != Version {
		return 0, fmt.Errorf("%w: reply version %d", ErrBadReplyVersion, head[0])
	}

	var addrLen int
	switch head[3] {
	case ATYPIPv4:
		addrLen = 4
	case ATYPIPv6:
		addrLen = 16
	case ATYPDomain:
		lenByte := make([]byte, 1)
		if err := ReadFull(conn, lenByte); err != nil {
			return 0, fmt.Errorf("read domain length: %w", err)
		}
		addrLen = int(lenByte[0])
	default:
		return 0, fmt.Errorf("%w: 0x%02x", ErrUnknownAddrType, head[3])
	}

	// Consume BND.ADDR and BND.PORT so the stream is left at application data.
	if addrLen+2 > 0 {
		if err := ReadFull(conn, make([]byte, addrLen+2)); err != nil {
			return 0, fmt.Errorf("read bound address: %w", err)
		}
	}
	return head[1], nil
}
