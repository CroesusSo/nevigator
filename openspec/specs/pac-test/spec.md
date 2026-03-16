## ADDED Requirements

### Requirement: Correct host extraction from URL input
The system SHALL extract the hostname from the test URL input regardless of input format.

#### Scenario: Full URL with scheme
- **WHEN** user tests `https://example.com/path?query=1`
- **THEN** system extracts `example.com` and evaluates route against it

#### Scenario: Bare hostname
- **WHEN** user tests `example.com`
- **THEN** system evaluates route against `example.com`

#### Scenario: Hostname with port
- **WHEN** user tests `example.com:8080`
- **THEN** system extracts `example.com` and evaluates route against it

#### Scenario: Subdomain
- **WHEN** user tests `sub.domain.example.co.uk`
- **THEN** system evaluates route against `sub.domain.example.co.uk`

#### Scenario: URL with IP address
- **WHEN** user tests `http://192.168.1.1:3000/api`
- **THEN** system extracts `192.168.1.1` and evaluates route against it
