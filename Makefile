.PHONY: build test clean

build:
	go build -o bin/proxy-pac ./cmd/proxy-pac

test:
	go test ./...

clean:
	rm -rf bin/
