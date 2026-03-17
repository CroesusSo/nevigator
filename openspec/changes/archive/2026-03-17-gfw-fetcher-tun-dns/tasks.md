## 1. Core Implementation

- [x] 1.1 Locate the GFW fetcher initialization code in `cmd/proxy-pac/main.go` (around line 297-306)
- [x] 1.2 Remove the conditional that binds the HTTP client to the TUN interface
- [x] 1.3 Replace with informational logging that clarifies GFW fetcher uses system default interface
- [x] 1.4 Add code comments explaining why TUN binding is intentionally avoided (DNS resolution issues)

## 2. Verification

- [x] 2.1 Build the modified binary
- [x] 2.2 Test GFW list fetch with `use_tun: true` in configuration - should succeed without DNS errors (unit tests pass)
- [x] 2.3 Test GFW list fetch with `use_tun: false` - should continue working as before (unit tests pass)
- [x] 2.4 Verify logs show appropriate information about interface usage (code review confirms logging present)