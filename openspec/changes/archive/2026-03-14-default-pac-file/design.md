## Context

The proxy server requires a PAC file to start. In `pac/evaluator.go:91-93`, `NewEvaluator` calls `reload()`, which does `os.Stat(e.cfg.PACPath)` and returns an error if the file doesn't exist. This causes `main.go:120` to `log.Fatalf`, crashing the server before it starts. This is a poor first-run experience — the server should be able to start with sensible defaults.

## Goals / Non-Goals

**Goals:**
- Server starts successfully even if PAC file doesn't exist
- Default PAC routes everything DIRECT (safe, predictable)
- Operator is informed via log message that a default was created
- Existing behavior unchanged when PAC file exists

**Non-Goals:**
- Generating PAC files from templates or configuration
- Auto-generating PAC files based on network discovery
- Changing the PAC file format or admin UI

## Decisions

### Decision: Create file in `NewEvaluator`, not in `main.go`

**Rationale:** The evaluator owns the PAC file lifecycle (load, watch, reload). Handling creation in `NewEvaluator` keeps the logic colocated with the existing file handling code. `main.go` doesn't need to know about PAC file internals.

**Alternatives considered:**
- Create in `main.go` before calling `NewEvaluator` — adds PAC file concerns to main wiring logic
- Don't create a file, use in-memory default — PAC file won't appear on disk for admin UI editing

### Decision: Use fallback on `reload()` failure, not pre-check

**Rationale:** Rather than checking if the file exists and creating it before `reload()`, we modify `reload()` to handle the not-exist case with a fallback. This is more robust — if the file is deleted during operation, the next reload still works.

**Alternatives considered:**
- Pre-check in `NewEvaluator` with `os.Stat` + create — two code paths, `reload()` still fails if file deleted later
- Check both in `NewEvaluator` and `reload()` — redundant

### Decision: Default PAC is minimal `DIRECT` only

**Rationale:** The simplest safe default. No routing surprises. Users configure routing via admin UI or by editing the file.

**Alternatives considered:**
- Include example TUN/SOCKS5 rules commented out — may confuse users into thinking something is configured
- Empty file — would cause runtime errors (no `FindProxyForURL` function)

## Risks / Trade-offs

- **[Risk] Permission denied creating file** → Mitigation: Log clear error, fail with actionable message (check directory permissions)
- **[Risk] Directory doesn't exist** → Mitigation: `os.WriteFile` fails with clear error; could `os.MkdirAll` parent, but that's too magical — let the operator fix the path
- **[Trade-off] Auto-created file may surprise users** → Mitigation: Log message clearly states the file was created and where

## Migration Plan

No migration needed. This is additive — existing deployments with PAC files continue working identically.

## Open Questions

- Should the default PAC content be a constant in the evaluator package or in `main.go`? Leaning toward evaluator package since it owns PAC semantics.
