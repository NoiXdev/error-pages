VERSION := $(shell sed -n 's/.*"version": *"\([^"]*\)".*/\1/p' version.json)

.PHONY: test lint vuln build run

test:
	go test ./...

lint:
	golangci-lint run ./...

vuln:
	go tool -modfile=tools/go.mod govulncheck ./...

build:
	go build -ldflags "-X main.version=$(VERSION)" -o bin/error-pages ./cmd/error-pages

run:
	go run -ldflags "-X main.version=$(VERSION)" ./cmd/error-pages
