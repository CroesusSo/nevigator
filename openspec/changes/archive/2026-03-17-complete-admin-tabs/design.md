## Context

The admin UI (`admin/handlers.go`) defines a tabbed interface with 7 navigation tabs: Summary, Connections, Rules, GFW List, Test URL, Proxies, and Settings. While the Proxies and Settings tabs are present in the navigation and their JavaScript event handlers (`loadProxies()`, `loadConfig()`, etc.) are fully implemented, the corresponding HTML panel containers (`<div id="panel-proxies">` and `<div id="panel-settings">`) are missing from the template. As a result, clicking these tabs shows blank content.

The backend APIs (`/api/proxies` for CRUD and testing, `/api/config` for settings) are already implemented and used by the JavaScript. The only missing piece is the static HTML structure to hold these elements.

## Goals / Non-Goals

**Goals:**
- Add the missing HTML panel content for Proxies and Settings tabs
- Ensure the panels contain the necessary form controls, tables, and buttons that match the existing JavaScript expectations
- Make the admin UI fully functional for managing external proxies and system configuration

**Non-Goals:**
- Changing or adding JavaScript functionality (already complete)
- Modifying backend APIs (already complete)
- Adding new configuration options or validation rules
- Redesigning the UI layout or styling

## Decisions

**Decision 1: Direct HTML injection into existing template**

*Approach*: Edit `admin/handlers.go` and insert the two panel `<div>` elements directly into the hardcoded HTML string.

*Rationale*: The UI is already a single static HTML blob embedded in Go code. Adding panels is simply a matter of inserting the appropriate HTML. No templating or external assets are used, so this is the simplest and most consistent approach.

*Alternatives considered*:
- Extract HTML into a separate file and serve statically: would require refactoring and is out of scope for this change.
- Use a client-side framework: not applicable.

**Decision 2: Panel structure based on existing JavaScript expectations**

*Approach*: The JS functions (`loadProxies`, `renderProxies`, `testNewProxy`, `saveDefaultProxy`, etc.) expect specific DOM IDs and elements. We'll build the panels to match these expectations exactly, using similar patterns as the existing panels (Summary, Rules, etc.).

*Rationale*: The JS is already written and tested (at least partially). Matching its expectations ensures the UI works without further changes.

## Risks / Trade-offs

**Risk**: The JavaScript might have bugs or missing features that will be exposed once the panels appear.

*Mitigation*: The existing admin UI is already in use for other tabs; the same patterns are applied here. Any JS bugs should be caught in testing and can be addressed in follow-up changes.

**Risk**: Hardcoded HTML in Go is difficult to maintain or modify.

*Mitigation*: This is a known technical debt, but changing it would be a separate effort. This change does not worsen it; it simply follows the existing pattern.

## Migration Plan

1. Deploy new binary with updated `admin/handlers.go`
2. No configuration changes required
3. No database migrations
4. Users can immediately access Proxies and Settings tabs after restart

## Open Questions

None. The implementation is straightforward based on existing JS and patterns.
