## Context

The admin UI (`admin/handlers.go`) has a broken Save button and a broken `/test` endpoint. Users cannot save PAC changes through the web UI, and testing URL routing produces incorrect results because host parsing is broken. The fsnotify watcher and `atomic.Value` swap mechanism already exist and work correctly — we just need to wire up the missing pieces.

## Goals / Non-Goals

**Goals:**
- Users can save PAC from the admin UI with validation feedback
- Save uses atomic writes (tmp + rename) to prevent corrupt PAC on crash
- `/test` correctly extracts hostnames from URLs
- Manual reload is removed from UI (fsnotify handles it)

**Non-Goals:**
- Authentication on admin endpoints
- Version history / undo for PAC changes
- Syntax highlighting in the editor
- Real-time status updates (polling, websockets)

## Decisions

### D1: Validate PAC before writing to disk

**Choice:** Compile PAC in a throwaway goja VM before writing to the actual file.

**Rationale:** If we write first and then the watcher's reload fails, we're left with a broken PAC and no way to signal the user. By validating in the POST handler, we can return 422 with the compile error before any file is modified. The PAC on disk is never broken.

**Alternative considered:** Write first, let watcher reload, return result. Problem: no way to get the reload error back to the HTTP response since `Reload()` only logs.

### D2: Atomic file write (tmp + rename)

**Choice:** `os.WriteFile(pacPath+".tmp", content)` then `os.Rename(pacPath+".tmp", pacPath)`.

**Rationale:** If the process crashes during write, a truncated PAC file could cause every new connection to fail. Atomic rename is the standard pattern — the file either exists fully or not at all. The fsnotify watcher already handles rename events (`fsnotify.Create`).

### D3: Let fsnotify handle reload (no explicit Reload call)

**Choice:** After writing, don't call `evaluator.Reload()` — let the watcher pick it up.

**Rationale:** Simpler handler, no need for `Reload()` to return errors. The gap between write and watcher is typically <1ms. New connections get the old PAC for a brief moment, which is acceptable.

### D4: Fix host parsing with net/url.Parse

**Choice:** Try `url.Parse(input)` first, extract `.Hostname()`. If that fails (bare hostname), pass input directly.

**Rationale:** Handles all common inputs: `https://example.com/path`, `example.com`, `sub.domain.co.uk:8080`. Bare hostnames work because PAC builtins like `dnsDomainIs` expect just the hostname without scheme.

### D5: Remove `handleReload` handler and Reload button

**Choice:** Remove the POST `/reload` endpoint and the Reload button from the UI.

**Rationale:** With atomic saves and fsnotify watching, manual reload is unnecessary. Keeping it adds confusion — users might click Reload thinking it reads the editor content (it doesn't, it reads from disk). Simpler is better.

## Risks / Trade-offs

**[R1] Watcher delay:** Brief window where new connections use old PAC after save. → Mitigation: typically <1ms, acceptable for this use case. User can refresh and test immediately.

**[R2] Watcher might miss event on some filesystems:** NFS, certain Docker mounts. → Mitigation: `os.WriteFile` + `os.Rename` triggers both `Write` and `Create` events which we already handle. Worst case, user can restart the proxy.

**[R3] No validation of PAC semantics (only syntax):** goja compile catches JS syntax errors, but a PAC that returns nonsense values won't be caught. → Mitigation: `/test` endpoint lets users verify behavior before saving. Out of scope for this change.

## Migration Plan

No migration needed — adding new handler, fixing existing JavaScript. No schema changes, no data migration, no downtime required.

**Rollback:** Revert the commit. Old broken behavior is restored, no data loss.
