## Why

The TUN device routing feature adds significant complexity to the codebase with interface discovery, status monitoring, configuration, and UI handling. It requires root-like privileges and special network setup, making deployment harder. The feature likely sees minimal usage compared to SOCKS5/EXTERNAL proxies. Removing TUN support simplifies the architecture, reduces the attack surface, and makes the project easier to maintain and deploy.

## What Changes

**BREAKING CHANGES:**
- Remove `tun_device` configuration field from proxy config
- Remove `TUN` as a valid route strategy from PAC rules
- Remove TUN IP discovery logic
- Remove TUN status reporting (device, IP, up/down) from status API and UI
- Remove TUN option from rule editor in admin UI
- Remove `use_tun` flag from GFW configuration (if present)
- Router will no longer support binding connections to a specific interface

Systems that previously used TUN routes must migrate to SOCKS5 or EXTERNAL proxy strategies.

## Capabilities

### Modified Capabilities
- `pac-proxy-tun`: The existing TUN routing capability will be **removed**. The capability spec will be updated to reflect that routing is limited to DIRECT, SOCKS5, EXTERNAL, and DEFAULT strategies only.

## Impact

**Affected code areas:**
- `pac/config.go` - Remove TUN-related fields from config structs
- `router/router.go` - Remove TUN case from dialWithRoute, delete ErrTUNNotConfigured
- `proxy/server.go` - Remove TUN IP tracking and SetTUNIP
- `cmd/proxy-pac/main.go` - Remove TUN discovery and configuration
- `admin/handlers.go` - Remove TUN device from config APIs
- `admin/static/index.html` - Remove TUN device field and TUN route option
- `admin/static/script.js` - Remove TUN from route options and UI logic
- `status/status.go` - Remove TUN fields from ServerStatus
- `README.md` and `config.yaml.example` - Update documentation and examples

**Tests to update/remove:**
- All tests referencing TUN routes or TUN device (in pac/, integration_test.go, gfw/ tests)

**Configuration:**
- Config files with `tun_device` will be invalid; startup should error with clear message
- PAC files containing `"route":"TUN"` will be rejected during load
