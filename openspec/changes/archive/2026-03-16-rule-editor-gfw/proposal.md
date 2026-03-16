# Structured Rule Editor with GFW List Integration

## Problem

The current admin UI only provides a raw JavaScript textarea for editing PAC files. Users must write PAC JS syntax manually — error-prone, no structure, no way to distinguish user rules from auto-generated rules.

There is no GFW list integration at all. Users who want to route GFW-blocked traffic through a proxy must manually download gfwlist.txt, decode base64, parse AdBlock Plus syntax, convert to PAC JS, and paste it into the textarea. This is tedious and must be repeated to stay current.

## Solution

Add a structured rule editor to the admin UI and integrate GFW list fetching/parsing. Rules from both sources coexist in a single PAC file, with user rules always taking precedence.

### Structured Rule Editor

Replace the raw JS textarea with a table-based UI for managing routing rules:

| # | Enabled | Match Type | Pattern | Route |
|---|---------|------------|---------|-------|
| 1 | ✓ | dnsDomainIs | .internal.corp | TUN |
| 2 | ✓ | shExpMatch | \*.onion | SOCKS5 127.0.0.1:9050 |
| 3 | ✓ | (default) | (catch-all) | DIRECT |

Users add/edit/delete rules via form inputs. No JavaScript knowledge required.

### GFW List Integration

A Go-based parser reads the gfwlist.txt format:
- Base64 decode
- Parse AdBlock Plus rules (||domain, /regex/, @@exceptions, comments)
- Convert to structured rule format
- Fetch with conditional GET (ETag/If-Modified-Since)
- Periodic background updates (configurable interval)
- Manual "Fetch Now" button in UI

### Rule Priority

```
1. User rules (priority 1-99)     ← evaluated first, always wins
2. GFW rules  (priority 100+)    ← evaluated second
3. Fallback: DIRECT
```

User rules shadow GFW rules. No conflict possible.

### GFW Configuration UI

```
Source URL: [https://github.com/gfwlist/gfwlist/raw/refs/heads/master/gfwlist.txt] [Save]
Local file: /etc/proxy-pac/gfw-rules.json
Last updated: 2026-03-15 14:30  (4721 rules)
[Fetch Now]    Auto-update: [Every 24h ▼]
```

### PAC File Format

Rules stored as structured JSON in a special comment block at the top of the PAC file. Generated JS follows below. The file remains a valid PAC file that works with any PAC consumer (browsers, OS, this proxy).

## Scope

- New package: `gfw/` (parser, fetcher, rule structs)
- Modified package: `admin/` (new UI pages, new API endpoints)
- Modified package: `pac/` (rule-aware PAC generation, merge logic)
- Modified config: `config.yaml` (GFW settings)
- PAC file format backward-compatible (raw JS still works)

## Non-goals

- Regex rule support (drop /regex/ rules from gfwlist, most are edge cases)
- Multiple GFW sources (single source URL for now)
- Rule import/export formats other than gfwlist
- Real-time collaborative editing
