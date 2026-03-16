## ADDED Requirements

### Requirement: Client-side validation of pattern fields
The system SHALL validate pattern values on the client before submitting updates, and prevent saving invalid data.

#### Scenario: Empty host_glob is rejected
- **WHEN** the match type is `host_glob` and the pattern input is empty (after trim)
- **THEN** the input shall be marked as invalid (e.g., red border)
- **AND** the save button shall be disabled or the save action shall be blocked
- **AND** an inline error message may be shown

#### Scenario: Empty url_glob is rejected
- **WHEN** the match type is `url_glob` and the pattern input is empty
- **THEN** the input shall be marked as invalid and save blocked

#### Scenario: Other match types may be empty
- **WHEN** the match type is `domain_suffix` or `exact`
- **THEN** no empty-pattern validation is enforced (server-side validation still applies)

#### Scenario: Validation re-checked live
- **WHEN** the user types into the pattern input
- **THEN** validation state is recomputed on each change
- **AND** if the input becomes valid, the save action becomes enabled
