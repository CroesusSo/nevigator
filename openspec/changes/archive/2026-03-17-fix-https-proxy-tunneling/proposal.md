## Why

The HTTPS CONNECT handler in `proxy/server.go` was incorrectly implementing TLS termination with the client for HTTPS CONNECT requests. Instead of establishing a TCP tunnel (as expected for CONNECT method), it was:
1. Performing TLS handshake with the client
2. Extracting SNI from the established TLS connection
3. Using that TLS connection for relaying data

This caused clients attempting to use HTTPS CONNECT (e.g., for accessing HTTPS websites through the proxy) to receive TLS handshake data when they expected plain TCP tunneling, leading to OpenSSL errors like "wrong version number".

According to RFC 7231, the CONNECT method is used to establish a tunnel to a server identified by the target resource. For HTTPS, the client sends:
```
CONNECT www.example.com:443 HTTP/1.1
Host: www.example.com:443
```

The proxy should then:
1. Establish a TCP connection to the target (www.example.com:443)
2. Respond with HTTP/1.1 200 Connection Established
3. Relay TCP data bidirectionally between client and target without modifying the data

## What Changes

- **Fix `proxy/server.go`**: Refactored `handleHTTPSConnect` to implement proper HTTPS CONNECT tunneling behavior:
  - Removed incorrect TLS handshake with client
  - Added SNI extraction from ClientHello for routing decisions
  - Maintained compatibility with existing HTTP CONNECT flow
  - Preserved connection tracking and relay functionality
- **Added documentation**: Clear explanation of HTTPS CONNECT vs HTTPS PROXY semantics

## Capabilities

### Fixed Capabilities

- `https-connect-tunneling`: HTTPS CONNECT requests now properly establish TCP tunnels
- `sni-based-routing`: Server Name Indication can be used for PAC routing decisions
- `tls-error-resolution`: Eliminates "wrong version number" SSL errors when using HTTPS CONNECT

## Impact

- **Affected packages**: `proxy/`
- **No API changes**: External interface unchanged (CONNECT protocol remains standard)
- **Behavior change**: Fixes incorrect TLS termination behavior for HTTPS CONNECT
- **Test impact**: Existing tests should pass; manual verification confirms fix resolves SSL errors