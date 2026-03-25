## Context

The current SSH tunnel implementation in `ssh/tunnel.go` provides basic connection lifecycle management with exponential backoff reconnection on failure. However, it relies on I/O errors to detect tunnel health, which means an SSH connection can become silently broken (e.g., network idle timeout, NAT state loss, server-side disconnect) without immediate detection. Clients may experience hangs or connection errors only when trying to send actual data. This design adds an optional proactive health check mechanism via keepalive messages.

Constraints:
- Must be backward compatible: existing SSH proxies without keepalive config must work unchanged
- Must not introduce significant performance overhead or blocking in the hot path
- Must integrate with existing tunnel state machine (`connected`, `reconnecting`, `failed`)
- No new external dependencies

## Goals / Non-Goals

**Goals:**
- Detect dead SSH tunnels proactively within a configurable time window
- Trigger automatic reconnection before clients are affected
- Provide configurable keepalive interval per proxy (enabled + interval)
- Maintain low overhead (minimal CPU, memory, network)
- Preserve all existing observable behaviors (public API, error types, blocking semantics)

**Non-Goals:**
- Changing the SSH tunnel API or existing failure modes
- Adding keepalive to SOCKS5 or TUN routes (out of scope)
- Implementing application-layer keepalives beyond SSH-level probes
- Supporting per-channel keepalive (global connection-level only)
- Changing reconnection backoff logic (reuse existing)
- Making timeout or max_failure configurable (fixed: 5s, 3)

## Decisions

**1. Keepalive Mechanism: SSH Global Request "keepalive@openssh.com"**

We will use the SSH global request `keepalive@openssh.com` as the keepalive probe. This is widely supported by OpenSSH and compatible SSH servers.

Alternatives considered:
- `tcp-keepalive@openssh.com`: more specific but same effect
- Sending data on a new channel: too heavyweight
- Custom global request: less portable

Rationale: Global requests don't require a channel; they are connection-level with minimal overhead. `keepalive@openssh.com` is standard and safe.

**2. TCP Keepalive: Set via net.Dialer.KeepAlive**

To detect network-level failures even if SSH keepalive blocks, we will set TCP keepalive on the underlying socket when establishing the connection:

```go
dialer := &net.Dialer{
    Timeout:   30 * time.Second,
    KeepAlive: 30 * time.Second,
}
conn, err := dialer.Dial("tcp", addr)
```

This enables the OS to send TCP-level keepalive probes, providing a safety net for half-open connections.

**3. Implementation: One Goroutine Per Tunnel**

Each tunnel spawns a single keepalive goroutine when entering `connected` state:
- Sleeps for `keepalive_interval`
- For each tick: creates `context.WithTimeout(parent, 5s)`, launches goroutine to send `client.SendRequest("keepalive@openssh.com", true, nil)` with result on channel, parent selects with timeout to avoid goroutine leaks
- On error (including timeout): logs warning, increments `keepaliveFailures`
- On success: resets `keepaliveFailures` to 0
- After 3 consecutive failures: logs error, closes the SSH client (triggers reconnection), exits

The goroutine stops when the tunnel closes or transitions to `reconnecting`.

Alternatives considered:
- Shared goroutine for multiple tunnels: adds complexity
- Using `wantReply=false`: only detects local write errors, not server liveness

Rationale: Simple, isolated, low-overhead. The per-t ping timeout prevents indefinite blocking if server doesn't respond.

**4. Simplified Configuration**

Add to `pac.ExternalProxy`:

```go
type KeepaliveConfig struct {
    Enabled  bool          `yaml:"enabled" default:"false"`
    Interval time.Duration `yaml:"interval" default:"30s"`
}
// Embedded:
Keepalive KeepaliveConfig `yaml:"keepalive,omitempty"`
```

Fixed behavior when enabled:
- Ping timeout: 5 seconds
- Max consecutive failures: 3

Rationale: Simpler UI, fewer decisions for users. Most operators just want "enable keepalive with reasonable defaults".

**5. Lifecycle Integration**

- Keepalive starts in `connect()` after `setClient(client)` marks tunnel `connected`
- Keepalive stops in `setClient()` before swapping clients (on reconnection) and in `Close()`
- Keepalive failures cause `client.Close()`, which triggers manager's reconnection via `failCh` signal

## Risks / Trade-offs

**[Risk]** Keepalive adds ~30s interval network overhead (one SSH global request per tunnel). With many tunnels, aggregate traffic could be noticeable.

**Mitigation**: Interval is configurable; users can increase for scarce networks.

**[Risk]** False positives: transient network glitch could trigger reconnection unnecessarily.

**Mitigation**: 3 consecutive failures with 5s timeout each = at least 30s detection window; tolerates brief outages.

**[Risk]** Server may not support `keepalive@openssh.com`. Responses with failure.

**Mitigation**: Failure counts toward threshold; after 3 failures, we reconnect. Server issue, not client bug.

**[Risk]** Goroutine leak if keepalive never stops.

**Mitigation**: Explicit cancellation on tunnel state transitions; tests will verify cleanup.

## Migration Plan

Additive and fully backward compatible:
1. Deploy new binary
2. Existing SSH proxies work unchanged (keepalive disabled)
3. Admins optionally enable keepalive per proxy via Admin UI or API
4. Rolling upgrades safe

Rollback: disable per proxy or revert binary.

## Open Questions

1. **Should we log keepalive failures?** Yes, at INFO or DEBUG level to aid troubleshooting.
2. **Should success be logged?** No, too noisy.
3. **What if server rejects keepalive consistently?** It will reconnect repeatedly. Admin should fix server config or disable keepalive.
