## ADDED Requirements

### Requirement: SSH Tunnel Proxy Support

The system SHALL support SSH tunnels as an external proxy type alongside SOCKS5. An SSH proxy SHALL be established by creating an SSH connection to the target server and using `ssh.Client.Dial` to multiplex TCP channels directly. No local SOCKS5 bridge SHALL be used.

#### Scenario: SSH proxy configuration
- **WHEN** an administrator defines an external proxy with `"kind": "ssh"`
- **THEN** the system SHALL interpret the proxy as an SSH tunnel
- **AND** require the following fields:
  - `address`: SSH server host, optionally including `user@` prefix and/or `:port` (e.g., `"ssh.example.com:2222"`, `"user@host"`, `"user@host:2222"`)
  - `ssh_port` (optional, overrides any port in `address`, default 22)
  - `username` (if not extracted from `address`'s `user@` prefix)
  - `auth_method`: either `"password"` or `"key"`
  - For `"password"` auth: `password` field required
  - For `"key"` auth: `private_key_path` field required (path to unencrypted PEM private key)
- **AND** optionally support `host_key_check` (`"strict"`, `"ignore"`) and `host_key_fingerprint` for pinning

#### Scenario: Router uses SSH proxy
- **GIVEN** a PAC rule returns `"EXTERNAL my-ssh-proxy"`
- **AND** the registry contains `my-ssh-proxy` with `kind: "ssh"`
- **WHEN** the router processes a connection request
- **THEN** the router SHALL call `tunnelMgr.Dial("my-ssh-proxy", destination)`
- **AND** the tunnel manager SHALL ensure an SSH connection is established (lazily on first use)
- **AND** return a `net.Conn` through the SSH channel to the destination
- **AND** if the SSH connection fails or the tunnel cannot be established, the router SHALL fallback to DIRECT (as with SOCKS5 failures)

#### Scenario: SSH connection lifecycle
- **WHEN** the first `Dial` request arrives for a given SSH proxy
- **THEN** the tunnel manager SHALL create a new `Tunnel` and initiate SSH handshake in a background goroutine
- **AND** the `Dial` call SHALL block until the tunnel is connected or fails (with a timeout derived from the router's dial timeout)
- **WHEN** subsequent `Dial` calls arrive while the tunnel is connected
- **THEN** they SHALL use the existing SSH connection immediately without blocking
- **WHEN** the SSH connection encounters an error (e.g., EOF from server)
- **THEN** the tunnel SHALL enter `reconnecting` state
- **AND** future `Dial` calls SHALL block while reconnection attempts happen with exponential backoff
- **WHEN** reconnection succeeds
- **THEN** the tunnel SHALL return to `connected` state and unblock waiters

#### Scenario: SSH authentication with password
- **GIVEN** a proxy with `auth_method: "password"` and `password` set
- **WHEN** the tunnel establishes the SSH connection
- **THEN** the client SHALL authenticate using the provided password
- **IF** authentication fails
- **THEN** the tunnel SHALL record a permanent error and stop retrying
- **AND** subsequent `Dial` calls SHALL return that error until the proxy configuration is updated

#### Scenario: SSH authentication with private key
- **GIVEN** a proxy with `auth_method: "key"` and `private_key_path` pointing to a PEM file
- **WHEN** the tunnel establishes the SSH connection
- **THEN** the client SHALL read the private key and authenticate using public key auth
- **IF** the key file is not readable or invalid
- **THEN** the tunnel SHALL fail with an appropriate error and stop retrying

#### Scenario: Host key verification (strict)
- **GIVEN** a proxy with `host_key_check: "strict"` (default) and no `host_key_fingerprint`
- **WHEN** the tunnel establishes the SSH connection
- **THEN** the client SHALL use the system `~/.ssh/known_hosts` file to verify the server's host key
- **IF** the host key is not found in known_hosts
- **THEN** the connection SHALL fail with a host key verification error
- **IF** `host_key_fingerprint` is provided (hex string)
- **THEN** the client SHALL compare the server's key fingerprint to the provided value
- **AND** fail if they do not match

#### Scenario: Host key verification (ignore)
- **GIVEN** a proxy with `host_key_check: "ignore"`
- **WHEN** the tunnel establishes the SSH connection
- **THEN** the client SHALL accept any host key without verification (insecure)

#### Scenario: Proxy removal triggers tunnel closure
- **WHEN** an administrator deletes an SSH proxy from the registry
- **THEN** the tunnel manager SHALL close the corresponding tunnel's SSH connection
- **AND** remove the tunnel from its map
- **AND** any blocked `Dial` calls SHALL return an error immediately

#### Scenario: Proxy update forces tunnel recreation
- **WHEN** an SSH proxy's configuration is updated (e.g., changed port or key path)
- **THEN** the tunnel manager SHALL close the existing tunnel (if any)
- **AND** create a new tunnel with the updated configuration on next `Dial`
- **AND** any blocked `Dial` calls on the old tunnel SHALL error, allowing the router to fallback to DIRECT

#### Scenario: IPv6 support
- **GIVEN** an SSH proxy with IPv6 address (e.g., `address: "[2001:db8::1]:2222"`)
- **WHEN** the tunnel dials the SSH server
- **THEN** it SHALL correctly parse and format the address with brackets
- **AND** when dialing a destination through the tunnel, it SHALL also handle IPv6 destinations correctly

#### Scenario: Port precedence
- **GIVEN** a proxy with `address: "host:2222"` and `ssh_port: 3333`
- **WHEN** the tunnel dials the SSH server
- **THEN** it SHALL use port `3333` (the explicit `ssh_port` takes precedence over the port in `address`)
- **AND** if `ssh_port` is omitted, the port from `address` (if present) SHALL be used
- **AND** if neither provides a port, default to 22

#### Scenario: Graceful shutdown
- **WHEN** the proxy server is shutting down
- **THEN** the router's `Stop` method SHALL instruct the tunnel manager to close all tunnels
- **AND** the tunnel manager SHALL close all `ssh.Client` connections
- **AND** any blocked `Dial` calls SHALL return an error
- **AND** the shutdown SHALL complete within a reasonable timeout

#### Scenario: Backward compatibility
- **WHEN** a configuration does not specify `kind` for an external proxy
- **THEN** the system SHALL treat it as `kind: "socks5"` (default)
- **AND** all existing SOCKS5 proxies SHALL continue to work unchanged
- **AND** older binaries (without `kind` support) SHALL ignore the field and treat SSH proxies as SOCKS5, causing connection failures but not crashes

### Requirement: SSH tunnel worker pool implementation

The system SHALL replace the mutex-based `ssh.Tunnel` implementation with a worker-pool, atomic-state design that eliminates lock contention in the hot path while preserving the public API and all existing observable behaviors.

#### Scenario: API compatibility
- **WHEN** any existing code calls `Tunnel.Dial` or `Tunnel.Close`
- **THEN** the calls SHALL behave identically from the caller's perspective (same error types, same blocking semantics, same reconnection behavior)

#### Scenario: Existing tests pass
- **WHEN** the existing SSH tunnel test suite is executed without modification
- **THEN** all tests SHALL pass

#### Scenario: Performance improvement
- **WHEN** a high volume of concurrent HTTPS requests route through an SSH tunnel
- **THEN** the system SHALL demonstrate reduced tail latency and higher throughput compared to the mutex-based implementation
- **AND** no deadlocks or data races SHALL occur under load
