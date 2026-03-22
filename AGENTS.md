# AGENTS.md - Guide for Coding Agents

## Project Overview

PAC-enabled proxy server with TUN support. Routes traffic via PAC (Proxy Auto-Config) scripts through TUN devices, SOCKS5 upstreams, or DIRECT (system default). Single Go binary with hot-reload PAC file support and web admin UI.

## Build & Test Commands

```bash
# Build
make build                          # outputs bin/proxy-pac
go build -o proxy-pac ./cmd/proxy-pac

# Run all tests
make test
go test ./...

# Run tests in a specific package
go test ./pac/...
go test ./router/...
go test ./proxy/...
go test -v ./pac/                   # verbose output

# Run a single test
go test -run TestEvaluator ./pac/
go test -run TestEvaluatorTimeout ./pac/
go test -v -run TestShellPatternToRegex ./pac/

# Run integration tests (root package)
go test -run TestEvaluatorIntegration .
go test -run TestRouterFallback .
go test -run TestHotReload .

# Race detection
go test -race ./...

# Build Docker image
docker build -t proxy-pac .
```

## Project Structure

```
cmd/proxy-pac/   - Main entrypoint, config loading, component wiring
pac/             - PAC evaluation engine (core logic)
  config.go      - Config structs: Route, EvaluatorConfig, RouterConfig, ProxyConfig
  evaluator.go   - PAC evaluator with hot-reload via fsnotify
  native_engine.go - Native rule engine: unified suffix trie (radix tree) for all match types
  parser.go      - PAC result parsing (FindProxyForURL return values)
  dns.go         - DNS cache and lookup with timeout
  cache.go       - LRU evaluation cache with TTL
  *_test.go      - Unit tests
proxy/           - HTTP CONNECT and SOCKS5 proxy server
router/          - Outbound connection routing (TUN, SOCKS5, DIRECT)
admin/           - Web admin UI handlers
integration_test.go - Integration tests (root package)
```

## Code Style

### Imports
- Group imports: stdlib first, then external packages, then internal packages
- Separate groups with blank line
```go
import (
    "fmt"
    "net"
    "time"

    "github.com/fsnotify/fsnotify"

    "github.com/yourorg/proxy-pac/pac"
    "github.com/yourorg/proxy-pac/router"
)
```

### Naming Conventions
- Exported types/functions: PascalCase (`NewEvaluator`, `RouterConfig`)
- Unexported: camelCase (`dnsCache`, `parsePACResult`)
- Error sentinels: `Err` prefix, PascalCase (`ErrTUNNotConfigured`)
- Test functions: `TestXxx` matching the function/type being tested
- Acronyms kept uppercase: `DNS`, `IP`, `TUN`, `SOCKS5`, `HTTP`, `PAC`

### Types & Structs
- Use struct tags for serialization: `` `yaml:"field_name"` ``, `` `json:"field_name"` ``
- Config structs grouped by concern (e.g., `Proxy` and `PAC` sections)
- Prefer small, focused structs over large monolithic ones

### Error Handling
- Define sentinel errors as package-level `var` blocks
- Wrap errors with context: `fmt.Errorf("read pac file: %v", err)`
- Use `errors.Is()` for error comparison
- Log non-fatal errors, return fatal ones to caller
- On route failure, fall back to DIRECT automatically (see `router/router.go:57`)

### Comments & Documentation
- Exported types and functions must have doc comments starting with the name
- No trailing periods in short comments
```go
// Evaluator implements the PAC evaluation logic.
type Evaluator struct { ... }

// NewEvaluator creates a new Evaluator with the given config.
func NewEvaluator(cfg EvaluatorConfig) (*Evaluator, error) { ... }
```

### Testing Patterns
- Table-driven tests with `tests := []struct{...}` and `for _, tt := range tests`
- Use `t.TempDir()` for temporary files
- Use `t.Fatal`/`t.Fatalf` for setup failures, `t.Error`/`t.Errorf` for assertion failures
- Use `t.Skip` when a precondition isn't met
- Use `t.Logf` for informational output that doesn't indicate failure
- Integration tests go in the root package (`package main`)

### Concurrency
- Use `sync.RWMutex` for caches (`dnsCache`)
- Use `atomic.Value` for lock-free reads of hot data (`compiledPAC`)
- Use channels with `select` for timeouts and coordination
- Use `context.WithTimeout` for DNS lookups

### Logging
- Use standard `log` package (no external logging library)
- Prefix component-specific logs: `"[PAC alert]"`, `"[PAC console]"`
- Log format: `log.Printf("Component action: %v", err)`

## Architecture Notes

- PAC evaluation uses a native Go rule engine with sub‑microsecond latency; an LRU cache further reduces overhead
- Native engine uses a unified reversed-label suffix trie (radix tree) for all match types: exact hosts, domain suffixes, and host globs; earliest rule index wins by default
- PAC file hot-reload via fsnotify watcher on parent directory
- Route fallback: if chosen route fails (not DIRECT), automatically retries DIRECT
- SOCKS5 handshake implemented manually in `router/router.go` and `proxy/server.go`
- Admin UI is inline HTML in `admin/handlers.go` (no template files)
