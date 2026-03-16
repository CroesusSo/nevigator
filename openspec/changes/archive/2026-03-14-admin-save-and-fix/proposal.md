## Why

The admin UI's Save button is broken in three ways: it reloads before saving, POSTs to a nonexistent route (`/admin/pac`), and no handler exists to write the PAC file. Users cannot edit PAC through the web UI. Additionally, the `/test` endpoint has a broken host parser that passes raw input to PAC evaluation instead of extracting the hostname.

## What Changes

- **New handler `handleSavePAC`**: Accepts POST to `/pac` with PAC content, validates before writing (compile check via fresh goja VM), uses atomic write (tmp + rename), returns 422 with error details on invalid PAC
- **Fix `/test` host parsing**: Use `net/url.Parse` to extract hostname from full URLs, bare hostnames, or host:port strings
- **Update admin UI**: Fix Save button flow (POST to `/pac`), remove manual Reload button (fsnotify watcher handles it), surface validation errors to user
- **Remove dead code**: Remove the broken `json.Marshal` host parsing attempt and the `handleReload` handler (no longer needed from UI)

## Capabilities

### New Capabilities
- `pac-save`: Admin endpoint for validated, atomic PAC file writes. Validates PAC JavaScript before writing to disk. Uses tmp file + atomic rename to prevent truncated PAC on crash.

### Modified Capabilities
- `pac-test`: Fix host extraction from URL input. Currently passes raw query parameter; will properly parse URLs, bare hostnames, and host:port strings.

## Impact

- `admin/handlers.go`: Add `handleSavePAC`, fix `handleTest`, update `handleUI` JavaScript, register new route
- `pac/evaluator.go`: No changes needed (watcher and Reload remain as-is)
- No new dependencies
- No breaking changes to existing endpoints (only adding POST `/pac`)
