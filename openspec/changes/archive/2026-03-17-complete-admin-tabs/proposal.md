## Why

The admin UI includes "Proxies" and "Settings" tabs in the navigation, but the corresponding panel content is missing from the HTML template. Users see these tabs but cannot interact with them—they show blank content. This incomplete UI prevents administrators from managing external proxy configurations and viewing/editing system settings, which are core features of the proxy management interface.

## What Changes

- Add HTML panel content for the **Proxies** tab: displays upstream proxies table, default proxy selector, add proxy modal, and associated controls
- Add HTML panel content for the **Settings** tab: displays form fields for listener addresses, PAC file path, TUN device, with save functionality
- No changes to backend APIs—they already exist and are fully functional (`/api/proxies`, `/api/config`)
- JavaScript functions for these tabs are already implemented; they only need the DOM elements to act upon

This change completes the admin UI, making all tabs functional.

## Capabilities

### New Capabilities
- `admin-ui-tabs`: The admin interface shall provide fully functional Proxies and Settings panels that integrate with existing backend APIs and JavaScript controllers.

### Modified Capabilities
*(none)*

## Impact

- **Files modified**: `admin/handlers.go` (HTML template)
- **No API changes**: Existing endpoints (`/api/proxies`, `/api/config`) already support the needed functionality
- **No database changes**: All configuration remains in YAML file
- **Frontend**: Only template HTML is added; existing JS uses the elements without modification
