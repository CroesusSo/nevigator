## ADDED Requirements

### Requirement: PAC script evaluation
The system SHALL evaluate a JavaScript PAC file to determine routing for a given destination host.

#### Scenario: Simple PAC returns TUN
- **WHEN** the PAC file contains `function FindProxyForURL(url, host) { return "TUN"; }`
- **WHEN** a client requests `example.com:443`
- **THEN** the evaluator returns `Route{Strategy: "TUN"}`

#### Scenario: PAC returns SOCKS5
- **WHEN** the PAC file returns `"SOCKS5 1.2.3.4:1080"`
- **THEN** the evaluator returns `Route{Strategy: "SOCKS5", Address: "1.2.3.4:1080"}`

#### Scenario: PAC syntax error falls back to DIRECT
- **WHEN** the PAC file has invalid JavaScript
- **THEN** the evaluator returns `Route{Strategy: "DIRECT"}` and logs an error

### Requirement: TUN route binding
When the routing strategy is TUN, the system SHALL establish the outbound connection with its source IP bound to the TUN interface IP.

#### Scenario: TUN binding sets local address
- **GIVEN** the TUN device `tun0` has IP `10.0.0.2`
- **WHEN** the router creates an outbound connection to `8.8.8.8:53`
- **THEN** the connection's local address is `10.0.0.2:<ephemeral_port>`

### Requirement: SOCKS5 upstream
When the routing strategy is SOCKS5, the system SHALL connect to the specified SOCKS5 server and perform a SOCKS5 handshake before relaying traffic.

#### Scenario: Successful SOCKS5 handshake
- **GIVEN** a SOCKS5 server listening on `1.2.3.4:1080`
- **WHEN** the router routes a request with `Route{Strategy: "SOCKS5", Address: "1.2.3.4:1080"}`
- **THEN** the router establishes a TCP connection to `1.2.3.4:1080`
- **AND** sends SOCKS5 greeting and connect request
- **AND** receives a succeeded reply

### Requirement: Fallback to DIRECT
If the chosen route (TUN or SOCKS5) fails to establish a connection, the system SHALL automatically attempt a DIRECT route.

#### Scenario: TUN failure falls back
- **GIVEN** the TUN device does not exist
- **WHEN** the evaluator returns `Route{Strategy: "TUN"}`
- **THEN** the router tries to dial with TUN IP and fails
- **AND** the router automatically retries with `Route{Strategy: "DIRECT"}`
- **AND** if DIRECT succeeds, the connection is established

#### Scenario: SOCKS5 failure falls back
- **GIVEN** the SOCKS5 server is unreachable
- **WHEN** the evaluator returns `Route{Strategy: "SOCKS5", Address: "1.2.3.4:1080"}`
- **THEN** the router fails to connect to the SOCKS5 server
- **AND** the router automatically retries with DIRECT

### Requirement: Hot reload of PAC file
The system SHALL monitor the PAC file for changes and reload it without restarting the proxy.

#### Scenario: File modification triggers reload
- **GIVEN** the PAC file is loaded and serving requests
- **WHEN** the PAC file is modified on disk
- **THEN** the evaluator recompiles the new PAC script
- **AND** subsequent `Evaluate` calls use the new PAC

### Requirement: Admin UI for PAC management
The system SHALL provide a web-based UI on a configurable address to view, edit, test, and reload the PAC file.

#### Scenario: Edit and save PAC
- **WHEN** an admin opens the admin UI in a browser
- **THEN** the current PAC content is displayed in a textarea
- **WHEN** the admin edits the PAC and clicks Save
- **THEN** the new PAC is written to the PAC file location
- **AND** the evaluator reloads the PAC immediately

#### Scenario: Test URL evaluation
- **WHEN** the admin enters a URL in the test field and clicks Test
- **THEN** the UI displays the resulting route (strategy and address) without affecting live proxy traffic

## REMOVED Requirements

(No requirements removed in this change)
