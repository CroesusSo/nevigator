## REMOVED Requirements

### Requirement: TUN routing support
The system SHALL support routing traffic through a TUN device by binding the outbound connection to the TUN interface IP.

**Reason**: TUN device support is being removed to simplify the codebase, reduce deployment complexity, and focus on network-based proxy routing.
**Migration**: Users requiring VPN-like routing MUST deploy a SOCKS5 or EXTERNAL proxy on the VPN gateway and route to that proxy instead of using TUN. Existing TUN routes in PAC files must be manually replaced with SOCKS5 or EXTERNAL routes.

#### Scenario: TUN route selection removed
- **WHEN** a PAC rule previously returned "TUN"
- **THEN** the router would bind the outbound connection to the TUN interface IP
- **NOW REMOVED**: The system SHALL reject PAC files containing any rule with `"route":"TUN"` during evaluator initialization.

### Requirement: TUN device configuration
The proxy configuration SHALL include an optional `tun_device` field specifying the TUN interface name (e.g., "tun0").

**Reason**: With TUN routing removed, the `tun_device` configuration is obsolete.
**Migration**: Remove the `tun_device` field from all configuration files. No replacement is provided.

#### Scenario: TUN device IP discovery removed
- **WHEN** the proxy started with a `tun_device` configured
- **THEN** the main function would call `discoverTUNIP` to obtain the interface IP
- **NOW REMOVED**: The system SHALL fail to start if `tun_device` is present in the configuration, returning error: "TUN device support has been removed. Remove 'tun_device' from proxy config."

### Requirement: TUN status reporting
The system SHALL report TUN device information in the Status API and Admin UI, including `tunDevice`, `tunIP`, and `tunUp` fields.

**Reason**: TUN status is no longer relevant after feature removal.
**Migration**: Status consumers must no longer expect TUN-related fields. The Admin UI Summary Tab will not display TUN device information.

#### Scenario: Status API TUN fields removed
- **WHEN** a client requests `/api/status`
- **THEN** the response JSON SHALL NOT contain `tunDevice`, `tunIP`, or `tunUp` keys
- **EXISTING**: Previously these fields indicated the configured TUN device name, discovered IP, and whether the interface was up.

### Requirement: GFW `use_tun` flag
The GFW fetcher configuration may include a `use_tun` boolean to control whether HTTP client binds to TUN interface.

**Reason**: The distinction between TUN and system interface for GFW fetching is moot after TUN removal. All HTTP clients use system default routing.
**Migration**: Remove the `use_tun` field from GFW configuration if present. The fetcher always uses system network interfaces.

## MODIFIED Requirements

### Requirement: Routing strategies
The system SHALL support multiple routing strategies for outbound connections, including at minimum: TUN, SOCKS5, DIRECT, EXTERNAL, and DEFAULT.

**Modified to**: The system SHALL support the following routing strategies: SOCKS5, DIRECT, EXTERNAL, and DEFAULT. TUN routing is no longer supported. The `Route.Strategy` type remains a string, but the only valid values are `"SOCKS5"`, `"DIRECT"`, `"EXTERNAL"`, and `"DEFAULT"`.

**Impact on Interface**: The `Route` struct in `pac/config.go` remains unchanged, but code handling `Strategy == "TUN"` SHALL be removed. The router's `dialWithRoute` function SHALL no longer have a `case "TUN":` branch.

#### Scenario: Route evaluation excludes TUN
- **WHEN** the evaluator processes PAC rules
- **THEN** if any rule has `"route":"TUN"`, the native engine construction SHALL fail with error: "TUN routing is no longer supported. Use SOCKS5, EXTERNAL, or DIRECT instead."
- **AND** the system SHALL not start until the PAC file is corrected.

### Requirement: Configuration schema
The proxy configuration section (`proxy:`) MAY include a `tun_device` string field for TUN interface name.

**Modified to**: The `proxy:` configuration section SHALL NOT include a `tun_device` field. The presence of this field during configuration loading SHALL cause an error and prevent startup.

**New Error Condition**: In `cmd/proxy-pac/main.go → loadConfig()`, after unmarshaling, if `cfg.Proxy.TUNDevice != ""`, return an error with a clear message indicating TUN support has been removed.

#### Scenario: Rejection of legacy config
- **WHEN** an administrator starts the proxy with a config containing `tun_device: tun0`
- **THEN** the binary SHALL exit with non-zero status, logging: "ERROR: TUN device support has been removed. Remove 'tun_device' from the proxy configuration and update any PAC rules using TUN."

### Requirement: PAC file format and validation
PAC files may contain structured rules in `__PAC_RULES__` with a `route` field that can be "TUN", "SOCKS5 host:port", "EXTERNAL <id>", "DEFAULT", or "DIRECT".

**Modified to**: The allowable values for the `route` field in PAC rules SHALL be: "SOCKS5 host:port", "EXTERNAL <id>", "DEFAULT", "DIRECT". The value "TUN" is invalid and SHALL cause the PAC evaluator to fail to load the file.

#### Scenario: PAC load rejects TUN routes
- **WHEN** `pac.NewNativeRuleEngine` is called with PAC content containing any rule where `route == "TUN"`
- **THEN** the function SHALL return an error: "PAC contains unsupported TUN route at rule ID <id>. TUN routing has been removed; use SOCKS5 or EXTERNAL proxy instead."
- **AND** the evaluator's reload mechanism SHALL log the error and retain the previous compiled PAC (if any) or use fallback route until fixed.

### Requirement: Admin UI Settings tab
The Settings tab SHALL allow editing of the TUN device name via an input field.

**Modified to**: The Settings tab SHALL NOT include any input for TUN device. The UI must be simplified to show only listener addresses, PAC path, TLS settings, and PAC performance options.

#### Scenario: UI removes TUN field
- **WHEN** an administrator opens the Settings tab
- **THEN** they SHALL no longer see a "TUN device" input field
- **AND** the config API (`/api/config`) SHALL not include `tun_device` in its JSON response
- **AND** the config update API (`PUT /api/config`) SHALL ignore any `tun_device` field in the request (or return error if provided).

### Requirement: Admin UI Rules editor
When creating or editing a rule, the route dropdown SHALL include "TUN" as an option.

**Modified to**: The route dropdown SHALL only include: "DIRECT", "SOCKS5 127.0.0.1:1080" (customizable), "EXTERNAL" (populated from proxy registry), and "DEFAULT". The "TUN" option SHALL be removed from the list.

#### Scenario: Route selector in UI
- **WHEN** an administrator creates a new rule
- **THEN** the Route `<select>` element SHALL NOT contain a `<option value="TUN">`
- **AND** JavaScript constants for route tooltips SHALL not include a TUN entry

## UNCHANGED Capability Aspects

The following aspects of the `pac-proxy-tun` capability remain unchanged and continue to apply:

- Native rule engine implementation (suffix trie, exact matches, host globs)
- LRU evaluation cache with TTL
- DNS cache (if enabled)
- Automatic fallback to DIRECT on route failure (for non-DIRECT routes)
- Hot-reload of PAC files via fsnotify
- External proxy registry (EXTERNAL and DEFAULT strategies) - unchanged
- GFW list integration and PAC generation (use_tun flag removed, but generation itself unaffected)
- HTTPS proxy support with TLS termination
- HTTP CONNECT and SOCKS5 protocol handling
- Connection tracking and status reporting (minus TUN fields)
- Admin UI for rules management, proxy registry, GFW config, cache stats, etc.
