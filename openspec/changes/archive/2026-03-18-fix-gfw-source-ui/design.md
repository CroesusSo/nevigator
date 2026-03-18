## Context

The admin UI's Settings tab includes an input field for the GFW list source URL (`#gfw-url`). The backend provides `GET /api/gfw/config` to retrieve current GFW configuration, including `sourceURL`. However, the UI's `loadConfig()` function only fetches `/api/config` (proxy settings) and never reads the GFW source URL, leaving the input blank (only placeholder visible).

## Goals / Non-Goals

**Goals:**
- On page load, the GFW source URL field SHALL be pre‑filled with the current value from the configuration.
- The change SHOULD be minimal and not alter existing backend APIs.

**Non-Goals:**
- Changing the GFW fetcher behavior or configuration storage.
- Merging GFW settings into the `/api/config` endpoint.
- Adding client‑side validation beyond basic UI feedback.

## Decisions

**Decision: Extend `loadConfig()` with an extra fetch**

We will modify the JavaScript `loadConfig()` function to make a second `fetch('/api/gfw/config')` call after (or alongside) the existing `/api/config` call. When the response arrives, we will set `document.getElementById('gfw-url').value = data.sourceURL || ''`.

- **Why this approach?** It’s the smallest, safest change:
  - Backend already provides `/api/gfw/config`; no server code change needed.
  - Keeps separation of concerns: `/api/config` for proxy settings, `/api/gfw/config` for GFW settings.
  - No risk to existing config loading behavior; we simply add a new independent request.

- **Alternatives considered:**
  - *Merge GFW fields into `/api/config`*: Would require modifying server responses and possibly the config JSON format. More invasive, breaks backward compatibility for clients expecting the old `/api/config` shape. Rejected.
  - *Separate `loadGFWConfig()` function*: Could work, but would require ensuring it’s called from the right place. Over‑engineering for a single field. Simpler to extend `loadConfig()` directly.
  - *Store GFW source in runtime and embed in HTML on initial render*: Would require server‑side template changes. The current approach is pure client‑side, faster to implement and deploy.

## Risks / Trade-offs

- **Risk:** The additional network request could fail (e.g., if GFW fetcher is not configured). Mitigation: Use `.catch()` to log error and leave the field untouched; placeholder remains OK. The GFW source may be empty anyway.
- **Trade‑off:** Slight increase in initial page load HTTP traffic (~1 extra GET). Negligible overhead.

## Migration Plan

1. Update the embedded JavaScript in `admin/handlers.go` (`handleUI`) to include the additional fetch in `loadConfig()`.
2. No config migration needed; existing installations automatically benefit.
3. Release note: “Admin UI now displays current GFW source URL in Settings.”

## Open Questions

- Should the GFW configuration block also display `local_rules_path` and `user_rules_path`? That could be a future enhancement but is out of scope for this minimal fix.
