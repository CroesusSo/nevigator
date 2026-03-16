# Design: Static Analysis and Linting

## Linter Choice

**golangci-lint** — the standard Go meta-linter. Runs multiple analyzers in one pass, caches results, supports config via `.golangci.yml`.

## Enabled Analyzers

| Analyzer | Purpose | What it catches |
|----------|---------|-----------------|
| `staticcheck` | Comprehensive static analysis | Dead code, unreachable branches, incorrect nil usage, deprecated APIs |
| `unused` | Unused code detection | Unused variables, functions, struct fields, parameters |
| `govet` | Go vet integration | Printf format mismatches, struct tag issues, unreachable code |
| `ineffassign` | Ineffectual assignments | Variables assigned but never read afterward |

### Why these four

These four cover the review findings and similar issues without false-positive noise. Other linters like `errcheck` (unhandled errors) or `gosec` (security) can be added later but require more investment to tune.

## Config File

`.golangci.yml` in the project root:

```yaml
run:
  timeout: 5m
  modules-download-mode: readonly

linters:
  disable-all: true
  enable:
    - staticcheck
    - unused
    - govet
    - ineffassign

issues:
  exclude-dirs:
    - bin
  max-issues-per-linter: 0
  max-same-issues: 0
```

## Makefile Targets

```makefile
lint:
	golangci-lint run ./...

lint-fix:
	golangci-lint run --fix ./...
```

## Fix: Deduplicate `regeneratePAC`

Move the function from `cmd/proxy-pac/main.go` to `gfw/pacgen.go` as an exported function:

```go
// RegeneratePAC reads user and GFW rules, generates PAC, and writes it atomically.
func RegeneratePAC(userPath, gfwPath, pacPath string) error
```

Both `cmd/proxy-pac/main.go` and `admin/handlers.go` call `gfw.RegeneratePAC()`.

## Fix: Route String Injection

In `gfw/generator.go:76`, change route interpolation from `%s` to `%q`:

```go
// Before
sb.WriteString(fmt.Sprintf("    return \"%s\";\n", route))
// After
sb.WriteString(fmt.Sprintf("    return %q;\n", route))
```

`%q` produces a Go-quoted string literal, which escapes `"` and other special characters.

## Fix: Dead Code

- `gfw/store.go:41-44` — remove empty if-block, keep `rule.Enabled = true`
- `gfw/parser.go:86-88` — keep the leading-dot guard (defensive, not dead)
