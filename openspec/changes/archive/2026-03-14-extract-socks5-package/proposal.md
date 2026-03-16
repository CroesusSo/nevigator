## Why

SOCKS5 protocol byte-level logic (address encoding/decoding, reply codes, constants) is duplicated across `router/router.go` and `proxy/server.go`. Both files independently implement magic numbers (`0x01`, `0x03`, `0x04`) and hand-rolled serialization for the same protocol. Changes to SOCKS5 behavior require manual sync across two files with no compile-time safety.

## What Changes

- **New package `socks5/`**: Shared constants, address serialization/deserialization, reply helpers
- **Refactor `router/router.go`**: Use `socks5.SerializeAddr()` and `socks5` constants instead of inline byte manipulation
- **Refactor `proxy/server.go`**: Use `socks5.DeserializeAddr()` and `socks5` constants instead of inline parsing
- **No behavior changes**: Protocol remains identical; only internal code organization changes

## Capabilities

### New Capabilities

- `socks5-protocol`: Shared SOCKS5 protocol primitives — constants (`ATYPIPv4`, `ATYPDomain`, `ATYPIPv6`, `REP*`), `SerializeAddr(host, port)`, `DeserializeAddr(buf)`, `EncodeReply(rep, atyp, bndAddr, bndPort)`, `ParseReply(buf)` helpers

### Modified Capabilities

- `router-routing`: Router's `dialSOCKS5()` now imports `socks5/` for constants and address serialization
- `proxy-server`: Proxy's `handleSOCKS5()` now imports `socks5/` for constants and address deserialization

## Impact

- **Affected packages**: `router/`, `proxy/`, new `socks5/`
- **No API changes**: External interface unchanged (PAC format, config, admin API)
- **No dependency changes**: No new external imports
- **Test impact**: `socks5/` gets its own unit tests; existing router/proxy tests verify no regressions
