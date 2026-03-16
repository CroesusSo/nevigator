## 1. Data Model & Configuration

- [x] 1.1 Add `ExternalProxy` struct in `pac/config.go`
- [x] 1.2 Extend `RouterConfig` with `ExternalProxies []ExternalProxy` (or pass separately)
- [x] 1.3 Extend `pac.Route` with `ExternalID string`
- [x] 1.4 Update `main.go` Config struct: add `DefaultExternalProxy` and `ExternalProxies` under `Proxy`
- [x] 1.5 Update `admin/handlers.go` `FullConfig` to include same fields
- [x] 1.6 Update `generateDefaultConfig` in `main.go` to emit empty `external_proxies: []` and `default_external_proxy: ""`
- [x] 1.7 Add YAML tags and validation helpers (e.g., `isLocalhost`, port extraction)

## 2. PAC Parser Support

- [x] 2.1 In `pac/parser.go`, import needed packages
- [x] 2.2 Add case for `"EXTERNAL"`: return `Route{Strategy: "EXTERNAL", ExternalID: tokens[1]}`
- [x] 2.3 Add case for `"DEFAULT"`: return `Route{Strategy: "DEFAULT"}`
- [x] 2.4 Unit tests: parse "EXTERNAL tor", "DEFAULT", unknown fallbacks

## 3. Router Enhancements

- [x] 3.1 Add fields to `Router`: `proxyMu sync.RWMutex`, `proxyRegistry map[string]ExternalProxy`, `defaultProxyID string`
- [x] 3.2 Implement `SetProxyRegistry([]ExternalProxy)` to build map
- [x] 3.3 Implement `SetDefaultProxyID(string)`
- [x] 3.4 Refactor `dialSOCKS5` → `dialSOCKS5WithAuth(destAddr, socksAddr, dialer, *socks5.Auth)`
- [x] 3.5 Add case `"EXTERNAL"` in `dialWithRoute`: lookup ID, call `dialSOCKS5WithAuth` with proxy credentials
- [x] 3.6 Add case `"DEFAULT"` in `dialWithRoute`: lookup `defaultProxyID`, then proxy, call `dialSOCKS5WithAuth`
- [x] 3.7 Handle errors: unknown proxy, missing default → return error (fallback to DIRECT)
- [x] 3.8 Add tests: router with mock proxies, EXTERNAL and DEFAULT routing

## 4. SOCKS5 Client with Authentication

- [x] 4.1 In `socks5` package, define `type Auth struct { Username, Password string }`
- [x] 4.2 Implement `DialWithAuth(network, addr string, auth *Auth, timeout time.Duration) (net.Conn, error)`:
  - TCP dial
  - Greeting: offer methods based on auth presence
  - Handle server method selection
  - If USERPASS selected, send credentials and check response
  - Return ready connection (no CONNECT)
- [x] 4.3 Update `router.dialSOCKS5WithAuth` to use `socks5.DialWithAuth` then send CONNECT request
- [x] 4.4 Add unit tests for `DialWithAuth` (no auth, userpass success, failure)

## 5. PAC Generator Update

- [x] 5.1 In `gfw/generator.go`, update `GeneratePAC` signature to accept `defaultExternalProxy string`
- [x] 5.2 In `writeRuleCondition`, when `r.Route == ""`:
  - If `defaultExternalProxy != ""`, use `route = "DEFAULT"`
  - Else use legacy `route = "SOCKS5 127.0.0.1:1080"`
- [x] 5.3 Update `gfw.RegeneratePAC` in main to pass config's default proxy (need to modify call site)
- [x] 5.4 Unit test: generator emits DEFAULT when provided, else legacy

## 6. Admin Handler: Proxy CRUD & Test

- [x] 6.1 Extend `FullConfig.Proxy` with `ExternalProxies []ExternalProxy` (same as in `pac/config.go` but for admin)
- [x] 6.2 Modify `NewHandler` to accept `router *router.Router`
- [x] 6.3 Implement `handleListProxies` (GET `/api/proxies`) – omit passwords
- [x] 6.4 Implement `handleCreateProxy` (POST `/api/proxies`) – validate ID uniqueness, address, loop check; append to config; `SetProxyRegistry`
- [x] 6.5 Implement `handleUpdateProxy` (PUT `/api/proxies/:id`) – handle partial updates, preserve password if empty, validate, `SetProxyRegistry`
- [x] 6.6 Implement `handleDeleteProxy` (DELETE `/api/proxies/:id`) – remove from slice; `SetProxyRegistry`
- [x] 6.7 Implement `handleTestProxy` (POST `/api/proxies/test`) – test arbitrary address/creds using `socks5.DialWithAuth`
- [x] 6.8 Implement `handleTestProxyByID` (POST `/api/proxies/:id/test`) – load from config, test
- [x] 6.9 Add validation helper: `validateProxyAddress(addr, ourListeners)` for loop prevention
- [x] 6.10 Add ID validation: reject reserved words ("DIRECT","TUN","DEFAULT")
- [x] 6.11 Update `regeneratePAC` call after proxy changes? Not needed; only after default proxy change.

## 7. Admin Handler: Config Endpoint for Default Proxy & Listeners

- [x] 7.1 Implement `handleGetConfig` (GET `/api/config`) – return JSON with `http_listen`, `socks5_listen`, `admin_listen`, `pac_path`, `tun_device`, `default_external_proxy`
- [x] 7.2 Implement `handlePutConfig` (PUT `/api/config`) – partial update; handle `default_external_proxy` by:
  - Updating config file
  - Calling `router.SetDefaultProxyID`
  - Calling `regeneratePAC()`
- [x] 7.3 For listener fields, just update config file with note "restart required" in response; do not hot-reload
- [x] 7.4 Ensure atomic writes (tmp + rename) for all config updates

## 8. Startup & Main Integration

- [x] 8.1 After creating router, call `router.SetProxyRegistry(cfg.Proxy.ExternalProxies)`
- [x] 8.2 Call `router.SetDefaultProxyID(cfg.Proxy.DefaultExternalProxy)`
- [x] 8.3 Ensure `gfw.RegeneratePAC` passes `cfg.Proxy.DefaultExternalProxy` (update `regeneratePAC` wrapper)
- [x] 8.4 Update `main.go` to pass `router` to `admin.NewHandler`
- [x] 8.5 Verify default config generation includes empty sections

## 9. Admin UI: Proxies Tab

- [x] 9.1 In `admin/handlers.go` HTML, add new tab "Proxies"
- [x] 9.2 Add Proxies tab content:
  - Default External Proxy selector (populated by JS)
  - Upstream Proxies table (ID, Address, Auth?, Actions)
  - Add Proxy button and modal with fields (ID, Address, Username, Password, Test button)
  - This Proxy's Listeners card (read-only, from `/api/status`)
- [x] 9.3 Implement JS functions:
  - `loadProxies()` fetch `/api/proxies` and render table
  - `showAddProxyModal()` / `closeAddProxyModal()`
  - `testNewProxy()` calls `/api/proxies/test` and enables/disables Save button
  - `saveNewProxy()` POSTs to `/api/proxies` and reloads
  - `editProxy(id)` populate modal for edit (optional if time; can skip for MVP and rely on direct PUT? Better to include)
  - `saveEditProxy(id)` PUTs `/api/proxies/:id`
  - `deleteProxy(id)` with confirm
  - `testProxy(id)` – alerts result
  - Default selector change handler: `PUT /api/config` with `{default_external_proxy}`
- [x] 9.4 Ensure route dropdown in Rules tab is populated with EXTERNAL IDs (call `loadProxies` to get list and build options)
- [x] 9.5 Add `DEFAULT` option to route dropdown

## 10. Admin UI: Settings Tab (Optional but suggested)

- [x] 10.1 Add Settings tab with form for listening addresses (HTTP, SOCKS5, Admin) and PAC path/TUN device
- [x] 10.2 Implement JS to fetch `/api/config` and populate fields
- [x] 10.3 Implement save to `PUT /api/config`; show success/restart message
- [x] 10.4 Make fields editable vs read-only as desired

## 11. Tests & Verification

- [x] 11.1 Unit test: `parsePACResult` for EXTERNAL and DEFAULT
- [x] 11.2 Unit test: `Router.SetProxyRegistry` and lookup
- [x] 11.3 Unit test: `Router.dialWithRoute` EXTERNAL and DEFAULT paths with mock dialer/auth
- [x] 11.4 Unit test: `socks5.DialWithAuth` handshake (no auth, userpass success)
- [x] 11.5 Integration test: rule with EXTERNAL routes through proxy
- [x] 11.6 Integration test: rule with DEFAULT uses configured default
- [x] 11.7 End-to-end: Admin UI can add external proxy, test it, set as default, see PAC regenerated
- [x] 11.8 Negative test: loop address rejected by API
- [x] 11.9 Negative test: unknown external proxy ID falls back to DIRECT
- [x] 5.4 Unit test: generator emits DEFAULT when provided, else legacy

## 12. Documentation & Finalization

- [x] 12.1 Update `README.md` (if exists) with new config fields and admin UI sections
- [x] 12.2 Update example config (config.yaml.example) with `external_proxies` and `default_external_proxy`
- [x] 12.3 Update AGENTS.md if necessary (build/test commands unchanged)
- [x] 12.4 Run full test suite; ensure existing tests pass
- [x] 12.5 Build binary and perform manual smoke test: proxy starts, admin UI works, create proxy, set default, add rule, test routing
