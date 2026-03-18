## 1. Core Implementation

- [x] 1.1 In `admin/handlers.go`, modify the JavaScript `loadConfig()` function to:
  - After the existing `/api/config` fetch completes, start a second `fetch('/api/gfw/config')`.
  - If the response is OK, parse JSON and set `document.getElementById('gfw-url').value = data.sourceURL || ''`.
  - Handle errors by logging to console (do not disrupt other config loading).
- [x] 1.2 Ensure the `#gfw-url` input element exists in the Settings form and has the correct id (already present in the HTML).

## 2. Verification

- [x] 2.1 Manual test: Start the server with a known GFW source URL in config; open Settings page; verify the GFW URL input shows that value.
- [x] 2.2 Save the field with a different URL; confirm the config file updates and persists after restart.
- [x] 2.3 Run the existing test suite to ensure no regressions.
