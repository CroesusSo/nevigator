## Why

The `admin/handlers.go` file contains significant code duplication (~12 instances of config read/unmarshal pattern, plus repeated atomic write, find-by-ID, and keepalive handling patterns). This makes the code harder to maintain, test, and modify safely.

## What Changes

- Extract config read/unmarshal into a shared helper method on `AdminHandler`
- Extract atomic config write into a shared helper method
- Extract find-proxy-by-ID into a helper method
- Extract keepalive default interval logic into the proxy config validation
- **No behavioral changes** - purely internal refactoring

## Capabilities

### New Capabilities
(None - purely internal refactor, no new capabilities)

### Modified Capabilities
(None - no requirement changes)

## Impact

- **Affected**: `admin/handlers.go` (~1600 lines → smaller, cleaner)
- **No API changes**: All HTTP endpoints behave identically
- **No behavior changes**: Same config read/write semantics
