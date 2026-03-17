## Why

The proxy currently only accepts plain HTTP connections on its HTTP port (default :8080). Users attempting to use the HTTPS proxy protocol (e.g., `curl -x https://localhost:8080`) receive an SSL error "wrong version number" because the server responds with plain HTTP instead of performing a TLS handshake. This limits the proxy's use in environments that require encrypted proxy connections and prevents compatibility with clients that mandate HTTPS proxy protocols.

## What Changes

- **Add `https_listen` configuration** to enable TLS-wrapped proxy connections on a separate port (or same port with different protocol)
- **Create TLS listener** in proxy server that wraps the HTTP handler with TLS using existing `TLSConfig`
- **Modify server start logic** to conditionally start HTTPS proxy when `https_listen` is configured and certificates are available
- **Update status reporting** to include HTTPS listener address
- **Maintain backward compatibility** - existing HTTP and SOCKS5 functionality unchanged

## Capabilities

### New Capabilities
- `https-proxy-listener`: Accepts TLS-encrypted proxy connections and routes them identically to HTTP CONNECT requests

### Modified Capabilities
*(none)*

## Impact

- `pac/config.go`: Add `HTTPSListen` field to `ProxyConfig`
- `proxy/server.go`: Add HTTPS listener initialization and graceful shutdown
- `cmd/proxy-pac/main.go`: Wire `https_listen` config value into `ProxyConfig`
- Configuration schema: Add `https_listen` under `proxy:` section
