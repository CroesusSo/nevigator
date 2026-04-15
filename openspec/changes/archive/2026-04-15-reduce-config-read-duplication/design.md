## Context

The `admin/handlers.go` file contains ~1600 lines with significant code duplication. The main patterns that need extraction are:

1. **Config read/unmarshal** - 12 identical blocks reading and parsing YAML config
2. **Atomic config write** - 3 blocks performing tmp-write-rename pattern
3. **Find proxy by ID** - 5 loops searching for proxy by ID
4. **Keepalive default interval** - 2 blocks setting default 30s interval

## Goals / Non-Goals

**Goals:**
- Reduce code duplication in admin/handlers.go
- Extract reusable helper methods on AdminHandler
- Maintain identical behavior before and after refactor

**Non-Goals:**
- No new functionality or API changes
- No refactoring of other packages
- No performance improvements (currently adequate)

## Decisions

### 1. Config read helper

**Option A**: Extract `readConfig() (*FullConfig, error)` method on AdminHandler
- **Chosen**: Yes - reduces 12 duplicate blocks to single method call
- Centralizes error handling

### 2. Atomic write helper

**Option A**: Extract `writeConfig(cfg *FullConfig) error` method on AdminHandler
- **Chosen**: Yes - encapsulates tmp-write-rename-error-cleanup pattern
- Handles temp file cleanup on failure

### 3. Find proxy by ID helper

**Option A**: Extract `findProxyByID(id string) (*ExternalProxy, error)` method
- **Chosen**: Yes - reduces 5 duplicate loops
- Returns error if not found (consistent with callers)

### 4. Keepalive validation

**Option A**: Move default interval logic into proxy creation/update validation
- **Chosen**: Yes - single location for default value enforcement
- Cleaner than repeating in multiple handlers

## Risks / Trade-offs

- **Risk**: Breaking existing error handling behavior
  - **Mitigation**: Copy exact error handling from existing code into helpers
  - Write tests to verify behavior matches

- **Risk**: Helper method bloat
  - **Mitigation**: Keep methods focused; each does one thing

## Migration Plan

1. Add helper methods to AdminHandler (no behavior change)
2. Update each call site to use helpers
3. Remove duplicated code blocks
4. Run tests to verify behavior unchanged
5. Refactor complete - same deploy process

## Open Questions

None - implementation is straightforward extraction of existing patterns.
