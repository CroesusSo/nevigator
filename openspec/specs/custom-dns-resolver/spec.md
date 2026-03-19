## ADDED Requirements (Partial – PAC portions deprecated)

### Requirement: Custom DNS resolver configuration (GFW fetcher only)

The system SHALL support a configurable list of DNS servers for the GFW list fetcher. This configuration is available under `gfw:custom_dns_servers` in the YAML config as a list of `host:port` strings. An empty or omitted list indicates use of the system default DNS resolver.

**Note:** The PAC evaluator's `dnsResolve()` builtin is no longer supported in the native‑only engine, so custom DNS for PAC is not applicable.

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
- **WHEN** the configuration does NOT specify `gfw.custom_dns_servers` or the list is empty
- **WHEN** the GFW fetcher performs an HTTP request
- **THEN** the system's default DNS resolver (`net.DefaultResolver`) SHALL be used

#### Scenario: Custom DNS with multiple servers
- **WHEN** `gfw.custom_dns_servers` contains multiple entries, e.g., `["1.1.1.1:53", "8.8.8.8:53"]`
- **WHEN** a DNS query is performed by the fetcher
- **THEN** the resolver SHALL try the first server
- **IF** the first server fails, it SHALL try the next server in order
- **WHEN** a server responds successfully, the result SHALL be returned

#### Scenario: Logging of DNS mode at startup
- **WHEN** the application starts and `gfw.custom_dns_servers` is non‑empty
- **THEN** the log SHALL include `[GFW] Using custom DNS: [servers...]`
- **WHEN** the list is empty, no special log is emitted

### Implementation notes

- The GFW fetcher creates an `http.Client` with a custom `Dialer` that uses a `net.Resolver` configured with the provided DNS servers.
- The `custom_dns_servers` field under `pac:` in configuration is retained for backward compatibility but has no effect on PAC evaluation (since `dnsResolve` is not available).
