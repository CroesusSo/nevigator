## Context

The current PAC-enabled proxy server handles HTTP CONNECT tunneling but lacks HTTPS support. This limitation prevents secure web traffic proxying and restricts the server's use cases. The system needs TLS termination capabilities to handle HTTPS connections through CONNECT tunneling while maintaining existing routing flexibility.

## Goals / Non-Goals

**Goals:**
- Add HTTPS CONNECT tunneling with TLS termination support
- Maintain backward compatibility with existing HTTP CONNECT functionality
- Extend PAC evaluation for TLS-specific routing decisions
- Add certificate management for secure connections
- Support SNI for proper routing decisions
- Integrate seamlessly with existing SOCKS5 and TUN routing

**Non-Goals:**
- Full TLS inspection/man-in-the-middle capabilities
- Certificate generation for arbitrary domains
- Performance optimization for high-throughput TLS connections
- Advanced TLS features like ALPN or HTTP/2 over TLS

## Decisions

### TLS Termination Strategy
- **Decision**: Add TLS termination layer between HTTP server and routing logic
- **Rationale**: Clean separation of concerns, maintains existing architecture
- **Alternative**: Modify existing HTTP server - rejected due to complexity and breaking changes

### Certificate Management
- **Decision**: Implement CA certificate management with trust store
- **Rationale**: Security best practices, proper certificate validation
- **Alternative**: Trust all certificates - rejected due to security risks

### PAC Evaluation Extension
- **Decision**: Add TLS-specific strategies (TLS_TUN, TLS_SOCKS5, TLS_DIRECT)
- **Rationale**: Consistent with existing routing patterns, extensible
- **Alternative**: Separate HTTPS PAC - rejected due to duplication

### Router Integration
- **Decision**: Extend router with TLS-specific dialing methods
- **Rationale**: Maintains existing routing logic, clear separation
- **Alternative**: Separate TLS router - rejected due to complexity

### Security Configuration
- **Decision**: Configurable certificate validation with trust settings
- **Rationale**: Flexibility for different deployment scenarios
- **Alternative**: Hard-coded validation - rejected due to lack of flexibility

## Risks / Trade-offs

[Certificate Management Complexity] → Implement modular cert manager with caching
[Performance Overhead] → Use connection pooling and async operations
[Security Vulnerabilities] → Comprehensive testing and security review
[Backward Compatibility] → Maintain existing HTTP CONNECT functionality
[SNI Privacy] → Configurable SNI exposure policy