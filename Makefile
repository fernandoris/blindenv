VERSION ?= 0.1.0
COMMIT  ?= $(shell git rev-parse --short HEAD 2>/dev/null || echo none)
DATE    ?= $(shell git log -1 --format=%cI 2>/dev/null || echo unknown)
PREFIX  ?= /usr/local

VERSION_PKG := github.com/fernandoris/blindenv/pkg/version
LDFLAGS     := -X $(VERSION_PKG).Version=$(VERSION) -X $(VERSION_PKG).Commit=$(COMMIT) -X $(VERSION_PKG).Date=$(DATE)

.PHONY: fmt check build install

fmt:
	gofmt -w .

check:
	@test -z "$$(gofmt -l .)" || (echo "gofmt found unformatted files:"; gofmt -l .; exit 1)
	go vet ./...
	CGO_ENABLED=0 go build ./...
	CGO_ENABLED=0 go test ./...

build:
	CGO_ENABLED=0 go build -ldflags "$(LDFLAGS)" -o bin/blindenv ./cmd/blindenv

install: build
	install -d "$(PREFIX)/bin"
	install -m 0755 bin/blindenv "$(PREFIX)/bin/blindenv"
