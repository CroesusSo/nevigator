## Why

The SOCKS5 proxy implementation in `router/router.go` was only reading 4 bytes of the SOCKS5 reply instead of the full 10-byte reply (VER, REP, RSV, ATYP, BND.ADDR, BND.PORT). This left extra bytes in the connection buffer that were interpreted as TLS handshake data, causing the "wrong version number" SSL error when establishing HTTPS connections through SOCKS5 proxies.

## What Changes

- **Fix `router/router.go`**: Changed `resp := make([]byte, 4)` to `resp := make([]byte, 10)` in the `dialSOCKS5WithAuth` function to properly consume the entire SOCKS5 reply.
- **Added documentation**: Comment explaining that BND.ADDR and BND.PORT are ignored per SOCKS5 spec but must be consumed to leave the connection in a clean state.

## Capabilities

### Fixed Capabilities

- `socks5-proxy`: SOCKS5 proxy connections now work correctly without leaving extra bytes in the connection buffer
- `https-via-socks5`: HTTPS connections can now be properly established when routed through SOCKS5 proxies

## Impact

- **Affected packages**: `router/`
- **No API changes**: External interface unchanged
- **Test impact**: No existing tests were affected; the bug was causing intermittent SSL errors in real usage