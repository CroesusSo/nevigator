## Context

The admin UI's Edit button for SSH tunnels suffers from two bugs:

1. **Incorrect display of keepalive interval**: `GET /api/proxies/:id` returns `Keepalive.Interval` as a raw integer (nanoseconds) because `time.Duration` marshals to integer in JSON. The frontend expects a duration string like `"30s"` and displays the raw integer (e.g., `30000000000`), which is unusable for administrators.

2. **Missing default interval on create**: When creating a new SSH proxy with keepalive enabled but no interval specified, the backend does not apply the default 30s interval. This is inconsistent with the YAML unmarshaling logic (which applies a 30s default) and the `UnmarshalJSON` already defined for `KeepaliveConfig`.

The edit modal (`showEditSSHModal` in `admin/static/script.js`) reads `proxy.keepalive.interval` and sets the input value. When the value is an integer, it shows a huge number. When saved, the interval may be omitted, leading to keepalive with zero interval or misconfiguration.

## Goals / Non-Goals

**Goals:**
- Ensure `GET /api/proxies/:id` returns `keepalive.interval` as a duration string (e.g., `"30s"`)
- Ensure `POST /api/proxies` applies the 30s default when keepalive is enabled and interval is omitted
- Ensure `PUT /api/proxies/:id` also respects defaulting (though existing code already does this via `UnmarshalJSON` on the config struct)
- Maintain backward compatibility with existing configuration files and frontend code

**Non-Goals:**
- Modify the frontend JavaScript (the existing code works correctly if the backend returns proper strings)
- Change keepalive behavior at runtime (only data representation and defaulting)
- Support custom interval formats beyond what `time.ParseDuration` accepts

## Decisions

### Decision 1: Add `MarshalJSON` to `KeepaliveConfig` in `pac/config.go`

**Rationale:** The root cause is that `time.Duration`'s default JSON marshaling produces an integer. By implementing a custom `MarshalJSON` method, we centralize the string conversion for all JSON outputs of `KeepaliveConfig`, ensuring consistency across `GET /api/proxies/:id`, `PUT /api/proxies/:id` responses, and any future use. This approach is clean, requires no changes to handlers that directly encode `ExternalProxy`, and follows the existing pattern of `UnmarshalJSON` for the opposite direction.

**Alternatives considered:**
- Manually construct the JSON response in `handleGetProxyByID` to convert the interval to string. This would work but would duplicate logic if other endpoints later return full proxy details. Central marshaling is preferred.
- Change the frontend to parse integer nanoseconds into seconds and append "s". This would be brittle and hide backend inconsistencies.

### Decision 2: Apply default keepalive interval in `handleCreateProxy`

**Rationale:** The `handleCreateProxy` function currently builds a new `ExternalProxy` from the request and does not apply the 30s default when `keepalive.enabled` is true but `keepalive.interval` is empty. This is inconsistent with `KeepaliveConfig.UnmarshalYAML` and `UnmarshalJSON`, which set a default of 30s. To ensure consistent behavior, we add the same defaulting after parsing the keepalive fields.

**Alternatives considered:**
- Rely on `KeepaliveConfig.UnmarshalJSON` by embedding the config. Not applicable because we are constructing the struct manually, not unmarshaling from JSON.

### Decision 3: No changes needed to `handleUpdateProxy`

**Rationale:** `handleUpdateProxy` writes the updated `ExternalProxy` back to the config file, which is then marshaled to YAML. The default interval is not needed during updates because the existing proxy's keepalive interval is preserved unless explicitly changed. The `UnmarshalJSON` handling for `KeepaliveConfig` is used when the config is next read, not during the update. However, we should ensure that if the update payload disables keepalive, that's respected.

## Risks / Trade-offs

- **Risk**: Older frontend versions might expect an integer interval and break if we start sending strings.
  **Mitigation**: The frontend has always used `String(interval)` which produces a string of digits; this is already broken. The fix makes it correct. No known older version expects integer.

- **Risk**: The `MarshalJSON` method adds slight overhead to JSON encoding of `ExternalProxy`.
  **Mitigation**: Overhead is negligible (formatting a duration string). No performance impact.

- **Trade-off**: The `MarshalJSON` method returns an empty string for interval when it is zero. This matches the expected API contract: if keepalive is disabled, interval may be empty.

## Migration Plan

No data migration needed. Existing configuration files with keepalive intervals stored as `time.Duration` in memory will now marshal as strings. Existing YAML files remain readable via `UnmarshalYAML`. After deployment, the admin UI will immediately display the correct interval format.

## Open Questions

None. The fix is straightforward and well-localized.