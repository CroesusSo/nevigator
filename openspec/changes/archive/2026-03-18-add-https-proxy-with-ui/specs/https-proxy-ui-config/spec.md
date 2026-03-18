## ADDED Requirements

### Requirement: Admin UI exposes HTTPS proxy listener configuration
The Settings tab in the admin UI SHALL provide an input field for the `https_listen` address. Users SHALL be able to set and save this value through the UI.

#### Scenario: Set HTTPS listener via UI
- **WHEN** the user opens the Settings tab
- **THEN** an input field labeled "HTTPS Proxy (listen)" SHALL be visible
- **WHEN** the user enters a value (e.g., `:8443`) and clicks "Save Configuration"
- **THEN** the config file SHALL contain the `https_listen` line with the saved value
- **AND** the server SHALL log that a restart is required for listener changes

#### Scenario: Load existing HTTPS listener from config
- **WHEN** the user opens the Settings tab
- **THEN** the HTTPS input field SHALL be pre-filled with the current `https_listen` value from the config
- **IF** `https_listen` is not set, the field SHALL be empty

### Requirement: Admin UI exposes TLS certificate configuration
The Settings tab SHALL provide input fields for TLS certificate settings used by the HTTPS proxy. These fields SHALL include at minimum: `certificate_file`, `key_file`. Optional fields MAY include: `ca_file`, `client_auth`.

#### Scenario: Set TLS certificate paths via UI
- **WHEN** the user is on the Settings tab
- **THEN** input fields for TLS settings SHALL be visible (e.g., "Certificate file", "Key file")
- **WHEN** the user enters paths to certificate and key files and saves
- **THEN** the config file SHALL contain a `tls:` section with those paths
- **THEN** the server SHALL apply those TLS settings after restart (or log restart required)

#### Scenario: Load existing TLS settings
- **WHEN** the user opens the Settings tab
- **THEN** the TLS input fields SHALL be pre-filled with values from the config's `tls:` block if present

### Requirement: Config API includes HTTPS and TLS fields
The `/api/config` endpoint SHALL include `https_listen` and `tls` (with its subfields) in its JSON responses. The endpoint SHALL accept these fields on PUT requests and write them to the config file.

#### Scenario: GET /api/config returns HTTPS and TLS fields
- **WHEN** a client makes a GET request to `/api/config`
- **THEN** the JSON response SHALL contain `https_listen` (string or null)
- **AND** SHALL contain a `tls` object with `certificate_file`, `key_file`, and other TLS fields (strings, may be null)

#### Scenario: PUT /api/config updates HTTPS and TLS settings
- **WHEN** a client sends a PUT request to `/api/config` with JSON body containing `https_listen` and/or `tls` fields
- **THEN** the server SHALL update those fields in the config file
- **THEN** the server SHALL preserve other fields unchanged
- **THEN** the server SHALL respond with `restart_required: true` if `https_listen` or any TLS field changed
- **AND** the config file after write SHALL reflect the new values

#### Scenario: Backward compatibility for configs without HTTPS/TLS
- **WHEN** a config file exists without `https_listen` or `tls` sections
- **WHEN** the admin UI loads the config
- **THEN** the GET response SHALL include `https_listen` as empty string and `tls` as an object with empty fields
- **AND** the UI SHALL function correctly (empty inputs)
- **WHEN** the user saves without modifying those fields
- **THEN** the config file MAY gain empty `https_listen` and `tls` sections (still valid)
- **AND** the proxy SHALL run with HTTPS disabled (as before)

### Requirement: No data loss when saving via UI
When the user clicks "Save Configuration" on the Settings tab, any existing configuration values not represented in the UI form SHALL be preserved in the config file. The admin UI SHALL NOT remove fields that it does not edit.

#### Scenario: Preserve existing fields not in UI
- **GIVEN** a config file containing `https_listen`, `tls`, and other fields
- **WHEN** the user opens Settings, makes a change to an unrelated field (e.g., `http_listen`), and saves
- **THEN** the resulting config file SHALL retain the original `https_listen` and `tls` values unchanged
- **AND** the changed field SHALL be updated

This requirement must be ensured by the implementation: either by including all fields in the PUT payload, or by merging updates rather than overwriting entire sections.
