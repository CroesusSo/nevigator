## ADDED Requirements

### Requirement: GFW rules are in a separate hidden section
The system SHALL display GFW rules in a dedicated table section, separated from user rules, and hidden by default.

#### Scenario: GFW section hidden initially
- **WHEN** the Rules tab is loaded
- **THEN** only the User Rules table and the "Add Rule" form are visible
- **AND** the GFW rules table is not rendered in the DOM

#### Scenario: Show GFW rules checkbox
- **WHEN** the user checks the "Show GFW rules (N rules)" checkbox
- **THEN** the GFW rules section becomes visible
- **AND** the table displays all current GFW rules (or a lazy-rendered subset if optimizations apply)
- **AND** the checkbox label shows the total count of GFW rules

#### Scenario: Hide GFW rules
- **WHEN** the user unchecks the "Show GFW rules" checkbox
- **THEN** the GFW rules section is hidden from view

#### Scenario: GFW table styling
- **WHEN** the GFW rules table is visible
- **THEN** it has a light grey background (`#f5f5f5`) to distinguish it from the user table
- **AND** it does not include columns for On/Off toggles, edit, or delete actions
- **AND** it shows only Match Type and Pattern columns
