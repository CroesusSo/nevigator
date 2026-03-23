## ADDED Requirements

### Requirement: SSH Authentication Methods

The system SHALL support two SSH authentication methods: password-based and private key-based. These methods SHALL be explicitly declared via `auth_method` in the proxy configuration. The system SHALL NOT attempt to infer the method automatically.

#### Scenario: Password authentication
- **GIVEN** a proxy with `auth_method: "password"` and a non-empty `password`
- **WHEN** the SSH connection is established
- **THEN** the client SHALL send `ssh.Password` as the authentication method
- **AND** the provided password SHALL be used
- **IF** the server rejects the password
- **THEN** the connection SHALL fail with an authentication error
- **AND** the tunnel SHALL mark the proxy as permanently failed (no further retries)

#### Scenario: Private key authentication
- **GIVEN** a proxy with `auth_method: "key"` and `private_key_path` set to a readable PEM file
- **WHEN** the SSH connection is established
- **THEN** the client SHALL read the private key from the file
- **AND** parse it using `ssh.ParsePrivateKey`
- **AND** offer the corresponding public key to the server
- **AND** upon successful challenge, complete authentication

#### Scenario: Private key file must be readable
- **GIVEN** a proxy with `auth_method: "key"` and `private_key_path` pointing to a non-existent or unreadable file
- **WHEN** the tunnel attempts to connect
- **THEN** the connection SHALL fail with an error indicating file read failure
- **AND** the tunnel SHALL mark the proxy as permanently failed

#### Scenario: Private key file permissions warning
- **WHEN** a private key file with overly permissive permissions (world-readable) is loaded
- **THEN** the system SHALL log a warning (but still use the key)
- **RATIONALE**: SSH is strict about key permissions; we should inform the operator but not block operation

#### Scenario: Unencrypted private keys only
- **GIVEN** a proxy with `auth_method: "key"` and an encrypted private key (passphrase-protected PEM)
- **WHEN** the tunnel attempts to parse the key
- **THEN** `ssh.ParsePrivateKey` SHALL fail with a passphrase error
- **AND** the connection SHALL fail
- **RATIONALE**: Passphrase management is out of scope for MVP; operators should use unencrypted keys or switch to password auth

#### Scenario: Missing authentication credentials
- **GIVEN** a proxy with `kind: "ssh"` but no `auth_method` or incomplete credentials (e.g., `auth_method: "key"` but no `private_key_path`)
- **WHEN** the tunnel attempts to connect
- **THEN** the connection SHALL fail with a configuration error
- **AND** the tunnel SHALL mark the proxy as permanently failed

#### Scenario:fallback to password when key fails? NO
- **GIVEN** a proxy with `auth_method: "key"` but the key is invalid
- **WHEN** authentication fails
- **THEN** the tunnel SHALL NOT automatically try `auth_method: "password"`
- **RATIONALE**: The configuration explicitly selects one method; mixing methods is ambiguous and a configuration error

#### Scenario: Multiple authentication methods
- **GIVEN** an SSH server requiring keyboard-interactive or other methods
- **WHEN** the client only offers password or public key
- **THEN** authentication MAY fail if the server does not accept those methods
- **AND** this is considered a configuration mismatch; operator must align proxy `auth_method` with server expectations

#### Scenario: Username handling
- **GIVEN** a proxy with `address: "user@host"` and a separate `username` field
- **WHEN** the tunnel parses the configuration
- **THEN** the `username` from the `address` (if present) SHALL override the `username` field
- **RATIONALE**: `user@host` syntax is convenient; it takes precedence to avoid ambiguity

#### Scenario: Empty username
- **GIVEN** a proxy with no `username` and `address` without `user@` prefix
- **WHEN** the tunnel attempts to authenticate
- **THEN** the SSH client SHALL use the current OS user as the SSH username (as per `ssh` default behavior)
- **WARNING**: This may not match the intended account on the remote server; administrators should explicitly set `username`

#### Scenario: Password reuse between SOCKS5 and SSH
- **GIVEN** a proxy with `kind: "ssh"` and `password` set for SSH password auth
- **THEN** the same `password` field SHALL NOT be interpreted as a SOCKS5 password (since SSH proxies do not use SOCKS5 between router and tunnel)
- **RATIONALE**: The `ExternalProxy` struct reuses `Password` for both SOCKS5 and SSH. Context (proxy.Kind) determines interpretation. This is acceptable as long as documentation clarifies.

#### Scenario: Credential storage security
- **WHEN** an SSH proxy is configured with a password or private key path
- **THEN** these sensitive strings SHALL NOT be logged or exposed via admin API responses (masks in list view)
- **AND** the configuration file SHOULD have restrictive filesystem permissions (e.g., 0600)
- **AND** the admin UI SHALL treat these fields as secrets (input type="password", no auto-complete)
