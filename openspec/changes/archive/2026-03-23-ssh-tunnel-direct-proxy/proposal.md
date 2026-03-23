## Why

The system currently supports external SOCKS5 proxies only. SSH tunneling is a widely used method to route traffic through secure shell connections, providing an alternative to SOCKS5 proxies. Adding SSH tunnel support allows users to leverage existing SSH infrastructure for proxying network traffic, which is particularly useful in environments where SOCKS5 is not available but SSH access is.

## What Changes

- **NEW**: Add SSH tunnel as an external proxy type (alongside SOCKS5)
- **MODIFIED**: Extend external proxy registry to support SSH proxy configuration
- **NEW**: SSH connection management (establish, maintain, and close tunnels)
- **NEW**: SSH authentication support (password and key-based)
- **MODIFIED**: Router to directly dial through SSH tunnels (no local SOCKS5 bridge)

## Capabilities

### New Capabilities
- `ssh-tunnel-proxy`: SSH tunneling as an external proxy, including connection establishment, direct channel multiplexing, and lifecycle management
- `ssh-auth-methods`: SSH authentication with password and private key support

### Modified Capabilities
- `external-proxy-registry`: ADDED SSH proxy type with configuration for SSH host, port, username, authentication method, and optional private key; registry now supports multiple proxy types (SOCKS5 and SSH)
- `router-routing`: `EXTERNAL <id>` resolution now supports both SOCKS5 and SSH proxies, with SSH using direct channel multiplexing through an SSH client

## Impact

- Config: new `kind` field in external proxy entries (`socks5` or `ssh`)
- Config: SSH-specific fields (ssh_port, auth_method, private_key_path, host_key_check)
- Registry: type-based proxy instantiation logic
- Router: For SSH proxies, calls tunnel manager's `Dial` directly instead of `dialSOCKS5WithAuth`
- Admin UI: SSH proxy configuration form and test connectivity
- Dependencies: SSH client library (`golang.org/x/crypto/ssh`)
