## Why

The admin UI has three major omissions:

1. The Proxies tab's "Edit" button uses `prompt()` dialogs that only allow editing a proxy's address, username, and password. It misleadingly prompts for the ID (which cannot be changed) and omits SSH-specific fields (SSH port, auth method, private key path, host key check) and keepalive configuration. This forces admins to delete/recreate proxies for any SSH configuration changes.

2. The Summary tab does not display the HTTPS proxy listener, even though it is part of the server's listening addresses.

3. The Summary tab does not show SSH tunnel status (connection state, remote address, errors), making it hard to monitor tunnel health. No other tab provides this information.

These gaps hinder effective management and observability of the proxy server.

## What Changes

- Replace the prompt-based edit UI in the Proxies tab with proper modal dialogs (similar to the "Add Proxy" modals) that display all editable fields based on the proxy's `kind`
- Show the proxy ID as read-only (not editable) to avoid confusion
- Add a new backend endpoint `GET /api/proxies/:id` that returns the full `ExternalProxy` configuration (including SSH and keepalive fields) to pre-fill the edit modal
- Update the JavaScript `editProxy()` function to open the appropriate modal (SOCKS5 or SSH), fetch the proxy details, pre-fill the form, and handle submission via `PUT /api/proxies/:id`
- For SOCKS5 edit modal: address, username, password
- For SSH edit modal: address, SSH port, username, auth method, private key path, host key check, host key fingerprint, keepalive enabled/interval
- Add HTTPS proxy listener to the Summary tab's "Listening Addresses" card
- Add a new "Tunnels" tab that displays SSH tunnel status: proxy ID, remote address, connection state, and last error (if any)
- Fetch SSH tunnel status from the existing `SSHTunnels` field in `/api/status` response and display it in a table

## Capabilities

### Modified Capabilities

- `admin-ui-tabs`: 
  - The "User edits an existing proxy" scenario SHALL use modal dialogs showing all relevant fields (ID as read-only, address, credentials, and depending on proxy kind: SSH port, auth method, private key, host key check, keepalive). The behavior SHALL be consistent with the respective "Add" modals.
  - The Summary tab SHALL display the HTTPS proxy listener (from `httpsListen`) in the "Listening Addresses" card.
  - The system SHALL provide a new "Tunnels" tab showing SSH tunnel status in a table: Proxy ID, Remote Address, State, Last Error. The data SHALL be fetched from `/api/status` `sshTunnels` field.
- `external-proxy-registry`: Add a new scenario: `GET /api/proxies/:id` returns the full ExternalProxy JSON object including all SSH-specific fields (`ssh_port`, `auth_method`, `private_key_path`, `host_key_check`, `host_key_fingerprint`) and the `keepalive` object.

## Impact

- **admin/static/index.html**: Add edit modal(s) for SOCKS5 and SSH proxies; add HTTPS listener line to Summary tab; add new Tunnels tab panel with table structure
- **admin/static/script.js**: Replace `editProxy()` with modal launchers; add `showEditSocks5Modal()`, `showEditSSHModal()`, `saveEditSocks5Proxy()`, `saveEditSSHProxy()`; add `loadTunnels()` and `renderTunnels()` for new tab; extend `loadStatus()` to include HTTPS; add tab switching logic
- **admin/handlers.go**: Add `handleGetProxyByID` for `GET /api/proxies/:id`
