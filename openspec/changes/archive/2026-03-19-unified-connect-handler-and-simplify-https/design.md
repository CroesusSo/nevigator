# Problem

1. **HTTPS handler selection bug**: The `topHandler` routes CONNECT requests to `handleHTTPSConnect` only when `port == 443 && s.tlsConfig != nil`. This means:
   - CONNECT to port 8443 over the HTTPS listener uses the wrong handler (plain HTTP), causing TLS protocol errors.
   - The correct behavior is to detect whether the *client connection* is TLS, regardless of destination port.

2. **Unused TLS_* strategies**: `Route.Strategy` defines `TLS_TUN`, `TLS_SOCKS5`, `TLS_DIRECT` that duplicate the behavior of `TUN`, `SOCKS5`, `DIRECT`. They contribute no value and should be removed.

3. **SNI complexity**: The original design aimed to use SNI for PAC routing, but it's unnecessary for the core use case (destination-based routing). The `tlsSNI()` builtin is a stub and can be removed.

# Solution

## 1. Unified CONNECT Handler

Replace the dual handlers (`handleHTTPConnect`, `handleHTTPSConnect`) with a single `handleCONNECT` that handles both HTTP and HTTPS connections.

In `proxy/server.go`:
- Remove the conditional in `topHandler` that checks `port == 443 && s.tlsConfig != nil` to route to different handlers.
- All CONNECT requests should call `s.handleCONNECT(w, r, host, port)`.
- Rename `handleHTTPConnect` to `handleCONNECT`.
- Inside `handleCONNECT`, after hijacking the client connection, detect TLS for **protocol tracking only**:
  ```go
  protocol := "http"
  if _, isTLS := clientConn.(*tls.Conn); isTLS {
      protocol = "https"
  }
  ```
- Remove any SNI-based protocol variations (like `"https-sni:"` prefixes).
- Remove the separate `handleHTTPSConnect` function entirely.
- The destination `host` is already extracted correctly from the CONNECT request (handles IPv6 brackets).

**Result**: The same handler works for both listeners. The protocol field in connection tracking shows `"http"` or `"https"` based on whether the client connection was TLS.

## 2. Drop TLS_* Route Strategies

Remove the unused route strategies:
- In `pac/config.go`: delete `"TLS_TUN"`, `"TLS_SOCKS5"`, `"TLS_DIRECT"` from the valid strategies list and comments.
- In `router/router.go`: delete the `case "TLS_TUN", "TLS_SOCKS5", "TLS_DIRECT":` clauses from the `dialWithRoute` switch. Unknown strategies should fall through to `DIRECT`.
- In `router/router_test.go`: remove tests for `TLS_TUN` and `TLS_SOCKS5`.

## 3. Remove tlsSNI Builtin

In `pac/builtins.go`, delete the entire registration block for `tlsSNI`. This builtin is a stub and unused; removing it simplifies the PAC environment.

## 4. Evaluator Signature Unchanged

The `Evaluator.Evaluate(host)` method already takes only the destination `host`. **No SNI parameter is added**. The `EvaluatorCache` continues to key solely on `host`.

## 5. Cache Key

`EvaluatorCache` uses only the destination `host` as the key. No change needed.

## 6. Admin UI Cleanup (Optional but Recommended)

Check `admin/handlers.go` for the `matchTypeOptions` list. If `"client_sni"` is present, remove it to prevent users from creating rules with a match type that is no longer supported by the native engine.

## 7. No Changes to PAC Generator

`gfw/generator.go` already generates `ruleToCondition` for match types; since we're not adding `MatchClientSNI`, no changes needed.

# Data Flow

```
Client --(CONNECT example.com:443)--> Proxy
   HTTP listener (plain)            |
   HTTPS listener (TLS)             |
       ↓ Hijack                    |
       ↓ Detect TLS? (for tracking only)
       ↓ evaluator.Evaluate(host)
       → Route (TUN/SOCKS5/DIRECT/EXTERNAL/DEFAULT)
       → router.Route(dest, route)
       → outboundConn
       ← "200 Connection Established"
       ← Relay(bytes) →
```

# Migration Notes

- Users with `TLS_TUN` should change to `TUN`; `TLS_SOCKS5` → `SOCKS5`; `TLS_DIRECT` → `DIRECT`.
- No other changes required.

# Testing

## Unit Tests
- Existing tests in `router/router_test.go` for TLS_* strategies will be removed/updated.
- All other unit tests should pass unchanged.

## Integration Tests
- **HTTPS proxy to non-443**: Verify that an HTTPS listener (with TLS) can successfully CONNECT to a destination on a non-standard port (e.g., 8443).
- **HTTPS proxy to port 443**: Ensure existing HTTPS proxy tests (listener startup, disabled without TLS) continue to pass.
- **HTTP proxy**: All existing HTTP CONNECT tests must pass (including IPv6 tests).
- **Protocol tracking**: Verify that connection tracking reports `protocol` as `"https"` for connections from the HTTPS listener, and `"http"` for the HTTP listener.

## Manual Testing
```bash
# Start proxy with HTTPS listener configured
# Test CONNECT to example.com:443 via HTTPS proxy
curl -x https://localhost:8443 https://example.com

# Test CONNECT to example.com:8443 (non-443) via HTTPS proxy
curl -x https://localhost:8443 https://example.com:8443

# Test via HTTP proxy (no TLS)
curl -x http://localhost:8080 https://example.com
```

# Alternatives Considered

- **Add SNI-based routing**: Could be added in the future if needed by exposing `tlsSNI()` builtin and adding `MatchClientSNI` to native engine. Rejected now as unnecessary complexity.
- **Keep dual handlers**: Would require duplicate logic; unified handler is simpler and more maintainable.
