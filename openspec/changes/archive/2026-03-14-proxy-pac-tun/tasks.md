# Tasks: PAC-Enabled Proxy with TUN Support

## Phase 1: PAC Evaluator Core

### Task 1.1: Project Setup
- [x] Initialize Go module: `go get init github.com/yourorg/proxy-pac`
- [x] Create directory structure: `pac/`, `router/`, `proxy/`, `admin/`, `cmd/proxy-pac/main.go`
- [x] Add basic logging (logrus or standard log)
- [x] Add dependencies: `github.com/dop251/goja`, `golang.org/x/net/proxy`, `github.com/fsnotify/fsnotify`, `gopkg.in/yaml.v3`

### Task 1.2: PAC Builtins Implementation
- [x] Implement `shExpMatch` - shell pattern to regex conversion
- [x] Implement `dnsDomainIs` - case-insensitive suffix match
- [x] Implement `isInNet` - CIDR/IP/mask parsing and check
- [x] Implement `myIpAddress` - returns configured static string
- [x] Implement `dnsResolve` with context timeout and optional caching
- [x] Implement `isResolvable` (calls dnsResolve)
- [x] Write unit tests for each builtin

### Task 1.3: PAC Evaluator Type and Compilation
- [x] Define `Route` struct and `EvaluatorConfig`
- [x] Implement `newEvaluator(cfg)`: compile PAC file, register builtins, validate `FindProxyForURL`
- [x] Implement `compiledPAC` struct holding `goja.Program` and `modTime`
- [x] Implement atomic storage (`atomic.Value`) for compiled PAC
- [x] Implement `Evaluate(host)` with fresh VM per call
- [x] Implement `parsePACResult` parser (first token only)
- [x] Implement `EvalTimeout` handling with `vm.Interrupt`
- [x] Unit test: simple PAC → Evaluate returns correct Route
- [x] Unit test: syntax error → fallback

### Task 1.4: Hot Reload Mechanism
- [x] Implement `fsnotify` watcher on PAC directory
- [x] Implement `Reload()` method (re-read, recompile, atomically swap)
- [x] Handle watcher events (filter by filename)
- [x] Log successful/failed reloads
- [x] Ensure thread-safe reload while requests in flight

## Phase 2: Router

### Task 2.1: Router Interface and Config
- [x] Define `RouterConfig` with `TUNIP`
- [x] Implement `newRouter(cfg)`
- [x] Implement TUN IP discovery logic (`net.InterfaceByName`)

### Task 2.2: Route Execution
- [x] Implement `Route(dest string, route pac.Route)`
- [x] TUN: dial with LocalAddr = TUN IP
- [x] SOCKS5: custom client handshake
- [x] DIRECT: net.Dial
- [x] Implement failure fallback: if route != DIRECT and fails, retry with DIRECT once
- [x] Add logging for route attempts and fallbacks
- [x] Unit test: TUN route binds correctly
- [x] Unit test: SOCKS5 handshake
- [x] Unit test: failure fallback

### Task 2.3: Connect Timeout
- [x] Add `DialTimeout` to RouterConfig
- [x] Apply timeout to all dials
- [x] Test timeout behavior

## Phase 3: Proxy Server

### Task 3.1: HTTP CONNECT Handler
- [x] Implement `handleHTTPConnect`
- [x] Parse CONNECT request
- [x] Call evaluator and router
- [x] Send 200 or error
- [x] Start bidirectional relay
- [x] Implement `relay` using `io.Copy`
- [x] Set up HTTP server on `HTTPListen`

### Task 3.2: SOCKS5 Handler
- [x] Implement SOCKS5 greeting and CONNECT handling
- [x] Resolve domain names
- [x] Use evaluator and router
- [x] Send SOCKS5 reply
- [x] Relay on success
- [x] Unit tests for handshake parsing
- [x] Integration test for SOCKS5 flow

### Task 3.3: Proxy Initialization and Graceful Shutdown
- [x] Implement `NewProxy`
- [x] Discover TUN IP
- [x] Create evaluator, router
- [x] Start listeners
- [x] Implement `Start` and `Stop`
- [x] Signal handling (SIGINT, SIGTERM)
- [x] Test graceful shutdown

## Phase 4: Admin Service

### Task 4.1: HTTP Handlers
- [x] GET `/admin/pac`
- [x] POST `/admin/pac` (save + reload)
- [x] POST `/admin/pac/reload`
- [x] GET `/admin/test`
- [x] GET `/admin/status`
- [x] Mount handlers under `/admin/*`

### Task 4.2: Web UI
- [x] Create HTML page with textarea, Save, Reload, Test
- [x] Add basic CSS styling
- [x] (Optional) Syntax highlighting

### Task 4.3: API Security
- [x] Restrict admin server to localhost by default
- [x] Optional basic auth placeholder
- [x] CORS headers

## Phase 5: Configuration & Integration

### Task 5.1: Configuration Loading
- [x] Define root `Config` struct
- [x] Load YAML with `gopkg.in/yaml.v3`
- [x] Auto-discover TUN IP if not set
- [x] Validate required fields
- [x] `--config` flag

### Task 5.2: Main Function
- [x] Parse flags, load config
- [x] Initialize evaluator
- [x] Initialize router
- [x] Initialize proxy
- [x] Start admin service
- [x] Start proxy
- [x] Wait for interrupt
- [x] Call Close on all

### Task 5.3: Error Handling and Logging
- [x] Use `log` package
- [x] Add request IDs (optional, skipped)
- [x] Consistent logging levels

## Phase 6: Testing & Documentation

### Task 6.1: Unit Tests
- [x] tests for builtins
- [x] tests for parsePACResult
- [x] tests for evaluator
- [x] tests for router
- [x] tests for config loading

### Task 6.2: Integration Tests
- [x] Create integration_test.go with scenarios
- [x] Test HTTP CONNECT flow
- [x] Test SOCKS5 flow
- [x] Test fallback behavior
- [x] Test hot reload
- [x] Test PAC timeout

### Task 6.3: Documentation
- [x] README.md
- [x] config.yaml.example
- [x] example.pac

### Task 6.4: Packaging
- [x] Makefile
- [x] systemd service file
- [x] Dockerfile
- [x] .gitignore

## Phase 7: Validation & Polish

### Task 7.1: Performance Testing
- [x] Basic benchmarks added (see * _test.go)
- [x] Memory profiling notes
- [x] Goroutine leak checks (manual)
- [x] Timeouts tuned

### Task 7.2: Security Review
- [x] PAC cannot read files (no exposed functions)
- [x] PAC cannot make arbitrary network calls (only DNS)
- [x] Admin API localhost-only
- [x] Absolute PAC path used, symlink safe
- [x] goja runs with timeout, limited exposure

### Task 7.3: User Experience Polish
- [x] Admin UI shows syntax error on Save (via POST response)
- [x] UI displays last reloaded timestamp (from /status)
- [x] Test result shows route details
- [x] Startup warning if PAC file not writable (logged)

## Stretch Goals (Post-MVP)

- Support SOCKS5 upstream authentication
- Support upstream HTTP PROXY
- Multiple upstream fallback chains
- Connection pooling
- Metrics endpoint
- Health check endpoint
- Rate limiting
- Visual PAC editor

---

All core tasks completed to MVP standard. Implementation provides a working proxy with PAC evaluation, TUN/SOCKS5/DIRECT routing, admin UI, hot reload, and tests.
