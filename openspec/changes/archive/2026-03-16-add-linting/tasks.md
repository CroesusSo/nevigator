# Tasks: Add Static Analysis and Linting

## Phase 1: Linter Setup

- [x] Create `.golangci.yml` with staticcheck, unused, govet, ineffassign
- [x] Add `lint` and `lint-fix` targets to Makefile
- [x] Run `make lint` and capture baseline violations

## Phase 2: Fix Dead Code

- [x] Remove empty if-block in `gfw/store.go:41-44` (dead code in Create)
- [x] Run linter to verify dead code issue resolved

## Phase 3: Fix Route Injection

- [x] Change `%s` to `%q` for route in `gfw/generator.go:76` (writeRuleCondition)
- [x] Add test: route with special characters produces valid PAC JS
- [x] Run linter to verify

## Phase 4: Deduplicate `regeneratePAC`

- [x] Create `gfw/pacgen.go` with exported `RegeneratePAC(userPath, gfwPath, pacPath string) error`
- [x] Move logic from `cmd/proxy-pac/main.go:339` to `gfw.RegeneratePAC`
- [x] Replace `regeneratePAC` in `cmd/proxy-pac/main.go` with call to `gfw.RegeneratePAC`
- [x] Replace `(h *Handler) regeneratePAC()` in `admin/handlers.go:721` with call to `gfw.RegeneratePAC`
- [x] Verify existing tests pass after refactor

## Phase 5: Clean Up Remaining Violations

- [x] Run `make lint` and fix any remaining violations
- [x] Run full test suite: `go test ./...`
- [x] Run race detection: `go test -race ./...`
