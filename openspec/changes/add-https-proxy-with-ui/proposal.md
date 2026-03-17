## Why

The HTTPS proxy listener was added in a previous change, but the admin UI lacks configuration for `https_listen` and TLS certificates. When users edit and save the configuration through the UI, these fields are dropped, preventing HTTPS proxy from being used. This change completes the feature by adding the necessary UI controls and fixing the config serialization.

## What Changes

- Add `https_listen` input field in Settings form
- Add TLS configuration section with fields: `certificate_file`, `key_file`, `ca_file`, `client_auth`, etc.
- Update admin API (`handleGetConfig`, `handlePutConfig`) to include `https_listen` and `tls` settings in JSON
- Extend `FullConfig.Proxy` struct to include `HTTPSListen` and `TLSConfig` fields
- Ensure config save preserves all fields (no data loss)
- Update JavaScript `loadConfig()` and `saveConfig()` to handle new fields

## Capabilities

### New Capabilities
- `https-proxy-ui-config`: Admin UI configuration for HTTPS proxy listener and TLS settings

### Modified Capabilities
*(none)*

## Impact

- `admin/handlers.go`: `FullConfig` struct, `handleGetConfig`, `handlePutConfig`
- `admin/handlers.go`: HTML template (Settings form) and JavaScript
- No changes to core proxy implementation (already complete)
