## ADDED Requirements

### Requirement: Server summary displays listening addresses
The system SHALL display all listening addresses for the HTTP proxy, SOCKS5 proxy, and admin UI in the Summary tab.

#### Scenario: All listeners configured
- **WHEN** the server starts with HTTP on `:8080`, SOCKS5 on `:1080`, and Admin on `127.0.0.1:9090`
- **THEN** the Summary tab shows HTTP: `0.0.0.0:8080`, SOCKS5: `0.0.0.0:1080`, Admin: `127.0.0.1:9090`

#### Scenario: API returns listening addresses
- **WHEN** a GET request is made to `/api/status`
- **THEN** the response includes `httpListen`, `socks5Listen`, and `adminListen` fields with the configured addresses

### Requirement: Server summary displays TUN status
The system SHALL display the TUN device name, IP address, and up/down status in the Summary tab.

#### Scenario: TUN device is up
- **WHEN** the TUN device `tun0` exists and has IP `10.0.0.2`
- **THEN** the Summary tab shows TUN: `tun0 → 10.0.0.2` with status indicator "up"

#### Scenario: TUN device is down or missing
- **WHEN** the configured TUN device does not exist or has no IP
- **THEN** the Summary tab shows TUN: device name with status indicator "down"

#### Scenario: API returns TUN status
- **WHEN** a GET request is made to `/api/status`
- **THEN** the response includes `tunDevice`, `tunIP` (or null), and `tunUp` boolean

### Requirement: Server summary displays connection counts
The system SHALL display the number of active connections and total connections since server start.

#### Scenario: No active connections
- **WHEN** no clients are connected
- **THEN** the Summary tab shows Active: `0` and Total: `N` (where N is connections since start)

#### Scenario: Active connections exist
- **WHEN** 5 clients are currently connected
- **THEN** the Summary tab shows Active: `5` and Total: `N`

#### Scenario: API returns connection counts
- **WHEN** a GET request is made to `/api/status`
- **THEN** the response includes `activeConnections` (integer) and `totalConnections` (integer)

### Requirement: Summary tab auto-refreshes
The Summary tab SHALL periodically refresh its data to reflect current server state.

#### Scenario: Data refreshes automatically
- **WHEN** the Summary tab is displayed
- **THEN** the data refreshes every 5 seconds without user interaction
