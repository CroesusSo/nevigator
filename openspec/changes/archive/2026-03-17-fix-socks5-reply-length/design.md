# Design: Fix SOCKS5 Reply Length

## Problem
The SOCKS5 proxy implementation was incorrectly reading only 4 bytes of the SOCKS5 server reply instead of the full 10-byte reply. According to the SOCKS5 protocol specification, the reply consists of:
- VER (1 byte): SOCKS version
- REP (1 byte): Reply code
- RSV (1 byte): Reserved
- ATYP (1 byte): Address type
- BND.ADDR (variable): Bound address
- BND.PORT (2 bytes): Bound port

When only 4 bytes were read, the remaining 6 bytes (BND.ADDR and BND.PORT) stayed in the connection buffer. For HTTPS connections, these extra bytes were interpreted as the beginning of a TLS handshake, causing SSL errors like "wrong version number".

## Solution
Increase the buffer size from 4 to 10 bytes to properly read the entire SOCKS5 reply. Although the BND.ADDR and BND.PORT fields are not used by the application (as per SOCKS5 spec, they're ignored), they must still be consumed from the buffer to prevent them from interfering with subsequent protocol negotiations.

## Implementation Details

### Before
```go
// Read reply: VER, REP, RSV, ATYP, BND.ADDR, BND.PORT
resp := make([]byte, 4)
if err := socks5.ReadFull(conn, resp); err != nil {
    conn.Close()
    return nil, err
}
// ... processing of first 4 bytes ...
```

### After
```go
// Read reply: VER, REP, RSV, ATYP, BND.ADDR, BND.PORT
resp := make([]byte, 10)
if err := socks5.ReadFull(conn, resp); err != nil {
    conn.Close()
    return nil, err
}

// BND.ADDR and BND.PORT are ignored as per SOCKS5 spec
// but we must consume them to leave the connection in clean state
```

## Files Modified
- `router/router.go`: Fixed the `dialSOCKS5WithAuth` function

## Testing
While no specific unit tests were added for this fix, the change was verified by:
1. Manual testing of HTTPS connections through SOCKS5 proxies
2. Verification that the "wrong version number" SSL error no longer occurs
3. Confirming that regular SOCKS5 functionality remains intact

## Diagram
```
Before Fix:
Client --> SOCKS5 Handshake --> [Partial Reply Read (4 bytes)] --> [6 bytes left in buffer]
                                                               │
                                                               ▼
                                                        Interpreted as TLS Handshake
                                                               │
                                                               ▼
                                                  SSL Error: wrong version number

After Fix:
Client --> SOCKS5 Handshake --> [Full Reply Read (10 bytes)] --> [Connection clean]
                                                               │
                                                               ▼
                                                Proceed with intended protocol (HTTP/TLS)
                                                               │
                                                               ▼
                                                     Connection established successfully
```