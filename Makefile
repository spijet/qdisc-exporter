BINARY  := qdisc-exporter
PKG     := .

VERSION  := $(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
COMMIT   := $(shell git rev-parse --short HEAD 2>/dev/null || echo "unknown")
BUILT_AT := $(shell date -u +%Y-%m-%dT%H:%M:%SZ)

LDFLAGS := -ldflags="-s -w -X main.version=$(VERSION) -X main.commit=$(COMMIT) -X main.builtAt=$(BUILT_AT)"
GOFLAGS := -trimpath

.PHONY: build test clean

build:
	CGO_ENABLED=0 GOOS=linux go build $(GOFLAGS) $(LDFLAGS) -o $(BINARY) $(PKG)

test:
	go test ./...

clean:
	rm -f $(BINARY)
