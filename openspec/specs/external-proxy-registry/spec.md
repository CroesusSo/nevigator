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
