# Why

Fix the HTTPS proxy implementation to work correctly for all destination ports and simplify the architecture by removing unused TLS-specific routing strategies.

## What Changes

- **Fix**: Unified CONNECT handler that detects TLS via connection type, not destination port
- **Remove**: TLS_* route strategies (`TLS_TUN`, `TLS_SOCKS5`, `TLS_DIRECT`)
- **Remove**: Unused `tlsSNI()` builtin from PAC (SNI not needed for routing)
- **Confirm**: `Evaluator.Evaluate(host)` signature remains unchanged; no SNI parameter

## Capabilities

### Modified Capabilities
- `proxy-server`: Correct HTTPS proxy behavior for all ports
- `pac-evaluation`: No functional change (SNI not used)
- `router-logic`: Simplified route strategies

## Impact

- **Breaking**: Configs using `TLS_TUN`, `TLS_SOCKS5`, or `TLS_DIRECT` will default to `DIRECT` (should update to `TUN`, `SOCKS5`, `DIRECT`)
- **Fixed**: HTTPS proxy now works correctly for all CONNECT destinations (both port 443 and non-443)
- **Simplified**: Codebase decluttered with removal of TLS_* strategies and unused SNI-based routing
- **Compatible**: Existing PAC scripts and configs continue to work unchanged
