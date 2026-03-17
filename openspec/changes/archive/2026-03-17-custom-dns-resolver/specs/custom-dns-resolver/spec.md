## ADDED Requirements

### Requirement: Custom DNS resolver configuration
The system SHALL support a configurable list of DNS servers to use instead of the system default DNS resolver. This configuration SHALL be available under both `pac:` and `gfw:` sections in the YAML config as `custom_dns_servers` (a list of strings in `host:port` format). An empty or omitted list SHALL indicate use of the system default DNS resolver.

#### Scenario: Configure custom DNS for PAC
- **WHEN** the configuration file contains:
  ```yaml
  pac:
    custom_dns_servers: ["1.1.1.1:53", "8.8.8.8:53"]
  ```
- **WHEN** the PAC evaluator performs a DNS lookup via `dnsResolve()` or `isResolvable()`
- **THEN** the DNS query SHALL be sent directly to `1.1.1.1:53` (or fallback to `8.8.8.8:53` if first fails)
- **THEN** the system's `/etc/resolv.conf` SHALL NOT be used

#### Scenario: Configure custom DNS for GFW fetcher
- **WHEN** the configuration file contains:
  ```yaml
  gfw:
    custom_dns_servers: ["1.1.1.1:53"]
  ```
- **WHEN** the GFW fetcher performs an HTTP request to fetch the gfwlist
- **THEN** the hostname resolution for `raw.githubusercontent.com` SHALL use `1.1.1.1:53` directly
- **THEN** the system's DNS resolver SHALL NOT be used

#### Scenario: System DNS fallback when custom DNS not configured
- **WHEN** the configuration does NOT specify `custom_dns_servers` or the list is empty
- **WHEN** a DNS lookup is performed (PAC or GFW)
- **THEN** the system's default DNS resolver (`net.DefaultResolver`) SHALL be used
- **THEN** behavior SHALL be identical to prior versions

#### Scenario: Custom DNS with multiple servers
- **WHEN** `custom_dns_servers` contains multiple entries, e.g., `["1.1.1.1:53", "8.8.8.8:53"]`
- **WHEN** a DNS query is performed
- **THEN** the resolver SHALL try the first server
- **IF** the first server fails (no response or error), it SHALL try the next server in order
- **WHEN** a server responds successfully, the result SHALL be returned and subsequent queries MAY use the same server (caching applies at the resolver level per Go's implementation)

#### Scenario: Logging of DNS mode at startup
- **WHEN** the application starts
- **THEN** for PAC: if `custom_dns_servers` is non-empty, log `[PAC] Using custom DNS: [servers...]`
- **AND** for GFW: if `custom_dns_servers` is non-empty, log `[GFW] Using custom DNS: [servers...]`
- **AND** if empty, no special log (uses system DNS by default)

### Requirement: PAC DNS cache integration
The PAC evaluator's DNS cache SHALL use the configured custom resolver (if any) for lookups. The `dnsCache` struct SHALL store an optional `*net.Resolver`. The `lookupHost` method SHALL use this resolver when set, otherwise fall back to `net.DefaultResolver`.

### Requirement: GFW HTTP client integration
The GFW fetcher SHALL use an `http.Client` whose `Dialer.Resolver` is set to the custom resolver when `custom_dns_servers` is configured. The fetcher's `HTTPClient` field MAY be replaced with a custom client after creation in `main.go`.
