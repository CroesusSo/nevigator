# Add Static Analysis and Linting

## Problem

The project has no static analysis or linting. A recent code review found dead code (empty if-blocks, redundant guards) and a low-severity injection vulnerability that a linter would catch automatically. These issues accumulate silently because `go build` and `go test` don't flag them.

Current checks: `go build` (compilation), `go test` (runtime behavior). Missing: dead code detection, unused variables, shadowed variables, potential nil dereferences, security patterns.

## Solution

Add `golangci-lint` as the project's linter with a focused set of checks:

- **staticcheck** — dead code, unreachable branches, incorrect nil checks
- **unused** — unused variables, functions, parameters
- **govet** — suspicious constructs (printf format mismatches, struct tags)
- **ineffassign** — assigned but never used values

Add a `make lint` target and fix all existing violations. Optionally add a CI step to fail on lint errors.

## Scope

- Add `.golangci.yml` config file
- Add `make lint` and `make lint-fix` targets to Makefile
- Fix all existing lint violations in the codebase
- Deduplicate `regeneratePAC` (found during review — shared logic between `main.go` and `admin/handlers.go`)
- Fix the route string injection in PAC generator (use `%q` instead of `%s`)

## Non-goals

- Adding golangci-lint to CI (separate concern, out of scope)
- Adding additional linters beyond the initial four (can expand later)
- Fixing style opinions that don't correspond to bugs or real issues
