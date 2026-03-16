## ADDED Requirements

### Requirement: Lazy rendering for GFW rules table
The system SHALL render only visible rows in the GFW rules table to maintain UI performance when displaying large rule sets.

#### Scenario: Initial render shows only visible rows
- **WHEN** the GFW section is expanded and contains more rules than fit in the viewport
- **THEN** only the rows that intersect the visible area (plus a small buffer) are rendered in the DOM
- **AND** the rest of the rules are not inserted until they scroll into view

#### Scenario: Scrolling loads additional rows
- **WHEN** the user scrolls within the GFW table container
- **THEN** the system shall compute the new visible range and update the rendered rows accordingly
- **AND** rows that scroll out of view may be removed from the DOM to free memory (optional)

#### Scenario: Row height is measured
- **WHEN** the GFW table is first shown
- **THEN** the system shall measure the computed height of one row to calculate the number of visible rows

#### Scenario: No lazy rendering when all rows fit
- **WHEN** the GFW rules count is small (fewer than the visible capacity)
- **THEN** all rows are rendered at once (no virtualization overhead)

### Requirement: Performance under large rule sets
The system SHALL keep the UI responsive even when the GFW list contains thousands of rules.

#### Scenario: Thousands of rules load quickly
- **WHEN** the GFW table is shown with 10,000 rules
- **THEN** the initial render completes within 200ms and the user can scroll smoothly
- **AND** DOM node count stays bounded by the number of visible rows plus a small buffer
