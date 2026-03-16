## Why

The current proxy supports routing through TUN devices, hardcoded SOCKS5 upstream addresses, or DIRECT. Administrators need to manage multiple upstream SOCKS5 proxies (e.g., Tor, commercial VPNs) and assign them to rules easily. Hardcoding addresses in each rule is inflexible and forces updates when proxy credentials change. This change introduces a central **external proxy registry** and a **default external proxy**, enabling rules to reference named proxies and simplifying configuration.

## What Changes

- Add `proxy.external_proxies` list in config YAML (id, address, username, password)
- Add `proxy.default_external_proxy` setting (optional)
- Extend `pac.Route` struct with `ExternalID` field
- Update `parsePACResult` to recognize `"EXTERNAL <id>"` and `"DEFAULT"` tokens
- Modify `Router` to lookup upstream proxies by ID and support optional SOCKS5 authentication
- Change PAC generator to use `DEFAULT` for rules with empty route when a default proxy is configured; otherwise preserve legacy `SOCKS5 127.0.0.1:1080` behavior
- Add admin UI: new Proxies tab (CRUD + connectivity test), default proxy selector, route dropdown updates (DIRECT, TUN, DEFAULT, EXTERNAL entries, legacy SOCKS5)
- Add API endpoints for proxy management and testing
- Enforce validation: external proxy addresses must not point to this proxy's own listening ports on localhost, IDs must be unique and reserved words (DIRECT, TUN, DEFAULT) are disallowed

**BREAKING**: None — all existing configurations and PAC scripts continue to work unchanged.

## Capabilities

### New Capabilities
- `external-proxy-registry`: Define and manage a named set of upstream SOCKS5 proxies in configuration and admin UI, with optional authentication. These proxies are used as outbound routes but are distinct from the proxy's own listening ports.
- `default-external-proxy`: Select one of the registered external proxies as the default. Rules that do not explicitly specify a route (common for GFW-generated rules) automatically use this default. The special PAC return value `"DEFAULT"` can also be used in rules to explicitly select the default.

### Modified Capabilities
- `pac-proxy-tun`: Update the `Route` structure to include an `ExternalID` field; extend supported PAC return values to include `"EXTERNAL <id>"` and `"DEFAULT"`; router resolves external proxies via a registry and performs SOCKS5 authentication when credentials are provided.

## Impact

- **Config**: YAML schema gains `proxy.external_proxies[]` and `proxy.default_external_proxy`
- **Packages**: `pac/config.go`, `pac/parser.go`, `router/router.go`, `gfw/generator.go`, `admin/handlers.go`
- **Admin UI**: New Proxies tab and Settings tab; route dropdown in Rules tab becomes dynamic
- **API**: New endpoints under `/api/proxies` (CRUD + test) and extension to `/api/config` for default proxy
- **socks5 package**: Extract common handshake logic, support username/password authentication