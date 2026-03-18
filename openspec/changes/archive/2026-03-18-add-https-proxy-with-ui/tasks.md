# Implementation Tasks

## 1. Backend Config Struct
- [x] 1.1: Add HTTPSListen field to FullConfig.Proxy
- [x] 1.2: Add TLS field to FullConfig.Proxy (certificate_file, key_file, ca_file, client_auth)
- [x] 1.3: Ensure struct tags match YAML and JSON conventions

## 2. Backend GET
- [x] 2.1: Update handleGetConfig to include https_listen in JSON response
- [x] 2.2: Include TLS fields in response (certificate_file, key_file, ca_file, client_auth)
- [x] 2.3: Ensure zero values handled gracefully

## 3. Backend PUT
- [x] 3.1: Update handlePutConfig to accept https_listen updates
- [x] 3.2: Accept TLS field updates
- [x] 3.3: Set restart_required when https_listen changes
- [x] 3.4: Set restart_required when any TLS field changes

## 4. HTML Form
- [x] 4.1: Add HTTPS Proxy input row in Listeners card
- [x] 4.2: Create TLS Settings card with inputs: certificate file, key file, CA file, client auth select
- [x] 4.3: Include helpful hint about TLS/restart requirement

## 5. JavaScript
- [x] 5.1: Update loadConfig() to populate https-listen and TLS inputs
- [x] 5.2: Update saveConfig() to include https_listen in payload
- [x] 5.3: Include TLS object in saveConfig() payload
- [x] 5.4: Update restart message to mention listeners/TLS
- [x] 5.5: Apply changes to both copies of saveConfig() in the HTML

## 6. Testing
- [x] 6.1: Unit tests for GET /api/config (cover new fields)
- [x] 6.2: Unit tests for PUT /api/config (cover new fields)
- [x] 6.3: Integration test for config persistence
- [x] 6.4: Test that no data loss occurs when saving
- [x] 6.5: Run all existing tests
- [x] 6.6: Fix any test failures

## 7. Documentation/Polish
- [x] 7.1: Add helpful hint in TLS card
- [x] 7.2: Add inline code comments describing new fields in config struct
- [x] 7.3: Test backward compatibility with configs lacking these fields
