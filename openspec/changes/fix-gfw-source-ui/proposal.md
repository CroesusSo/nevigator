## Why

The GFW (Great Firewall) list source URL is saved in the config and can be updated via the admin UI, but the Settings page does **not** display the current value on load. Users only see the placeholder text, making it difficult to know or change the existing URL. This is a usability gap: the input field should be pre‑filled with the current configuration.

## What Changes

- Extend the JavaScript `loadConfig()` function (in the admin UI) to also fetch the current GFW configuration from `/api/gfw/config`.
- Populate the `#gfw-url` input field with the returned `sourceURL` value.
- No change to backend APIs—they already exist (`GET /api/gfw/config`).

## Capabilities

### New Capabilities
- `gfw-ui-config`: The admin UI SHALL display the current GFW source URL in the Settings tab and allow saving it.

### Modified Capabilities
*(none)*

## Impact

- **File:** `admin/handlers.go` – modify the embedded JavaScript `loadConfig()` function.
- **No breaking changes** to APIs or config file format.
- **User-visible improvement:** Users can now see and verify the GFW source URL before saving.
