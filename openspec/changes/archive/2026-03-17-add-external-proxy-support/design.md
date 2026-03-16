## Context

The existing proxy supports three routing strategies: `TUN` (bind to TUN device), `SOCKS5` (hardcoded upstream address from rule), and `DIRECT`. Rules (user-created or GFW-generated) embed the exact SOCKS5 address directly in the PAC script. There is no central registry for upstream proxies, making credential updates painful and preventing easy switching between multiple upstreams.

**Current State:**

- Config YAML has no external proxy definitions.
- `pac.Route` has only `Strategy` and `Address`.
- `router.dialSOCKS5` supports no authentication.
- Admin UI cannot manage upstream proxies.
- GFW generator defaults to `"SOCKS5 127.0.0.1:1080"` for rules without explicit route.

**Constraints:**

- Must preserve backward compatibility with existing PAC files and rules.
- Admin UI must prevent routing loops (external proxy must not point to this proxy's own listener on localhost).
- Passwords will be stored in plaintext in config YAML (acceptable for this environment; future enhancement may add encryption).
- SOCKS5 authentication only supports username/password (no GSSAPI or others).

---

## Goals / Non-Goals

**Goals:**
- Introduce a central registry of external SOCKS5 proxies in configuration.
- Enable rules to reference a named external proxy via `"EXTERNAL <id>"` or the special `"DEFAULT"` token.
- Allow administrators to set a default external proxy that applies to rules with empty route (GFW rules).
- Provide admin UI for CRUD management of external proxies, including pre-save connectivity/authentication testing.
- Update router to resolve external proxies by ID, with optional authentication, and fallback to DIRECT on failure (existing behavior).
- Update PAC generator to emit `DEFAULT` for empty routes when a default proxy is configured.
- Validate that external proxy addresses do not create loops with this proxy's listeners on localhost.

**Non-Goals:**
- Hot-reload of external proxy registry (config file changes not watched; admin API updates router directly).
- Encrypted storage of proxy credentials.
- Per-proxy connection pooling or timeouts (use global dial timeout).
- SOCKS5 chaining (proxy → proxy).
- Upstream HTTP proxy support (only SOCKS5).
- Automated restart of listeners when admin edits listening ports via UI (will require manual restart).
- Validation of external proxy hostnames beyond localhost loop check.

---

## Decisions

### 1. Data Model

Define `ExternalProxy` in `pac/config.go` (or a common package):

```go
type ExternalProxy struct {
    ID       string `yaml:"id" json:"id"`
    Address  string `yaml:"address" json:"address"`  // host:port
    Username string `yaml:"username,omitempty" json:"username,omitempty"`
    Password string `yaml:"password,omitempty" json:"password,omitempty"`
}
```

Add to `RouterConfig` (or pass directly to `NewRouter`):
```go
type RouterConfig struct {
    TUNDevice   string
    TUNIP       net.IP
    DialTimeout time.Duration
    Proxies     []ExternalProxy // injected, not from YAML directly
}
```

Extend `pac.Route`:
```go
type Route struct {
    Strategy   string // "TUN", "DIRECT", "SOCKS5", "EXTERNAL", "DEFAULT"
    Address    string // for legacy "SOCKS5 host:port"
    ExternalID string // for "EXTERNAL <id>", empty otherwise
}
```

**Rationale:** Keeping `ExternalProxy` separate from `RouterConfig` allows clean YAML mapping. Passing the slice to `NewRouter` and then calling `SetProxyRegistry` gives flexibility for hot updates.

---

### 2. Router Registry and Default

`Router` fields:
```go
type Router struct {
    cfg             pac.RouterConfig
    tunIP           net.IP
    dialTimeout     time.Duration
    proxyMu         sync.RWMutex
    proxyRegistry   map[string]ExternalProxy // ID → proxy
    defaultProxyID  string
}
```

Methods:
- `SetProxyRegistry(proxies []ExternalProxy)`: builds map, replaces atomically under lock.
- `SetDefaultProxyID(id string)`: updates default, used for `DEFAULT` strategy.

In `dialWithRoute`, new cases:

```go
case "EXTERNAL":
    r.proxyMu.RLock()
    proxy, ok := r.proxyRegistry[route.ExternalID]
    r.proxyMu.RUnlock()
    if !ok {
        return nil, fmt.Errorf("external proxy not found: %s", route.ExternalID)
    }
    return r.dialSOCKS5WithAuth(destAddr, proxy.Address, dialer, &socks5.Auth{Username: proxy.Username, Password: proxy.Password})
case "DEFAULT":
    r.proxyMu.RLock()
    id := r.defaultProxyID
    r.proxyMu.RUnlock()
    if id == "" {
        // no default configured → fallback to DIRECT later; return error now
        return nil, errors.New("no default external proxy configured")
    }
    r.proxyMu.RLock()
    proxy, ok := r.proxyRegistry[id]
    r.proxyMu.RUnlock()
    if !ok {
        return nil, fmt.Errorf("default external proxy not found: %s", id)
    }
    return r.dialSOCKS5WithAuth(destAddr, proxy.Address, dialer, &socks5.Auth{Username: proxy.Username, Password: proxy.Password})
```

**Rationale:** Separate cases keep code clear. Registry lookup under `RWMutex` allows concurrent reads and rare updates.

---

### 3. SOCKS5 Client with Authentication

Existing `router.dialSOCKS5` only handles no-auth. We'll create a reusable function in `socks5` package:

```go
func DialWithAuth(network, addr string, auth *Auth, timeout time.Duration) (net.Conn, error)
```

It will:
- Dial TCP
- Send greeting: version 5, 1 or 2 methods (no-auth and/or userpass)
- Read server selection
- If server selects userpass and we have credentials, send username/password; else fail
- Return connected `net.Conn` (ready for CONNECT request)
- Caller then sends CONNECT request (as router currently does)

Router's `dialSOCKS5WithAuth` will use this and then perform the CONNECT request.

**Rationale:** Centralizes handshake; reusable by admin test endpoint.

---

### 4. PAC Parser Extension

In `pac/parser.go`, add cases:

```go
case "EXTERNAL":
    if len(tokens) >= 2 {
        return Route{Strategy: "EXTERNAL", ExternalID: tokens[1]}
    }
    fallthrough
case "DEFAULT":
    return Route{Strategy: "DEFAULT"}
```

**Rationale:** Simple tokenization; unknown tokens fall back to DIRECT.

---

### 5. PAC Generator Update

In `gfw/generator.go`, `writeRuleCondition`:

```go
route := r.Route
if route == "" {
    if meta.DefaultExternalProxy != "" {
        route = "DEFAULT"
    } else {
        route = "SOCKS5 127.0.0.1:1080" // legacy
    }
}
// write condition with route string
```

The generator's `Meta` struct needs `DefaultExternalProxy` field. The `GeneratePAC` function will accept it as parameter.

**Rationale:** Maintains backward compatibility while enabling new default behavior.

---

### 6. Admin UI: Proxies Tab

Structure:
- **Default External Proxy** selector at top: `<select>` populated with external proxy IDs plus `(none)`.
- **Upstream Proxies** table: columns ID, Address, Auth?, Actions (Edit, Delete, Test)
- **Add Proxy** button opens modal with fields: ID, Address, Username, Password, and **Test Connection** button. Save button disabled until test succeeds.
- **Settings** tab separated: shows and edits listener addresses (HTTP, SOCKS5, Admin) plus PAC path and TUN device. Saving writes config; displays "restart required" notice.

JavaScript:
- `loadProxies()` fetches `/api/proxies` and renders table.
- `testNewProxy()` calls `/api/proxies/test` with modal fields, enables Save on success.
- `saveNewProxy()` POSTs to `/api/proxies`, then reloads.
- `testProxy(id)` calls `/api/proxies/:id/test`.
- Default selector: whenever changed, `PUT /api/config` with `{default_external_proxy: id}` and then `regeneratePAC()`.

---

### 7. Admin API

Handlers in `admin/handlers.go`:

- `handleListProxies` (GET `/api/proxies`): reads config, returns array with id, address, hasAuth (bool), NOT password.
- `handleCreateProxy` (POST `/api/proxies`): validates ID (unique, not reserved), address (valid host:port, not loopback to our listeners), duplicates? call `router.SetProxyRegistry` after write.
- `handleUpdateProxy` (PUT `/api/proxies/:id`): handle partial updates (password optional). Validate ID immutability, address, loop check. Call `router.SetProxyRegistry`.
- `handleDeleteProxy` (DELETE `/api/proxies/:id`): remove from slice, write, call `router.SetProxyRegistry`.
- `handleTestProxy` (POST `/api/proxies/test`): test arbitrary credentials without saving.
- `handleTestProxyByID` (POST `/api/proxies/:id/test`): test stored proxy.
- `handleGetConfig` (GET `/api/config`): returns `proxy.http_listen`, `socks5_listen`, `admin_listen`, `pac_path`, `tun_device`, `default_external_proxy`.
- `handlePutConfig` (PUT `/api/config`): accepts partial JSON; updates only provided fields; for changes to `default_external_proxy`, also calls `router.SetDefaultProxyID` and `regeneratePAC()`; for listener changes, writes config but notes restart needed.

**Config persistence:** Use atomic write (tmp + rename). Read full config, modify, marshal YAML, write. Keep other sections unchanged.

---

### 8. Startup Integration

In `main.go` after creating `Router`:

```go
// Set external proxy registry
if len(cfg.Proxy.ExternalProxies) > 0 {
    router.SetProxyRegistry(cfg.Proxy.ExternalProxies)
}
// Set default external proxy
if cfg.Proxy.DefaultExternalProxy != "" {
    router.SetDefaultProxyID(cfg.Proxy.DefaultExternalProxy)
}
```

Also ensure `config.yaml` generation (`generateDefaultConfig`) includes empty `external_proxies: []` and `default_external_proxy: ""`.

---

### 9. Validation Logic

**Loop check** in proxy create/update:

```go
func validateProxyAddress(addr string, ourHTTP, ourSOCKS5, ourAdmin string) error {
    host, portStr, err := net.SplitHostPort(addr)
    if err != nil {
        return fmt.Errorf("invalid address: %w", err)
    }
    port, _ := strconv.Atoi(portStr)
    // Check against our ports if host is localhost
    if isLocalhost(host) {
        // Get our listening ports
        httpPort, _ := getPort(ourHTTP)
        socksPort, _ := getPort(ourSOCKS5)
        adminPort, _ := getPort(ourAdmin)
        if port == httpPort || port == socksPort || port == adminPort {
            return fmt.Errorf("proxy address conflicts with this proxy's listener on a loopback interface")
        }
    }
    return nil
}
func isLocalhost(host string) bool {
    host = strings.ToLower(host)
    return host == "127.0.0.1" || host == "localhost" || host == "::1"
}
```

**ID uniqueness:** Check against existing IDs in config slice.
**Reserved words:** `"DIRECT"`, `"TUN"`, `"DEFAULT"` rejected.

---

### 10. Backward Compatibility

- Existing configs lacking new fields load with empty external_proxies and empty default → behavior unchanged.
- Existing rules with route `"SOCKS5 127.0.0.1:1080"` remain valid and continue using that address.
- GFW rules with empty route: if default not set, generator uses legacy `SOCKS5 127.0.0.1:1080`; if default set, generator uses `DEFAULT`. No breaking change.
- Router's `SOCKS5` case still uses direct address; new `EXTERNAL` and `DEFAULT` cases are only triggered by new PAC output.

---

## Risks / Trade-offs

| Risk | Mitigation |
|------|------------|
| Plaintext passwords in config file | Document security implications; consider env var expansion or external secrets manager in future |
| External proxy becomes unavailable | Router fallback to DIRECT maintains connectivity |
| User edits config manually and introduces duplicate IDs or invalid addresses | Admin API validates; server startup should also validate and log errors without crashing |
| Loop detection only covers localhost; user could point to another instance of this proxy on a different IP | This is allowed; advanced users may intentionally chain; no mitigation beyond docs |
| Changing default external proxy does not update existing GFW rules until PAC regenerated | Admin handler calls `regeneratePAC()` immediately after updating default |
| Large number of external proxies causes dropdown slowdown | Proxy list is small (< 20 typically); no virtualization needed |

---

## Migration Plan

1. **Upgrade binary** (replace existing)
2. **Config file**: new fields optional; no changes required. Admin UI will now allow managing external proxies.
3. **Existing rules**: continue to work. Admin may optionally migrate hardcoded SOCKS5 rules to use `EXTERNAL <id>` for easier management.
4. **If using default**: Set `proxy.default_external_proxy` in config YAML or via UI. GFW rules will automatically adopt this default after next PAC regeneration (manual or auto-fetch).
5. **Rollback**: Downgrade binary; config fields are ignored by older versions (they don't read them). The binary remains compatible with older configs.

---

## Open Questions

1. Should the admin UI allow editing the default SOCKS5 fallback address (currently hardcoded `127.0.0.1:1080`)? Probably not; it's a legacy fallback.
2. Should we require a restart when external proxy definitions change? No, router registry updates instantly via admin API.
3. Should the test endpoint also attempt a CONNECT to a known host to verify full functionality? Not necessary; handshake success is sufficient.
4. Should we allow EXTERNAL proxy addresses to be the same as our SOCKS5 listener but on a different IP? Yes, only loopback is blocked.
5. What happens if `DEFAULT` is used but the configured default ID is later deleted? Router will error and fallback to DIRECT. Should we prevent deletion of a proxy that is set as default? We could block deletion or clear default. Likely allow deletion; default becomes broken and will fallback. Admin UI could show warning.