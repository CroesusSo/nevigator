## MODIFIED Requirements

### Requirement: Unsaved proxy test endpoint supports SSH tunnels

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

#### Scenario: Backward compatibility for missing `kind`
- **WHEN** a request is sent to `/api/proxies/test` without a `kind` field
- **THEN** the system SHALL treat it as `kind: "socks5"` (preserving existing behavior)
- **AND** SOCKS5 proxy testing SHALL work as before

#### Scenario: Request payload structure
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

#### Scenario: Response format consistency
- **WHEN** the test completes (success or failure)
- **THEN** the response SHALL be JSON with `Content-Type: application/json`
- **AND** on success: `{ "success": true }` with HTTP 200
- **AND** on failure (non-validation): `{ "success": false, "error": "..." }` with HTTP 200
- **AND** on validation error: `{ "error": "..." }` with HTTP 400 Bad Request

This matches the response format of the saved proxy test endpoint (`/api/proxies/:id/test`) for consistency.

#### Scenario: Frontend coordination
- **WHEN** the admin UI sends a test request for an SSH tunnel
- **THEN** it SHALL include all required SSH fields: `kind: "ssh"`, `auth_method`, `private_key_path` (if key auth) or `password` (if password auth), `ssh_port`, `host_key_check`
- **AND** may also include `host_key_fingerprint` if available
- **AND** the UI shall interpret the response and display success or error to the user
