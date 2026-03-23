## Why

The current admin UI handler (`admin/handlers.go`) contains a large embedded HTML string (over 100KB) that includes inline CSS and JavaScript. This monolithic structure:
- Makes the file difficult to maintain and navigate
- Violates separation of concerns (HTML, CSS, JS should be separate)
- Hinders collaboration between backend and frontend developers
- Prevents browser caching of static assets

By moving CSS and JavaScript to separate files and using Go's `embed` module, we achieve:
1. **Improved maintainability**: Each file type is isolated and easier to edit
2. **Better collaboration**: Frontend developers can work on CSS/JS without touching Go code
3. **Standard architecture**: Follows web best practices (separation of concerns)
4. **Potential caching benefits**: Browsers can cache CSS/JS separately (though assets are embedded, they're still cacheable)
5. **Cleaner code**: `handlers.go` reduced to HTTP routing logic only

## What Changes

- **NEW**: Create `admin/static/` directory with three files:
  - `index.html` (HTML structure, no inline CSS/JS)
  - `style.css` (all CSS from `<style>` block)
  - `script.js` (all JavaScript from `<script>` block)
- **NEW**: Use Go's `embed` module to embed the entire `static/` directory
- **NEW**: Add HTTP handler for serving static assets at `/static/*`
- **MODIFIED**: Update `handleUI` to serve `index.html` from embedded FS
- **MODIFIED**: Update `admin/handlers.go` to:
  - Import `embed` package
  - Declare embed directive for static assets
  - Add static file server for `/static/` path
- **UNCHANGED**: All API endpoints (`/api/*`, `/test`, `/pac`, `/status`) remain exactly the same

## Capabilities

### New Capabilities
- `static-asset-serving`: Serve embedded static files (CSS, JS) from `/static/` path
- `separate-frontend-assets`: Admin UI split into HTML, CSS, and JS files

### Modified Capabilities
- `admin-handler`: Minimal routing logic; delegates UI to embedded files

## Impact

- **File structure**: Adds `admin/static/index.html`, `admin/static/style.css`, `admin/static/script.js`
- **Code reduction**: `admin/handlers.go` reduces from ~2885 lines to ~150 lines (removes ~1500 lines of HTML/JS)
- **Build process**: No change, assets are still embedded in the binary via `//go:embed`
- **Runtime**: Slightly more efficient - no need to allocate a ~100KB string at startup; files read on-demand from embedded FS
- **Development**: Easier to edit frontend - can modify CSS/JS without opening a massive Go file
- **Tests**: May need updates to reflect that HTML no longer contains inline CSS/JS
- **Dependencies**: None beyond Go 1.16+ standard library