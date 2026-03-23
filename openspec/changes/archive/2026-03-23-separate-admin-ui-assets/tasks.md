# Tasks: Separate Admin UI Assets (HTML/CSS/JS)

## Implementation Tasks

### Phase 1: Extract Assets to Files

- [x] **1.1** Create `admin/static/` directory if it doesn't exist: `mkdir admin/static`
- [x] **1.2** Extract CSS: Read lines 219-286 from `handlers.go`, remove `<style>` and `</style>` tags, save as `admin/static/style.css`
- [x] **1.3** Extract JS: Read lines 673-1660 from `handlers.go`, remove `<script>` and `</script>` tags, save as `admin/static/script.js`
- [x] **1.4** Extract HTML:
  - Copy lines 215-218 (DOCTYPE, html, head start, title) to `admin/static/index.html`
  - Add `<link rel="stylesheet" href="/static/style.css">` in `<head>` after the title
  - Copy lines 287-672 (body content up to closing `</div>` before scripts) to `index.html`
  - Add `<script src="/static/script.js"></script>` before `</body>`
  - Copy lines 1661-1662 (closing `</body></html>`) to `index.html`
- [x] **1.5** Verify extracted files are well-formed HTML, CSS, and JS (can use linters or browser dev tools)

### Phase 2: Modify handlers.go

- [x] **2.1** Add `"embed"` to the import block in `admin/handlers.go` (alphabetically sorted)
- [x] **2.2** Add embed directive after imports:
  ```go
  //go:embed static/*
  var staticAssets embed.FS
  ```
- [x] **2.3** Replace `handleUI` function (currently ~450 lines) with the simplified version from design (uses `staticAssets.ReadFile("static/index.html")`)
- [x] **2.4** Add new `handleStatic` method to `Handler` type (from design) to serve CSS/JS files
- [x] **2.5** Update route registration (in `NewHandler` or equivalent) to add:
  ```go
  m.HandleFunc("/static/", h.handleStatic)
  ```
- [x] **2.6** Ensure `handleStatic` is registered BEFORE any catch-all route like "/" if applicable

### Phase 3: Update Tests

- [x] **3.1** Review `admin/handlers_test.go` for tests that check inline CSS/JS presence
  - Search for tests that look for `<style>`, `<script>`, or specific CSS/JS code in HTML response
- [x] **3.2** Update `TestUIHasEnhancedFeatures` or similar tests:
  - Change assertions to check for `<link rel="stylesheet" href="/static/style.css">` instead of inline CSS
  - Change assertions to check for `<script src="/static/script.js"></script>` instead of inline JS
  - Keep other assertions about HTML structure unchanged
- [x] **3.3** Add new tests for static asset handler:
  - `TestHandleStatic_CSS`: GET `/static/style.css` returns 200, correct Content-Type, non-empty body
  - `TestHandleStatic_JS`: GET `/static/script.js` returns 200, correct Content-Type, non-empty body
  - `TestHandleStatic_NotFound`: GET `/static/nonexistent.txt` returns 404
  - `TestHandleStatic_PathTraversal`: GET `/static/../handlers.go` returns 404

### Phase 4: Build and Verify

- [x] **4.1** Build binary: `make build` or `go build -o proxy-pac ./cmd/proxy-pac`
- [x] **4.2** Run binary and verify admin UI loads at `http://localhost:9090/` (or configured admin port)
- [x] **4.3** Check browser DevTools Network tab:
  - `index.html` loads (200)
  - `style.css` loads (200, text/css)
  - `script.js` loads (200, application/javascript)
- [x] **4.4** Verify all UI functionality works: tabs, forms, modals, API calls
- [x] **4.5** Run all admin tests: `go test ./admin -v`
- [x] **4.6** Run full test suite: `make test` or `go test ./...`

### Phase 5: Cleanup

- [x] **5.1** Remove the old HTML string variable from `handlers.go` (the ~1448 lines that were there)
- [x] **5.2** Verify no unused imports remain (should be minimal change since HTML was just a string)
- [x] **5.3** Count lines in `handlers.go` and confirm it's reduced by ~1500 lines (original ~2885 → now 1471)
- [ ] **5.4** Commit changes with a clear message: "refactor: separate admin UI into HTML/CSS/JS files using embed"

## Acceptance Criteria

- [ ] Binary builds without errors
- [ ] Admin UI loads correctly at `/` with proper styling and functionality
- [ ] CSS is served from `/static/style.css` with `text/css` Content-Type
- [ ] JavaScript is served from `/static/script.js` with `application/javascript` Content-Type
- [ ] All existing API endpoints function correctly (no regression)
- [ ] All admin tests pass (including any updated ones)
- [ ] `admin/handlers.go` line count reduced by at least 1400 lines
- [ ] No inline CSS or JavaScript remains in `index.html`

## Notes

- **Go version**: Requires Go 1.16+ (embed module). Verify `go.mod` has `go 1.16` or higher.
- **File paths**: The embed directive uses paths relative to the source file (`handlers.go`). Since static/ is in admin/, the directive is `//go:embed static/*`.
- **Security**: The `handleStatic` function must sanitize paths to prevent directory traversal (block `..`).
- **Development workflow**: After this change, to edit CSS or JS, developers modify the files in `admin/static/` and rebuild the binary. No code changes to `handlers.go` needed.