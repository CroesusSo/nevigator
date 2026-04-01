## Why

The admin UI's Edit button for SSH tunnels displays the keepalive interval as a large integer (nanoseconds) instead of a human-readable duration like "30s", making it confusing for administrators. Additionally, when keepalive is enabled but no interval is specified, the system fails to apply the default 30s interval, leading to potential misconfiguration. These bugs impair the usability and correctness of SSH proxy management.

## What Changes

- Fix `GET /api/proxies/:id` to return `keepalive.interval` as a duration string (e.g., "30s") instead of an integer (nanoseconds)
- Ensure that when creating or updating an SSH proxy with keepalive enabled but no interval provided, the system defaults to 30 seconds
- These changes affect the admin UI's Proxies tab and the external proxy registry API

## Capabilities

### Modified Capabilities

- **admin-ui-tabs**: The Edit modal for SSH proxies must correctly display and edit keepalive interval as a duration string.
- **external-proxy-registry**: The API must return keepalive interval in human-readable duration string format; the create and update handlers must apply default interval when enabled and not specified.

## Impact

- Files: `admin/handlers.go` (GET /api/proxies/:id, POST /api/proxies, PUT /api/proxies/:id), `pac/config.go` (JSON marshaling for KeepaliveConfig)
- Admin UI frontend: no changes needed if backend returns proper format (existing code expects string)
- Configuration: existing configs remain compatible; defaulting logic matches existing YAML behavior
