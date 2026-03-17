## Context

The HTTPS proxy feature (`add-https-proxy-listener`) added the backend capability to accept TLS-wrapped proxy connections on a separate port (`https_listen`) using TLS certificates. However, the admin UI's configuration page does not expose these settings, and the API endpoints that read/write the config file omit these fields. As a result, if a user generates a default config with `https_listen: :8443` and then clicks "Save Configuration" in the UI, the `https_listen` line (and any `tls:` block) is removed from the file, breaking HTTPS proxy.

The admin UI currently only manages a subset of configuration fields (HTTP/SOCKS5/Admin listeners, PAC path, TUN device, default external proxy). It uses a fixed `FullConfig` struct that excludes newer fields like `https_listen` and `tls`.

---

## Goals / Non-Goals

**Goals:**
- Add `https_listen` field to the Settings form
- Add TLS configuration section (certificate_file, key_file, ca_file, client_auth, min/max_version, etc.)
- Update admin API to return these fields in GET `/api/config`
- Update admin API to accept and persist these fields in PUT `/api/config`
- Ensure saving config preserves all existing fields (no data loss)
- Maintain backward compatibility: existing configs without these fields should continue to work

**Non-Goals:**
- Dynamic TLS certificate generation or ACME automation
- Client certificate management UI (just server certs for now)
- Real-time TLS configuration reload (restart required, as with listeners)

---

## Decisions

### Extend FullConfig.Proxy Struct

**Decision**: Add `HTTPSListen string` and `TLSConfig pac.TLSConfig` (or individual TLS fields) to the `FullConfig.Proxy` embedded struct in `admin/handlers.go`.

**Rationale**: Follow existing pattern. The `FullConfig` mirrors the main `Config` but with only fields needed for UI. We already have `HTTPListen`, `SOCKS5Listen`, `AdminListen`. Adding `HTTPSListen` is straightforward. For TLS, we can embed the existing `pac.TLSConfig` (which has yaml tags) or duplicate its fields with json tags.

**Implementation**:
```go
type FullConfig struct {
    Proxy struct {
        HTTPListen           string              `json:"http_listen"`
        SOCKS5Listen         string              `json:"socks5_listen"`
        AdminListen          string              `json:"admin_listen"`
        HTTPSListen          string              `json:"https_listen"`  // NEW
        PACPath              string              `json:"pac_path"`
        TUNDevice            string              `json:"tun_device"`
        DefaultExternalProxy string              `json:"default_external_proxy"`
        ExternalProxies      []pac.ExternalProxy `json:"external_proxies"`
        TLS                  pac.TLSConfig       `json:"tls"`            // NEW
    } `yaml:"proxy"`
    // ... PAC and GFW unchanged
}
```

---

### Handle Get Config

**Decision**: In `handleGetConfig`, after loading FullConfig from file, populate the response struct with all fields including new ones.

**Rationale**: The current response uses an anonymous struct; we can extend it to include `https_listen` and `tls` fields directly from the loaded FullConfig.

**Implementation**:
```go
cfg := struct {
    HTTPListen           string `json:"http_listen"`
    SOCKS5Listen         string `json:"socks5_listen"`
    AdminListen          string `json:"admin_listen"`
    HTTPSListen          string `json:"https_listen"`
    PACPath              string `json:"pac_path"`
    TUNDevice            string `json:"tun_device"`
    DefaultExternalProxy string `json:"default_external_proxy"`
    TLS                  struct { // flatten needed TLS fields
        CertificateFile string `json:"certificate_file"`
        KeyFile         string `json:"key_file"`
        CAFile          string `json:"ca_file"`
        ClientAuth      string `json:"client_auth"`
        // ... etc
    } `json:"tls"`
}{
    HTTPListen:   status.HTTPListen,
    SOCKS5Listen: status.SOCKS5Listen,
    AdminListen:  status.AdminListen,
    HTTPSListen:  full.Proxy.HTTPSListen, // NEW
    PACPath:      full.Proxy.PACPath,
    TUNDevice:    full.Proxy.TUNDevice,
    DefaultExternalProxy: full.Proxy.DefaultExternalProxy,
    TLS:          // map full.Proxy.TLS to flat struct
}
```

Alternatively, we could reuse `FullConfig` directly as the response (after filling live status fields). That would be simpler and automatically include all fields.

---

### Handle Put Config

**Decision**: Extend the `upd` struct in `handlePutConfig` to include `HTTPSListen` and TLS fields, and update the config accordingly.

**Rationale**: Mirror the GET. The PUT currently does partial updates: only non-zero fields are applied. We'll add similar update logic for HTTPS and TLS, with restart required for `https_listen` changes.

**Implementation**:
- Add `HTTPSListen string` and `TLSConfig` to the `upd` struct
- After unmarshaling, if `upd.HTTPSListen != ""`, set `cfg.Proxy.HTTPSListen = upd.HTTPSListen` and `restartRequired = true`
- If `upd.TLS` is non-empty, copy fields to `cfg.Proxy.TLS` (no restart needed for TLS alone)

---

### UI: Settings Form

**Decision**: Add two new form rows:
1. HTTPS Proxy (listen) input
2. TLS Configuration section (maybe collapsible) with inputs: certificate_file, key_file, ca_file, client_auth dropdown, etc.

**Rationale**: Follow existing form layout. The current Settings tab has rows for HTTP/SOCKS5/Admin. We'll insert HTTPS similarly. TLS fields can be in a separate card or within the same card.

**Placement**:
```html
<div class="card">
  <h3>Proxy Listeners</h3>
  <div class="form-row">
    <label>HTTP Proxy (listen)</label>
    <input id="http-listen" ...>
  </div>
  <div class="form-row">
    <label>SOCKS5 Proxy (listen)</label>
    <input id="socks5-listen" ...>
  </div>
  <div class="form-row">
    <label>Admin UI (listen)</label>
    <input id="admin-listen" ...>
  </div>
  <div class="form-row"> <!-- NEW -->
    <label>HTTPS Proxy (listen)</label>
    <input id="https-listen" value="...">
  </div>
</div>
<div class="card">
  <h3>TLS Settings</h3> <!-- NEW -->
  <div class="form-row">
    <label>Certificate file</label>
    <input id="tls-certificate" placeholder="/path/to/cert.pem">
  </div>
  <div class="form-row">
    <label>Key file</label>
    <input id="tls-key" placeholder="/path/to/key.pem">
  </div>
  <!-- other fields as needed -->
</div>
```

---

### UI: JavaScript

**Decision**: Update `loadConfig()` to populate `https-listen` and TLS fields. Update `saveConfig()` to include them in the JSON payload.

**Rationale**: Straightforward extension of existing pattern. The payload in `saveConfig()` must include all configurable fields.

**Implementation**:
```js
function loadConfig() {
  fetch('/api/config').then(r => r.json()).then(cfg => {
    // ... existing fields
    if (document.getElementById('https-listen'))
        document.getElementById('https-listen').value = cfg.https_listen || '';
    if (document.getElementById('tls-certificate'))
        document.getElementById('tls-certificate').value = cfg.tls?.certificate_file || '';
    // ...
  });
}

function saveConfig() {
  const payload = {
    default_external_proxy: ...,
    http_listen: ...,
    socks5_listen: ...,
    admin_listen: ...,
    https_listen: document.getElementById('https-listen').value, // NEW
    pac_path: ...,
    tun_device: ...,
    tls: { // NEW
      certificate_file: document.getElementById('tls-certificate').value,
      key_file: document.getElementById('tls-key').value,
      // ...
    }
  };
  fetch('/api/config', { method: 'PUT', ... });
}
```

---

## Risks / Trade-offs

**[Config file incompatibility]** Older configs lack `tls:` block. **Mitigation**: Zero values are fine; when saving, we'll write empty `tls:` section if fields are left blank. Could also preserve existing nested structure.

**[Partial writes]** The PUT currently only updates non-empty fields. If TLS fields are blank in UI, they might overwrite existing values with empty strings. **Mitigation**: Either treat empty fields as "no change" or explicitly document that blank fields clear the config. More intuitive: only update if non-empty, but then we can't clear a field. We'll aim for: if field is non-empty, update; if empty, leave unchanged. That's slightly more complex logic.

**[Security]** UI exposes file paths; no passwords but paths to private keys. **Mitigation**: This is same as other sensitive paths (PAC file). Ensure proper file permissions; no special handling needed.

**[Restart semantics]** Changing `https_listen` requires restart, but changing TLS files may also require restart (or maybe we can reload TLS without restart?). Currently the server loads TLS at startup; no hot reload. We'll treat TLS changes as requiring restart too, or we could extend the server to support TLS reload. For now, simpler: mark TLS changes as restart-required as well? The design should decide. Let's include a restart_required flag for TLS if we want; but keep it simple: only listener changes require restart; TLS changes can be hot-reloaded if we implement a reload method. However, that's out of scope. So maybe mark TLS changes as requiring restart as well, or do not apply TLS changes until restart (like listeners). We'll choose: TLS changes are applied immediately if we can reload the TLS config; but the server doesn't have a method to reload TLS mid-flight. Actually, the `Server` has a `loadTLSConfig` called only in NewServer. To support hot TLS reload, we'd need to add a method and wire it to some signal. That's a separate feature. For this change, we can either: (a) require restart for TLS changes, or (b) log a warning that TLS changes won't take effect until restart. Simpler: treat TLS config updates as requiring restart as well, so `restartRequired = true` when any TLS field differs. That matches the current user experience (restart needed for any config change? Actually currently only listeners require restart; PAC path can be hot-reloaded. But TLS is like listeners - needs restart. We'll add restartRequired for TLS changes.

**[Validation]** No validation of TLS file paths. **Mitigation**: Could add server-side validation that files exist when saving, but not required. We'll leave it as-is.

---

## Migration Plan

This is additive to the existing `add-https-proxy-listener` change. Steps:
1. Deploy the new binary (it has both backend and UI changes)
2. Existing configs that have `https_listen` and `tls` will now be editable via UI without losing them
3. Configs without those fields can set them via UI and save
4. Restart required after changing listener or TLS settings

No automatic migration needed. The change is backward compatible: if `https_listen` or `tls` are missing, the proxy runs with those disabled (existing behavior).

---

## Open Questions

- Should we expose all TLS config fields (min_version, max_version, cipher_suites, etc.) in UI? Possibly only the most common ones (cert, key, ca, client_auth). Advanced can be manual edit.
- Should TLS changes require restart? Probably yes.
- Should we display a warning that saving TLS settings requires restart? We'll show restart-required flag in the response.

Let's finalize the spec accordingly. We'll likely just include core TLS fields to keep UI manageable.
