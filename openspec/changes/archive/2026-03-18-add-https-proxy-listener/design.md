## Context

The proxy server currently offers two unencrypted entry points:
- HTTP CONNECT proxy on `HTTPListen` (default :8080)
- SOCKS5 proxy on `SOCKS5Listen` (default :11080)

There is no support for clients that require encrypted proxy connections via the HTTPS proxy protocol (`curl -x https://...`). The existing `TLSConfig` is used only for TLS termination on CONNECT requests to port 443 (a middlebox feature), not for accepting TLS-wrapped connections to the proxy itself.

The codebase already contains TLS infrastructure: certificate loading, `tls.Config` construction, and a dormant `CertManager`. This change will leverage that existing investment.

## Goals / Non-Goals

**Goals:**
- Add `https_listen` configuration to enable HTTPS proxy on a dedicated port (default `:8443`)
- Reuse existing HTTP CONNECT handler after TLS handshake (same routing logic)
- Use current `TLSConfig` for certificate loading and TLS parameters
- Maintain backward compatibility: HTTP and SOCKS5 unchanged
- Proper graceful shutdown of HTTPS listener
- Show HTTPS address in server status
- Fail gracefully if TLS config is invalid: log error but continue starting HTTP/SOCKS5

**Non-Goals:**
- Implement MITM/TLS inspection (different threat model)
- Support SOCKS5 over TLS (separate protocol)
- ALPN negotiation or HTTP/2 proxy support
- Certificate auto-generation or ACME automation
- Protocol auto-detection on a single port

## Decisions

### Separate Listening Port vs Protocol Detection

**Decision**: Add a separate `https_listen` configuration option with default `:8443`.

**Rationale**: Protocol detection (accepting both HTTP and TLS on same port) is complex, error-prone, and may break with clients that expect pure HTTPS. A separate port is:
- Clearer for operators (no ambiguity)
- Simpler to implement (use `tls.NewListener`)
- Compatible with existing tooling (curl `-x https://host:port` resolves to that port)
- Matches common proxy deployments (Squid, etc. use separate HTTP/HTTPS ports)

Default `:8443` chosen as conventional alternative to `:443` (doesn't require root, won't conflict with standard HTTPS web servers).

**Alternatives**: Single port with ALPN detection → Rejected due to complexity and risk of misidentification.

---

### HTTPS Server Architecture

**Decision**: Create a second `http.Server` instance with `TLSConfig` set, wrapped by `tls.NewListener`, sharing the same `mux` and `handler` as the HTTP server.

**Rationale**:
- The HTTP CONNECT handler (`handleHTTPConnect`) already implements all routing logic
- After TLS handshake, the connection is just HTTP, so existing handler works unchanged
- No code duplication; shared state (evaluator, router, connection tracking)
- Clean separation: HTTPS listener lifecycle independent of HTTP listener

**Implementation**:
```go
// In NewServer:
if cfg.HTTPSListen != "" && s.tlsConfig != nil {
    httpsLn, err := net.Listen("tcp", cfg.HTTPSListen)
    if err != nil { log and continue }
    tlsLn := tls.NewListener(httpsLn, s.tlsConfig)
    go s.httpsServer.Serve(tlsLn)  // httpsServer uses same mux
}
```

**Alternatives**:
- Modify `httpServer` to use `ListenAndServeTLS` → Rejected because that would bind to the same `Addr` and prevent HTTP on separate port. We want both HTTP and HTTPS simultaneously potentially.
- Duplicate handler logic → Rejected due to code duplication.

---

### TLS Configuration Source

**Decision**: Reuse the same `TLSConfig` structure from `pac.ProxyConfig` for both HTTPS listener and (existing) CONNECT termination.

**Rationale**: Single source of truth for certificate paths, cipher suites, client auth. Operators configure one TLS block. Already loaded in `loadTLSConfig`.

**Alternatives**: Separate fields like `https_tls` → Rejected (unnecessary complexity).

---

### Graceful Shutdown

**Decision**: Track HTTPS listener separately; call `httpsServer.Close()` during `Stop()` if HTTPS was started.

**Rationale**: Mirrors existing HTTP shutdown pattern. Both servers should close independently but during same shutdown sequence.

---

### Status Reporting

**Decision**: Extend `ServerStatus` with `HTTPSListen string` field showing the configured address (or empty if not configured).

**Rationale**: Operators need visibility into which interfaces are listening. Consistent with existing `HTTPListen` and `SOCKS5Listen`.

---

## Risks / Trade-offs

**[Certificate dependency]** → HTTPS listener fails to start if cert/key files are missing or invalid. **Mitigation**: Log warning during startup; continue with HTTP/SOCKS5. Error clearly states configuration issue.

**[Port conflict]** → If `https_listen` collides with another service, startup fails. **Mitigation**: Early detection via `net.Listen`; error message includes address and underlying error.

**[Increased surface area]** → Two entry points instead of one. **Mitigation**: Both use same authentication/authorization (PAC routing) so no additional risk.

**[TLS handshake overhead]** → CPU cost per connection. **Mitigation**: Expected for any TLS service; use session reuse (default in Go). Document performance expectations.

**[Configuration confusion]** → Users might set `https_listen` without `tls` config and wonder why HTTPS doesn't work. **Mitigation**: Log clear error at startup: "HTTPS proxy configured but TLS certificate not set".

## Migration Plan

This is an additive change with no breaking effects:

1. Deploy new binary (HTTPS feature dormant if not configured)
2. Add `https_listen` and `tls:` section to config.yaml (optional)
3. Restart proxy; HTTPS listener starts if config valid
4. Verify with `curl -x https://localhost:<https_port> https://www.google.com`
5. No rollback needed: simply remove `https_listen` to disable

## Open Questions

- Should HTTPS proxy respect PAC routing to EXTERNAL proxies that are HTTPS upstream? → Not applicable: HTTPS proxy is just a listener; routing logic unchanged.
- Should we allow different PAC evaluation for HTTPS proxy connections? → No, same evaluator; client IP and destination host are available.
- Do we need to handle `Proxy-Authorization` header for HTTPS proxy? → Already handled by CONNECT handler (it's just HTTP after all).