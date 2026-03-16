## Context

The current Rules UI displays a single table mixing user rules and GFW rules. User rules have Toggle and Delete actions but no Edit capability — to modify a rule, one must delete it and re-add. GFW rules are always visible (if configured), take up space, and show an On/Off toggle that is meaningless since they are read-only. Large GFW lists can contain thousands of rules, rendering all rows upfront can slow down the browser.

## Goals / Non-Goals

**Goals:**
- Allow inline editing of user rule fields (match, value, route) directly in the table
- Separate GFW rules into a distinct, hidden-by-default table with a show/hide toggle
- Apply client-side validation (e.g., host_glob/url_glob cannot be empty)
- Add tooltips explaining match types and route meanings
- Implement lazy rendering for the GFW table to maintain performance with large rule sets
- Support keyboard shortcuts: Enter to save, Escape to cancel
- Display route values as styled badges for quick scanning

**Non-Goals:**
- Persist the "Show GFW rules" checkbox state across page loads (no localStorage)
- Provide a "copy GFW to user" feature
- Make GFW rules editable or toggleable
- Change server-side APIs (reuse existing PUT /api/rules/{id})
- Add server-side validation beyond what exists (client-side is first line, server remains source of truth)

## Decisions

### 1. Inline editing via row-level edit mode

**Decision**: Clicking any editable cell in a user rule row activates "edit mode" for that row. All three fields (match, value, route) become form controls simultaneously. Save (✓) and cancel (✕) buttons appear adjacent to the value field. Enter triggers save; Escape triggers cancel.

**Rationale**: Editing all three fields together ensures atomic updates (one PUT request). Spreading edit activation across cells makes the UI feel responsive. Keyboard shortcuts improve ergonomics.

**Alternatives**:
- Modal dialog: More disruptive, slower, but clearer for many fields. Rejected because our fields are few and inline is faster.
- Inline per-cell editing: Would require three separate activations; inconsistent experience.

### 2. Separate GFW table + show/hide toggle

**Decision**: GFW rules are rendered in a separate `<div>` section below the user rules table, with a checkbox "Show GFW rules (N rules)". The section is hidden by default. When shown, the table displays only Match Type and Pattern columns (no On/Off, Actions). The section uses a grey background (`#f5f5f5`) to visually distinguish it.

**Rationale**: Reduces visual clutter, emphasizes user-controlled rules, respects that GFW is reference data. Hiding by default improves initial page performance.

**Alternatives**:
- Always show but collapsed: Requires extra click; similar effect. Checkbox is clear.
- Pagination for GFW: Overkill; lazy rendering suffices.

### 3. Client-side validation before PUT

**Decision**: Before sending the update request, check that:
- If match type is `host_glob` or `url_glob`, the value string is non-empty (trim).
- If empty, highlight the input with a red border and prevent save.

Server-side validation remains source of truth; this is UX polish.

**Rationale**: Prevents round-trip errors for common mistake. Simple to implement.

### 4. Lazy rendering for GFW table

**Decision**: When the GFW section becomes visible, measure the container height and row height (fixed). Render only rows that intersect the viewport plus a small buffer (e.g., ±5 rows). As user scrolls, update the slice. This is not full virtualization (no dynamic height), but basic windowing.

**Rationale**: Handles 5k+ rules without DOM bloat. Implementation is straightforward: `start = Math.floor(scrollTop / rowHeight) - buffer`, `end = start + visibleCount + 2*buffer`.

**Alternatives**:
- Use a virtual scroll library: Adds dependency; we can keep it simple.
- Server pagination: Unnecessary; entire GFW list is already in memory.

### 5. Tooltips via title attribute

**Decision**: Add `title` attributes to match type badges and route badges. Browser-native tooltip is sufficient. For richer help, consider small "?" icons next to table headings.

**Rationale**: Zero overhead. Clear enough.

### 6. Route badges

**Decision**: Reuse existing badge CSS classes but add specific ones:
- `badge-tun` (greenish)
- `badge-direct` (neutral)
- `badge-socks5` (orange)

**Rationale**: Consistent with existing UI patterns (badge-user, badge-gfw already exist).

## Risks / Trade-offs

- [Race condition] Two edits on same row from different tabs: Last write wins. Conflict resolution not needed for single-user admin UI.
- [Validation bypass] Client-side validation can be bypassed; server must still reject. Current `RuleManager.Update` does not validate match type or value content — only stores. This change does not require server-side format validation (out of scope). Client hints only.
- [Lazy render complexity] Adds scroll event listener; must clean up on hide to avoid memory leaks. Keep it simple to minimize risk.
- [Accessibility] Inline editing with keyboard support is good, but need to ensure focus management. Will add `tabindex` and ARIA labels.

## Migration Plan

Frontend-only change. Deploy new `admin/handlers.go` HTML/JS/CSS. No data migration. API unchanged. Zero downtime. Rollback: revert to previous `handlers.go`.

## Open Questions

- Should validation also prevent a route value like "SOCKS5" without address? Currently not prevented in Add form either. Should we enforce route format consistency?
- Tooltip text exact wording? e.g., for `host_glob`: "Matches hostnames using * and ? wildcards" or longer?
- Should the Edit/Cancel save buttons be icons (✓/✕) or text ("Save"/"Cancel")? Icons are concise but less discoverable. We could have both: icons with tooltips.
- Row height: Fixed? Assume 40px? Need to measure actual computed row height at runtime (getBoundingClientRect) to avoid hardcoding.
