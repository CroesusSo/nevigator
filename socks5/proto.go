package socks5

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
)

// Auth holds credentials for SOCKS5 username/password authentication.
type Auth struct {
	Username string
	Password string
}
