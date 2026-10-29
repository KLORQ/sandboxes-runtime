BIN ?= bin/app

.PHONY: build test check fmt lint vuln runtime-verify clean

build:
	go build ./...

test:
	go test -race -count=1 ./...

check:
	test -z "$$(gofmt -l .)"
	go vet ./...

fmt:
	gofmt -w .

lint:
	golangci-lint config verify
	golangci-lint run ./...

vuln:
	govulncheck ./...

runtime-verify:
	go test ./internal/manifest

clean:
	rm -rf bin
