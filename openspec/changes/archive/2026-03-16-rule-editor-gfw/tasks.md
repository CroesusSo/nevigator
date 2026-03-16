# Tasks: Structured Rule Editor with GFW Integration

## Phase 1: Rule Data Model

- [x] Create `gfw/rules.go` with Rule struct and MatchType constants
- [x] Add rule serialization/deserialization (JSON)
- [x] Write unit tests for rule marshaling

## Phase 2: GFW Parser

- [x] Create `gfw/parser.go` — AdBlock Plus rule parser
- [x] Handle all rule types: `||domain`, host globs, URL globs, `@@` exceptions
- [x] Skip comments, section headers, regex rules
- [x] Trim whitespace, skip empty lines
- [x] Create `gfw/parser_test.go` with real gfwlist samples
- [x] Test edge cases: empty input, all comments, mixed types, unicode

## Phase 3: GFW Fetcher

- [x] Create `gfw/fetcher.go` — HTTP fetcher with conditional GET
- [x] Implement base64 decode of gfwlist.txt
- [x] Implement ETag/If-Modified-Since support
- [x] Skip write if parsed rules are identical to existing
- [x] Atomic file write (tmp + rename)
- [x] Create `gfw/fetcher_test.go` with httptest server

## Phase 4: PAC Generator

- [x] Create `gfw/generator.go` — merge user + GFW rules into PAC JS
- [x] Convert rule types to PAC builtins (dnsDomainIs, shExpMatch, ==)
- [x] Embed structured JSON in `__PAC_RULES__` comment block
- [x] User rules first (priority 1-99), GFW rules second (priority 100+)
- [x] Fallback to DIRECT
- [x] Create `gfw/generator_test.go` — verify JS output

## Phase 5: Rule Storage

- [x] Implement user-rules.json read/write (atomic)
- [x] Implement gfw-rules.json read (written by fetcher)
- [x] Add user rule CRUD functions
- [x] Add rule reorder support

## Phase 6: Config Integration

- [x] Add GFW config struct to `config.yaml.example`
- [x] Load GFW config in `cmd/proxy-pac/main.go`
- [x] Wire fetcher on startup (if `fetch_on_startup: true`)
- [x] Implement periodic fetch loop with configurable interval

## Phase 7: Admin API Endpoints

- [x] Add GET /api/rules — list all rules
- [x] Add POST /api/rules — create user rule
- [x] Add PUT /api/rules/:id — update user rule
- [x] Add DELETE /api/rules/:id — delete user rule
- [x] Add PUT /api/rules/:id/toggle — enable/disable
- [x] Add PUT /api/rules/reorder — reorder rules
- [x] Add GET /api/gfw/config — get GFW settings
- [x] Add PUT /api/gfw/config — update GFW URL and interval
- [x] Add POST /api/gfw/fetch — manual trigger
- [x] Add GET /api/gfw/status — fetch status and stats

## Phase 8: Admin UI

- [x] Build Rules tab — table with add/edit/delete/reorder/toggle
- [x] Build GFW tab — source URL, interval, fetch button, status
- [x] Update existing Test URL tab (keep working)
- [x] Wire UI to new API endpoints

## Phase 9: PAC Generation Pipeline

- [x] Create PAC generator that reads user-rules.json + gfw-rules.json
- [x] Write generated PAC to proxy.pac
- [x] Trigger on: user rule change, GFW fetch complete, admin PAC save
- [x] Ensure fsnotify watcher catches PAC changes for evaluator hot-reload

## Phase 10: Integration & Testing

- [x] Integration test: user rules take precedence over GFW rules
- [x] Integration test: GFW fetch → PAC regen → evaluator hot-reload
- [x] Integration test: manual fetch from admin UI
- [x] Integration test: periodic background fetch
- [x] Integration test: backward compat with raw PAC file (no structured rules)
- [x] Run full test suite: `go test ./...`
