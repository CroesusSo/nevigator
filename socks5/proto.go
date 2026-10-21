package socks5

import "fmt"

const (
	Version = 0x05

	// Address types
	ATYPIPv4   = 0x01
	ATYPDomain = 0x03
	ATYPIPv6   = 0x04

	// Commands
	CMDConnect = 0x01

	// Reply codes
	REPSuccess              = 0x00
	REPGeneralFailure       = 0x01
	REPConnectionNotAllowed = 0x02
	REPNetworkUnreachable   = 0x03
	REPHostUnreachable      = 0x04
	REPConnectionRefused    = 0x05
	REPTTLExpired           = 0x06
	REPCommandNotSupported  = 0x07
	REPAddrTypeNotSupported = 0x08

	// Authentication methods
	AuthNone         = 0x00
	AuthUserPass     = 0x02
	AuthNoAcceptable = 0xFF

	// AuthUserPassVersion is the version byte of the RFC 1929 user/pass subprotocol.
	AuthUserPassVersion = 0x01

	// AuthSuccess is the status byte of a successful user/pass authentication.
	AuthSuccess = 0x00
)

// Errors surfaced while exchanging SOCKS5 handshake messages.
var (
	ErrBadReplyVersion   = fmt.Errorf("invalid SOCKS5 reply version")
	ErrUserPassTooLong   = fmt.Errorf("SOCKS5 username or password exceeds 255 bytes")
	ErrNoAuthMethodFound = fmt.Errorf("no mutually supported authentication method")
	ErrAuthVersion       = fmt.Errorf("invalid SOCKS5 user/pass subnegotiation version")
)

// Auth holds credentials for SOCKS5 username/password authentication.
type Auth struct {
	Username string
	Password string
}
