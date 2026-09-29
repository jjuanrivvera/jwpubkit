BINARY := pubkit
MODULE := github.com/jjuanrivvera/jwpubkit
VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
LDFLAGS := -s -w -X $(MODULE)/internal/cli.Version=$(VERSION)
PREFIX ?= $(HOME)/.local/bin
# El suelo de cobertura; el mismo número que .github/workflows/ci.yml.
# Va por debajo de lo medido a propósito: el total baila ~1 punto entre máquinas.
COVER_MIN ?= 45

.PHONY: build install test lint verify cover-check clean

build:
	CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -trimpath -ldflags '$(LDFLAGS)' -o bin/$(BINARY) .

install: build
	install -d '$(PREFIX)'
	install -m 0755 bin/$(BINARY) '$(PREFIX)/$(BINARY)'
	ln -sfn $(BINARY) '$(PREFIX)/jwlib'

test:
	go test ./...

lint:
	golangci-lint run ./...

# La puerta. No modifica el árbol: si algo está mal, falla y lo dice.
verify: lint
	@test -z "$$(gofmt -l . | tee /dev/stderr)" || { echo 'gofmt: los archivos de arriba no están formateados'; exit 1; }
	go vet ./...
	go test -coverprofile=coverage.out ./...
	@$(MAKE) --no-print-directory cover-check

# El mismo suelo que exige el CI, medido igual.
cover-check:
	@total=$$(go tool cover -func=coverage.out | awk '/^total:/ {print substr($$3, 1, length($$3)-1)}'); \
	awk -v t="$$total" -v min="$(COVER_MIN)" 'BEGIN { if (t+0 < min+0) { printf "cobertura %.1f%% < %s%%\n", t, min; exit 1 } printf "cobertura %.1f%% (mínimo %s%%)\n", t, min }'

clean:
	rm -rf bin dist coverage.out
