## Context

SOCKS5 protocol logic (address encoding/decoding, reply codes, constants) is duplicated between `router/router.go` (client role — dialing SOCKS5 upstreams) and `proxy/server.go` (server role — accepting SOCKS5 client connections). Both files independently define the same magic numbers (`0x01` for IPv4, `0x03` for domain, `0x04` for IPv6) and implement hand-rolled serialization for the same protocol. The duplication is ~30 lines of address logic plus scattered constants, creating a maintenance risk where protocol changes must be manually synced.

## Goals / Non-Goals

**Goals:**
- Extract shared SOCKS5 protocol primitives into a single `socks5/` package
- Eliminate magic numbers — named constants for all protocol bytes
- Provide clean `SerializeAddr` / `DeserializeAddr` helpers used by both caller and callee
- No behavior change — protocol remains byte-identical
- Add unit tests for the new package

**Non-Goals:**
- Adding SOCKS5 authentication support
- Supporting UDP ASSOCIATE or BIND commands
- Changing external APIs or configuration
- Refactoring non-SOCKS5 code in router/proxy

## Decisions

### Decision: Single `socks5/` package for both client and server logic

**Rationale:** The protocol constants and address encoding are identical whether you're the client (router dialing upstream) or server (proxy accepting clients). One package with shared types prevents drift.

**Alternatives considered:**
- Separate `socks5/client` and `socks5/server` packages — overkill for ~100 lines of protocol logic
- Put constants in `pac/` or `router/` — doesn't solve the cross-package duplication

### Decision: Functional API, not types

**Rationale:** The SOCKS5 logic is stateless. Functions like `SerializeAddr(host string, port uint16) []byte` are simpler than defining a `SOCKS5Request` struct with methods. Fits the existing codebase style (functional, minimal).

**Alternatives considered:**
- Define `Request` / `Reply` structs with `Encode()` / `Decode()` methods — adds ceremony for no benefit; the protocol is simple enough that functions suffice

### Decision: Keep `netFullRead` helper in router, move to socks5 package

**Rationale:** `netFullRead` (`router/router.go:188-196`) is a generic "read exactly N bytes" utility used only for SOCKS5 reading. Moving it to `socks5/` makes it available to both packages and better reflects its purpose.

## Risks / Trade-offs

- **[Risk] Behavioral regression during refactor** → Mitigation: existing integration tests (`integration_test.go`) and unit tests in `router/` and `proxy/` will catch regressions; socks5 package gets its own tests
- **[Risk] Over-engineering a small amount of duplication** → Mitigation: keeping the package minimal (~100 lines, 4-5 functions) avoids scope creep
- **[Trade-off] Extra import in two packages** → Acceptable; `socks5/` is internal and has zero external dependencies

## Migration Plan

1. Create `socks5/` package with constants, helpers, and tests
2. Refactor `router/router.go` — replace inline bytes with `socks5.` calls
3. Refactor `proxy/server.go` — replace inline bytes with `socks5.` calls
4. Run `go test ./...` — verify no regressions
5. Run `go vet ./...` — verify no issues

No deployment concerns — this is internal code reorganization only.

## Open Questions

- Should `socks5/` include a `ParseRequest()` high-level function for the proxy, or keep it lower-level (address + port helpers only)? Leaning toward lower-level — the proxy's request parsing is more involved (method negotiation, command validation) and stays in `proxy/server.go`.
