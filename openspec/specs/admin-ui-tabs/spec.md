## ADDED Requirements

The admin UI is accessed at `/` and provides a tabbed interface for managing the proxy. This capability specifies the missing panels for Proxies and Settings tabs.

### Requirement: Proxies panel displays upstream external proxies

The system SHALL provide a "Proxies" panel accessible via the "Proxies" tab. The panel SHALL contain:

- A "Default External Proxy" control: a dropdown (`<select id="default-proxy-select">`) listing all configured proxy IDs plus "(none)" option, plus a "Set Default" button that saves the selection via `PUT /api/config`
- A table showing configured external proxies with columns: ID, Address, Auth (Yes/No), Keepalive, Actions
- An "Add Proxy" button that opens a modal dialog for creating a new proxy
- The panel SHALL call the JavaScript function `loadProxies()` when shown to fetch data from `GET /api/proxies` and render the table

#### Scenario: User views the Proxies panel

- **WHEN** a user clicks the "Proxies" tab
- **THEN** the browser SHALL display the Proxies panel
- **AND** the system SHALL call `loadProxies()` which fetches `/api/proxies` and renders the list of proxies
- **AND** the default proxy dropdown SHALL be populated with the same proxy IDs

#### Scenario: User adds a new external proxy

- **WHEN** a user clicks "Add Proxy", fills in ID, address, optional credentials, and clicks "Save"
- **THEN** the browser SHALL send a `POST /api/proxies` with the JSON body
- **AND** on success, the modal SHALL close and the proxy list SHALL refresh

#### Scenario: User tests a proxy before saving

- **WHEN** a user enters address and credentials in the add modal and clicks "Test"
- **THEN** the browser SHALL send a `POST /api/proxies/test` with the connection details
- **AND** display "Success" or "Failed: ..." result
- **AND** enable the "Save" button only if the test succeeds

#### Scenario: User edits an existing proxy

- **WHEN** a user clicks "Edit" on a proxy row
- **THEN** the browser SHALL send `GET /api/proxies/:id` to fetch the full proxy configuration
- **AND** open the appropriate edit modal based on the proxy's `kind` (SOCKS5 or SSH)
- **AND** the modal SHALL display the proxy ID as read-only text (not an editable field)
- **AND** the modal SHALL pre-fill all other fields with the proxy's current configuration (address, username, password, and depending on kind: SSH port, auth method, private key path, host key check, host key fingerprint, keepalive enabled/interval)
- **WHEN** the user modifies fields and clicks "Save"
- **THEN** the browser SHALL send `PUT /api/proxies/:id` with the complete JSON body containing all edited fields
- **AND** on success, the modal SHALL close and the proxy list SHALL refresh

#### Scenario: User deletes a proxy

- **WHEN** a user clicks "Delete" and confirms
- **THEN** the browser SHALL send `DELETE /api/proxies/:id`
- **AND** remove the row from the table on success

#### Scenario: User sets the default proxy

- **WHEN** a user selects a proxy from the dropdown and clicks "Set Default"
- **THEN** the browser SHALL send `PUT /api/config` with `{ "default_external_proxy": "<id>" }`
- **THEN** show an alert "Default proxy saved" on success

### Requirement: Keepalive interval display and editing in SSH proxy edit modal

The edit modal for SSH proxies SHALL correctly display the keepalive interval as a human-readable duration string (e.g., "30s") and SHALL accept duration strings for editing.

#### Scenario: Keepalive interval displayed correctly
- **WHEN** a user clicks "Edit" on an SSH proxy with keepalive enabled and interval set to 30 seconds
- **THEN** the keepalive interval input field SHALL show "30s"
- **AND** the keepalive enabled checkbox SHALL be checked

#### Scenario: Keepalive interval saved correctly
- **WHEN** a user modifies the keepalive interval to "45s" and clicks "Save"
- **THEN** the browser SHALL send `PUT /api/proxies/:id` with `keepalive_interval: "45s"`
- **AND** on success, subsequent edits SHALL show "45s" in the interval field

### Requirement: Settings panel displays and edits system configuration

The system SHALL provide a "Settings" panel accessible via the "Settings" tab. The panel SHALL contain:

- A form with fields for:
  - `http_listen` (HTTP proxy listen address)
  - `socks5_listen` (SOCKS5 proxy listen address)
  - `admin_listen` (admin UI/API listen address)
  - `pac_path` (PAC file path)
  - `tun_device` (TUN device name)
- A "Save Configuration" button that sends the form data to `PUT /api/config`
- A hint that listener changes require a restart to take effect

#### Scenario: User opens Settings panel

- **WHEN** a user clicks the "Settings" tab
- **THEN** the browser SHALL call `loadConfig()` which fetches `/api/config` and populates the form fields with current values

#### Scenario: User saves configuration changes

- **WHEN** a user edits one or more fields and clicks "Save Configuration"
- **THEN** the browser SHALL send a `PUT /api/config` with the JSON payload
- **AND** show an alert "Config saved" (or with "(restart required)" if any listener field was changed)
- **AND** the form SHALL retain the new values locally (no auto-refresh)

#### Scenario: Changing default external proxy triggers PAC regeneration

- **WHEN** a user changes the default_external_proxy via the Proxies panel and saves
- **THEN** the backend SHALL update the router's default proxy ID immediately and regenerate the PAC file so GFW rules with empty route now use the new default

### Requirement: Summary tab displays complete listening addresses and system status

The Summary tab SHALL show:
- Listening addresses: HTTP, SOCKS5, Admin, HTTPS (if configured)
- Active connections count and total connections count
- SSH Tunnel Status: a table listing all SSH tunnels with columns: Proxy ID, Remote Address, State, Last Error

The data SHALL be fetched from `GET /api/status` (`httpListen`, `socks5Listen`, `adminListen`, `httpsListen`, `activeConns`, `totalConns`, `sshTunnels`). The Summary tab SHALL poll status every 5 seconds to update these values.

#### Scenario: User views the Summary tab

- **WHEN** a user clicks the "Summary" tab
- **THEN** the browser SHALL display the Summary panel
- **AND** the system SHALL call `loadStatus()` which fetches `/api/status` and populates the listening address fields (including HTTPS) and connection counts
- **AND** the system SHALL schedule periodic refresh every 5 seconds to update the displayed values
- **AND** the SSH Tunnels table SHALL render a row for each entry in `sshTunnels`

#### Scenario: HTTPS listener is displayed when configured

- **GIVEN** the server has an HTTPS listener configured
- **WHEN** the Summary tab is loaded
- **THEN** the UI SHALL show an "HTTPS Proxy" line in the "Listening Addresses" card with the configured address
- **AND** if no HTTPS listener is configured, that line SHALL show "-" or be omitted

#### Scenario: SSH tunnels are displayed in Summary

- **GIVEN** the Summary tab is showing
- **WHEN** SSH tunnels are present in `sshTunnels`
- **THEN** the SSH Tunnels table SHALL display each tunnel with its proxy ID, remote address, state, and last error
- **AND** if no tunnels exist, an empty-state message SHALL be shown

#### Scenario: Tunnel state changes are reflected

- **GIVEN** a tunnel is in "connected" state
- **WHEN** the SSH connection drops and the tunnel manager begins reconnecting
- **THEN** the next status fetch SHALL show `state: "reconnecting"` and possibly `lastError`
- **AND** the UI SHALL update accordingly without requiring a page reload

### Requirement: Proxies and Settings tabs integrate with existing JavaScript

The HTML panels SHALL include the DOM element IDs and structure expected by the existing JavaScript functions:

- Proxies panel: `<tbody id="proxies-body">`, `<select id="default-proxy-select">`, `<input id="new-proxy-id">`, `<input id="new-proxy-addr">`, `<input id="new-proxy-user">`, `<input id="new-proxy-pass">`, `<span id="new-proxy-test-result">`, `<button id="save-new-proxy-btn">`, and modal elements with corresponding IDs
- Additional edit modal elements for SOCKS5: `<div id="edit-socks5-modal">`, `<div id="edit-socks5-overlay">`, `<span id="edit-socks5-id">` (read-only text), `<input id="edit-socks5-addr">`, `<input id="edit-socks5-user">`, `<input id="edit-socks5-pass">`, `<span id="edit-socks5-test-result">`, `<button id="save-edit-socks5-btn">`
- Additional edit modal elements for SSH: `<div id="edit-ssh-modal">`, `<div id="edit-ssh-overlay">`, `<span id="edit-ssh-id">` (read-only text), `<input id="edit-ssh-addr">`, `<input id="edit-ssh-port">`, `<input id="edit-ssh-user">`, `<select id="edit-ssh-auth-method">`, `<input id="edit-ssh-pass">`, `<input id="edit-ssh-private-key-path">`, `<select id="edit-ssh-host-key-check">`, `<input id="edit-ssh-host-key-fingerprint">`, `<input id="edit-ssh-keepalive-enabled">`, `<input id="edit-ssh-keepalive-interval">`, `<span id="edit-ssh-test-result">`, `<button id="save-edit-ssh-btn">`
- Settings panel: `<input id="http-listen">`, `<input id="socks5-listen">`, `<input id="admin-listen">`, `<input id="pac-path">`, `<input id="tun-device">`
- Summary tab (tunnels): `<tbody id="tunnels-body">`

The existing `loadProxies`, `renderProxies`, `testNewProxy`, `saveNewProxy`, `editProxy`, `deleteProxy`, `testProxy`, `saveDefaultProxy`, `loadConfig`, `saveConfig`, `loadStatus`, and the new `renderTunnels` functions SHALL operate correctly once these elements are present. The `editProxy` function SHALL open the appropriate edit modal (SOCKS5 or SSH) based on proxy kind. The `showEditSocks5Modal`, `showEditSSHModal`, `saveEditSocks5Proxy`, `saveEditSSHProxy`, `closeEditSocks5Modal`, `closeEditSSHModal`, `toggleEditSSHAuthFields`, and `toggleEditSSHKeepalive` functions SHALL be implemented and function correctly.

#### Scenario: Edit modals integrate correctly

- **WHEN** the edit modals are present in the DOM
- **THEN** the existing `editProxy` function SHALL open the appropriate modal (SOCKS5 or SSH) based on proxy kind
- **AND** the `showEditSocks5Modal` and `showEditSSHModal` functions SHALL fetch `GET /api/proxies/:id` and populate all fields correctly
- **AND** the `saveEditSocks5Proxy` and `saveEditSSHProxy` functions SHALL send `PUT /api/proxies/:id` with complete data
- **AND** the `renderTunnels` function SHALL populate the tunnels table when called by `loadStatus`
