## Why

proxy-pac currently fails to start if `config.yaml` is not found in the current working directory. This forces users to manually create a config file before first use, creating unnecessary friction for a tool that already has sensible defaults for every configuration field. Users should be able to run the binary and have it work immediately.

## What Changes

- **Add `--conf` flag** to specify an explicit config file path
- **Config resolution priority**: `--conf` flag → `$HOME/.config/proxy-pac/config.yaml` → auto-generate default
- **Auto-generate default config** at `$HOME/.config/proxy-pac/config.yaml` when no config exists (first run)
- **Use `os.UserHomeDir()`** instead of `$HOME` env var for portable home directory resolution
- **Rename existing `-config` flag** to `--conf` (BREAKING for anyone using `-config`)

## Capabilities

### New Capabilities
- `config-resolution`: Config file discovery and auto-generation with XDG-compliant path

### Modified Capabilities

## Impact

- **Affected code**: `cmd/proxy-pac/main.go` — flag parsing, config loading, new resolution logic
- **New dependency**: `os.MkdirAll` for creating `~/.config/proxy-pac/` directory
- **User-facing change**: New `--conf` flag replaces `-config`; zero-config first-run experience
- **No changes to**: PAC evaluation, proxy server, router, admin UI
