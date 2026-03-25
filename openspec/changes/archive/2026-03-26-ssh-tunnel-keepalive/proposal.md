## Why

SSH tunnels can become silently broken due to network issues, idle timeouts, or server-side disconnections without the client detecting the problem. This leads to failed requests and poor user experience until the connection is manually restored. Adding keepalive support enables proactive detection of dead tunnels and automatic reconnection, improving reliability and reducing downtime.

## What Changes

- Add optional keepalive configuration to SSH proxy definitions with fields:
  - `enabled` (boolean, default false)
  - `interval` (duration, e.g., "30s", default 30s)
- Implement keepalive goroutine per tunnel that periodically sends SSH global request `keepalive@openssh.com`
- Use fixed per-ping timeout of 5s and max 3 consecutive failures (hardcoded)
- Set TCP keepalive on underlying socket (30s interval) via custom net.Dialer during SSH connection establishment
- Modify tunnel lifecycle management to react to keepalive failures by closing connection and triggering reconnection with exponential backoff
- Update admin UI to expose keepalive settings (checkbox + interval input) for SSH proxies
- Preserve backward compatibility: existing SSH proxies without keepalive config continue to work unchanged

**BREAKING**: None. This is purely additive and fully backward compatible.

## Capabilities

### New Capabilities

*(None — this change extends existing capability)*

### Modified Capabilities

- `ssh-tunnel-proxy`: Add keepalive requirements including configuration schema, periodic probe mechanism (interval), failure detection (5s timeout, 3 max failures), and reconnection handling. This modifies the SSH tunnel lifecycle and adds new optional fields to the proxy configuration.

## Impact

- **Code**: tunnel (`ssh/tunnel.go`) needs keepalive implementation; config structs need new fields; admin UI handlers need to render keepalive settings
- **APIs**: REST API `/api/proxies` schema extended with optional `keepalive` object (`enabled`, `interval`)
- **Dependencies**: None beyond standard library
- **Testing**: New unit tests for keepalive goroutine, failure detection, and reconnection triggers; integration tests simulating network partitions
- **Documentation**: Admin UI help text, config examples, migration guide (none needed)
