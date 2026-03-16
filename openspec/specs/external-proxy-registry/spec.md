## ADDED Requirements

### Requirement: External Proxy Registry
The system SHALL maintain a registry of external SOCKS5 proxies defined in configuration and manageable via admin UI and API. Each proxy entry SHALL include:
- `id`: unique string identifier (non-empty, not reserved)
- `address`: valid `host:port` string
- `username` (optional)
- `password` (optional)

The registry SHALL be used by the router to resolve `EXTERNAL <id>` and `DEFAULT` routing strategies.

#### Scenario: Add new external proxy via API
- **WHEN** an administrator POSTs `/api/proxies` with valid JSON body containing `id`, `address`, and optional credentials
- **THEN** the proxy SHALL be added to the registry
- **AND** the config file SHALL be updated atomically
- **AND** the router's internal registry SHALL be updated immediately

#### Scenario: List external proxies (no credentials)
- **WHEN** an administrator GETs `/api/proxies`
- **THEN** the response SHALL be a JSON array of objects containing `id`, `address`, and `has_auth` (boolean), without `password`

#### Scenario: Update external proxy
- **WHEN** an administrator PUTs `/api/proxies/:id` with updated fields
- **THEN** the existing proxy SHALL be updated (immutable `id` cannot be changed)
- **AND** if `password` is omitted, the existing password SHALL be preserved
- **AND** the registry SHALL be updated

#### Scenario: Delete external proxy
- **WHEN** an administrator DELETE `/api/proxies/:id`
- **THEN** the proxy SHALL be removed from the registry
- **AND** any rules referencing it SHALL produce `EXTERNAL <id>` in PAC but will fail at routing time (fallback to DIRECT)

#### Scenario: Test connectivity to an unsaved proxy
- **WHEN** an administrator POSTs `/api/proxies/test` with address and credentials
- **THEN** the system SHALL attempt a TCP connection followed by SOCKS5 greeting and optional username/password authentication
- **AND** return `{ "success": true }` on successful handshake, or `{ "success": false, "error": "..."} ` on failure

#### Scenario: Test connectivity to a saved proxy
- **WHEN** an administrator POSTs `/api/proxies/:id/test`
- **THEN** the system SHALL load the stored proxy definition from config
- **AND** perform the same connectivity test
- **AND** return the result

#### Scenario: Preventing loops
- **WHEN** an external proxy address uses `127.0.0.1`, `localhost`, or `::1` and its port matches any of this proxy's listening ports (HTTP, SOCKS5, Admin)
- **THEN** the API SHALL reject the request with HTTP 400 and appropriate error message
- **AND** the config SHALL NOT be updated

#### Scenario: Reserved ID validation
- **WHEN** an external proxy `id` is `"DIRECT"`, `"TUN"`, or `"DEFAULT"`
- **THEN** the API SHALL reject the request with HTTP 400

#### Scenario: Duplicate ID validation
- **WHEN** an external proxy with an `id` that already exists in the registry is created (or an update tries to change ID to an existing one)
- **THEN** the API SHALL reject the request with HTTP 409 Conflict

### Requirement: Default External Proxy
The system SHALL allow administrators to designate one registered external proxy as the default via `proxy.default_external_proxy` in configuration or via admin API. The special PAC return value `"DEFAULT"` SHALL instruct the router to use this default proxy. Additionally, the PAC generator SHALL use `"DEFAULT"` for rules with empty route when a default proxy is configured (falling back to legacy hardcoded SOCKS5 when no default is set).

#### Scenario: Router uses default proxy
- **WHEN** `Evaluator.Evaluate(host)` returns `Route{Strategy: "DEFAULT"}`
- **THEN** the router SHALL look up the configured `default_external_proxy` ID in the registry
- **AND** establish a SOCKS5 connection through that proxy
- **IF** the default proxy is not configured or not found, the router SHALL return an error, causing `Route` to fallback to DIRECT

#### Scenario: PAC generator emits DEFAULT
- **GIVEN** `proxy.default_external_proxy` is set to `"tor"`
- **WHEN** `gfw.GeneratePAC` is called with a rule having empty `Route`
- **THEN** the generated PAC SHALL contain `return "DEFAULT";` for that rule
- **IF** no default is set, the generator SHALL use `return "SOCKS5 127.0.0.1:1080";` (legacy behavior)

#### Scenario: Changing default proxy triggers PAC regeneration
- **WHEN** an administrator updates `default_external_proxy` via `/api/config`
- **THEN** the router's default proxy ID SHALL be updated immediately
- **AND** the PAC file SHALL be regenerated so that GFW rules (with empty route) now point to `DEFAULT`