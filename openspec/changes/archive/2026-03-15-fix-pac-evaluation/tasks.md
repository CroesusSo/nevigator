## 1. Fix FindProxyForURL Extraction

- [x] 1.1 In `pac/evaluator.go:Evaluate()`, replace `fnVal.Export().(func(string, string) string)` with `goja.AssertFunction(fnVal)`
- [x] 1.2 Update the function call to use goja calling convention: `fn(goja.Undefined(), vm.ToValue(""), vm.ToValue(host))`
- [x] 1.3 Handle the `(goja.Value, error)` return from the goja function call
- [x] 1.4 Extract the string result via `.String()` on the returned goja value

## 2. Update Default PAC Script

- [x] 2.1 Update `defaultPACContent` constant in `pac/evaluator.go` to route `.google.com` through TUN
- [x] 2.2 Use `dnsDomainIs(host, ".google.com")` for domain matching (standard PAC builtin)

## 3. Update Tests

- [x] 3.1 Update `TestEvaluator` — create PAC that returns "TUN", verify it works (already uses custom PAC, should pass with fix)
- [x] 3.2 Update `TestDefaultPACReturnsDirect` — split into two tests:
  - `TestDefaultPAC_RoutesGoogleToTUN` — verify `google.com` and `www.google.com` return TUN
  - `TestDefaultPAC_RoutesOtherToDirect` — verify `example.com` returns DIRECT
- [x] 3.3 Verify `TestEvaluatorTimeout` still passes (infinite loop PAC, timeout to DIRECT)
- [x] 3.4 Verify `TestEvaluatorCreatesDefaultPAC` still passes

## 4. Verify

- [x] 4.1 Run `go build ./...` to verify compilation
- [x] 4.2 Run `go test ./pac/... -v` to verify PAC tests pass
- [x] 4.3 Run `go test ./...` to verify all tests pass
- [x] 4.4 Run `go vet ./...` to check for issues
