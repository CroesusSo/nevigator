## ADDED Requirements

### Requirement: Auto-create default PAC file on missing
The system SHALL create a default PAC file at the configured path when the file does not exist at startup.

#### Scenario: PAC file missing at startup
- **WHEN** the server starts and the PAC file does not exist at `pac_path`
- **THEN** the system creates a PAC file with a `FindProxyForURL` function that returns `"DIRECT"` for all hosts
- **AND** logs a message indicating the default PAC file was created and its location

#### Scenario: PAC file exists at startup
- **WHEN** the server starts and the PAC file already exists at `pac_path`
- **THEN** the system loads the existing file without modification
- **AND** does not log a creation message

### Requirement: Default PAC behavior
The default PAC file SHALL route all traffic via DIRECT (system default route).

#### Scenario: Default PAC routes directly
- **WHEN** an auto-created default PAC file is evaluated for any host
- **THEN** the route returned is `DIRECT`

### Requirement: Graceful failure on create error
The system SHALL fail with a clear error message if the default PAC file cannot be created.

#### Scenario: Directory does not exist
- **WHEN** the parent directory of `pac_path` does not exist
- **THEN** the system fails to start with an error message indicating the directory issue

#### Scenario: Permission denied
- **WHEN** the process lacks write permission to create the PAC file
- **THEN** the system fails to start with an error message indicating a permission issue
