## ADDED Requirements

### Requirement: Tooltips explain match types and routes
The system SHALL provide hover tooltips for match type badges and route badges to help users understand their meanings.

#### Scenario: Tooltip for match type
- **WHEN** the user hovers over a match type badge (e.g., "domain_suffix")
- **THEN** a tooltip shall appear explaining what that match type does, e.g.:
  - `domain_suffix`: "Matches any host ending with this suffix"
  - `host_glob`: "Shell-style wildcards (* ?) for hostnames"
  - `url_glob`: "Shell-style wildcards applied to full URL"
  - `exact`: "Matches the host exactly"

#### Scenario: Tooltip for route
- **WHEN** the user hovers over a route badge (e.g., "TUN")
- **THEN** a tooltip shall appear explaining the route meaning, e.g.:
  - `TUN`: "Route traffic through the TUN device"
  - `DIRECT`: "Connect directly without proxy"
  - `SOCKS5 127.0.0.1:1080`: "Forward to SOCKS5 proxy at 127.0.0.1:1080"

#### Scenario: Tooltips work in both tables
- **WHEN** hovering over badges in either the User Rules or GFW Rules table
- **THEN** the tooltips shall appear consistently
