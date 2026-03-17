## Why

The HTTPS CONNECT handler in `proxy/server.go` was incorrectly implementing TLS termination with the client instead of tunneling for HTTPS CONNECT requests. This caused clients attempting to establish HTTPS tunnels through the proxy to receive TLS handshake data when they expected plain TCP tunneling, resulting in "wrong version number" SSL errors.

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