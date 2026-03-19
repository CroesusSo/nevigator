# Migration from JavaScript PAC to Native Rule Format

The proxy no longer evaluates JavaScript PAC files. Instead, PAC files must contain structured rules in the `__PAC_RULES__` JSON variable. This document explains how to convert existing custom PAC scripts.

## Background

Previously, the proxy used the goja JavaScript runtime to evaluate arbitrary PAC functions. For performance and simplicity, the proxy now uses a native Go rule engine that processes only the `__PAC_RULES__` JSON block. The JavaScript `FindProxyForURL` function is ignored.

## Converting a PAC Script

1. Identify the logic in your existing `FindProxyForURL` function.
2. Replace it with structured rule objects that define the same matching conditions and actions.
3. Assemble the rules into a JSON object with `version` and `rules` array.
4. Save the PAC file with the `var __PAC_RULES__ = {...};` prefix and a minimal `FindProxyForURL` stub (optional).

### Example

Old JavaScript PAC:

```javascript
function FindProxyForURL(url, host) {
  if (dnsDomainIs(host, ".internal.example")) {
    return "TUN";
  }
  if (shExpMatch(host, "*.onion")) {
    return "SOCKS5 127.0.0.1:9050";
  }
  if (shExpMatch(url, "http://blocked.net/*")) {
    return "DIRECT";
  }
  return "DIRECT";
}
```

New native format:

```javascript
var __PAC_RULES__ = {
  "version": 1,
  "rules": [
    {
      "id": "rule-1",
      "source": "user",
      "enabled": true,
      "match": "domain_suffix",
      "value": ".internal.example",
      "action": "PROXY",
      "route": "TUN"
    },
    {
      "id": "rule-2",
      "source": "user",
      "enabled": true,
      "match": "host_glob",
      "value": "*.onion",
      "action": "PROXY",
      "route": "SOCKS5 127.0.0.1:9050"
    },
    {
      "id": "rule-3",
      "source": "user",
      "enabled": true,
      "match": "url_glob",
      "value": "http://blocked.net/*",
      "action": "DIRECT",
      "route": "DIRECT"
    }
  ]
};
function FindProxyForURL(url, host) { return "DIRECT"; }
```

## Supported Match Types

The native engine supports:
- `exact` – exact hostname match (`host === "example.com"`)
- `domain_suffix` – domain suffix match (`dnsDomainIs(host, ".example.com")` or host equals suffix)
- `host_glob` – shell pattern match on host (`shExpMatch(host, "*.example")`)

`url_glob` is **not supported** and will be ignored. If you have URL‑glob rules, either convert them to host‑based patterns or remove them. The `enable_url_glob` configuration can hide them from the generated PAC, but they will not work.

## Rule Actions

- `action: "PROXY"` – the rule is part of the blacklist and routes to the specified `route`.
- `action: "DIRECT"` – the rule is an exception and overrides any blacklist match; routes to `DIRECT` or another route as specified by `route` (typically `DIRECT`).

The router automatically falls back to `DIRECT` if a chosen route (except `DIRECT`) fails.

## Generating PAC Programmatically

Use the `gfw.GeneratePAC` function (or the `gfw` command) to generate a valid PAC file from structured rule objects. The `Route` field in the rule determines the final proxy string.

## Validation

The admin UI validates the `__PAC_RULES__` JSON on save. Invalid JSON or schema errors will be rejected (HTTP 422). The proxy also logs validation errors during startup.

## Further Assistance

For questions, open an issue on GitHub.
