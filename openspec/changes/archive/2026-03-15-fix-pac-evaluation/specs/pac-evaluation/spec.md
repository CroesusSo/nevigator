## ADDED Requirements

### Requirement: PAC FindProxyForURL extraction
The system SHALL correctly extract and call the `FindProxyForURL` function from compiled PAC scripts.

#### Scenario: Extract callable function
- **WHEN** a PAC script defines `function FindProxyForURL(url, host)`
- **THEN** the evaluator extracts it as a callable function
- **AND** invokes it with the URL and host arguments

#### Scenario: Return route from PAC function
- **WHEN** FindProxyForURL returns "TUN"
- **THEN** the evaluator returns Route{Strategy: "TUN"}

#### Scenario: Return SOCKS5 route from PAC function
- **WHEN** FindProxyForURL returns "SOCKS5 127.0.0.1:9050"
- **THEN** the evaluator returns Route{Strategy: "SOCKS5", Address: "127.0.0.1:9050"}

#### Scenario: Non-function value
- **WHEN** a PAC script does not define FindProxyForURL or defines it as a non-function
- **THEN** the evaluator returns the configured fallback route

### Requirement: Default PAC routing rules
The default PAC script SHALL route google.com and its subdomains through TUN and all other traffic DIRECT.

#### Scenario: Google domain routed to TUN
- **WHEN** evaluating "google.com"
- **THEN** the route strategy is "TUN"

#### Scenario: Google subdomain routed to TUN
- **WHEN** evaluating "www.google.com"
- **THEN** the route strategy is "TUN"

#### Scenario: Non-google domain routed DIRECT
- **WHEN** evaluating "example.com"
- **THEN** the route strategy is "DIRECT"

### Requirement: Default PAC file creation
When no PAC file exists at the configured path, the system SHALL create one with the default PAC content.

#### Scenario: Auto-create default PAC
- **WHEN** the PAC file does not exist at the configured path
- **THEN** the system creates the file with default PAC content
- **AND** the file contains `FindProxyForURL` and `dnsDomainIs`
