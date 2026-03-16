## ADDED Requirements

The admin UI is accessed at `/` and provides a tabbed interface for managing the proxy. This capability specifies the missing panels for Proxies and Settings tabs.

### Requirement: Proxies panel displays upstream external proxies

The system SHALL provide a "Proxies" panel accessible via the "Proxies" tab. The panel SHALL contain:

- A "Default External Proxy" control: a dropdown (`<select id="default-proxy-select">`) listing all configured proxy IDs plus "(none)" option, plus a "Set Default" button that saves the selection via `PUT /api/config`
- A table showing configured external proxies with columns: ID, Address, Auth (Yes/No), Actions
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
- **THEN** the browser SHALL prompt for new address, optional username/password (ID cannot change)
- **THEN** on confirmation, send `PUT /api/proxies/:id` with the updates
- **AND** refresh the list on success

#### Scenario: User deletes a proxy

- **WHEN** a user clicks "Delete" and confirms
- **THEN** the browser SHALL send `DELETE /api/proxies/:id`
- **AND** remove the row from the table on success

#### Scenario: User sets the default proxy

- **WHEN** a user selects a proxy from the dropdown and clicks "Set Default"
- **THEN** the browser SHALL send `PUT /api/config` with `{ "default_external_proxy": "<id>" }`
- **THEN** show an alert "Default proxy saved" on success

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

### Requirement: Proxies and Settings tabs integrate with existing JavaScript

The HTML panels SHALL include the DOM element IDs and structure expected by the existing JavaScript functions:

- Proxies panel: `<tbody id="proxies-body">`, `<select id="default-proxy-select">`, `<input id="new-proxy-id">`, `<input id="new-proxy-addr">`, `<input id="new-proxy-user">`, `<input id="new-proxy-pass">`, `<span id="new-proxy-test-result">`, `<button id="save-new-proxy-btn">`, and modal elements with corresponding IDs
- Settings panel: `<input id="http-listen">`, `<input id="socks5-listen">`, `<input id="admin-listen">`, `<input id="pac-path">`, `<input id="tun-device">`

The existing `loadProxies`, `renderProxies`, `testNewProxy`, `saveNewProxy`, `editProxy`, `deleteProxy`, `testProxy`, `saveDefaultProxy`, `loadConfig`, and `saveConfig` functions SHALL operate correctly once these elements are present.
