## 1. Backend API Extension

- [x] 1.1 Add `handleGetProxyByID` in `admin/handlers.go` that reads config, finds the proxy by ID, and returns its full `ExternalProxy` JSON (including all SSH and keepalive fields)
- [x] 1.2 Register the new route in `RegisterRoutes`: `mux.HandleFunc("/api/proxies/", h.handleProxyByID)` (already exists for PUT/DELETE). Ensure `handleProxyByID` dispatches `GET` to the new handler
- [x] 1.3 Update `handleProxyByID` to handle `GET` method by calling `handleGetProxyByID` (keep existing PUT/DELETE)

## 2. Frontend - Summary and Tunnels Tab

- [x] 2.1 Add HTTPS listener field to Summary tab HTML: add a "HTTPS Proxy" line in the "Listening Addresses" card with `<span id="s-https">-</span>`
- [x] 2.2 Add SSH Tunnels card to Summary tab (instead of separate tab): add a new card with table (columns: Proxy ID, Remote Address, State, Last Error) and `<tbody id="tunnels-body">`
- [x] 2.3 Ensure the Tunnels card is part of the Summary panel HTML
- [x] 2.4 Update `loadStatus()` in script.js to populate the HTTPS field (`s-https`) from `status.httpsListen`
- [x] 2.5 Implement `renderTunnels()` to populate `tunnels-body` with rows for each SSH tunnel (proxy ID, remoteAddr, state, lastError)
- [x] 2.6 Call `renderTunnels()` from `loadStatus()` so tunnels load with Summary
- [x] 2.7 No separate tab switching needed; Summary tab automatically shows tunnels
- [x] 2.8 Ensure status polling (every 5s) updates Summary including tunnels data

## 3. Frontend - Edit Modal UI (HTML)

- [x] 3.1 Add edit socks5 proxy modal HTML in `admin/static/index.html` with fields: ID (read-only span), address, username, password, test result, Save/Cancel buttons
- [x] 3.2 Add edit ssh tunnel modal HTML with fields: ID (read-only span), address, SSH port, username, auth method (select), password (for password auth), private key path, host key check (select), host key fingerprint, keepalive (checkbox+interval), test result, Save/Cancel buttons
- [x] 3.3 Ensure the edit modals are hidden by default (`display:none`) and positioned similarly to add modals (overlay + modal div)

## 4. Frontend - Edit Modal Logic (JavaScript)

- [x] 4.1 Replace `editProxy(id)` in `admin/static/script.js` to fetch `GET /api/proxies/:id`, determine proxy kind, call `showEditSocks5Modal()` or `showEditSSHModal()`, and close any edit modal if already open
- [x] 4.2 Implement `showEditSocks5Modal(id)` that fetches the proxy data, populates the edit modal fields, and shows it
- [x] 4.3 Implement `showEditSSHModal(id)` that fetches the proxy data, populates all SSH-specific fields, and shows it
- [x] 4.4 Implement `saveEditSocks5Proxy()` that gathers form data, sends `PUT /api/proxies/:id` with JSON body, and refreshes proxy list on success
- [x] 4.5 Implement `saveEditSSHProxy()` that gathers form data (including SSH fields and keepalive), sends `PUT /api/proxies/:id`, and refreshes on success
- [x] 4.6 Add toggle behavior for keepalive interval visibility in SSH edit modal (hide interval when unchecked, show when checked) and default to empty or existing value
- [x] 4.7 Add toggle behavior for SSH auth method in edit modal (show password field when password, show private key field when key)
- [x] 4.8 Wire test buttons in edit modals to call `testProxy(id)` (existing endpoint) for simplified testing; no separate unsaved test needed since data is saved
- [x] 4.9 Ensure ID is displayed as read-only text (not an input) in edit modals
- [x] 4.10 Update `closeEditSocks5Modal()` and `closeEditSSHModal()` to hide modals and clear data (if needed)

## 5. Validation and Consistency

- [x] 5.1 Verify that `PUT /api/proxies/:id` accepts all fields sent by edit modals (address, username, password, ssh_port, auth_method, private_key_path, host_key_check, host_key_fingerprint, keepalive). The backend already parses all these fields; no code change needed.
- [x] 5.2 Ensure keepalive interval is parsed as duration string and defaults to 30s if enabled and empty (backend `handleUpdateProxy` already does this)
- [x] 5.3 Test edit flow for SOCKS5 proxy: ID read-only, address editable, test button works, save updates config and router registry
- [x] 5.4 Test edit flow for SSH proxy: all SSH fields editable, auth method switching works, test button works, save updates everything
- [x] 5.5 Test Summary tab shows HTTPS listener correctly (if configured, otherwise "-")
- [x] 5.6 Test Tunnels tab displays SSH tunnels with correct state, updates when status changes

## 6. Documentation and Polish

- [x] 6.1 Review the admin UI tables to confirm keepalive column displays properly for both add and edit contexts
- [x] 6.2 Ensure user feedback: show "Updated" alert on successful edit; preserve modal state on error
- [x] 6.3 Verify that errors from the backend (400/500) are displayed to the user in the edit modals
- [x] 6.4 Run existing admin UI tests to ensure no regressions
