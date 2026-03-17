## ADDED Requirements

### Requirement: HTTPS proxy listener accepts encrypted connections
The system SHALL accept TLS-encrypted proxy connections on the address specified by the `https_listen` configuration option.

#### Scenario: Successful HTTPS proxy connection
- **WHEN** `https_listen` is configured and TLS certificates are valid
- **WHEN** a client establishes a TCP connection to the `https_listen` address
- **WHEN** the client performs a TLS handshake using supported protocol versions
- **THEN** the server SHALL complete the TLS handshake successfully
- **THEN** the server SHALL treat the decrypted stream as an HTTP CONNECT proxy connection

#### Scenario: HTTPS listener not configured
- **WHEN** `https_listen` is not set or empty
- **THEN** the server SHALL NOT start an HTTPS proxy listener
- **THEN** the server SHALL continue starting other components (HTTP proxy, SOCKS5)

#### Scenario: TLS configuration invalid
- **WHEN** `https_listen` is set but TLS certificate files are missing or invalid
- **THEN** the server SHALL log an error indicating the TLS configuration problem
- **THEN** the server SHALL NOT start the HTTPS proxy listener
- **THEN** the server SHALL continue starting other components

### Requirement: HTTPS proxy uses provided TLS configuration
The system SHALL use the `tls` configuration block (certificate_file, key_file, etc.) for the HTTPS listener's TLS parameters.

#### Scenario: TLS parameters applied
- **WHEN** the HTTPS listener starts
- **THEN** it SHALL use the certificate from `tls.certificate_file`
- **THEN** it SHALL use the private key from `tls.key_file`
- **THEN** it SHALL apply any configured `tls.min_version`, `tls.max_version`, `tls.cipher_suites`, and other TLS options

### Requirement: HTTPS proxy routes identically to HTTP proxy
After TLS handshake, the HTTPS proxy SHALL process requests exactly as the HTTP proxy does, using the same PAC evaluation and routing logic.

#### Scenario: HTTPS CONNECT request routed via PAC
- **WHEN** a client establishes an HTTPS proxy connection and sends `CONNECT www.example.com:443 HTTP/1.1`
- **WHEN** the PAC file returns a route (e.g., `SOCKS5 1.2.3.4:1080`)
- **THEN** the proxy SHALL route the connection through the specified SOCKS5 upstream
- **THEN** the proxy SHALL return `200 Connection Established` and relay traffic

#### Scenario: PAC evaluation uses client IP and destination
- **WHEN** an HTTPS proxy request arrives
- **THEN** the evaluator SHALL receive the client's IP address (from the connection)
- **THEN** the evaluator SHALL receive the destination host (`www.example.com`)
- **THEN** routing decisions SHALL be identical to those made for HTTP proxy requests from the same client

### Requirement: Connection tracking and status
The system SHALL track HTTPS proxy connections in the same manner as HTTP and SOCKS5 connections and include the HTTPS listener address in server status.

#### Scenario: HTTPS connection appears in status
- **WHEN** an active HTTPS proxy connection exists
- **THEN** `server.Connections()` SHALL include that connection with `Protocol = "https-proxy"` (or similar identifier)
- **THEN** `server.Status()` SHALL report `HTTPSListen` with the configured address (if HTTPS listener is running)
