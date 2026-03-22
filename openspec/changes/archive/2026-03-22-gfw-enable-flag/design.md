# Design: GFW Enable/Disable Toggle

## Architectural Overview

We introduce a boolean `disabled` flag that gates all GFW-related autonomous activity (fetching) and influences PAC generation. The flag is stored in the main configuration and mirrored in the admin `FullConfig`. The `Fetcher` holds an atomic copy for concurrent checks. Changing the flag triggers immediate PAC regeneration to reflect the new rule set.

```
┌──────────────────────────────────────────────────────────┐
│ Config (YAML)                                            │
│   gfw:                                                    │
│     disabled: false                                       │
│     source_url: ...                                       │
│                                                            │
│ ↓ loaded by main → Fetcher.SetDisabled(false)           │
│                                                            │
│ Fetcher.disabled (atomic)                                │
│   ├─ Startup fetch (if !disabled && fetch_on_startup)   │
│   ├─ Periodic loop (each tick: if !disabled) fetch      │
│   └─ Manual fetch (API: if !disabled)                   │
│                                                            │
│ PAC Regeneration                                          │
│   enabled = !fetcher.disabled                            │
│   → RegeneratePAC(..., enabled, ...)                     │
│   → if enabled: include GFW rules                       │
│     else: only user rules                                │
└──────────────────────────────────────────────────────────┘
```

## Configuration Changes

**File**: `cmd/proxy-pac/main.go`

Add to `Config.GFW`:

```go
GFW struct {
    Disabled          bool          `yaml:"disabled" json:"disabled"`
    SourceURL         string        `yaml:"source_url"`
    LocalRulesPath    string        `yaml:"local_rules_path"`
    UserRulesPath     string        `yaml:"user_rules_path"`
    UseTUN            bool          `yaml:"use_tun"`
    UpdateInterval    time.Duration `yaml:"update_interval"`
    FetchOnStartup    bool          `yaml:"fetch_on_startup"`
    HTTPTimeout       time.Duration `yaml:"http_timeout"`
    CustomDNSServers  []string      `yaml:"custom_dns_servers"`
}
```

Zero value `false` means **not disabled** → GFW works. This preserves backward compatibility: old configs without `disabled` behave as before. Default in generated config: `disabled: false`.

Similarly, in `admin/handlers.go` extend `FullConfig.GFW` with the same field and tags.

No changes needed to `pac/config.go` (GFW config is separate).

## Fetcher Changes

**File**: `gfw/fetcher.go`

Add atomic disabled flag:

```go
import "sync/atomic"

type Fetcher struct {
    SourceURL  string
    LocalPath  string
    HTTPClient *http.Client
    disabled   int32 // 0=enabled, 1=disabled

    mu        sync.Mutex
    lastFetch time.Time
    lastError string
    ruleCount int
    etag      string
    lastMod   string
}
```

Initialize in `NewFetcher`:

```go
return &Fetcher{
    SourceURL:  sourceURL,
    LocalPath:  localPath,
    HTTPClient: &http.Client{Timeout: timeout},
    disabled:   0,
}
```

Add methods:

```go
func (f *Fetcher) SetDisabled(disabled bool) {
    if disabled {
        atomic.StoreInt32(&f.disabled, 1)
    } else {
        atomic.StoreInt32(&f.disabled, 0)
    }
}

func (f *Fetcher) IsDisabled() bool {
    return atomic.LoadInt32(&f.disabled) == 1
}
```

Gate `Fetch`:

```go
func (f *Fetcher) Fetch(ctx context.Context) FetchResult {
    if f.IsDisabled() {
        return FetchResult{Error: fmt.Errorf("GFW fetching disabled")}
    }
    // ... rest unchanged ...
}
```

## Main Integration

**File**: `cmd/proxy-pac/main.go`

After creating the fetcher:

```go
if cfg.GFW.SourceURL != "" {
    gfwFetcher = gfw.NewFetcher(cfg.GFW.SourceURL, gfwPath, httpTimeout)
    gfwFetcher.SetDisabled(cfg.GFW.Disabled)

    // Startup fetch
    if cfg.GFW.FetchOnStartup && !gfwFetcher.IsDisabled() {
        go func() {
            log.Printf("[GFW] Fetching on startup...")
            result := gfwFetcher.Fetch(context.Background())
            // ... existing handling ...
            if result.Updated {
                regeneratePAC(userPath, gfwPath, pacPath, cfg.Proxy.DefaultExternalProxy, cfg.PAC.EnableURLGlob)
            }
        }()
    }

    // Periodic fetch
    if cfg.GFW.UpdateInterval > 0 {
        go func() {
            ticker := time.NewTicker(cfg.GFW.UpdateInterval)
            defer ticker.Stop()
            for range ticker.C {
                if gfwFetcher.IsDisabled() {
                    continue
                }
                log.Printf("[GFW] Periodic fetch...")
                result := gfwFetcher.Fetch(context.Background())
                if result.Updated {
                    regeneratePAC(userPath, gfwPath, pacPath, cfg.Proxy.DefaultExternalProxy, cfg.PAC.EnableURLGlob)
                }
            }
        }()
    }
}
```

Note: `regeneratePAC` (defined later in the file) currently calls `gfw.RegeneratePAC(userPath, gfwPath, pacPath, defaultExternalProxy, enableURLGlob)`. We'll modify that helper to also accept `enabled bool`:

```go
func regeneratePAC(userPath, gfwPath, pacPath string, defaultExternalProxy string, enableURLGlob bool, enabled bool) {
    if err := gfw.RegeneratePAC(userPath, gfwPath, pacPath, defaultExternalProxy, enableURLGlob, enabled); err != nil {
        log.Printf("[PAC] Regeneration failed: %v", err)
    }
}
```

Update all call sites accordingly (the two in main.go). Pass `!gfwFetcher.IsDisabled()` as `enabled`.

## Admin API

**File**: `admin/handlers.go`

### FullConfig

Add `Disabled` to `FullConfig.GFW` (with yaml/JSON tags). Ensure it's part of JSON responses.

### handleGetGFWConfig

Currently returns only a subset. Extend to include all relevant fields:

```go
func (h *Handler) handleGetGFWConfig(w http.ResponseWriter, r *http.Request) {
    if h.fetcher == nil {
        w.Header().Set("Content-Type", "application/json")
        json.NewEncoder(w).Encode(map[string]interface{}{
            "configured": false,
        })
        return
    }

    // Load full config to get Disabled and other persisted values
    data, err := os.ReadFile(h.configPath)
    var cfg FullConfig
    if err == nil {
        yaml.Unmarshal(data, &cfg)
    }

    w.Header().Set("Content-Type", "application/json")
    json.NewEncoder(w).Encode(map[string]interface{}{
        "configured":     true,
        "enabled":        !cfg.GFW.Disabled, // send enabled for UI convenience
        "sourceURL":      h.fetcher.SourceURL,
        "localRulesPath": h.gfwPath,
        "userRulesPath":  h.userPath,
        "fetchOnStartup": cfg.GFW.FetchOnStartup,
        "updateInterval": cfg.GFW.UpdateInterval,
        // include other fields as needed
    })
}
```

Because the UI naturally thinks in terms of "enabled", we return `enabled: !Disabled`. Internally we store `Disabled`.

### handlePutGFWConfig

Use pointer fields to distinguish omitted from explicit false. After updating config and fetcher state, always call `h.regeneratePAC()`.

### handleGWFFetch

The manual fetch endpoint (`/api/gfw/fetch`) must respect the disabled flag. Before calling `h.fetcher.Fetch`, check `h.fetcher.IsDisabled()`. If disabled, return HTTP 403 with JSON `{"error":"GFW fetching disabled"}`. This prevents manual fetches when GFW is turned off. The UI should disable the "Fetch Now" button when the checkbox is unchecked.

## Admin UI

**File**: `admin/handlers.go` (HTML/JS part)

### HTML

In the GFW Configuration card, add:

```html
<div class="form-row">
  <label>Enable GFW</label>
  <input type="checkbox" id="gfw-enabled">
</div>
```

Place before the Source URL row.

Also in the status section, display enabled/disabled state:

```html
<div id="gfw-status">Loading...</div>
```

We'll enhance the status to include `"enabled": true/false` from API.

### JavaScript

When the GFW tab is activated, fetch config and populate fields:

```js
fetch('/api/gfw/config')
  .then(r => r.json())
  .then(cfg => {
    if (cfg.configured) {
      document.getElementById('gfw-enabled').checked = cfg.enabled;
      document.getElementById('gfw-url').value = cfg.sourceURL;
      // ... other fields ...
    }
  });
```

Implement `saveGFWEnabled()`:

```js
function saveGFWEnabled() {
  const enabled = document.getElementById('gfw-enabled').checked;
  fetch('/api/gfw/config', {
    method: 'PUT',
    headers: {'Content-Type': 'application/json'},
    body: JSON.stringify({ enabled: enabled })
  })
  .then(r => r.json())
  .then(() => reloadGFWStatus());
}
```

Optionally, when `gfw-enabled` is unchecked, disable the URL input and Fetch button:

```js
document.getElementById('gfw-enabled').addEventListener('change', function() {
  const disabled = !this.checked;
  document.getElementById('gfw-url').disabled = disabled;
  document.getElementById('gfw-fetch-btn').disabled = disabled;
});
```

Update `reloadGFWStatus()` to fetch status and include enabled/disabled indicator.

## PAC Generator

**File**: `gfw/pacgen.go`

Change signature (append `enabled bool` to minimize change):

```go
func RegeneratePAC(userPath, gfwPath, pacPath string, defaultExternalProxy string, enableURLGlob bool, enabled bool) error
```

Inside, if `!enabled`, set `gfwRules` empty:

```go
var gfwRules []Rule
if enabled {
    gfwRules, err = LoadRules(gfwPath)
    if err != nil {
        log.Printf("[PAC] Failed to load GFW rules: %v", err)
        gfwRules = []Rule{}
    }
}
```

Update callers accordingly (see Main Integration and Admin API).

## Concurrency Considerations

- `Fetcher.disabled` is atomic; safe for concurrent reads by loops and API.
- `SetDisabled` updates atomically; the next check in loops sees new value.
- A fetch in progress when disabling will complete; the result may update `gfw-rules.json` but that's okay because `disabled` only affects future fetches. However, if disabled, should we still allow the fetch result to be written? Possibly yes: user manually enabled temporarily and then disabled before fetch finished; it's acceptable to update rules. Not a security issue.
- PAC regeneration triggered by disabled change reads GFW rules from disk; if a fetch is concurrently writing, the atomic rename ensures readers see consistent state.

## Backward Compatibility

- Config: old files without `disabled` → Go zero value `false` → GFW enabled. No migration needed.
- Default config template includes `disabled: false` explicitly for clarity.
- Admin API returns `enabled` (computed) so older UI (if any) still works.
- `RegeneratePAC` signature change requires updating all callers; only two call sites exist in the codebase, both within this repo. No external API exposure.

## OpenAPI Spec (Optional)

If we maintain an API spec, endpoints would change:

- `GET /api/gfw/config` → response includes `enabled` (boolean)
- `PUT /api/gfw/config` → accepts `{enabled?: boolean, sourceURL?: string}`

## Validation

- No special validation: `disabled` is boolean.
- If `sourceURL` is empty, `disabled` has no effect (no fetcher created). Still persisted.

## Monitoring & Logs

No new log statements required. Existing `[GFW]` logs remain; when disabled, fetch attempts are blocked early and may log "GFW fetching disabled" if we choose to log in `Fetch`. We could add debug-level log: `log.Printf("[GFW] Fetch blocked: disabled")`. Optional.

## Rollout

1. Release new binary with code changes.
2. Existing users: config unchanged, behavior identical.
3. New feature accessible via config `disabled: true` or admin UI checkbox.
4. No database or state migration.

## Future Considerations

- Could expose `disabled` as separate from config file (runtime only), but persistence is desired.
- Might want to disable only fetching but still include existing GFW rules (different semantics). Out of scope; if needed later, introduce `include` flag.
