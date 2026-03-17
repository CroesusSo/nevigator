## 1. Configuration Changes

- [x] 1.1 Add `HTTPSListen` field to `ProxyConfig` struct in `pac/config.go`
- [x] 1.2 Update `defaultConfigYAML` in `cmd/proxy-pac/main.go` to include `https_listen: ":8443"` (with default value)
- [x] 1.3 Ensure config parsing loads `https_listen` into `ProxyConfig.HTTPSListen`

## 2. HTTPS Listener Implementation

- [x] 2.1 In `proxy/server.go`, add `httpsServer *http.Server` field to `Server` struct
- [x] 2.2 In `NewServer`, after creating `httpServer`, check if `cfg.HTTPSListen != "" && cfg.TLSConfig` is valid
- [x] 2.3 If conditions met, create a second `http.Server` with same `Handler` (mux) and `TLSConfig` set
- [x] 2.4 Start HTTPS listener in a goroutine with `tls.NewListener` wrapping the TCP listener
- [x] 2.5 Add error handling: log failure to start HTTPS server but don't block overall startup
- [x] 2.6 Update `ServerStatus` to include `HTTPSListen string` field
- [x] 2.7 Update `Status()` method to populate `HTTPSListen` from config

## 3. Graceful Shutdown

- [x] 3.1 In `Server.Stop()`, if `httpsServer` is non-nil, call `httpsServer.Close()`
- [x] 3.2 Ensure both HTTP and HTTPS servers shut down cleanly on signal

## 4. Main Integration

- [x] 4.1 In `cmd/proxy-pac/main.go`, after loading config, ensure `cfg.Proxy.HTTPSListen` is passed to `proxy.ProxyConfig`
- [x] 4.2 Verify TLSConfig is passed as before (unchanged)
- [x] 4.3 Default config generation includes placeholder `https_listen: ":8443"` (default value)

## 5. Testing and Validation

- [x] 5.1 Write unit test: `NewServer` creates HTTPS server when `HTTPSListen` set and `TLSConfig` valid
- [x] 5.2 Write unit test: `NewServer` does NOT create HTTPS server when `HTTPSListen` empty
- [x] 5.3 Write unit test: `NewServer` logs error when `HTTPSListen` set but `TLSConfig` invalid
- [x] 5.4 Integration test: configure `https_listen`, start server, run `curl -x https://localhost:<https_port> https://www.google.com` and verify success
- [x] 5.5 Verify `Status().HTTPSListen` returns configured address when HTTPS enabled
- [x] 5.6 Run all existing tests to ensure no regressions
