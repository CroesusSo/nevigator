# Design: Fix SSH Tunnel Unsaved Test

## Overview

The admin UI provides a "Test" button to verify proxy connectivity before saving. For SSH tunnels, this fails because the backend endpoint `/api/proxies/test` only supports SOCKS5. This design fixes that by extending the endpoint to handle both SOCKS5 and SSH proxies.

## Architecture Context

```
┌──────────────────────────────────────────────────────────────┐
│ Admin UI (script.js)                                          │
│  - testNewSocks5Proxy() → /api/proxies/test (SOCKS5)        │
│  - testNewSSHProxy()  → /api/proxies/test (SSH) ← BROKEN    │
└──────────────────────────────────────────────────────────────┘
                              │
                              ▼
┌──────────────────────────────────────────────────────────────┐
│ admin/handlers.go                                             │
│  - handleTestProxy (POST /api/proxies/test)                  │
│    Currently: always uses socks5.DialWithAuth                │
│    Fixed: branch on request.kind                             │
└──────────────────────────────────────────────────────────────┘
                              │
                              ▼
┌──────────────────────────────────────────────────────────────┐
│ ssh package                                                  │
│  - NewTunnel(cfg) → *Tunnel                                 │
│  - Tunnel.Dial(dest, timeout) → net.Conn                    │
│  (handles SSH connection + channel establishment)           │
└──────────────────────────────────────────────────────────────┘
```

## Component Responsibilities

### Backend (`admin/handlers.go`)

**File**: `admin/handlers.go:1074-1097`

The `handleTestProxy` function will be updated to:

1. **Accept extended request payload** with JSON schema:

```json
{
  "type": "object",
  "required": ["address"],
  "properties": {
    "address": { "type": "string" },
    "username": { "type": "string" },
    "password": { "type": "string" },
    "kind": { "type": "string", "enum": ["socks5", "ssh"], "default": "socks5" },
    "auth_method": { "type": "string", "enum": ["password", "key"] },
    "private_key_path": { "type": "string" },
    "ssh_port": { "type": "integer", "minimum": 1, "maximum": 65535, "default": 22 },
    "host_key_check": { "type": "string", "enum": ["strict", "ignore"], "default": "strict" },
    "host_key_fingerprint": { "type": "string" }
  }
}
```

2. **Go request struct**:

```go
type testProxyRequest struct {
    Address         string `json:"address"`
    Username        string `json:"username"`
    Password        string `json:"password"`
    Kind            string `json:"kind"`                     // "socks5" (default) or "ssh"
    AuthMethod      string `json:"auth_method"`              // SSH: "password" or "key"
    PrivateKeyPath  string `json:"private_key_path"`         // SSH: key auth
    SSHPort         int    `json:"ssh_port"`                // SSH: port, default 22
    HostKeyCheck    string `json:"host_key_check"`          // SSH: "strict" (default) or "ignore"
    HostKeyFingerprint string `json:"host_key_fingerprint"` // SSH: optional pin
}
```

3. **Processing logic**:

```go
func (h *Handler) handleTestProxy(w http.ResponseWriter, r *http.Request) {
    var req testProxyRequest
    if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
        http.Error(w, "Invalid JSON", http.StatusBadRequest)
        return
    }
    if req.Address == "" {
        http.Error(w, "address required", http.StatusBadRequest)
        return
    }
    // Default kind
    if req.Kind == "" {
        req.Kind = "socks5"
    }

    timeout := 5 * time.Second

    // SSH branch
    if req.Kind == "ssh" {
        // Validation
        if req.AuthMethod == "" {
            http.Error(w, "auth_method required for SSH proxy", http.StatusBadRequest)
            return
        }
        if req.AuthMethod == "key" && req.PrivateKeyPath == "" {
            http.Error(w, "private_key_path required for key auth", http.StatusBadRequest)
            return
        }
        if req.AuthMethod == "password" && req.Password == "" {
            http.Error(w, "password required for password auth", http.StatusBadRequest)
            return
        }
        // Normalize host_key_check (default strict)
        hostKeyCheck := req.HostKeyCheck
        if hostKeyCheck != "ignore" {
            hostKeyCheck = "strict"
        }

        // Build proxy config
        proxy := pac.ExternalProxy{
            ID:                "test-unsaved",
            Kind:              "ssh",
            Address:           req.Address,
            Username:          req.Username,
            Password:         req.Password,
            SSHPort:          req.SSHPort,
            AuthMethod:       req.AuthMethod,
            PrivateKeyPath:   req.PrivateKeyPath,
            HostKeyCheck:     hostKeyCheck,
            HostKeyFingerprint: req.HostKeyFingerprint,
        }

        // Create temporary tunnel
        tunnel := ssh.NewTunnel(proxy)
        defer tunnel.Close()

        // Connect and test
        conn, err := tunnel.Dial("1.1.1.1:80", timeout)
        if err != nil {
            w.Header().Set("Content-Type", "application/json")
            json.NewEncoder(w).Encode(map[string]interface{}{
                "success": false,
                "error": err.Error(),
            })
            return
        }
        conn.Close()
        w.Header().Set("Content-Type", "application/json")
        json.NewEncoder(w).Encode(map[string]interface{}{"success": true})
        return
    }

    // SOCKS5 branch (existing code)
    // ... (unchanged, keep current logic)
}
```

4. **Key details**:
   - Use `defer tunnel.Close()` immediately after successful `ssh.NewTunnel` to guarantee cleanup even if `Dial` panics or errors
   - The `tunnel.Dial` call includes built-in timeout handling (includes SSH connection + channel open)
   - On validation failure, return HTTP 400 with plain text error (consistent with existing behavior)
   - On SSH connection failure (bad credentials, host key mismatch, network unreachable), return HTTP 200 with `{"success": false, "error": "..."}` (consistent with saved test)
   - Normalize `host_key_check`: any value other than `"ignore"` becomes `"strict"`

### Frontend (`admin/static/script.js`)

**File**: `admin/static/script.js:633-681`

The `testNewSSHProxy()` function already collects all necessary fields. It must include `host_key_check` in the payload:

```javascript
function testNewSSHProxy() {
    const addr = document.getElementById('new-ssh-addr').value.trim();
    const user = document.getElementById('new-ssh-user').value.trim();
    const port = document.getElementById('new-ssh-port').value;
    const authMethod = document.getElementById('new-ssh-auth-method').value;
    const pass = document.getElementById('new-ssh-pass').value;
    const privateKeyPath = document.getElementById('new-ssh-private-key-path').value.trim();
    const hostKeyCheck = document.getElementById('new-ssh-host-key-check').value; // ADD
    const resultSpan = document.getElementById('new-ssh-test-result');

    if (!addr) { /* error */ }

    resultSpan.textContent = 'Testing...';
    const data = {
        address: addr,
        username: user,
        password: pass,
        kind: 'ssh',
        auth_method: authMethod,
        private_key_path: privateKeyPath,
        ssh_port: parseInt(port) || 22,
        host_key_check: hostKeyCheck // Include in payload
    };
    // fetch POST /api/proxies/test
}
```

The `testNewSocks5Proxy()` remains unchanged; it may send `kind: "socks5"` or omit it (backend defaults to SOCKS5).

### Data Flow

#### Successful SSH test flow

```
UI: Fill SSH modal → click Test
  ↓
POST /api/proxies/test with JSON body (all SSH fields)
  ↓
admin/handlers.go: handleTestProxy
  ↓ decode → validate → build pac.ExternalProxy
  ↓
ssh.NewTunnel(proxy) → Tunnel{state:closed, closing:false}
  ↓
tunnel.Dial("1.1.1.1:80", 5s)
  ↓
tunnel.connectLoop() (goroutine)
  ↓
ssh.Dial(tcp+SSH handshake+auth)
  ↓
tunnel.client.Dial("tcp", "1.1.1.1:80") → open channel
  ↓
return net.Conn → test returns success → Close()
  ↓
tunnel.Close() sets closing=true, closes client
  ↓
HTTP 200 { "success": true }
```

#### Error flow (SSH auth failure)

```
ssh.Dial fails (e.g., "password denied")
  ↓
tunnel.lastErr set, state=reconnecting, closing=true via defer
  ↓
tunnel.Dial returns err
  ↓
handleTestProxy returns 200 { "success": false, "error": "..." }
  ↓
UI: Shows error message
```

## Edge Cases & Error Handling

| Condition | Handling | HTTP Status | Response |
|-----------|----------|-------------|----------|
| Missing `address` | Validation | 400 | `{"error":"address required"}` |
| `kind: "ssh"`, missing `auth_method` | Validation | 400 | `{"error":"auth_method required for SSH proxy"}` |
| `auth_method: "key"`, missing `private_key_path` | Validation | 400 | `{"error":"private_key_path required for key auth"}` |
| `auth_method: "password"`, missing `password` | Validation | 400 | `{"error":"password required for password auth"}` |
| Invalid `host_key_check` value | Normalize to `"strict"` | N/A | Uses strict verification |
| SSH connection fails (bad creds, host key, network) | Report error | 200 | `{"success":false,"error":"..."}` |
| Timeout after 5s | Report error | 200 | `{"success":false,"error":"dial timeout: ..."}` |
| `ssh.NewTunnel` panic (unlikely) | Recovered? | 500 | Server error |
| SOCKS5 path (default) | Existing behavior | as-is | `{"success":true}` or `{"success":false,"error":"..."}` |

## Alternative Approaches Considered

### Option A: Use tunnel manager for unsaved tests
- Would require temporarily registering a proxy with the manager and then removing it
- Introduces side effects and risk of interfering with persistent tunnels
- **Rejected** in favor of direct `ssh.NewTunnel` usage, which is simpler and isolated

### Option B: Make `dialSSH` public and use it directly
- `dialSSH` is currently private; could be exported as `DialSSH`
- But `NewTunnel` + `Dial` already encapsulates connection management and cleanup
- **Rejected** as unnecessary; `ssh.NewTunnel` is the intended public API

## Dependencies

- `github.com/yourorg/proxy-pac/ssh` (already imported in many packages, including `admin/handlers.go` after change)
- No new external dependencies

## Testing Strategy

### Unit Tests (admin package)

In `admin/handlers_test.go` (or new `handlers_ssh_test.go`):
- Use `httptest.NewRecorder` and `httptest.NewRequest`
- Mock `ssh.NewTunnel` to return a fake `*ssh.Tunnel` with controlled `Dial` and `Close` methods
- Table-driven tests:

| Test | Input | Expected | Mock behavior |
|------|-------|----------|---------------|
| SOCKS5 default | `{address:"host:1080"}` | 200 `{"success":true}` | `socks5.DialWithAuth` returns nil |
| SOCKS5 with kind | `{address:"host:1080","kind":"socks5"}` | 200 `{"success":true}` | same |
| SSH valid | `{address:"host","kind":"ssh","auth_method":"password","password":"pass"}` | 200 `{"success":true}` | `tunnel.Dial` returns nil |
| SSH missing auth_method | `{address:"host","kind":"ssh"}` | 400 `{"error":"auth_method required..."}` | N/A |
| SSH key without private_key_path | `{address:"host","kind":"ssh","auth_method":"key"}` | 400 `{"error":"private_key_path required..."}` | N/A |
| SSH password without password | `{address:"host","kind":"ssh","auth_method":"password"}` | 400 `{"error":"password required..."}` | N/A |
| SSH Dial failure | `{...}` | 200 `{"success":false,"error":"..."}` | `tunnel.Dial` returns error |
| Verify cleanup | any SSH | `tunnel.Close` called exactly once | check mock call count |

### Integration Tests

Create `admin/integration_ssh_test.go`:
- Use a real SSH server (e.g., `github.com/yourorg/proxy-pac/ssh/test-server` or a container)
- Start server on random port in `TestMain` or test setup
- POST `/api/proxies/test` with correct credentials → expect 200 success
- POST with wrong password → expect 200 with `success:false`
- Ensure no goroutine leaks (run with `-race` and `go test -v`)

### Manual Testing

1. Start proxy-pac binary
2. Open Admin UI → Proxies tab → "Add SSH Tunnel"
3. Fill fields with valid SSH credentials → click "Test" → expect "Success!"
4. Change password → click "Test" → expect failure message
5. Test SOCKS5 proxy → ensure still works

## Rollback Plan

If the SSH unsaved test misbehaves after deployment:
1. Revert `admin/handlers.go` changes to previous version (keeps SOCKS5-only)
2. Optionally revert `admin/static/script.js` changes (removes `host_key_check` from payload, though harmless if sent)
3. No data migration needed; Config unchanged

## Monitoring & Observability

- Admin server logs: `[SSH] Tunnel test-unsaved connecting to ...` and result logs from `ssh` package
- No new metrics needed; UI shows success/failure directly
- Errors are reported to user with descriptive messages

## Risks & Mitigations

| Risk | Likelihood | Impact | Mitigation |
|------|-----------|--------|------------|
| Resource leak if `tunnel.Close()` not called | Low | Medium | Use `defer tunnel.Close()` immediately after `NewTunnel` |
| SSH test blocks longer than 5s | Low | Low | `tunnel.Dial` uses its own timeout mechanism; wrapper ensures total time bounded |
| Backend crashes on unknown `host_key_check` value | Low | Low | Normalize to `"strict"` for any value other than `"ignore"` |
| Frontend doesn't send new fields | Low | Medium | UI already has fields; just include them; add unit test to verify payload |
| Breaking existing API consumers | Very Low | Medium | Default `kind` to `"socks5"` ensures compatibility; no fields required |
| Host key verification fails for users without known_hosts | Medium | Medium | Current implementation logs warning and allows; acceptable for test endpoint |

## Open Questions

**Q**: Should we log validation failures at WARN level?  
**A**: Not necessary; they are user errors, not system issues. Return 400 with message is enough.

**Q**: Should we allow customizing test destination?  
**A**: No, fixed to `1.1.1.1:80` for consistency and simplicity.

**Q**: What if `ssh.NewTunnel` returns nil or error?  
**A**: `NewTunnel` only fails if memory allocation fails (practically never). Not handling separately.

## References

- Existing saved proxy test: `admin/handlers.go:1024-1072` (`handleTestProxyByID`)
- SSH tunnel implementation: `ssh/tunnel.go:46-54` (`NewTunnel`), `:59-126` (`Dial`)
- `pac.ExternalProxy` struct: `pac/config.go:46-60`
- Frontend SSH test: `admin/static/script.js:633-681`
