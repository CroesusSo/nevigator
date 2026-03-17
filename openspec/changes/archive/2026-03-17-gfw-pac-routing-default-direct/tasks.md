## 1. Parser Modification

- [x] 1.1 Modify `gfw/parser.go` route() function to return empty string for blacklist entries
- [x] 1.2 Update unit tests in `gfw/parser_test.go` to reflect new behavior

## 2. Generator Logic Update

- [x] 2.1 Modify `gfw/generator.go` writeRuleCondition() function to use DIRECT as fallback when no default proxy configured
- [x] 2.2 Update unit tests in `gfw/generator_test.go` for both scenarios (with/without default proxy)

## 3. Verification

- [x] 3.1 Run all GFW-related tests to ensure no regressions
- [x] 3.2 Run PAC integration tests to verify end-to-end behavior
- [x] 3.3 Test with default proxy configured: GFW blacklist → DEFAULT → uses proxy
- [x] 3.4 Test without default proxy configured: GFW blacklist → DIRECT → direct connection
- [x] 3.5 Verify whitelist rules unchanged in both scenarios