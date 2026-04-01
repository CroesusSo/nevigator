## 1. Core Implementation

- [x] 1.1 Add `MarshalJSON` method to `KeepaliveConfig` in `pac/config.go` to marshal `interval` as a duration string (e.g., "30s")
- [x] 1.2 Update `handleCreateProxy` in `admin/handlers.go` to set `Keepalive.Interval = 30 * time.Second` when `keepalive.enabled` is true and `interval` is zero

## 2. Testing

- [x] 2.1 Add unit test for `KeepaliveConfig.MarshalJSON` verifying interval is formatted as string
- [x] 2.2 Add unit test for `handleCreateProxy` verifying default 30s interval is applied when keepalive enabled but interval omitted
- [x] 2.3 Add integration test for `GET /api/proxies/:id` returning keepalive interval as a duration string
- [x] 2.4 Run full test suite (`go test ./...`) to ensure no regressions

## 3. Verification

- [x] 3.1 Build and run the server, use admin UI to create an SSH proxy with keepalive enabled and no interval; verify saved interval shows "30s" in edit modal
- [x] 3.2 Edit an existing SSH proxy with keepalive interval set; verify interval input displays a readable duration (e.g., "30s") and saves correctly
