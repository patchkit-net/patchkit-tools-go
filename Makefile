BINARY_NAME=pkt
VERSION?=dev
COMMIT=$(shell git rev-parse --short HEAD 2>/dev/null || echo "none")
DATE=$(shell date -u +%Y-%m-%dT%H:%M:%SZ)
LDFLAGS=-ldflags "-s -w -X main.version=$(VERSION) -X main.commit=$(COMMIT) -X main.date=$(DATE)"

.PHONY: build test lint clean all

build:
	CGO_ENABLED=0 go build $(LDFLAGS) -o dist/$(BINARY_NAME) ./cmd/pkt

build-cgo:
	CGO_ENABLED=1 go build $(LDFLAGS) -o dist/$(BINARY_NAME) ./cmd/pkt

test:
	go test -race -cover ./...

lint:
	go vet ./...

clean:
	rm -rf dist/

all: lint test build

# Cross-compilation (pure Go only)
build-all:
	GOOS=linux   GOARCH=amd64 CGO_ENABLED=0 go build $(LDFLAGS) -o dist/$(BINARY_NAME)-linux-amd64       ./cmd/pkt
	GOOS=linux   GOARCH=arm64 CGO_ENABLED=0 go build $(LDFLAGS) -o dist/$(BINARY_NAME)-linux-arm64       ./cmd/pkt
	GOOS=darwin  GOARCH=amd64 CGO_ENABLED=0 go build $(LDFLAGS) -o dist/$(BINARY_NAME)-darwin-amd64      ./cmd/pkt
	GOOS=darwin  GOARCH=arm64 CGO_ENABLED=0 go build $(LDFLAGS) -o dist/$(BINARY_NAME)-darwin-arm64      ./cmd/pkt
	GOOS=windows GOARCH=amd64 CGO_ENABLED=0 go build $(LDFLAGS) -o dist/$(BINARY_NAME)-windows-amd64.exe ./cmd/pkt
