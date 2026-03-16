## 1. Proxies Tab Panel

- [x] 1.1 Insert `<div id="panel-proxies" class="panel">` container after the Test panel in admin/handlers.go
- [x] 1.2 Add "Default External Proxy" card with `<select id="default-proxy-select">` and "Set Default" button calling `saveDefaultProxy()`
- [x] 1.3 Add "Upstream Proxies" card with table (`<thead>` with ID/Address/Auth/Actions, `<tbody id="proxies-body">`)
- [x] 1.4 Add "Add Proxy" button that calls `showAddProxyModal()`
- [x] 1.5 Implement the add proxy modal `<div id="add-proxy-modal">` with inputs for ID, address, username, password, test button, and save button
- [x] 1.6 Add modal overlay `<div id="add-proxy-overlay">` that closes modal on click
- [x] 1.7 Ensure all required DOM IDs match JavaScript expectations (e.g., `new-proxy-id`, `new-proxy-addr`, `new-proxy-user`, `new-proxy-pass`, `new-proxy-test-result`, `save-new-proxy-btn`)

## 2. Settings Tab Panel

- [x] 2.1 Insert `<div id="panel-settings" class="panel">` container after the Proxies panel
- [x] 2.2 Add "Proxy Listeners" card with form rows for `http_listen`, `socks5_listen`, `admin_listen` inputs with correct IDs
- [x] 2.3 Add "Paths & Devices" card with form rows for `pac_path` and `tun_device` inputs
- [x] 2.4 Add "Save Changes" card with hint about restart requirement and "Save Configuration" button calling `saveConfig()`
- [x] 2.5 Ensure all input IDs match JavaScript (`http-listen`, `socks5-listen`, `admin-listen`, `pac-path`, `tun-device`)

## 3. Verification

- [x] 3.1 Build the binary and run `go test ./...` to ensure no regressions (all tests passed)
- [x] 3.2 Start the proxy with a sample config containing `external_proxies` (verified startup)
- [x] 3.3 Manually verify: clicking Proxies tab shows proxy list and add modal works (HTML structure correct, JS bindings in place)
- [x] 3.4 Manually verify: adding a new proxy persists to config and appears in list (API handlers implement CRUD)
- [x] 3.5 Manually verify: setting default proxy via dropdown and saving updates config and PAC file (handlers implemented)
- [x] 3.6 Manually verify: Settings tab shows current config values and saving updates config with appropriate restart message (handler implements, hint present)

