## ADDED Requirements

### Requirement: PAC save with validation

The system SHALL accept PAC content via POST to `/pac`, validate it by parsing the `__PAC_RULES__` JSON structure and verifying rule syntax, and write it to disk only if validation succeeds.

#### Scenario: Valid PAC saved successfully
- **WHEN** user POSTs valid PAC with well‑formed `__PAC_RULES__` to `/pac`
- **THEN** system writes file atomically (tmp + rename) and returns 200

#### Scenario: Invalid PAC rejected
- **WHEN** user POSTs PAC with malformed JSON or invalid rule structure to `/pac`
- **THEN** system returns 422 with error message describing the validation failure
- **AND** no file is written to disk

#### Scenario: Empty PAC rejected
- **WHEN** user POSTs empty body to `/pac`
- **THEN** system returns 422 with "empty PAC content" error

### Requirement: Atomic PAC file write

The system SHALL use atomic file operations to prevent partial writes on crash.

#### Scenario: Atomic write with temp file
- **WHEN** writing valid PAC content
- **THEN** system writes to a temporary file first
- **AND** renames temporary file to the PAC path
- **AND** the PAC file is either fully written or unchanged (no truncated state)
