## Why

The current Rules UI requires users to delete and re-add a rule to change its pattern, match type, or route. This is tedious for frequent edits. Additionally, GFW rules are mixed with user rules in a single table, making it hard to focus on user-managed rules and creating confusion about what can be edited. Large GFW lists (thousands of rules) also impact rendering performance when always shown.

## What Changes

- **Inline editing** for user rule fields: match type, pattern, route all editable directly in the table row with save/cancel buttons
- **Separate GFW rules table** that is hidden by default, with a checkbox to show/hide; displayed in a grey-background read-only section with its own heading showing count
- **Client-side validation** preventing empty `host_glob` or `url_glob` patterns
- **Tooltips** for match type and route badges explaining their behavior
- **Lazy rendering** for the GFW table to handle large rule sets efficiently
- **Keyboard shortcuts**: Enter to save, Escape to cancel while editing inline
- **Visual distinction**: Route values displayed as styled badges

## Capabilities

### New Capabilities
- `admin-inline-rule-editing`: Users can edit rule fields (match, value, route) inline without modal dialogs
- `admin-gfw-separate-view`: GFW rules are displayed in a separate, optionally-visible table with distinct styling
- `admin-ui-validation`: Client-side validation prevents submitting empty wildcard patterns
- `admin-ui-tooltips`: Hover tooltips explain match types and route meanings
- `admin-performance-lazy-render`: Large GFW rule tables use lazy rendering to maintain UI responsiveness

### Modified Capabilities

## Impact

- Modified: `admin/handlers.go` (HTML UI, JavaScript handlers, CSS)
- New: No new packages
- APIs: None (uses existing `/api/rules/{id}` PUT endpoint)
- Performance: Lazy rendering reduces initial DOM size for GFW lists
