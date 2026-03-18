## Context

The proxy server uses an `http.ServeMux` to route requests. The mux is registered with a single pattern `/` for `handleHTTPConnect`, which handles `CONNECT` requests. However, `http.ServeMux` only matches URL paths that start with the pattern. For a CONNECT request, the URL path is the target authority (e.g., `www.google.com:443`), which does **not** start with `/`. As a result, CONNECT requests never match and the mux returns 404.

## Goals / Non-Goals

**Goals:**
- Ensure `CONNECT` requests reach `handleHTTPConnect` for both HTTP and HTTPS listeners.
- Preserve admin UI (`/`) and API (`/api/...`) routing unchanged.
- Minimal code change; no performance impact.

**Non-Goals:**
- Modifying SOCKS5 (already independent).
- Changing CONNECT handling logic itself (just routing).
- Adding new configuration options.

## Decisions

**Decision 1: Top-level interceptor**

Wrap the mux with a small `http.Handler` that checks `r.Method == "CONNECT"` and calls `s.handleHTTPConnect` directly; otherwise delegates to the mux.

```
topHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
    if r.Method == http.MethodConnect {
        s.handleHTTPConnect(w, r)
        return
    }
    mux.ServeHTTP(w, r)
})
```

- **Why this approach?** Simplicity and clarity. It avoids fighting the mux pattern matching and works transparently for both HTTP and HTTPS servers. The wrapper is installed as the server's `Handler` (instead of the mux directly). Admin routes (which are on the mux) still work because they use non-CONNECT methods.
- **Alternatives:**
  - Register the CONNECT handler with an empty pattern `""`: Not supported; `ServeMux` requires valid patterns starting with `/`.
  - Separate `http.ServeMux` for CONNECT: Overcomplicated.
  - Modify `handleHTTPConnect` to be a regular handler and register it on the mux with a catch-all pattern using a custom `ServeMux`: not worth.

**Decision 2: Apply to both HTTP and HTTPS servers**

In `proxy.NewServer`, after building the `mux`, create the `topHandler` and assign it to both `s.httpServer.Handler` and `s.httpsServer.Handler` (if HTTPS is configured). This ensures consistent behavior.

- **Why not create separate wrappers?** One wrapper is sufficient; both servers share the same logic.

## Risks / Trade-offs

- **Risk:** Malformed non-CONNECT requests with unusual paths might still be handled by the mux as before. Mitigation: none needed; behavior unchanged.
- **Trade-off:** The wrapper adds one extra function call before every non-CONNECT request (negligible overhead).

## Migration Plan

1. Update `proxy/server.go` to create `topHandler`.
2. Replace `s.httpServer.Handler = mux` with `s.httpServer.Handler = topHandler`.
3. Replace `s.httpsServer.Handler = mux` (when created) with `topHandler`.
4. No config changes needed; restart required for effects.
5. Test: `curl -x http://host:8080 https://example.com` should succeed.

## Open Questions

- Should the wrapper also handle `CONNECT` over HTTPS (i.e., when the client connects to the HTTPS proxy)? Yes, both servers use the same wrapper.

- Do we need to adjust logging or metrics to reflect CONNECT handling path? No.
