.PHONY: build test clean lint lint-fix

build:
	go build -o bin/proxy-pac ./cmd/proxy-pac

test:
	go test ./...

clean:
	rm -rf bin/

lint:
	golangci-lint run ./...

lint-fix:
	golangci-lint run --fix ./...
