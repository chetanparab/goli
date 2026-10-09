VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
LDFLAGS := -s -w -X main.version=$(VERSION)
PREFIX  ?= $(HOME)/.local

.PHONY: build test lint install run clean

build:
	go build -ldflags "$(LDFLAGS)" -o goli ./cmd/goli

test:
	go test -race ./...

lint:
	go vet ./...
	golangci-lint run

## install: build, copy to $(PREFIX)/bin and restart the background server if installed
install: build
	install -d $(PREFIX)/bin
	install -m 0755 goli $(PREFIX)/bin/goli
	@launchctl kickstart -k gui/$$(id -u)/dev.goli.server 2>/dev/null && echo "restarted goli server" || true

run: build
	./goli serve

clean:
	rm -rf goli dist
