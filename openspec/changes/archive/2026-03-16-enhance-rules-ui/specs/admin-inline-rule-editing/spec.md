## ADDED Requirements

### Requirement: User rules are editable inline
The system SHALL allow users to edit their own rules directly in the Rules table without opening a modal dialog.

#### Scenario: Row enters edit mode
- **WHEN** a user clicks any editable field (match, value, or route) in a user rule row
- **THEN** that row switches to edit mode where:
  - the match cell shows a `<select>` dropdown with all match type options
  - the value cell shows an `<input>` pre-filled with current pattern
  - the route cell shows a `<select>` with route choices
  - save (✓) and cancel (✕) buttons appear adjacent to the value input

#### Scenario: Save updates the rule
- **WHEN** the user clicks the save button (or presses Enter) while in edit mode
- **THEN** the system sends `PUT /api/rules/{id}` with the updated fields
- **AND** on success, the row returns to display mode showing the new values
- **AND** the PAC file is regenerated automatically

#### Scenario: Cancel reverts changes
- **WHEN** the user clicks the cancel button (or presses Escape) while in edit mode
- **THEN** the row returns to display mode with original values unchanged
- **AND** no API request is made

#### Scenario: Only user rules are editable
- **WHEN** a user clicks a GFW rule row
- **THEN** no edit mode is activated (GFW rules are read-only)
