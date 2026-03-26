## 1. Core Configuration Changes

- [x] 1.1 Remove `TUNDevice` field from `ProxyConfig` struct in `pac/config.go`
- [x] 1.2 Remove `TUNIP` and `TUNDevice` fields from `RouterConfig` struct in `pac/config.go`
- [x] 1.3 Remove `tun_device` from `FullConfig.Proxy` struct in `admin/handlers.go`
- [x] 1.4 Remove `UseTUN` field from `GFW` struct in `cmd/proxy-pac/main.go` and `admin/handlers.go`
- [x] 1.5 Remove TUN-related fields from `ServerStatus` struct in `status/status.go`

## 2. Router Implementation

- [x] 2.1 Remove `case "TUN":` branch from `router.Router.dialWithRoute()` in `router/router.go`
- [x] 2.2 Remove `ErrTUNNotConfigured` sentinel error from `router/router.go`
- [x] 2.3 Remove `tunIP` field from `Router` struct and initialization in `router/router.go`

## 3. Proxy Server Changes

- [x] 3.1 Remove `tunDevice` and `tunIP` fields from `Server` struct in `proxy/server.go`
- [x] 3.2 Remove `SetTUNIP()` method from `Server` in `proxy/server.go`
- [x] 3.3 Remove TUN IP status checking logic from `Server.Status()` method

## 4. Main Function Updates

- [x] 4.1 Remove `discoverTUNIP()` function from `cmd/proxy-pac/main.go`
- [x] 4.2 Remove TUN device configuration from main: delete `cfg.Proxy.TUNDevice` usage and `RouterConfig.TUNIP` setting
- [x] 4.3 Remove TUN IP assignment to `localIP` in main
- [x] 4.4 Remove TUN-related log statements from main
- [x] 4.5 Add validation: if `cfg.Proxy.TUNDevice != ""`, return error "TUN device support has been removed..."

## 5. PAC Evaluator and Native Engine

- [x] 5.1 Add validation in `pac.NewNativeRuleEngine()` to reject any rule with `route == "TUN"`, return error
- [x] 5.2 Update `routeFromString()` to not handle "TUN" (can remove case, but keep function for other routes)

## 6. Admin Handler and API

- [x] 6.1 Remove `TUNDevice` field from `FullConfig.Proxy` JSON/YAML tags in `admin/handlers.go`
- [x] 6.2 Remove TUN device from `/api/config` GET response in `handleGetConfig`
- [x] 6.3 Remove TUN device handling from `/api/config` PUT in `handlePutConfig`
- [x] 6.4 Remove `UseTUN` from `FullConfig.GFW` and related logic in GFW config handlers
- [x] 6.5 Update `regeneratePAC()` call if needed (use_tun flag removal)

## 7. Admin UI - HTML

- [x] 7.1 Remove "TUN Device" row from Summary tab in `admin/static/index.html` (line ~30)
- [x] 7.2 Remove "TUN" option from route dropdown in Rules tab (line ~77)
- [x] 7.3 Remove "TUN device" input field from Settings tab (lines ~333-336)
- [x] 7.4 Remove TUN-related tooltip text from header

## 8. Admin UI - JavaScript

- [x] 8.1 Remove `TUN` entry from `tooltips.route` object in `admin/static/script.js`
- [x] 8.2 Remove TUN handling from `loadStatus()` (tunDevice, tunIP, tunUp display)
- [x] 8.3 Remove TUN route from `routeOptions` arrays in rule editor (lines ~198, 918)
- [x] 8.4 Remove TUN CSS class from route badge rendering

## 9. Documentation and Examples

- [x] 9.1 Remove TUN references from `README.md` (update examples, feature list, configuration)
- [x] 9.2 Remove `tun_device` line from `config.yaml.example`
- [x] 9.3 Update example PAC snippets to not include TUN routes
- [x] 9.4 Remove mention of TUN in Deepest Match section

## 10. Test Suite Cleanup

- [x] 10.1 Rewrite `TestRouterFallback` in `integration_test.go` to use SOCKS5 instead of TUN
- [x] 10.2 Remove TUN route tests from `pac/evaluator_test.go` (agent)
- [x] 10.3 Remove TUN route tests from `pac/evaluator_native_test.go` (agent)
- [x] 10.4 Remove TUN route tests from `pac/native_engine_test.go` (agent)
- [x] 10.5 Remove TUN from `pac/builtins_test.go` (agent)
- [x] 10.6 Remove TUN benchmarks from `pac/bench_test.go` (agent)
- [x] 10.7 Remove TUN route references from `gfw` package tests (agent)
- [x] 10.8 Manual fixes: status/status_test.go, admin/handlers_test.go, main_test.go, native_engine_test.go adjustments
- [x] 10.9 Verify all tests compile and pass after TUN removal

## 11. Verification

- [x] 11.1 Run full test suite: `go test ./...`
- [x] 11.2 Build binary: `go build -o proxy-pac ./cmd/proxy-pac`
- [x] 11.3 Test startup with config containing `tun_device` → should error
- [x] 11.4 Test PAC load with TUN route → should error
- [x] 11.5 Test that SOCKS5, EXTERNAL, DEFAULT, DIRECT routes still work
- [x] 11.6 Verify Admin UI loads correctly
- [x] 11.7 Verify `/api/config` response has no TUN fields

## 12. Capability Spec Update (Rename)

- [x] 12.1 Rename `openspec/specs/pac-proxy-tun.md` to `pac-routing.md` and update content
- [x] 12.2 Update any references to old spec name in other specs/docs (if any)
