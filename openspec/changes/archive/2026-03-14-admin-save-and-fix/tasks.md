## 1. Fix `/test` endpoint host parsing

- [x] 1.1 Rewrite `handleTest` to use `net/url.Parse` for URL extraction, fall back to raw input for bare hostnames
- [x] 1.2 Add test cases in `admin/handlers_test.go` covering full URLs, bare hostnames, host:port, and IP addresses

## 2. Add PAC save handler

- [x] 2.1 Implement `handleSavePAC` that reads request body, compiles in throwaway goja VM, returns 422 on error
- [x] 2.2 Implement atomic write: `os.WriteFile` to tmp path, then `os.Rename` to actual PAC path
- [x] 2.3 Register POST `/pac` route in `RegisterRoutes`

## 3. Update admin UI JavaScript

- [x] 3.1 Fix `save()` to POST content to `/pac`, display validation errors from 422 response
- [x] 3.2 Remove `reload()` function and Reload button from the HTML
- [x] 3.3 Show success/error feedback to user after save attempt

## 4. Cleanup

- [x] 4.1 Remove `handleReload` handler and its route registration
- [x] 4.2 Verify tests pass: `go test ./...`
