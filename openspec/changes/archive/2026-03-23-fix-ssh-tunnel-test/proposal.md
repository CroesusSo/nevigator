## Why

The admin UI provides a "Test" button for verifying proxy connectivity before saving. While the saved proxy test (`/api/proxies/:id/test`) supports both SOCKS5 and SSH tunnels, the unsaved test endpoint (`/api/proxies/test`) **only supports SOCKS5**. When testing an SSH tunnel configuration prior to saving, the backend ignores the `kind: "ssh"` field and attempts a SOCKS5 handshake, which always fails. This creates a poor user experience and forces users to save first, then test, potentially creating invalid configurations. We need to fix the unsaved test endpoint to properly handle SSH tunnels.

## What Changes

- **Backend**: Extend `POST /api/proxies/test` handler to:
  - Accept `kind` field (default "socks5" for backward compatibility)
  - Accept SSH-specific parameters (`auth_method`, `private_key_path`, `ssh_port`, `host_key_check`, `host_key_fingerprint`)
  - Validate SSH parameters; return HTTP 400 on missing or invalid required fields
  - Use `ssh.NewTunnel` + `tunnel.Dial` for SSH tests (similar to saved proxy test)
  - Keep existing SOCKS5 logic unchanged
  - No changes to saved proxy test endpoint (`/api/proxies/:id/test`) which already works correctly
- **Frontend**: Update `testNewSSHProxy()` in `admin/static/script.js` to include `host_key_check` in the JSON payload (the UI already has this field). `host_key_fingerprint` is reserved for future UI addition but backend will accept it if sent.
- **Tests**: Add unit tests for SSH validation scenarios and integration test with real SSH server

## Capabilities

### Modified Capabilities
- `external-proxy-registry`: Extend unsaved proxy test endpoint (`POST /api/proxies/test`) to support SSH tunnel testing.

### New Capabilities
*(none)*

## Impact

**Files Modified**:
- `admin/handlers.go` (extend `handleTestProxy`)
- `admin/static/script.js` (include `host_key_check` in SSH test payload)
- `admin/handlers_test.go` or new test file (unit tests)
- Potential new integration test file

**API Change**: 
- Request body for `/api/proxies/test` gains optional fields: `kind`, `auth_method`, `private_key_path`, `ssh_port`, `host_key_check`, `host_key_fingerprint`
- **Backward compatible**: missing `kind` defaults to `"socks5"`; existing SOCKS5 tests unaffected

**No Breaking Changes**: No changes to saved proxy test or any existing endpoints.

**Dependencies**: Existing `ssh` package provides required tunnel functionality. No new external dependencies.

**Performance**: Negligible; unsaved test creates a temporary tunnel that is closed immediately after connectivity check.
