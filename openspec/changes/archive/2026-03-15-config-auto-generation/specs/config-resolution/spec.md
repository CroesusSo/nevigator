## ADDED Requirements

### Requirement: Config file resolution priority
The system SHALL resolve the config file path using the following priority order: explicit flag, XDG user config, auto-generation.

#### Scenario: Explicit flag takes priority
- **WHEN** user starts the program with `--conf /path/to/config.yaml`
- **THEN** the system loads config from the specified path regardless of whether `~/.config/proxy-pac/config.yaml` exists

#### Scenario: XDG config found
- **WHEN** user starts the program without `--conf` flag
- **AND** `~/.config/proxy-pac/config.yaml` exists
- **THEN** the system loads config from `~/.config/proxy-pac/config.yaml`

#### Scenario: Auto-generate default config
- **WHEN** user starts the program without `--conf` flag
- **AND** `~/.config/proxy-pac/config.yaml` does not exist
- **THEN** the system creates `~/.config/proxy-pac/` directory if needed
- **AND** generates a default `config.yaml` file with all fields set to their defaults
- **AND** loads the generated config and continues startup

### Requirement: Home directory resolution
The system SHALL resolve the user's home directory using `os.UserHomeDir()` for portable path construction.

#### Scenario: Home directory available
- **WHEN** the system needs to construct the default config path
- **THEN** it uses `os.UserHomeDir()` to determine the home directory
- **AND** constructs the path as `<home>/.config/proxy-pac/config.yaml`

#### Scenario: Home directory unavailable
- **WHEN** `os.UserHomeDir()` returns an error
- **THEN** the system logs a warning and continues with in-memory defaults
- **AND** does not attempt to write a config file

### Requirement: Default config auto-generation
The system SHALL generate a valid default config file when no config is found.

#### Scenario: Successful generation
- **WHEN** no config file is found and the system decides to auto-generate
- **AND** the parent directory `~/.config/proxy-pac/` can be created
- **AND** the config file can be written
- **THEN** the generated file contains valid YAML with all proxy and pac fields set to their documented defaults

#### Scenario: Generation fails due to permissions
- **WHEN** the system attempts to create `~/.config/proxy-pac/` or write the config file
- **AND** the operation fails due to filesystem permissions
- **THEN** the system logs a warning with the error details
- **AND** continues startup using in-memory defaults

### Requirement: Config flag rename
The system SHALL accept `--conf` as the flag name for specifying a config file path.

#### Scenario: Flag override
- **WHEN** user passes `--conf /custom/path.yaml`
- **THEN** the system uses that path for config loading
- **AND** does not search for or generate the default config path
