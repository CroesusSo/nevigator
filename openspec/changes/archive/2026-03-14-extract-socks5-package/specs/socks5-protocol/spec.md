## ADDED Requirements

### Requirement: Shared SOCKS5 protocol constants
The `socks5` package SHALL define named constants for all SOCKS5 protocol byte values used by router and proxy packages.

#### Scenario: Address type constants exist
- **WHEN** a package imports `socks5`
- **THEN** it can reference `socks5.ATYPIPv4` (0x01), `socks5.ATYPDomain` (0x03), `socks5.ATYPIPv6` (0x04)

#### Scenario: Reply code constants exist
- **WHEN** a package imports `socks5`
- **THEN** it can reference `socks5.REPSuccess` (0x00), `socks5.REPGeneralFailure` (0x01), `socks5.REPCommandNotSupported` (0x07), `socks5.REPAddrTypeNotSupported` (0x08), and other standard reply codes

### Requirement: Address serialization
The `socks5` package SHALL provide a function to serialize a host:port pair into SOCKS5 wire format bytes (ATYP + address + port).

#### Scenario: Serialize IPv4 address
- **WHEN** `socks5.SerializeAddr("192.168.1.1", 443)` is called
- **THEN** it returns bytes: `[0x01, 0xC0, 0xA8, 0x01, 0x01, 0x01, 0xBB]` (ATYP=IPv4 + 4 octets + big-endian port)

#### Scenario: Serialize domain name
- **WHEN** `socks5.SerializeAddr("example.com", 80)` is called
- **THEN** it returns bytes: `[0x03, 0x0B, 'e','x','a','m','p','l','e','.','c','o','m', 0x00, 0x50]` (ATYP=domain + length byte + domain + big-endian port)

#### Scenario: Serialize IPv6 address
- **WHEN** `socks5.SerializeAddr("::1", 8080)` is called
- **THEN** it returns bytes with ATYP=0x04 followed by 16-byte IPv6 address and port

### Requirement: Address deserialization
The `socks5` package SHALL provide a function to deserialize SOCKS5 wire format bytes (from a request buffer) into a host string and port number.

#### Scenario: Deserialize IPv4 address
- **WHEN** `socks5.DeserializeAddr(buf, offset)` is called with a buffer containing `[0x01, 0xC0, 0xA8, 0x01, 0x01, 0x01, 0xBB]` at the given offset
- **THEN** it returns host="192.168.1.1", port=443, newOffset, nil

#### Scenario: Deserialize domain name
- **WHEN** `socks5.DeserializeAddr(buf, offset)` is called with domain-encoded bytes
- **THEN** it returns host="example.com", port=80, newOffset, nil

#### Scenario: Deserialize IPv6 address
- **WHEN** `socks5.DeserializeAddr(buf, offset)` is called with IPv6-encoded bytes
- **THEN** it returns host="::1", port=8080, newOffset, nil

#### Scenario: Handle unknown address type
- **WHEN** `socks5.DeserializeAddr(buf, offset)` is called with an unknown ATYP byte
- **THEN** it returns an error

### Requirement: Full read utility
The `socks5` package SHALL provide a helper that reads exactly N bytes from a connection, retrying short reads.

#### Scenario: Read exact bytes
- **WHEN** `socks5.ReadFull(conn, buf)` is called where buf length is 4
- **THEN** it reads exactly 4 bytes from conn, blocking until all bytes are read or an error occurs
- **AND** returns the number of bytes read (always `len(buf)` on success) and any error

### Requirement: Reply construction
The `socks5` package SHALL provide a function to construct standard SOCKS5 reply packets.

#### Scenario: Build success reply
- **WHEN** `socks5.EncodeReply(socks5.REPSuccess)` is called
- **THEN** it returns the 10-byte reply: `[0x05, 0x00, 0x00, 0x01, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00]`

#### Scenario: Build error reply with specific code
- **WHEN** `socks5.EncodeReply(socks5.REPCommandNotSupported)` is called
- **THEN** it returns a reply with REP byte set to 0x07
