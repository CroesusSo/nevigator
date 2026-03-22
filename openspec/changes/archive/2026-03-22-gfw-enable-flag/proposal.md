# GFW Enable/Disable Toggle

## Problem

The GFW (Great Firewall list) integration is enabled as long as a `source_url` is configured. There is no separate control to temporarily disable fetching and inclusion without removing the URL. Users must edit the config file to comment out or remove the `source_url` to disable GFW, which is inconvenient and error-prone.

Additionally, the admin UI lacks a toggle to enable/disable GFW, requiring file edits for what should be a simple on/off switch.

## Solution

Add a boolean `disabled` field to GFW configuration (YAML: `gfw.disabled`, default `false`). When `true`:

- Startup fetch is skipped
- Periodic fetch loop does nothing
- Manual fetch via UI returns an error (no-op)
- PAC generation excludes GFW rules (only user rules are included)
- The feature can be re-enabled via config file or admin UI toggle

This is backward compatible: existing configs without the field behave as `disabled: false` (i.e., GFW enabled if `source_url` is present).

## Goals

- Add `disabled bool` to GFW config (YAML, JSON, default `false`)
- Fetcher respects flag via atomic state (gate all fetch operations)
- Admin API exposes and persists the flag
- Admin UI provides a checkbox to toggle GFW on/off
- PAC generation conditionally includes GFW rules based on flag
- Backward compatible (no changes required to existing configs)

## Non-Goals

- Do NOT change GFW list format or parsing
- Do NOT modify fetching logic beyond gating
- Do NOT change rule merging order when enabled
- Do NOT support disabling GFW via `source_url` absence only (the flag is the new canonical control)

## Scope

### In Scope
- Config structs (`cmd/proxy-pac/main.go`, `admin/handlers.go`)
- `Fetcher` atomic disabled flag and gate checks
- Main startup and periodic fetch gating
- Admin API endpoints (`GET /api/gfw/config`, `PUT /api/gfw/config`)
- Admin UI: checkbox in GFW tab, save handler, status display
- `gfw.RegeneratePAC` signature change to accept `enabled bool`
- All callers of `RegeneratePAC` updated
- Documentation updates (README, AGENTS.md)

### Out of Scope
- Changing GFW rule format or generator
- Changing TUN or routing behavior
- Per‑category enable (e.g., only disable fetching but still include old rules) — GFW disabled means neither fetch nor include

## Dependencies

- Builds on existing config and admin infrastructure
- No new external dependencies

## Risks & Mitigations

| Risk | Mitigation |
|------|------------|
| Default behavior flips for existing users if we use `enabled` (default false) | Use `disabled` field (default false) → GFW remains enabled by default |
| Race between disabled check and fetch start | Atomic checks; disabling only prevents next cycle, not in‑flight fetch (acceptable) |
| Admin UI not reflecting file edits | UI controls via API; file edits require UI restart to see change (acceptable) |
| PAC regeneration concurrent with fetcher writes | Current atomic rename is safe; no new risk |
| Forgetting to pass `enabled` to some `RegeneratePAC` call | Centralize decision in Handler; review call sites |

## Open Questions

- Should "Fetch Now" button be disabled when GFW is disabled? Likely yes.
- Should status line show "Disabled" clearly? Yes.
- Should we allow `disabled` key even when `source_url` missing? Harmless; treat as no‑op.

## Success Criteria

- Config file can contain `disabled: true` to turn off GFW.
- Admin UI toggle persists to config and updates PAC immediately.
- Startup fetch, periodic loop, and manual fetch respect the flag.
- PAC file includes GFW rules only when not disabled.
- Backward compatible: existing installations work unchanged.
