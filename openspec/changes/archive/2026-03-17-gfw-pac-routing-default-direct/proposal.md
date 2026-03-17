## Why

Currently, GFW blacklist rules generate PAC configurations that route traffic to a hardcoded SOCKS5 proxy address (127.0.0.1:1080). When no proxy is running at that address, the connection fails and the router falls back to DIRECT after logging an error. This creates unnecessary connection attempts, error logs, and latency. The desired behavior is to use the configured default external proxy when available, and fall back to direct connection when no proxy is configured, without attempting connections to hardcoded addresses.

## What Changes

- Modify GFW rule parsing to return empty Route for blacklist entries (instead of hardcoded proxy)
- Update PAC generation logic to return "DEFAULT" when default external proxy is configured, otherwise "DIRECT"
- Whitelist rules continue to return "DIRECT" as before
- No changes to router logic - leverages existing fallback mechanism

## Capabilities

### Modified Capabilities
- `gfw-pac-generation`: Change how GFW rules are converted to PAC routing directives
  - Blacklist rules now respect default external proxy configuration
  - When no default proxy configured, blacklist rules route DIRECT instead of attempting connection to hardcoded proxy

## Impact

- GFW fetcher and PAC generator modules (`gfw/` package)
- PAC evaluation behavior for GFW-sourced rules
- Reduced unnecessary connection attempts and error logs when default proxy not configured
- More semantically correct routing: no proxy configured = direct connection