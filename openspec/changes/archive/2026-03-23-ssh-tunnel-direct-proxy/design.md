## Context

The system currently supports external SOCKS5 proxies only. Users have requested SSH tunneling as an alternative proxy method, leveraging SSH's built-in dynamic port forwarding capability. This would allow routing traffic through SSH tunnels without requiring a separate SOCKS5 proxy server.

## Goals / Non-Goals

**Goals:**
- Extend external proxy registry to support SSH proxy type alongside SOCKS5
- Implement SSH connection management with authentication (password and private key)
- Route `EXTERNAL <id>` through SSH tunnels directly (no local SOCKS5 bridge)
- Maintain backwards compatibility with existing SOCKS5 external proxies
- Provide admin UI/API for SSH proxy configuration and testing

**Non-Goals:**
- Multi-hop SSH chains
- SSH agent forwarding
- Advanced SSH options like compression, keepalive intervals
- Shell access through the SSH connection
- Local SOCKS5 bridge or port forwarding

## Decisions

### 1. SSH Proxy Type in ExternalProxy

**Decision:** Extend `ExternalProxy` with a `Kind` field (`socks5` or `ssh`) and SSH-specific fields.

```go
type ExternalProxy struct {
    ID       string `yaml:"id" json:"id"`
    Kind     string `yaml:"kind,omitempty" json:"kind,omitempty"` // "socks5" (default) or "ssh"
    Address  string `yaml:"address" json:"address"`               // SOCKS5 host:port; for SSH: host[:port] or user@host[:port]
    Username string `yaml:"username,omitempty" json:"username,omitempty"`
    Password string `yaml:"password,omitempty" json:"password,omitempty"`

    // SSH-only (ignored if Kind != "ssh")
    SSHPort           int    `yaml:"ssh_port,omitempty" json:"ssh_port,omitempty"`         // default 22; overrides any port in Address
    AuthMethod        string `yaml:"auth_method,omitempty" json:"auth_method,omitempty"` // "password" or "key" (required if Kind=="ssh")
    PrivateKeyPath    string `yaml:"private_key_path,omitempty" json:"private_key_path,omitempty"`
    HostKeyCheck      string `yaml:"host_key_check,omitempty" json:"host_key_check,omitempty"`     // "strict" (default) or "ignore"
    HostKeyFingerprint string `yaml:"host_key_fingerprint,omitempty" json:"host_key_fingerprint,omitempty"` // optional hex fingerprint for pinning
}
```

**Rationale:** Keeps backward compatibility by making `Kind` default to `"socks5"`. Clean separation of concerns. Allows future extension (e.g., adding more proxy types).

#### Address and Port Parsing

For `Kind == "ssh"`, the `Address` field may take several forms:
- `"host"` → host = `host`, port = `SSHPort` or default 22
- `"host:port"` → host = `host`, port = `port` (unless `SSHPort` overrides)
- `"user@host"` → username = `user` (overrides `Username` field), host = `host`
- `"user@host:port"` → username = `user`, host = `host`, port = `port`

If both `Address` and `SSHPort` specify a port, `SSHPort` takes precedence. If neither provides a port, default to 22.

The `Username` field is used if not overridden by the `user@` prefix in `Address`. At least one must be non-empty; if both are empty, the SSH client will use the current OS user (not recommended).

#### Required Fields for SSH

When `Kind == "ssh"`, the following fields are **required**:
- `AuthMethod` must be either `"password"` or `"key"`
- If `AuthMethod == "password"`, `Password` must be non-empty
- If `AuthMethod == "key"`, `PrivateKeyPath` must be set and point to a readable PEM file

If any required field is missing, the configuration is invalid and the tunnel will not attempt to connect; the error should be surfaced via API validation and admin UI.

### 2. SSH Tunnel Implementation (Direct Dial)

**Decision:** For each SSH proxy, establish an SSH connection and use `ssh.Client.Dial` directly to multiplex channels. No local SOCKS5 bridge or port forwarding.

**Rationale:** Simpler architecture (fewer components). Lower latency (no extra handshake). Direct channel multiplexing is natively supported by `golang.org/x/crypto/ssh`. The router already has a branch for different proxy types; adding SSH as a direct case is straightforward.

**Alternatives considered:**
- *Local SOCKS5 bridge*: More complex—requires managing a local listener, SOCKS5 server, and bridging connections. Unnecessary indirection.
- *Use system ssh command*: External dependency, hard to manage, not portable.

### 3. SSH Tunnel Manager

**Decision:** Create an `ssh.Manager` that:
- Maintains a map of active SSH tunnels (`*ssh.Client`) by proxy ID
- Each tunnel is lazily established on first use
- Reconnects on failure with exponential backoff (transient failures only)
- Supports hot-reload: `UpdateProxies` closes removed tunnels, updates configs
- Provides `Dial(id, dest, timeout time.Duration) (net.Conn, error)` method

The `Dial` method SHALL block until either:
- The tunnel is connected and a channel to `dest` is successfully opened, OR
- The connection attempt (including any necessary tunnel establishment) exceeds `timeout`, OR
- A permanent error occurs (auth failure, host key mismatch, configuration error)

The timeout includes the SSH handshake and the subsequent `client.Dial` to the destination. It ensures that the router's overall `dialTimeout` is respected and that a hung tunnel connection does not block indefinitely.

**Tunnel state machine:**
```
┌─────────┐
│closed   │
└────┬────┘
     │ first Dial()
     ▼
┌─────────┐
│connecting│───SSH handshake───┐
└────┬────┘                 │
     │ success               │ failure (retryable)
     ▼                       │
┌─────────┐                  │
│connected│◄─────────────────┘
└────┬────┘
     │ channel error / EOF
     ▼
┌─────────┐
│reconnecting│ (backoff then → connecting)
└─────────┘
```

**Tunnel state machine:**
```
┌─────────┐
│closed   │
└────┬────┘
     │ first Dial()
     ▼
┌─────────┐
│connecting│───SSH handshake───┐
└────┬────┘                 │
     │ success               │ failure (retryable)
     ▼                       │
┌─────────┐                  │
│connected│◄─────────────────┘
└────┬────┘
     │ channel error / EOF
     ▼
┌─────────┐
│reconnecting│ (backoff then → connecting)
└─────────┘
```

**Rationale:** Centralized lifecycle management. Lazy initialization reduces resource usage. Reconnection improves reliability. Direct `Dial` method integrates cleanly with router.

### 4. Router Integration

**Decision:** Modify `router.Router`:
- Add `tunnelMgr *ssh.Manager` field (optional)
- Add `SetTunnelManager(*ssh.Manager)` method
- In `dialWithRoute`, for `EXTERNAL`/`DEFAULT`:
  - Look up proxy from registry
  - If `proxy.Kind == "ssh"` → `return r.tunnelMgr.Dial(proxy.ID, dest)`
  - Else (socks5) → `return r.dialSOCKS5WithAuth(dest, proxy.Address, ...)`

**Rationale:** Minimal changes. Router already distinguishes SOCKS5 from TUN/DIRECT. Adding SSH as another branch is natural. Error handling and fallback to DIRECT remain unchanged.

### 5. Configuration

**Decision:** Extend `config.yaml` schema:

```yaml
proxy:
  external_proxies:
    - id: "ssh-server"
      kind: "ssh"                # default: "socks5"
      address: "user@host" or "host"
      ssh_port: 22               # optional, default 22
      username: "user"           # required (or extracted from address)
      auth_method: "password" | "key"
      password: "secret"         # if auth_method=password
      private_key_path: "/path/to/key"  # if auth_method=key
      host_key_check: "strict" | "ignore"  # default "strict"
```

**Rationale:** Familiar YAML consistent with existing proxies. Reuses `username`/`password` for SSH password auth. Explicit `kind` avoids ambiguity.

### 6. Host Key Verification

**Decision:** Default `host_key_check: "strict"` uses system `~/.ssh/known_hosts`. Alternatively, allow fingerprint pinning via `host_key_fingerprint` field. `ignore` disables verification (insecure).

**Rationale:** Security best practice: verify host keys. Known_hosts is familiar to SSH users. Fingerprint pinning is useful for headless or automated deployments.

### 7. Admin UI & API

**Decision:** Extend existing external proxy API (`/api/proxies`) to accept `kind` and SSH-specific fields. In UI, add SSH type option in proxy form with conditional fields. Test endpoint:
- For SSH: attempt `tunnelMgr.Dial(id, "1.1.1.1:80")` and close immediately
- For SOCKS5: existing SOCKS5 handshake test

**Rationale:** Leverages existing patterns. Minimal UI changes.

## Architecture

```
Client → Proxy → PAC Eval → Route(EXTERNAL "ssh-gfw")
                                ↓
                         Router.dialWithRoute
                                ↓
                  ┌──────────────┴──────────────┐
                  ↓                             ↓
          proxy.Kind=="socks5"           proxy.Kind=="ssh"
                  ↓                             ↓
         dialSOCKS5WithAuth()          tunnelMgr.Dial(id, dest, timeout)
                                                     ↓
                                              ssh.Client.Dial("tcp", dest)
```

**Tunnel Manager:**
```
Manager {
  proxies map[id]ExternalProxy    // config snapshot
  tunnels map[id]*Tunnel          // runtime state
}

Tunnel {
  mu        sync.Mutex
  cfg       ExternalProxy
  client    *ssh.Client
  backoff   time.Duration
  closing   bool
  cond      *sync.Cond            // for state notifications
}
```

## Logging

The system SHALL emit structured logs for SSH tunnel events at appropriate levels:

- **INFO**: Tunnel state transitions (connecting → connected, reconnecting → connected, tunnel closed)
- **WARNING**: Recoverable errors triggering reconnection (network down, connection refused)
- **ERROR**: Permanent errors (authentication failure, host key mismatch, invalid config, key file read error)
- **DEBUG** (optional): Detailed handshake traces (disabled by default)

All log messages SHALL include the proxy `id` for correlation. Example:

```
[INFO] SSH tunnel "ssh-gfw" connecting to user@ssh.example.com:22
[ERROR] SSH tunnel "ssh-gfw" authentication failed: password rejected
[WARNING] SSH tunnel "ssh-gfw" disconnected; reconnecting in 2s
```

Rationale: Operators need visibility into tunnel health. Including the proxy ID allows filtering and correlation with PAC rules.

## Risks / Trade-offs

**Risk:** SSH private key security (file permissions, exposure)
- Mitigation: Enforce strict file permissions (0600); never log key content; admin UI uploads to secure temp files

**Risk:** Permanent authentication failures lock users out
- Mitigation: Tunnel stops retrying after auth/hostkey failures; admin UI shows clear error; config change triggers reconnect attempt

**Risk:** SSH server down causes transient failures
- Mitigation: Exponential backoff with jitter; per-tunnel single-fight to avoid thundering herd

**Risk:** Resource leaks on rapid config churn
- Mitigation: `Tunnel.Close()` idempotent; `ssh.Client.Close()` closes all channels; ensure manager removes closed tunnels from map

**Risk:** Goroutine leaks in `Dial` waiters
- Mitigation: Use condition variable broadcast on tunnel closure; waiters check `closing` flag and return promptly

**Risk:** IPv6 addresses not handled correctly
- Mitigation: Use `net.JoinHostPort` for both SSH target and destination; test IPv6 scenarios

## Migration Plan

1. Deploy new binary with SSH support; existing configs unchanged
2. Admins add SSH proxies via API/UI with `kind: "ssh"`
3. Router establishes tunnels on first use; fallback to DIRECT on failure
4. PAC generator emits `"EXTERNAL <id>"` as before
5. No data migration (optional `kind` field defaults to socks5)
6. Downgrade: older binary ignores `kind` and treats all as SOCKS5; SSH proxies fail but system remains functional for SOCKS5

## Open Questions

1. Should we support SSH without explicit port? Yes, default 22.
2. Should `user@host` syntax set `Username`? Yes, parse and override config field.
3. Encrypted private keys (passphrase)? Not in MVP; assume unencrypted keys.
4. Host key verification: use known_hosts or fingerprint? Support both; default known_hosts if exists, else fail unless `strict` allows first-use trust? Simpler: require known_hosts entry or fingerprint; or `ignore` explicitly.
5. Should we expose tunnel metrics (connection count, bytes)? Possibly via `/api/status` extension.
6. Keepalive? Not in MVP; could add periodic `SendRequest("keepalive@openssh.com")` later.
7. Auth method selection: auto-detect from key file presence? Prefer explicit config to avoid ambiguity.

## Dependencies

- `golang.org/x/crypto/ssh` (imported in new `ssh/manager.go`)

## Files to Modify

- `pac/config.go`: Extend `ExternalProxy`
- `router/router.go`: Add `tunnelMgr` field, `SetTunnelManager`, modify `dialWithRoute`
- `cmd/proxy-pac/main.go`: Create manager if any SSH proxies; wire into router; ensure shutdown
- `admin/handlers.go`: Extend proxy config schema, validation, test endpoint
- `go.mod`: Add `golang.org/x/crypto/ssh`

## New Files

- `ssh/manager.go`: Manager implementation
- `ssh/tunnel.go`: Tunnel state machine and Dial method
- Possibly `ssh/config.go` for parsing SSH-specific config fields
