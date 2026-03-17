## ADDED Requirements

### Requirement: GFW fetcher uses system default network interface
The GFW list fetcher SHALL use the system's default network interface for all HTTP requests, regardless of whether the TUN interface is enabled for routing proxied traffic.

#### Scenario: Fetch GFW list with TUN enabled
- **WHEN** the configuration has `use_tun: true` and a valid TUN device
- **WHEN** the GFW fetcher performs an HTTP request to download the gfwlist.txt
- **THEN** the HTTP client SHALL NOT bind to the TUN interface
- **THEN** DNS resolution SHALL use the system's default DNS resolver
- **THEN** the connection SHALL be established through the system's default network interface

#### Scenario: Fetch GFW list with TUN disabled
- **WHEN** the configuration has `use_tun: false` or no valid TUN device
- **WHEN** the GFW fetcher performs an HTTP request
- **THEN** the HTTP client SHALL use the system's default network interface
- **THEN** the fetch SHALL succeed with normal DNS resolution

## MODIFIED Requirements

(No modified requirements - this is a new capability specification)