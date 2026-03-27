## Context

The admin UI's Proxies tab currently allows adding and testing external proxies (SOCKS5 and SSH) via proper modal dialogs. However, the "Edit" functionality for existing proxies is implemented using `prompt()` dialogs that only expose address, username, and password fields. Omitted are SSH-specific settings (SSH port, auth method, private key path, host key check, host key fingerprint) and the keepalive configuration. Additionally, the UI misleadingly prompts for "ID (cannot change)" while still presenting an editable field for it. There is no backend endpoint to fetch the full configuration of a single proxy; the list endpoint returns only a subset of fields.

**Current flow:** Click "Edit" → prompts for ID, address, username, password → sends `PUT /api/proxies/:id` with partial data.

**Desired flow:** Click "Edit" → fetch full proxy via `GET /api/proxies/:id` → open pre-filled modal with all relevant fields → send `PUT /api/proxies/:id` with complete data.

## Goals / Non-Goals

**Goals:**
- Replace prompt-based edit dialogs with modal dialogs consistent with the existing "Add Proxy" UI
- Show the proxy ID as read-only (not in an input field)
- Display all editable fields appropriate to the proxy's kind:
  - SOCKS5: address, username, password, keepalive enabled/interval
  - SSH: address, SSH port, username, auth method (select), private key path, host key check, host key fingerprint, keepalive enabled/interval, password (if auth method = password)
- Pre-fill the modal with the proxy's current configuration
- Add a new backend endpoint `GET /api/proxies/:id` returning the full `ExternalProxy` object

**Non-Goals:**
- Modify the existing `ExternalProxy` data model
- Change the `PUT /api/proxies/:id` handler (it already accepts all fields)
- Change the `GET /api/proxies` list endpoint response format
- Merge add and edit modals into a single dynamic modal (we'll keep separate SOCKS5 and SSH modals for simplicity, mirroring the existing add modals)

## Decisions

### 1. Two separate edit modals (SOCKS5 and SSH)

**Decision:** Create dedicated edit modals (`#edit-socks5-modal`, `#edit-ssh-modal`) mirroring the existing add modals, with read-only ID display and pre-filled values.

**Rationale:** Matches the existing pattern used for adding proxies, minimizing cognitive load and code complexity. The two-modal approach is straightforward and avoids writing complex conditional logic to show/hide SSH-specific fields dynamically. It's consistent with how the add modals are already implemented.

**Alternatives considered:** A single modal with dynamic field switching based on proxy kind. This would reduce HTML duplication but require more JavaScript state management and could be harder to maintain. Given the simplicity of the modals, duplication is acceptable.

### 2. New endpoint for full proxy details

**Decision:** Introduce `GET /api/proxies/:id` that returns the complete `ExternalProxy` JSON (including SSH-specific and keepalive fields). The edit modal will call this endpoint to fetch data before opening.

**Rationale:** The list endpoint (`GET /api/proxies`) intentionally returns a subset for display purposes. Adding a detail endpoint preserves backward compatibility, follows RESTful principles, and avoids overloading the list response with data not needed in the table. It also allows future clients to fetch individual proxies without pulling the entire registry.

**Alternatives considered:** Modify `GET /api/proxies` to return full objects. This would break any external client that expects the current response shape. Since the list endpoint is already used by the UI and potentially others, changing it would be a breaking change. The new endpoint is safer and more granular.

### 3. ID presentation

**Decision:** Show the proxy ID as plain text (e.g., `<span>my-proxy-id</span>`) in the modal header or a static field, not as an input. The `PUT` request targets `/api/proxies/:id`, so there's no need to send the ID in the body. The existing add modals have an ID input; the edit modals will omit it.

**Rationale:** Prevents user confusion about whether the ID can be changed. Immutability of ID is a business rule (used as key in config and router registry). Making it read-only clarifies that.

### 4. Keepalive fields

**Decision:** Include keepalive enabled checkbox and interval input only in the SSH edit modal. The SOCKS5 edit modal SHALL NOT include keepalive fields.

**Rationale:** The `ExternalProxy.Keepalive` field is defined for both proxy kinds in the data structure, but its purpose is to maintain persistent SSH connections. SOCKS5 proxies do not require keepalive; the existing SOCKS5 add modal also lacks keepalive. We will not add keepalive to SOCKS5 UI to avoid unnecessary complexity and because the underlying `Keepalive` config is ignored for SOCKS5 by the router. The table's keepalive display should still show the value if present, but it's effectively an SSH-only feature.

## Risks / Trade-offs

| Risk | Mitigation |
|------|------------|
| Code duplication between add and edit modals could lead to maintenance drift | Document the pattern; future refactor can extract shared modal logic if needed. The current scope is small. |
| Forgetting to include new fields (SSH-specific) when `ExternalProxy` evolves | Add tests that verify the edit modals cover all non-zero fields; code review should check. |
| The new endpoint might inadvertently expose sensitive data (passwords, private key paths) | This is consistent with existing behavior: config file stores these in plaintext and the API already returns them in the list (only `has_auth` mask). The edit modal needs the actual password to pre-fill; we'll keep it as-is. No additional risk. |

## Migration Plan

1. Deploy backend change that adds `handleGetProxyByID` for `GET /api/proxies/:id`. This is a new endpoint; no existing clients affected.
2. Deploy frontend changes: add edit modal HTML, replace `editProxy()` with modal launchers, add fetch-and-populate logic, add save handlers that send `PUT` with full data.
3. No database or config file changes required.
4. Rollback: Revert to previous version. The old edit prompts would be restored automatically if only frontend is rolled back; the new endpoint is unused by any other code, so removing it is safe.

## Open Questions

- **Q:** Should the edit modal for SOCKS5 include keepalive fields? **A:** No. Keepalive is an SSH-specific feature for maintaining persistent tunnels. SOCKS5 does not use keepalive. The UI will omit these fields for SOCKS5 to avoid confusion.
- **Q:** What if the proxy's `kind` is unknown (e.g., legacy value)? **A:** Fall back to SOCKS5 modal; the form submits all fields anyway, and backend will accept unknown ones as ignored.
- **Q:** Should the host key fingerprint field be displayed in the edit modal? **A:** Yes, if present. It's part of `ExternalProxy` and relevant for SSH pinning. The add SSH modal does not have it (currently only has host_key_check), but the spec includes it. We should include it in the edit modal to allow editing. We'll add it as an optional text field.

## Additional Enhancements: Summary HTTPS & Tunnels Tab

### Context

The Summary tab currently shows HTTP, SOCKS5, and Admin listeners but omits the HTTPS listener, which is part of the listening addresses returned by `/api/status`. Additionally, there is no UI for SSH tunnel health, even though `/api/status` returns `SSHTunnels` with state (connected/connecting/reconnecting), remote address, and last error.

### Decisions

#### 1. HTTPS listener in Summary tab

**Decision:** Add a fourth line in the "Listening Addresses" card for HTTPS Proxy, fetching `httpsListen` from the same `/api/status` call used by the Summary tab.

**Rationale:** Consistent with existing pattern; no new data source needed. The field `HTTPSListen` is already populated in `ServerStatus`.

#### 2. Separate Tunnels tab

**Decision:** Create a new "Tunnels" tab in the UI that displays SSH tunnel status in a table with columns: Proxy ID, Remote Address, State, Last Error. Data is fetched from `/api/status` `sshTunnels` map.

**Rationale:** SSH tunnels are a distinct concept from listening addresses and connections. A dedicated tab provides a clear place to monitor tunnel health without cluttering Summary. The Summary tab remains for foundational server info; Tunnels tab for operational visibility into SSH infrastructure.

**Alternatives considered:** Show tunnels on Summary tab in a separate card. This was rejected because tunnels could be numerous and need more space; a separate tab allows better layout and potential future expansion (e.g., reconnect controls).

#### 3. Loading tunnel data

**Decision:** The `loadStatus()` function will be extended to also store tunnel data; a new `renderTunnels()` will populate the Tunnels tab table when it becomes active.

**Rationale:** Follows existing pattern (Summary loads once, Connections loads on demand). Tunnel status is relatively static (updated via polling or manual refresh). Simple approach: use same 5-second interval as Summary to auto-refresh status.

### Risks / Trade-offs

| Risk | Mitigation |
|------|------------|
| Tunnels tab may show no tunnels if none configured | Show "No tunnels configured" placeholder message |
| Frequent polling could increase load | Reuse existing 5s interval; no extra requests (status endpoint already queried for Summary). Store latest data centrally and reuse for both Summary and Tunnels. |
| State values might evolve | Display as-is; state is a string from tunnel manager. |

### Migration

No migration needed. These are additive UI changes. The `/api/status` endpoint already includes `HTTPSListen` and `SSHTunnels`; no backend changes required.
