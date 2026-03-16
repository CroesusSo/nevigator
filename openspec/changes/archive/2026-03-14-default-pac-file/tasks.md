## 1. Implement default PAC creation in evaluator

- [x] 1.1 Add `defaultPACContent` constant to `pac/evaluator.go` with a minimal DIRECT PAC
- [x] 1.2 Modify `reload()` to handle `os.IsNotExist` by creating the default file and logging
- [x] 1.3 Ensure the created file is then loaded normally (call `reload()` again or fall through)

## 2. Write tests

- [x] 2.1 Add test: evaluator creates default PAC when file doesn't exist
- [x] 2.2 Add test: evaluator does not overwrite existing PAC file
- [x] 2.3 Add test: default PAC returns DIRECT for any host
- [x] 2.4 Add test: evaluator fails with clear error when directory doesn't exist

## 3. Verify

- [x] 3.1 Run `go test ./pac/...` — all tests pass
- [x] 3.2 Run `go vet ./pac/...` — no issues
- [x] 3.3 Run `go build -o bin/proxy-pac ./cmd/proxy-pac` — builds successfully
