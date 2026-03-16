## Context

Currently `cmd/proxy-pac/main.go` loads config from a hardcoded `config.yaml` in CWD. If missing, `os.ReadFile` returns an error and `main()` calls `log.Fatalf`, exiting immediately. The `loadConfig` function already applies sensible defaults to every field when values are absent — the only barrier is the file read itself. This is a common pattern for portable CLI tools that users download and run from any directory.

## Goals / Non-Goals

**Goals:**
- Zero-config first run: user downloads binary and runs it, it works immediately
- XDG-compliant config location: `~/.config/proxy-pac/config.yaml`
- Explicit override via `--conf` flag
- Portable home directory resolution (no reliance on `$HOME` env var)

**Non-Goals:**
- Multi-file config or config inheritance
- Hot-reload of main config (only PAC file has hot-reload today)
- Config validation beyond YAML parsing (existing behavior)

## Decisions

### Config Resolution Order

1. `--conf <path>` — explicit override, always takes priority
2. `~/.config/proxy-pac/config.yaml` — user-level config (XDG)
3. Auto-generate default at `~/.config/proxy-pac/config.yaml`

Rationale: Follows XDG Base Directory spec. Explicit flag is highest priority. Auto-generation ensures first run succeeds without manual setup.

### Home Directory via `os.UserHomeDir()`

Use Go's `os.UserHomeDir()` rather than reading `$HOME` env var directly. This works on all platforms and handles edge cases where `$HOME` may be unset (containers, systemd services).

### Flag Name: `--conf`

Replace the existing `-config` flag with `--conf`. Shorter, conventional. **BREAKING** for users scripting with `-config` — acceptable as this is a configuration tool and scripts can easily adapt.

### Directory Creation with `os.MkdirAll`

When auto-generating config, create the full directory path `~/.config/proxy-pac/` with `os.MkdirAll(path, 0755)`. Write the config file with `os.WriteFile(path, defaultContent, 0644)`.

### Default Config Content

Generate from the existing defaults already defined in `loadConfig()` (lines 46-66 of main.go). The default YAML will mirror `config.yaml.example` structure but use the same values that `loadConfig` applies as fallbacks.

## Risks / Trade-offs

- **Permission denied on `~/.config/`**: If the directory cannot be created (e.g., filesystem permissions), the write fails. Mitigation: catch the error, log a warning, and continue with in-memory defaults.
- **Binary location ambiguity**: Users may expect config next to binary. Mitigation: clearly document XDG path in help text and logs.
- **Breaking `-config` flag**: Any scripts using `-config` will break. Mitigation: this is a config tool, and the change is a simple rename.
