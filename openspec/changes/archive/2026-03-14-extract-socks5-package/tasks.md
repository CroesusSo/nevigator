## 1. Create socks5 package

- [x] 1.1 Create `socks5/` directory and `socks5/proto.go` with constants (ATYP, REP, CMD, VERSION)
- [x] 1.2 Create `socks5/address.go` with `SerializeAddr(host string, port uint16) []byte`
- [x] 1.3 Add `DeserializeAddr(buf []byte, offset int) (host string, port uint16, newOffset int, err error)` to `socks5/address.go`
- [x] 1.4 Create `socks5/read.go` with `ReadFull(conn net.Conn, buf []byte) error` (move from `router/router.go`)
- [x] 1.5 Create `socks5/reply.go` with `EncodeReply(rep byte) []byte` helper

## 2. Write socks5 package tests

- [x] 2.1 Create `socks5/address_test.go` — test Serialize/Deserialize round-trip for IPv4, domain, IPv6
- [x] 2.2 Create `socks5/address_test.go` — test error on unknown ATYP, truncated buffer
- [x] 2.3 Create `socks5/read_test.go` — test ReadFull with short reads
- [x] 2.4 Create `socks5/reply_test.go` — test EncodeReply produces correct 10-byte packets

## 3. Refactor router/router.go

- [x] 3.1 Replace `netFullRead` calls with `socks5.ReadFull`
- [x] 3.2 Replace inline address serialization in `dialSOCKS5` with `socks5.SerializeAddr`
- [x] 3.3 Replace `socsk5Errors` map with `socks5` reply code constants
- [x] 3.4 Remove `netFullRead` function from router/router.go
- [x] 3.5 Remove `socsk5Errors` map and `mapSOCKS5Error` function from router/router.go

## 4. Refactor proxy/server.go

- [x] 4.1 Replace inline ATYP parsing in `handleSOCKS5` with `socks5.DeserializeAddr`
- [x] 4.2 Replace magic number reply bytes with `socks5.EncodeReply()` and `socks5` constants
- [x] 4.3 Replace inline version/cmd checks with `socks5` constants

## 5. Verify

- [x] 5.1 Run `go test ./...` — all existing tests pass
- [x] 5.2 Run `go test -race ./...` — no race conditions
- [x] 5.3 Run `go vet ./...` — no issues
- [x] 5.4 Run `go build -o bin/proxy-pac ./cmd/proxy-pac` — builds successfully
