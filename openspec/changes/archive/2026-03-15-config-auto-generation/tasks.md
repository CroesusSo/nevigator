## 1. Config Resolution Logic

- [x] 1.1 Add `resolveConfigPath(confFlag string) (string, bool)` function that returns the config path and whether it needs generation
- [x] 1.2 Implement resolution priority: if `confFlag` is non-empty, return it directly
- [x] 1.3 Use `os.UserHomeDir()` to resolve `~/.config/proxy-pac/config.yaml` as default path
- [x] 1.4 Check if default path exists with `os.Stat`; return path if found
- [x] 1.5 Return default path with `needsGeneration=true` if file does not exist
- [x] 1.6 If `os.UserHomeDir()` fails, log warning and return empty path with `needsGeneration=false`

## 2. Default Config Generation

- [x] 2.1 Add `generateDefaultConfig(path string) error` function
- [x] 2.2 Extract default YAML content from `loadConfig` defaults into a package-level `defaultConfigYAML` constant
- [x] 2.3 Create parent directory with `os.MkdirAll(filepath.Dir(path), 0755)`
- [x] 2.4 Write default config with `os.WriteFile(path, defaultContent, 0644)`
- [x] 2.5 Log success message after writing

## 3. Flag and Main Integration

- [x] 3.1 Rename `-config` flag to `--conf` in `main()`
- [x] 3.2 Update `main()` to call `resolveConfigPath(*confFlag)` before `loadConfig`
- [x] 3.3 If `needsGeneration` is true, call `generateDefaultConfig` and log info message
- [x] 3.4 If `generateDefaultConfig` fails, log warning and continue with in-memory defaults
- [x] 3.5 If `resolveConfigPath` returns empty path, create in-memory `Config` with all defaults and skip `loadConfig`
- [x] 3.6 Update `loadConfig` error handling: only fatal if path was explicitly provided via `--conf`

## 4. Tests

- [x] 4.1 Add `TestResolveConfigPath_FlagPriority` — flag always wins
- [x] 4.2 Add `TestResolveConfigPath_ExistingFile` — finds file in `~/.config/proxy-pac/`
- [x] 4.3 Add `TestResolveConfigPath_AutoGenerate` — returns path when no file exists
- [x] 4.4 Add `TestGenerateDefaultConfig` — verifies file creation and valid YAML content
- [x] 4.5 Add `TestGenerateDefaultConfig_PermissionDenied` — gracefully handles write failure
- [x] 4.6 Add `TestDefaultConfig` — verifies in-memory defaults

## 5. Verify

- [x] 5.1 Run `go build -o proxy-pac ./cmd/proxy-pac` to verify compilation
- [x] 5.2 Run `go test ./cmd/proxy-pac/ -v` to verify all tests pass
- [x] 5.3 Run `go vet ./...` to check for issues
- [x] 5.4 Manual test: run `./proxy-pac` without any config, verify it creates `~/.config/proxy-pac/config.yaml` and `~/.config/proxy-pac/proxy.pac`
- [x] 5.5 Manual test: run `./proxy-pac --conf /tmp/custom.yaml` with custom config, verify PAC is created at `/tmp/proxy.pac`
