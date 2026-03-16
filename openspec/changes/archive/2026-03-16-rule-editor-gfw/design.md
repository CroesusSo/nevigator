# Design: Structured Rule Editor with GFW Integration

## Architecture Overview

```
┌──────────────────────────────────────────────────────────────────┐
│                        Admin UI                                  │
│  ┌─────────────┐  ┌────────────────┐  ┌──────────────────────┐  │
│  │ Rules Table  │  │ GFW Config     │  │ Test URL             │  │
│  │ (CRUD)      │  │ (URL, fetch)   │  │                      │  │
│  └──────┬──────┘  └───────┬────────┘  └──────────────────────┘  │
│         │                 │                                      │
├─────────┼─────────────────┼──────────────────────────────────────┤
│         ▼                 ▼            Admin Handlers            │
│  /api/rules (CRUD)  /api/gfw (config, fetch, status)            │
│         │                 │                                      │
├─────────┼─────────────────┼──────────────────────────────────────┤
│         ▼                 ▼                                      │
│  ┌──────────────┐  ┌────────────┐  ┌──────────────────────┐     │
│  │ user-rules.  │  │ GFW        │  │ gfw-rules.json       │     │
│  │ json         │  │ Fetcher    │─────────────────────▶   │     │
│  └──────┬───────┘  └────────────┘  └──────────┬───────────┘     │
│         │                                      │                 │
│         │         PAC Generator                │                 │
│         │                                      │                 │
│         └──────────────┬───────────────────────┘                 │
│                        ▼                                         │
│               ┌────────────────┐                                 │
│               │ proxy.pac      │  (JSON comment + generated JS)  │
│               └───────┬────────┘                                 │
│                       │ fsnotify                                 │
│                       ▼                                          │
│               ┌────────────────┐                                 │
│               │ Evaluator      │  (existing hot-reload)          │
│               └────────────────┘                                 │
└──────────────────────────────────────────────────────────────────┘
```

## New Package: `gfw/`

### `gfw/rules.go` — Rule Types

```go
// MatchType describes how a rule matches traffic.
type MatchType string

const (
    MatchDomainSuffix MatchType = "domain_suffix" // ||example.com
    MatchHostGlob     MatchType = "host_glob"     // shExpMatch pattern
    MatchURLGlob      MatchType = "url_glob"      // full URL pattern
    MatchExact        MatchType = "exact"          // exact host match
)

// Rule represents a single routing rule.
type Rule struct {
    ID      string    `json:"id"`
    Source  string    `json:"source"`  // "user" or "gfw"
    Enabled bool      `json:"enabled"`
    Match   MatchType `json:"match"`
    Value   string    `json:"value"`
    Action  string    `json:"action"`  // "PROXY" or "DIRECT" (for exceptions)
    Route   string    `json:"route"`   // resolved route: "TUN", "SOCKS5 x:port"
}
```

### `gfw/parser.go` — AdBlock Plus Rule Parser

Parses decoded gfwlist text into `[]Rule`.

```
Input rules          →    MatchType        Value             Action
─────────────────────────────────────────────────────────────────────
||google.com         →    domain_suffix    .google.com       PROXY
||1.2.3.4            →    exact            1.2.3.4           PROXY
google.com/          →    host_glob        *google.com*      PROXY
@@||internal.com     →    domain_suffix    .internal.com     DIRECT
! comment            →    (skip)
[AutoProxy ...]      →    (skip)
/regex/              →    (skip)  // out of scope
```

Parsing rules:
- Lines starting with `!` → skip (comments)
- Lines starting with `[` → skip (section headers)
- Lines starting with `@@` → whitelist (Action=DIRECT), strip `@@` prefix, parse remainder
- Lines starting with `||` → domain suffix, strip `||`
- Lines starting with `|` → URL glob, strip `|`
- Lines starting with `/` and ending with `/` → skip (regex, out of scope)
- Everything else → host glob pattern
- Trim whitespace, skip empty lines

### `gfw/fetcher.go` — HTTP Fetcher with Conditional GET

```go
type Fetcher struct {
    SourceURL  string
    LocalPath  string
    HTTPClient *http.Client  // configurable timeout, retry
}

// FetchResult contains the outcome of a fetch attempt.
type FetchResult struct {
    Updated   bool
    RuleCount int
    Error     error
}

// Fetch downloads gfwlist.txt, decodes, parses, and writes gfw-rules.json.
// Uses ETag/If-Modified-Since for conditional GET.
// Skips write if parsed rules are identical to existing file.
func (f *Fetcher) Fetch(ctx context.Context) FetchResult
```

Flow:
1. HEAD or GET with `If-None-Match` / `If-Modified-Since` from stored metadata
2. If 304 → return `Updated: false`
3. If 200 → decode base64, parse, compare with existing
4. If identical → return `Updated: false`
5. If different → atomic write (tmp + rename), return `Updated: true`

### `gfw/generator.go` — PAC JS Generator

```go
// GeneratePAC merges user rules and GFW rules into PAC JavaScript.
func GeneratePAC(userRules []Rule, gfwRules []Rule, gfwMeta GFWMeta) string
```

Output format:
```javascript
/* __PAC_RULES__
{ ...structured JSON... }
*/
function FindProxyForURL(url, host) {
  // User rules
  if (dnsDomainIs(host, ".internal.corp"))
    return "TUN";
  // GFW rules
  if (dnsDomainIs(host, ".google.com"))
    return "SOCKS5 127.0.0.1:1080";
  // ... more ...
  return "DIRECT";
}
```

Rule-to-JS conversion:
```
domain_suffix(".google.com")  →  dnsDomainIs(host, ".google.com")
host_glob("*.google.com")     →  shExpMatch(host, "*.google.com")
url_glob("*google.com*")      →  shExpMatch(url, "*google.com*")
exact("1.2.3.4")              →  host == "1.2.3.4"
```

## Modified Package: `admin/`

### New API Endpoints

| Method | Path | Description |
|--------|------|-------------|
| GET | /api/rules | List all rules (user + GFW) |
| POST | /api/rules | Create a user rule |
| PUT | /api/rules/:id | Update a user rule |
| DELETE | /api/rules/:id | Delete a user rule |
| PUT | /api/rules/:id/toggle | Enable/disable a rule |
| PUT | /api/rules/reorder | Reorder user rules |
| GET | /api/gfw/config | Get GFW config (URL, interval, status) |
| PUT | /api/gfw/config | Update GFW config (URL, interval) |
| POST | /api/gfw/fetch | Trigger manual GFW fetch |
| GET | /api/gfw/status | Fetch status (last update, rule count, errors) |

### Modified UI (`handleUI`)

The admin HTML expands to include:
1. **Rules tab** — table with add/edit/delete/reorder/toggle
2. **GFW tab** — source URL input, update interval dropdown, fetch button, status display
3. **Test tab** — existing test URL (unchanged)

Raw JS textarea removed (or moved to an "Advanced" view).

## Modified Config

```yaml
proxy:
  http_listen: 0.0.0.0:8080
  socks5_listen: 0.0.0.0:1080
  admin_listen: 127.0.0.1:9090
  pac_path: /etc/proxy-pac/proxy.pac
  tun_device: tun0

pac:
  local_ip: ""
  dns_timeout: 500ms
  eval_timeout: 200ms
  enable_dns_cache: true

gfw:
  source_url: "https://github.com/gfwlist/gfwlist/raw/refs/heads/master/gfwlist.txt"
  local_rules_path: "/etc/proxy-pac/gfw-rules.json"
  user_rules_path: "/etc/proxy-pac/user-rules.json"
  update_interval: 24h
  fetch_on_startup: true
  http_timeout: 30s
```

## File Layout After Implementation

```
gfw/
  rules.go        - Rule struct, MatchType constants
  parser.go       - AdBlock Plus rule parser
  fetcher.go      - HTTP fetcher with conditional GET
  generator.go    - PAC JS generation from rules
  parser_test.go  - Parser tests with real gfwlist samples
  fetcher_test.go - Fetcher tests with httptest
  generator_test.go - Generator output tests
```

## Backward Compatibility

- Raw PAC files without `__PAC_RULES__` comment still work (evaluator unchanged)
- If user edits raw PAC via old API, structured rules are ignored until next PAC read
- Existing config files work without `gfw` section (defaults to no GFW)
- No migration needed — users opt-in by adding GFW config
