package socks5

// EncodeReply constructs a standard 10-byte SOCKS5 reply packet.
// BND.ADDR and BND.PORT are set to 0.0.0.0:0 (IPv4 unspecified).
func EncodeReply(rep byte) []byte {
	return []byte{
		Version, rep, 0x00, // VER, REP, RSV
		ATYPIPv4,               // ATYP (IPv4)
		0x00, 0x00, 0x00, 0x00, // BND.ADDR (0.0.0.0)
		0x00, 0x00, // BND.PORT (0)
	}
}
