## ADDED Requirements

### Requirement: Keepalive interval format in API responses

The `keepalive.interval` field in all API responses (`GET /api/proxies`, `GET /api/proxies/:id`) SHALL be a duration string (e.g., "30s") rather than an integer representing nanoseconds.

#### Scenario: GET /api/proxies/:id returns keepalive interval as string
- **WHEN** an administrator requests `GET /api/proxies/:id` for an SSH proxy with keepalive enabled and interval 30 seconds
- **THEN** the response JSON SHALL contain `"keepalive": { "enabled": true, "interval": "30s" }`
- **AND** the `interval` field SHALL NOT be an integer like `30000000000`

### Requirement: Default keepalive interval on proxy creation

When creating an SSH proxy via `POST /api/proxies` with `keepalive.enabled = true` and no `keepalive.interval` provided, the system SHALL set `keepalive.interval` to "30s".

#### Scenario: Create SSH proxy with keepalive enabled but no interval
- **WHEN** an administrator POSTs `/api/proxies` with JSON body containing `"kind": "ssh"`, `"keepalive_enabled": true`, and no `keepalive_interval`
- **THEN** the created proxy SHALL have `keepalive.interval = "30s"`
- **AND** the response SHALL include `"keepalive_interval": "30s"`
- **AND** subsequent `GET /api/proxies/:id` SHALL return `"keepalive": { "enabled": true, "interval": "30s" }`
