## Why

Add HTTPS proxy support to the PAC-enabled proxy server to handle secure HTTPS connections through CONNECT tunneling. The current system only supports HTTP CONNECT, limiting its use cases and preventing secure web traffic proxying.

## What Changes

- **BREAKING**: Add TLS termination layer to handle HTTPS CONNECT requests
- Add certificate management system for TLS connections
- Extend PAC evaluation engine with TLS-specific routing strategies
- Add HTTPS-specific routing logic to the router
- Enhance security configuration for certificate validation and trust management
- Support for SNI (Server Name Indication) in routing decisions

## Capabilities

### New Capabilities
- `https-proxy`: HTTPS CONNECT tunneling and TLS termination support
- `tls-routing`: TLS-specific routing strategies and configuration
- `certificate-management`: CA certificate handling and validation
- `sni-handling`: Server Name Indication support for routing decisions

### Modified Capabilities
- `pac-evaluation`: Extend with TLS-specific built-in functions and strategies
- `router-logic`: Add TLS-specific routing methods and connection handling
- `security-config`: Enhanced security settings for certificate validation

## Impact

- Core proxy server architecture (proxy/server.go)
- PAC evaluation engine (pac/evaluator.go)
- Router logic (router/router.go)
- Configuration system (config structures)
- Security model and certificate handling
- Integration with existing SOCKS5 and TUN support