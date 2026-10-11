VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
LDFLAGS := -s -w -X main.version=$(VERSION)
PREFIX  ?= $(HOME)/.local

.PHONY: build test lint install run clean notices

build:
	go build -ldflags "$(LDFLAGS)" -o goli ./cmd/goli

test:
	go test -race ./...

lint:
	go vet ./...
	golangci-lint run

## install: build, copy to $(PREFIX)/bin and restart the background server if it's installed
install: build
	install -d $(PREFIX)/bin
	install -m 0755 goli $(PREFIX)/bin/goli
	@$(PREFIX)/bin/goli restart 2>/dev/null || true

run: build
	./goli serve

clean:
	rm -rf goli dist

## notices: regenerate THIRD_PARTY_NOTICES.md (run after changing dependencies)
notices:
	tools/notices.sh > THIRD_PARTY_NOTICES.md
