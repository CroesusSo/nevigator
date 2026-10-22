.PHONY: build test smoke clean lint lint-fix

build:
	go build -o bin/proxy-pac ./cmd/proxy-pac

test:
	go test ./...

# End-to-end check: builds the real binary, starts it and exercises the admin
# API, the SOCKS5 listener and the HTTP proxy. Separate from `test` because it
# builds and runs a process, and the offline environment may lack a usable
# network for the admin port.
smoke:
	go test -count=1 -v -timeout 5m ./smoke/

clean:
	rm -rf bin/

lint:
	golangci-lint run ./...

lint-fix:
	golangci-lint run --fix ./...
