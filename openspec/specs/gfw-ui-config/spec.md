## ADDED Requirements

### Requirement: Settings page pre-fills GFW source URL
The admin UI Settings tab SHALL display the current GFW source URL in the GFW configuration input field when the page loads.

#### Scenario: Load Settings with GFW source configured
- **GIVEN** the system has a GFW source URL configured (e.g., `https://raw.githubusercontent.com/gfwlist/gfwlist/master/gfwlist.txt`)
- **WHEN** the user opens the Settings tab
- **THEN** the input field with id `gfw-url` SHALL have its value set to that configured URL

#### Scenario: Load Settings without GFW source configured
- **GIVEN** no GFW source URL is configured
- **WHEN** the user opens the Settings tab
- **THEN** the `gfw-url` input SHALL be empty (but still present)
- **AND** saving an empty value SHALL clear the source URL
