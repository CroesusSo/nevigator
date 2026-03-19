# Implementation Tasks

## 1. Extend EvaluatorConfig

- [x] Add `DNSMaxEntries int` and `DNSTTL time.Duration` to `pac/config.go` with appropriate YAML tags (`dns_max_entries`, `dns_ttl`)
- [x] Set default values in `NewEvaluator` if zero: `DNSMaxEntries = 100`, `DNSTTL = 5 * time.Minute`

## 2. Update dnsCache construction

- [x] Change `newDNSCache` signature to accept both `ttl time.Duration` and `maxEntries int`
- [x] Update `NewEvaluator` to call: `e.dnsCache = newDNSCache(e.cfg.DNSTTL, e.cfg.DNSMaxEntries)` (instead of hardcoded 5m)

## 2. Implement LRU in dnsCache

- [x] In `pac/dns.go`, add fields to `dnsCache`: `lruList *list.List`, `maxEntries int`
- [x] Modify `newDNSCache` to initialize `lruList` and set `maxEntries` (default 100 if <= 0)
- [x] Update `get(host)`:
  - On hit, move host to front of `lruList` (if `lruList` exists)
- [x] Update `set(host, addrs)`:
  - Insert/update entry
  - Push host to front of `lruList`
  - If `maxEntries > 0 && len(entries) > maxEntries`, evict LRU (back of list)
- [x] Ensure `lookupHost` unchanged

## 3. Update Default Config Generator

- [x] In `cmd/proxy-pac/main.go` (or config generation function), add `dns_max_entries` and `dns_ttl` to the default PAC config block with values `100` and `5m`

## 4. Tests

- [x] Add unit test in `pac/dns_test.go`: Insert 101 unique hosts with `DNSMaxEntries=100`; verify size stays at 100 and LRU eviction occurs
- [x] Add test for TTL expiry still works
- [x] Add test for `get` moves entry to MRU
- [x] Update any tests that construct `EvaluatorConfig` to include new fields if needed (or rely on defaults) — No changes needed; defaults applied in `NewEvaluator`

## 5. Admin UI

- [x] In `admin/handlers.go`, add `DNSMaxEntries int` and `DNSTTL time.Duration` to the `PAC` struct with YAML tags `dns_max_entries` and `dns_ttl`

## 6. Documentation

- [x] Update `README.md` to document `dns_max_entries` and `dns_ttl` configuration options
