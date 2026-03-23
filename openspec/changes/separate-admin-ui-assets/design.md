# Design: Separate Admin UI Assets (HTML/CSS/JS)

## Overview

This change splits the admin UI into separate HTML, CSS, and JavaScript files, embedded in the binary using Go's `embed` module. The admin UI will be served from:
- `/` → `index.html`
- `/static/style.css` → CSS stylesheet
- `/static/script.js` → JavaScript code

## Current Implementation

The `handleUI` function in `admin/handlers.go` (lines 213-1664) returns a single large HTML string:
```go
html := `<!DOCTYPE html>...` // ~1448 lines of HTML with embedded <style> and <script>
w.Write([]byte(html))
```

The entire HTML (including 72 lines of CSS and 987 lines of JS) is inline.

## Proposed Implementation

### 1. File Structure

```
admin/
├── handlers.go          # Refactored to ~150 lines
├── static/
│   ├── index.html       # HTML only (no <style> or <script> tags)
│   ├── style.css        # All CSS from <style> block
│   └── script.js        # All JavaScript from <script> block
├── handlers_test.go     # May need minor updates
└── config_api_test.go   # Unchanged
```

### 2. Content Extraction

**index.html**:
- Start with line 215 from handlers.go
- Remove lines 219-286 (the `<style>...</style>` block)
- Remove lines 673-1660 (the `<script>...</script>` block)
- Keep the `<head>` section but add:
  ```html
  <link rel="stylesheet" href="/static/style.css">
  ```
  in the `<head>` (after the title)
- Keep the closing `</body></html>` but add before `</body>`:
  ```html
  <script src="/static/script.js"></script>
  ```

**style.css**:
- Extract lines 220-285 (all CSS rules)
- Remove the `<style>` and `</style>` tags
- Save as plain CSS file

**script.js**:
- Extract lines 674-1659 (all JavaScript code)
- Remove the `<script>` and `</script>` tags
- Save as plain JavaScript file

### 3. Embed Setup in handlers.go

After the imports, add:

```go
import (
    "embed"
    // ... other imports
)

//go:embed static/*
var staticAssets embed.FS
```

### 4. Updated handleUI Function

```go
func (h *Handler) handleUI(w http.ResponseWriter, r *http.Request) {
    // Serve the embedded index.html
    data, err := staticAssets.ReadFile("static/index.html")
    if err != nil {
        http.Error(w, "Internal Server Error", http.StatusInternalServerError)
        log.Printf("Failed to read embedded UI: %v", err)
        return
    }
    w.Header().Set("Content-Type", "text/html")
    w.Write(data)
}
```

### 5. Add Static Asset Handler

Add a new method to `Handler`:

```go
func (h *Handler) handleStatic(w http.ResponseWriter, r *http.Request) {
    // r.URL.Path is like "/static/style.css"
    // Strip the "/static/" prefix to get the file path in embed FS
    filePath := strings.TrimPrefix(r.URL.Path, "/static/")
    
    // Security: prevent directory traversal
    if strings.Contains(filePath, "..") {
        http.Error(w, "Not Found", http.StatusNotFound)
        return
    }
    
    // Read the embedded file
    data, err := staticAssets.ReadFile("static/" + filePath)
    if err != nil {
        http.Error(w, "Not Found", http.StatusNotFound)
        return
    }
    
    // Set Content-Type based on file extension
    ext := strings.ToLower(filepath.Ext(filePath))
    switch ext {
    case ".css":
        w.Header().Set("Content-Type", "text/css; charset=utf-8")
    case ".js":
        w.Header().Set("Content-Type", "application/javascript; charset=utf-8")
    default:
        w.Header().Set("Content-Type", "application/octet-stream")
    }
    
    w.Write(data)
}
```

### 6. Update Router Registration

In `NewHandler` or wherever routes are registered (currently lines 94-164), add:

```go
// Existing: m.HandleFunc("/", h.handleUI)
// Add:
m.HandleFunc("/static/", h.handleStatic)  // Must come BEFORE "/" catch-all if using http.StripPrefix pattern
```

If using `http.StripPrefix`, adjust route registration accordingly.

### 7. MIME Types

- CSS files → `text/css; charset=utf-8`
- JavaScript files → `application/javascript; charset=utf-8`

Both are cacheable; we could add `Cache-Control: public, max-age=3600` but since assets are embedded and change with binary rebuild, it's not critical.

## API Contracts

No changes to API contracts:
- All API endpoints (`/api/*`, `/test`, `/pac`, `/status`, etc.) unchanged
- The HTML at `/` is semantically identical (just references external CSS/JS)
- All JavaScript functionality remains the same
- CSS classes and IDs unchanged

## Dependencies

- Go 1.16+ (for `embed` module)
- Standard library: `embed`, `path/filepath`, `strings`

## Risks and Mitigations

### Risk: Path traversal via /static/../ or similar
**Mitigation**: Strip `/static/` prefix, check for `..`, and prepend `static/` only for known-safe filenames. The simple check `strings.Contains(filePath, "..")` blocks traversal.

### Risk: Missing MIME types
**Mitigation**: Explicitly set `Content-Type` based on file extension; default to `application/octet-stream` for unknown extensions.

### Risk: Static handler catching unintended paths
**Mitigation**: Register `/static/` route with appropriate precedence (before any catch-all routes). Only files in `static/` directory are embedded, so no risk of leaking other files.

### Risk: Tests that search for `<style>` or `<script>` tags in HTML response
**Mitigation**: Update such tests to check for `<link>` and `<script src>` instead, or test CSS/JS content via `/static/` endpoints.

### Risk: Browser caching of stale assets during development
**Mitigation**: Since assets are embedded and change with binary rebuild, developers must rebuild to see CSS/JS changes. Document this workflow.

## Testing

1. **Existing UI tests**:
   - `TestUIHasEnhancedFeatures` may need updates to check for `<link>` and `<script src>` instead of inline CSS/JS
   - Verify the test still ensures all expected elements are present

2. **New static asset tests**:
   - Test that `GET /static/style.css` returns CSS with correct Content-Type
   - Test that `GET /static/script.js` returns JavaScript with correct Content-Type
   - Test that unknown static paths (e.g., `/static/foo.txt`) return 404
   - Test that path traversal attempts (e.g., `/static/../handlers.go`) return 404

3. **Integration**:
   - Start the server and visit `/` in a browser; verify UI loads correctly and CSS/JS are fetched
   - Check Network tab: CSS and JS should be served with correct MIME types
   - Verify all UI functionality (tabs, forms, modals) works as before

## Alternative Considered

### Alternative: Use http.FileServer with http.FS
Could wrap embedded FS with `http.FS(staticAssets)` and use `http.FileServer`. But since we only have three specific files, a custom handler is simpler and gives more control over MIME types and error handling.

### Alternative: Bundle CSS/JS into HTML at build time
Some tools concatenate and minify CSS/JS into HTML for single-file deployment. We're doing the opposite - separating them for maintainability. This is the right direction.

### Alternative: Keep CSS/JS inline but in separate Go string variables
Would still mix concerns and make editing painful. Not recommended.

## Implementation Order

1. Extract CSS and JS to separate files (one-time copy)
2. Update HTML to reference external files
3. Add embed directive and static handler to Go
4. Update tests
5. Verify build and functionality

## Migration Path

- No database or config migration needed
- Old binaries continue to work (no config changes)
- New binary replaces old one directly