## Modified Capability: external-proxy-registry

This spec delta modifies `openspec/specs/external-proxy-registry/spec.md` to add SSH tunnel proxy support.

### Changes to `ExternalProxy` structure

The `ExternalProxy` struct SHALL be extended with a new field:

- `kind`: optional string, default `"socks5"`. Values: `"socks5"` or `"ssh"`.

Additionally, the following SSH-specific fields SHALL be added (ignored if `kind != "ssh"`):

- `ssh_port`: integer, default 22
- `auth_method`: string, either `"password"` or `"key"`
- `private_key_path`: string (required if `auth_method == "key"`)
- `host_key_check`: string, either `"strict"` (default) or `"ignore"`
- `host_key_fingerprint`: optional string (hex) for pinning
- `keepalive`: optional object with fields:
  - `enabled`: boolean, default false
  - `interval`: duration string (e.g., "30s"), default "30s" when enabled

The existing `address` field SHALL be interpreted differently based on `kind`:
- For `socks5`: `host:port` of the SOCKS5 server (as before)
- For `ssh`: may include `user@` prefix and optional `:port`. The port, if present, SHALL be used unless `ssh_port` is explicitly set (which takes precedence). If no port is provided (in address or `ssh_port`), default to 22.

The existing `username` and `password` fields SHALL be reused:
- For `socks5`: as before (SOCKS5 authentication)
- For `ssh`: `username` is the SSH username; `password` is used for SSH password authentication when `auth_method == "password"`

### New Scenarios

#### Scenario: Create SSH proxy via API
- **WHEN** an administrator POSTs `/api/proxies` with JSON body containing `"kind": "ssh"` and the required SSH fields
- **THEN** the proxy SHALL be added to the registry with kind ssh
- **AND** the config file SHALL be updated atomically
- **AND** the router's tunnel manager SHALL be notified (via `UpdateProxies`)

#### Scenario: Validation of SSH proxy creation
- **WHEN** an administrator POSTs `/api/proxies` with `"kind": "ssh"` but missing required fields (`auth_method`, or credentials for chosen method)
- **THEN** the API SHALL return HTTP 400 Bad Request
- **AND** the response SHALL include an error message indicating which fields are missing or invalid
- **EXAMPLES**:
  - `auth_method: "key"` without `private_key_path` → 400 "private_key_path required for key auth"
  - `auth_method: "password"` without `password` → 400 "password required for password auth"
  - missing `auth_method` → 400 "auth_method required for SSH proxy"

#### Scenario: Update SSH proxy validation
- **WHEN** an administrator PUTs `/api/proxies/:id` with `"kind": "ssh"` and incomplete credentials
- **THEN** the API SHALL return HTTP 400 with appropriate error message
- **AND** the proxy SHALL NOT be updated

#### Scenario: SSH proxy test via API
- **WHEN** an administrator POSTs `/api/proxies/:id/test` for an SSH proxy
- **THEN** the system SHALL attempt to establish an SSH connection using the configured parameters
- **AND** return `{ "success": true }` on success, or `{ "success": false, "error": "..." }` on failure
- **AND** not affect the existing tunnel state (this is a test connection only)

#### Scenario: Router resolves EXTERNAL to SSH tunnel
- **WHEN** the router's `dialWithRoute` is called with a route `EXTERNAL <id>` where the proxy has `kind: "ssh"`
- **THEN** the router SHALL call `tunnelMgr.Dial(id, destination)` instead of `dialSOCKS5WithAuth`
- **AND** the tunnel manager SHALL return a `net.Conn` through the established SSH channel

#### Scenario: SSH tunnel reconnection
- **WHEN** an SSH connection is lost (e.g., server closes, network blip)
- **THEN** the tunnel manager SHALL attempt to reconnect with exponential backoff
- **AND** future `Dial` calls SHALL block until reconnection succeeds or fails permanently

#### Scenario: Backward compatibility for missing `kind`
- **WHEN** a proxy config is loaded that does not include the `kind` field
- **THEN** the system SHALL treat it as `kind: "socks5"` (preserving existing behavior)

#### Scenario: Older binary compatibility
- **WHEN** an older binary (without SSH support) loads a config containing `kind: "ssh"`
- **THEN** it SHALL ignore the `kind` field and treat the proxy as `socks5`
- **AND** connection attempts SHALL fail with a SOCKS5 handshake error (rather than crashing)
- **RATIONALE**: Enables safe downgrade in case of issues

### Updated Configuration Example

```yaml
proxy:
  external_proxies:
    - id: "ssh-gfw"
      kind: "ssh"
      address: "user@ssh.example.com"
      ssh_port: 22
      username: "user"           # optional if in address
      auth_method: "key"
      private_key_path: "/etc/keys/id_rsa"
      host_key_check: "strict"
      keepalive:
        enabled: true
        interval: 30s
```

### Impact on Existing Scenarios

All existing scenarios for SOCKS5 proxies remain unchanged. The addition of `kind` and SSH fields does not break existing configurations when omitted.

### Additional Requirement: SSH Tunnel Status

The system SHALL expose SSH tunnel status via the `/api/status` endpoint. The status JSON SHALL include an `ssh_tunnels` object mapping proxy IDs to their current state.

#### Scenario: Status endpoint includes SSH tunnels
- **WHEN** an administrator GETs `/api/status`
- **THEN** the response SHALL contain an `ssh_tunnels` field
- **AND** each entry SHALL include:
  - `state`: one of `"connected"`, `"connecting"`, `disconnected"`, `"reconnecting"`
  - `last_error`: string if the tunnel is in a failed state, otherwise `null`
  - `remote_addr`: the SSH server address being connected to
  - (optional) `active_channels`: number of currently open channels through the tunnel

#### Scenario: Tunnel state transitions are reflected in status
- **GIVEN** an SSH proxy is configured
- **WHEN** the tunnel is connecting
- **THEN** `/api/status` SHALL show `state: "connecting"`
- **WHEN** the SSH connection succeeds
- **THEN** `/api/status` SHALL show `state: "connected"` and `last_error: null`
- **WHEN** the SSH connection fails with a transient error
- **THEN** `/api/status` SHALL show `state: "reconnecting"` and the error in `last_error`
- **WHEN** the tunnel is removed or disabled
- **THEN** `/api/status` SHALL no longer include that proxy ID

### ADDED Requirement: Unsaved proxy test endpoint supports SSH tunnels

The `POST /api/proxies/test` endpoint SHALL support both SOCKS5 and SSH proxy types based on the `kind` field in the request payload. This allows testing SSH tunnel configurations before saving them to the registry.

#### Scenario: Unsaved SOCKS5 proxy test
- **WHEN** an administrator POSTs `/api/proxies/test` with a payload containing `kind: "socks5"` (or missing `kind`)
- **THEN** the system SHALL test the connection using SOCKS5 handshake
- **AND** return `{ "success": true }` on successful SOCKS5 connection, or `{ "success": false, "error": "..." }` on failure
- **AND** complete within 5 seconds (including DNS, TCP connect, SOCKS5 handshake)

#### Scenario: Unsaved SSH proxy test
- **WHEN** an administrator POSTs `/api/proxies/test` with a payload containing `kind: "ssh"` and required SSH fields (`auth_method`, and either `password` for password auth or `private_key_path` for key auth)
- **THEN** the system SHALL construct a temporary `ExternalProxy` configuration from the request fields
- **AND** establish an SSH connection using that configuration (without affecting the tunnel manager's persistent tunnels)
- **AND** open a TCP channel to the test destination `1.1.1.1:80`
- **AND** return `{ "success": true }` on successful SSH connection and channel open, or `{ "success": false, "error": "..." }` on failure
- **AND** clean up all SSH resources immediately after the test (tunnel closed, goroutines stopped)
- **AND** the entire operation SHALL time out after 5 seconds if not completed

**Note**: The test destination `1.1.1.1:80` is chosen because it is a globally routable anycast IP (Cloudflare DNS) with port 80 typically open, providing a uniform connectivity check not affected by local routing rules.

#### Scenario: Unsaved SSH proxy test validation
- **WHEN** an administrator POSTs `/api/proxies/test` with `kind: "ssh"` but missing required fields
- **THEN** the API SHALL return HTTP 400 Bad Request
- **AND** the response SHALL include an error message indicating which fields are missing or invalid
- **EXAMPLES**:
  - `auth_method: "key"` without `private_key_path` → 400 "private_key_path required for key auth"
  - `auth_method: "password"` without `password` → 400 "password required for password auth"
  - missing `auth_method` → 400 "auth_method required for SSH proxy"

#### Scenario: Unsaved SSH test respects host key verification
- **WHEN** an administrator POSTs `/api/proxies/test` with `kind: "ssh"` and `host_key_check` field set to `"strict"` or `"ignore"`
- **THEN** the SSH connection SHALL apply the specified host key verification behavior
- **AND** if `host_key_fingerprint` is provided, it SHALL be used for pinning (hex string, colons optional)
- **WHEN** `host_key_check` is not provided or has any value other than `"ignore"`
- **THEN** the default SHALL be `"strict"` (using known_hosts with fingerprint pinning if provided)
- **WHEN** `host_key_check` is `"strict"` but known_hosts verification fails (missing entry or fingerprint mismatch)
- **THEN** the connection SHALL fail with an appropriate error (`"host key verification failed"` or `"host key fingerprint mismatch"`)

#### Scenario: Unsaved SSH test does not affect tunnel manager
- **GIVEN** a tunnel manager is running with existing SSH tunnels
- **WHEN** an administrator POSTs `/api/proxies/test` with SSH parameters
- **THEN** the test SHALL use a temporary `ssh.Tunnel` instance created via `ssh.NewTunnel`
- **AND** SHALL NOT register with the tunnel manager
- **AND** SHALL NOT modify any existing tunnel state
- **AND** SHALL NOT leave any goroutines running after completion (tunnel.Close invoked)

#### Scenario: Backward compatibility for unsaved test (missing `kind`)
- **WHEN** a request is sent to `/api/proxies/test` without a `kind` field
- **THEN** the system SHALL treat it as `kind: "socks5"` (preserving existing behavior)
- **AND** SOCKS5 proxy testing SHALL work as before

#### Scenario: Request payload structure for unsaved test
The request body SHALL be a JSON object with the following schema:

```json
{
  "type": "object",
  "required": ["address"],
  "properties": {
    "address": { "type": "string" },
    "username": { "type": "string" },
    "password": { "type": "string" },
    "kind": { "type": "string", "enum": ["socks5", "ssh"], "default": "socks5" },
    "auth_method": { "type": "string", "enum": ["password", "key"] },
    "private_key_path": { "type": "string" },
    "ssh_port": { "type": "integer", "minimum": 1, "maximum": 65535, "default": 22 },
    "host_key_check": { "type": "string", "enum": ["strict", "ignore"], "default": "strict" },
    "host_key_fingerprint": { "type": "string" }
  }
}
```

#### Scenario: Response format consistency for unsaved test
- **WHEN** the test completes (success or failure)
- **THEN** the response SHALL be JSON with `Content-Type: application/json`
- **AND** on success: `{ "success": true }` with HTTP 200
- **AND** on failure (non-validation): `{ "success": false, "error": "..." }` with HTTP 200
- **AND** on validation error: `{ "error": "..." }` with HTTP 400 Bad Request

This matches the response format of the saved proxy test endpoint (`/api/proxies/:id/test`) for consistency.

#### Scenario: Frontend coordination for unsaved test
- **WHEN** the admin UI sends a test request for an SSH tunnel
- **THEN** it SHALL include all required SSH fields: `kind: "ssh"`, `auth_method`, `private_key_path` (if key auth) or `password` (if password auth), `ssh_port`, `host_key_check`
- **AND** may also include `host_key_fingerprint` if available
- **AND** the UI shall interpret the response and display success or error to the user

### ADDED Requirement: Get full proxy details by ID

The system SHALL provide an endpoint `GET /api/proxies/:id` that returns the complete `ExternalProxy` configuration for the requested proxy, including all SSH-specific fields and keepalive settings.

#### Scenario: Fetch full configuration of a proxy
- **WHEN** an administrator sends `GET /api/proxies/:id` for an existing proxy
- **THEN** the response SHALL be HTTP 200 with `Content-Type: application/json`
- **AND** the JSON body SHALL contain the full `ExternalProxy` object with the following fields present if applicable:
  - `id`
  - `address`
  - `kind` (if set, default "socks5")
  - `username`
  - `password`
  - `ssh_port`
  - `auth_method`
  - `private_key_path`
  - `host_key_check`
  - `host_key_fingerprint`
  - `keepalive` object with `enabled` and `interval`
- **WHEN** the specified proxy ID does not exist
- **THEN** the response SHALL be HTTP 404 Not Found

#### Scenario: Edit UI fetches full proxy data before opening modal
- **WHEN** the admin UI user clicks "Edit" on a proxy row
- **THEN** the browser SHALL first issue `GET /api/proxies/:id` to retrieve the complete configuration
- **AND** the browser SHALL open the edit modal for the appropriate kind (SOCKS5 or SSH)
- **AND** the modal SHALL pre-fill all editable fields from the retrieved data
- **AND** the proxy ID SHALL be displayed as static text (not in an editable input)

#### Scenario: Backward compatibility of list endpoint
- **WHEN** an administrator sends `GET /api/proxies` (list endpoint)
- **THEN** the response format SHALL remain unchanged: each proxy object contains only `id`, `address`, `kind`, `has_auth`, `keepalive_enabled`, `keepalive_interval` fields
- **AND** the list endpoint SHALL NOT include sensitive fields like `password` or `private_key_path`
- **RATIONALE**: The list endpoint is used for the table display; it does not need full credentials and exposing them would increase attack surface.

