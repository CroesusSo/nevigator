# Why

The current DNS cache (`dnsCache`) is unbounded — it can grow indefinitely limited only by TTL expiration. In high-churn environments (e.g., accessing many CDNs or large-scale deployments), this could lead to excessive memory usage. We need a configurable limit to cap the number of entries. Additionally, the DNS TTL is hardcoded to 5 minutes and not configurable, and the auto-generated config file does not include DNS cache parameters.

# What Changes

- Add `DNSMaxEntries` (default 100) and `DNSTTL` (default 5m) to `EvaluatorConfig`
- Implement LRU eviction in `dnsCache` to respect `DNSMaxEntries`
- Make DNS TTL configurable (was hardcoded to 5m)
- Update default config generator to include the new DNS cache parameters
- Update admin UI's configuration form to expose the new settings
- No DNS statistics added

# Capabilities

### Modified Capabilities
- `pac-evaluation`: More robust DNS caching with bounded memory

# Impact

- **Backward compatible**: Default values preserve current behavior (100 entry limit, 5m TTL)
- **Configuration**: Users can tune DNS cache size and TTL via YAML
- **Memory**: Prevents unbounded growth in DNS cache
- **Generated config**: New config files will include DNS cache parameters
