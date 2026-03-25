## ADDED Requirements

### Requirement: SSH tunnel worker pool implementation

The system SHALL replace the mutex-based `ssh.Tunnel` implementation with a worker-pool, atomic-state design that eliminates lock contention in the hot path while preserving the public API and all existing observable behaviors.

#### Scenario: API compatibility
- **WHEN** any existing code calls `Tunnel.Dial` or `Tunnel.Close`
- **THEN** the calls SHALL behave identically from the caller's perspective (same error types, same blocking semantics, same reconnection behavior)

#### Scenario: Existing tests pass
- **WHEN** the existing SSH tunnel test suite is executed without modification
- **THEN** all tests SHALL pass

#### Scenario: Performance improvement
- **WHEN** a high volume of concurrent HTTPS requests route through an SSH tunnel
- **THEN** the system SHALL demonstrate reduced tail latency and higher throughput compared to the mutex-based implementation
- **AND** no deadlocks or data races SHALL occur under load
