# Tasks for GFW Enable/Disable Toggle

## Category 1: Configuration
- [x] 1.1 Add `Disabled bool \`yaml:"disabled" json:"disabled"\`` to `Config.GFW` in `cmd/proxy-pac/main.go`
- [x] 1.2 Ensure default `Config.GFW.Disabled` is `false` (zero value) and add comment; include in `defaultConfigYAML()` as `disabled: false`
- [x] 1.3 Add `Disabled bool \`yaml:"disabled" json:"disabled"\`` to `FullConfig.GFW` in `admin/handlers.go`
- [x] 1.4 Verify config loading does not override zero value; no special handling needed

## Category 2: Fetcher
- [x] 2.1 Add `disabled int32` atomic field to `Fetcher` struct in `gfw/fetcher.go`
- [x] 2.2 Implement `SetDisabled(bool)` method
- [x] 2.3 Implement `IsDisabled() bool` method
- [x] 2.4 Initialize `disabled: 0` in `NewFetcher`
- [x] 2.5 At start of `Fetch`, check `if f.IsDisabled() { return FetchResult{Error: fmt.Errorf("GFW fetching disabled")} }`

## Category 3: Main Integration
- [x] 3.1 After creating fetcher, call `gfwFetcher.SetDisabled(cfg.GFW.Disabled)`
- [x] 3.2 Modify `regeneratePAC` helper signature to accept `enabled bool` and propagate to `gfw.RegeneratePAC`
- [x] 3.3 Update both calls to `regeneratePAC` in `cmd/proxy-pac/main.go` to pass `!gfwFetcher.IsDisabled()` as `enabled`
- [x] 3.4 Gate startup fetch: `if cfg.GFW.FetchOnStartup && !gfwFetcher.IsDisabled() { ... }`
- [x] 3.5 Gate periodic fetch loop: inside ticker, add `if gfwFetcher.IsDisabled() { continue }`

## Category 4: PAC Generator
- [x] 4.1 Change `gfw.RegeneratePAC` signature to **append** `enabled bool` as last parameter (after `enableURLGlob bool`)
- [x] 4.2 Inside `RegeneratePAC`, if `!enabled`, set `gfwRules = nil` (empty) and meta `RuleCount = 0`
- [x] 4.3 Keep user rules unchanged regardless of enabled flag
- [x] 4.4 Update all callers: `cmd/proxy-pac/main.go` and `admin/handlers.go`

## Category 5: Admin API
- [x] 5.1 Update `handleGetGFWConfig` to load full config from file and return JSON with fields: `enabled` (computed as `!cfg.GFW.Disabled`), `sourceURL`, `localRulesPath`, `userRulesPath`, `fetchOnStartup`, `updateInterval`
- [x] 5.2 Extend `handlePutGFWConfig` request struct to accept `Disabled` and/or `Enabled` fields
- [x] 5.3 On PUT, merge changes: update `h.fetcher.SourceURL`, call `h.fetcher.SetDisabled(disabled)` based on request, and update `FullConfig` fields in memory
- [x] 5.4 Persist merged `FullConfig` to `h.configPath` atomically
- [x] 5.5 After updating config, call `h.regeneratePAC()` to regenerate PAC with new `enabled` state
- [x] 5.6 Ensure `regeneratePAC` in `admin/handlers.go` reads `!h.fetcher.IsDisabled()` when calling `gfw.RegeneratePAC`

## Category 6: Admin UI
- [x] 6.1 In GFW tab HTML, add a checkbox row: `<div class="form-row"><label>Enable GFW</label><input type="checkbox" id="gfw-enabled"></div>`
- [x] 6.2 Modify GFW tab JavaScript: on tab load, fetch config and set `#gfw-enabled` checked state based on `enabled` field
- [x] 6.3 Implement `saveGFWEnabled()` function that PUTs `{enabled: checkbox.checked}` to `/api/gfw/config` and refreshes status
- [x] 6.4 Attach `saveGFWEnabled` to a button next to the checkbox or auto‑save on change
- [x] 6.5 When checkbox is unchecked, disable URL input and Fetch Now button for clarity
- [x] 6.6 Update `reloadGFWStatus()` to show "Enabled" or "Disabled" state

## Category 7: Documentation
- [x] 7.1 Update `README.md` to document `disabled: false` configuration option under GFW section
- [x] 7.2 Update `AGENTS.md` to reflect new `GFW.Disabled` field in config
- [x] 7.3 Optionally update `openspec/specs/pac-proxy-tun.md` to mention GFW can be disabled

## Category 8: Testing
- [x] 8.1 Unit test: `TestFetcherIsDisabled` initial false, after `SetDisabled(true)` true
- [x] 8.2 Unit test: `Fetcher.Fetch` returns error when disabled
- [x] 8.3 Integration test: config with `disabled: true` results in PAC containing only user rules (no GFW)
- [x] 8.4 API test: PUT `/api/gfw/config` with `{"disabled": true}` returns 200, updates config file, updates fetcher state, and regenerates PAC without GFW rules
- [x] 8.5 End‑to‑end manual test: toggle via UI, verify PAC and fetcher behavior

## Category 9: Cleanup and Review
- [x] 9.1 Search for any hard‑coded assumptions that GFW is always enabled
- [x] 9.2 Ensure default config template (`defaultConfigYAML`) includes `disabled: false`
- [x] 9.3 Run full test suite (`go test ./...`) and fix any failures
- [x] 9.4 Build binary and verify config parsing and default behavior
- [x] 9.5 Review thread safety of atomic flag usage

---

## Notes

- The flag is named `disabled` for backward compatibility (zero = enabled). Internally, `Fetcher` uses `disabled`; API exposes `enabled` as user‑friendly boolean (inverse).
- All gating checks must use the atomic `IsDisabled()` to avoid races.
- PAC regeneration must reflect the current enabled state; it is triggered on any config change that affects the flag.
