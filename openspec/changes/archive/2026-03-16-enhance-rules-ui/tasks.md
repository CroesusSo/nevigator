## 1. Table structure and state preparation

 - [x] 1.1 Add CSS classes for route badges: `.badge-tun`, `.badge-direct`, `.badge-socks5`
 - [x] 1.2 In `renderRules`, add data attributes to each row: `data-id`, `data-route`, etc.
 - [x] 1.3 In `renderRules`, render match type as badge with tooltip `title`
 - [x] 1.4 In `renderRules`, render route as badge with `badge-<route>` class and tooltip `title`
 - [x] 1.5 In `renderRules`, wrap pattern value in `<code>` with `class="pattern"` and `onclick="startEdit(...)"` (only for user rules)
 - [x] 1.6 Add inline edit controls (initially hidden) structure: `<span class="edit-controls" style="display:none">[✓][✕]</span>`

## 2. Inline edit mode JavaScript

 - [x] 2.1 Add `editRowId` global variable to track which row is in edit mode
 - [x] 2.2 Implement `startEdit(id)` function that:
 - [x] 2.3 Implement `saveEdit(id)` function that:
 - [x] 2.4 Implement `cancelEdit()` function that:
 - [x] 2.5 Wire keyboard: listen for `Enter` when pattern input focused → call `saveEdit(editRowId)`; `Escape` → `cancelEdit()`
 - [x] 2.6 Modify `renderRules` to render edit buttons only for user rules (`r.source === 'user'`)
 - [x] 2.7 Ensure GFW rules never show edit affordances (no onclick)

## 3. GFW separation and toggle

 - [x] 3.1 Add a new section below User Rules with checkbox: `<input type="checkbox" id="show-gfw"> Show GFW rules (<span id="gfw-count">0</span> rules)`
 - [x] 3.2 Add a container `<div id="gfw-section" style="display:none">` with grey background CSS
 - [x] 3.3 Inside container, add a table with headings: Match Type, Pattern
 - [x] 3.4 Implement `loadGFWRules()` that fetches `/api/rules` (or separate endpoint if filtered), filters GFW rules (`source==='gfw'`), and renders them into the GFW table body
 - [x] 3.5 When checkbox changes, toggle `#gfw-section` visibility; if showing, call `loadGFWRules()`
 - [x] 3.6 After `loadRules()`, also update `#gfw-count` with count of GFW rules
 - [x] 3.7 Apply CSS grey background to `#gfw-section` table rows

## 4. Client-side validation

 - [x] 4.1 In `startEdit()`, set initial validation state on the pattern input
 - [x] 4.2 Add `validatePattern(match, value)` function:
 - [x] 4.3 On input `keyup`/`change` in pattern edit field, re-validate and update UI:
 - [x] 4.4 In `saveEdit()`, check validity before sending; if invalid, return early

## 5. Tooltips

 - [x] 5.1 Add tooltip text definitions: map match types to descriptions
 - [x] 5.2 In `renderRules` for user table, set `title` attribute on match badge: `title = tooltips.match[r.match]`
 - [x] 5.3 In `renderRules` for GFW table, set `title` similarly on match cells
 - [x] 5.4 In `renderRules`, set `title` attribute on route badges: descriptive text based on route value

## 6. Route badges CSS

 - [x] 6.1 Add new CSS classes:
```css
.badge-tun { background: #e8f5e9; color: #2e7d32; }
.badge-direct { background: #f5f5f5; color: #666; }
.badge-socks5 { background: #fff3e0; color: #e65100; }
```
 - [x] 6.2 In `renderRules`, compute CSS class for route: `badge-` + lower-case route token (TUN → tun, DIRECT → direct, SOCKS5 → socks5)

## 7. Lazy rendering for GFW table

 - [x] 7.1 Add state variables: `gfwRowHeight`, `gfwVisibleStart`, `gfwVisibleEnd`
 - [x] 7.2 When GFW section becomes visible (checkbox checked), measure first row height: `gfwRowHeight = document.querySelector('#gfw-section tbody tr').offsetHeight`
 - [x] 7.3 Implement `renderGFWSlice()` that:
 - [x] 7.4 Attach scroll event listener to GFW table container that triggers `renderGFWSlice()`
 - [x] 7.5 When `loadGFWRules()` fetches data, store full array in memory and call `renderGFWSlice()` if visible

## 8. Refinements and polish

 - [x] 8.1 Add evaluation order hint text above User Rules: "Rules are evaluated top-to-bottom. User rules always take precedence over GFW rules."
 - [x] 8.2 Ensure the "Add Rule" form remains visible (not inside any toggleable section)
 - [x] 8.3 Verify that GFW table does not include On/Off, Actions columns
 - [x] 8.4 Add `?` icons next to Match Type and Route column headings with brief tooltips explaining each type
 - [x] 8.5 Ensure table headings (User Rules, GFW Rules) show count of rules (e.g., "User Rules (3 user rules)", "GFW Rules (1,847 rules, read-only)")
 - [x] 8.6 Make sure `loadGFWRules()` runs once and caching avoids refetch on each show

## 9. Tests and verification

 - [x] 9.1 Add unit test for validation function in admin handlers test
 - [x] 9.2 Add test that GFW rules are not rendered if checkbox unchecked
 - [x] 9.3 Add test that inline edit works for user rules and sends correct PUT payload
 - [x] 9.4 Add test that GFW rules never show edit affordances
 - [x] 9.5 Run `go test ./...`
 - [x] 9.6 Run `go vet ./...`
 - [x] 9.7 Manual smoke test: create rule, inline edit, cancel, validation, toggle GFW, lazy scroll

## 10. Optional nice-to-haves (if time)
- [x] 10.1 Persist "Show GFW" checkbox state in localStorage
- [x] 10.2 Add "Copy GFW to user" button as future enhancement

## 11. GFW fetch through TUN

 - [x] 11.1 Add `use_tun` field to GFW config struct (yaml:"use_tun")
 - [x] 11.2 In main.go, after discovering TUN IP, if `cfg.GFW.UseTUN` && tunIP != nil, set custom HTTP client with LocalAddr=tunIP
 - [x] 11.3 Update defaultConfigYAML to include `use_tun: true`
  - [x] 11.4 Test: ensure GFW fetch uses TUN interface (verify with network capture or logs)

## 12. Change default SOCKS5 port to 11080

 - [x] 12.1 Update defaultConfigYAML socks5_listen to ":11080"
 - [x] 12.2 Update hardcoded defaults in main.go: SOCKS5Listen default
