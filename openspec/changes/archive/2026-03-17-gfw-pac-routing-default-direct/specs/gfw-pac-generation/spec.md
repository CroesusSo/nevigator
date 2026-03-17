# Capability: GFW PAC Generation (Modified)

## Overview

This capability describes how GFW (Great Firewall) rules are converted to PAC (Proxy Auto-Config) routing directives. The modification changes how blacklist rules are handled based on default external proxy configuration.

## Behavior Changes

### Before Modification
- GFW blacklist rules (no `@@` prefix) always generated PAC returning `"SOCKS5 127.0.0.1:1080"`
- GFW whitelist rules (`@@` prefix) always generated PAC returning `"DIRECT"`
- Regardless of default external proxy configuration

### After Modification
- GFW blacklist rules:
  - **When default external proxy IS configured**: PAC returns `"DEFAULT"`
  - **When default external proxy IS NOT configured**: PAC returns `"DIRECT"`
- GFW whitelist rules: Always return `"DIRECT"` (unchanged)

## Detailed Specification

### PAC Generation Logic

When generating PAC from GFW rules:

1. **For each rule**:
   - If rule.Action == "DIRECT" (whitelist): return `"DIRECT"`
   - If rule.Action == "PROXY" (blacklist):
     - If default external proxy is configured: return `"DEFAULT"`
     - Else: return `"DIRECT"`

### Route Resolution in PAC Evaluator

The PAC evaluator parses return values as follows:
- `"DEFAULT"` → Route{Strategy: "DEFAULT"}
- `"DIRECT"` → Route{Strategy: "DIRECT"}
- `"SOCKS5 host:port"` → Route{Strategy: "SOCKS5", Address: "host:port"}

### Router Behavior (Unchanged)

The router handles these routes:
- **Strategy: "DEFAULT"**: 
  - Looks up configured default proxy ID in external proxy registry
  - If found: establishes SOCKS5 connection through that proxy
  - If not found: returns error, triggering fallback to DIRECT
- **Strategy: "DIRECT"**: establishes direct connection
- **Fallback Logic**: If any non-DIRECT route fails, automatically retries with DIRECT

## Configuration Impact

### YAML Configuration
```yaml
proxy:
  # ... other settings ...
  default_external_proxy: "my-proxy-id"  # optional, references external proxy by ID
  
external_proxies:
  - id: "my-proxy-id"
    address: "proxy.example.com:1080"
    # ... optional auth ...
```

### Behavioral Examples

**Scenario 1: Default proxy configured**
- GFW blacklist rule for `.example.com`
- Default external proxy configured as "corp-proxy" → "proxy.corp:1080"
- PAC generated: `return "DEFAULT";`
- At runtime: Router uses "corp-proxy" for .example.com traffic

**Scenario 2: No default proxy configured**
- GFW blacklist rule for `.example.com`
- No default external proxy configured
- PAC generated: `return "DIRECT";`
- At runtime: Direct connection for .example.com traffic

**Scenario 3: Whitelist rule (unchanged)**
- GFW whitelist rule for `.internal.com` (@@ prefix)
- PAC generated: `return "DIRECT";`
- At runtime: Direct connection regardless of proxy configuration

## Testing Requirements

### Unit Tests
1. Test PAC generation with default proxy configured → blacklist rules return "DEFAULT"
2. Test PAC generation with no default proxy configured → blacklist rules return "DIRECT"
3. Test PAC generation → whitelist rules always return "DIRECT"
4. Test PAC parser correctly handles "DEFAULT", "DIRECT", and "SOCKS5" return values

### Integration Tests
1. Verify end-to-end flow with default proxy configured:
   - GFW blacklist rule → PAC: "DEFAULT" → Router: uses configured proxy
2. Verify end-to-end flow with no default proxy configured:
   - GFW blacklist rule → PAC: "DIRECT" → Router: direct connection
3. Verify whitelist rules always result in direct connection
4. Verify automatic fallback still works for misconfigured proxies

## Dependencies

- Changes to `gfw/parser.go`: Modify route() function
- Changes to `gfw/generator.go`: Modify writeRuleCondition() function
- No changes required to router logic
- No changes required to PAC evaluator or parser

## Backward Compatibility

- **Configuration**: No breaking changes - all existing configs work
- **Behavior**: 
  - When default proxy configured: More efficient (no hardcoded proxy attempts)
  - When no default proxy configured: Same end result (DIRECT) but more efficient (no connection attempts/logs)
- **Whitelist rules**: Completely unchanged behavior