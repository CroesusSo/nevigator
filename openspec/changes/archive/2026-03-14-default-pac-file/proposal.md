## Why

The proxy server crashes at startup if the PAC file doesn't exist (`stat pac file: no such file or directory`). This forces users to manually create a PAC file before the first run, adding friction to setup and causing confusing failure on fresh installs, Docker containers, and new environments.

## What Changes

- **Auto-create default PAC file**: When the PAC file path doesn't exist at startup, create a minimal default PAC that routes everything DIRECT
- **Log the action**: Log when a default PAC file is created so the operator knows it happened
- **Preserve existing behavior**: If the PAC file exists, behavior is unchanged

## Capabilities

### New Capabilities

- `default-pac-creation`: Auto-create a default PAC file at startup when the configured path doesn't exist

### Modified Capabilities

(none — no existing spec-level requirements change)

## Impact

- **Affected code**: `pac/evaluator.go` — `NewEvaluator()` and `reload()` methods
- **No API changes**: Admin UI, config format, and PAC script format unchanged
- **No new dependencies**: Uses existing `os` package
