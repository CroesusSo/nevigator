## ADDED Requirements

### Requirement: Keepalive interval display and editing in SSH proxy edit modal

The edit modal for SSH proxies SHALL correctly display the keepalive interval as a human-readable duration string (e.g., "30s") and SHALL accept duration strings for editing.

#### Scenario: Keepalive interval displayed correctly
- **WHEN** a user clicks "Edit" on an SSH proxy with keepalive enabled and interval set to 30 seconds
- **THEN** the keepalive interval input field SHALL show "30s"
- **AND** the keepalive enabled checkbox SHALL be checked

#### Scenario: Keepalive interval saved correctly
- **WHEN** a user modifies the keepalive interval to "45s" and clicks "Save"
- **THEN** the browser SHALL send `PUT /api/proxies/:id` with `keepalive_interval: "45s"`
- **AND** on success, subsequent edits SHALL show "45s" in the interval field
