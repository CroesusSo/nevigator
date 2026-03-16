## ADDED Requirements

### Requirement: Active connections are tracked
The system SHALL track each active client connection including its client address, destination, route type, protocol, and start time.

#### Scenario: HTTP CONNECT connection tracked
- **WHEN** a client establishes an HTTP CONNECT tunnel through the proxy
- **THEN** the connection is recorded with client address, destination host:port, route strategy (TUN/SOCKS5/DIRECT), protocol "http", and start timestamp

#### Scenario: SOCKS5 connection tracked
- **WHEN** a client establishes a SOCKS5 connection through the proxy
- **THEN** the connection is recorded with client address, destination host:port, route strategy, protocol "socks5", and start timestamp

#### Scenario: Connection removed on close
- **WHEN** a tracked connection closes (either side disconnects)
- **THEN** the connection is removed from the active connections list

### Requirement: Connections tab displays active connections
The system SHALL display a table of all active connections in the Connections tab.

#### Scenario: Table columns shown
- **WHEN** the Connections tab is displayed
- **THEN** the table shows columns: ID, Client Address, Destination, Route, Protocol, Duration

#### Scenario: Duration updates live
- **WHEN** the Connections tab is displayed and connections are active
- **THEN** the Duration column shows elapsed time in `HH:MM:SS` format, updating on each refresh

#### Scenario: Empty state
- **WHEN** no connections are active
- **THEN** the Connections tab shows a message indicating no active connections

### Requirement: Connections tab auto-refreshes
The Connections tab SHALL periodically refresh to show current connections.

#### Scenario: Data refreshes automatically
- **WHEN** the Connections tab is displayed
- **THEN** the connection list refreshes every 2 seconds

### Requirement: Connections API returns active connections
The system SHALL provide an API endpoint returning the list of active connections.

#### Scenario: GET /api/connections
- **WHEN** a GET request is made to `/api/connections`
- **THEN** the response is a JSON array of connection objects, each with `id`, `clientAddr`, `destAddr`, `route`, `protocol`, and `startedAt` (ISO 8601 timestamp)

#### Scenario: Empty connections
- **WHEN** no connections are active and GET /api/connections is called
- **THEN** the response is an empty JSON array `[]`
