BINARY := pubkit
MODULE := github.com/jjuanrivvera/jwpubkit
VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
LDFLAGS := -s -w -X $(MODULE)/internal/cli.Version=$(VERSION)
PREFIX ?= $(HOME)/.local/bin
# The coverage floor; the same number as .github/workflows/ci.yml. It sits under the
# measured total on purpose: that total drifts about a point between machines.
COVER_MIN ?= 67

.PHONY: build install test lint security verify cover-check clean

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

# The gate. It changes nothing: when something is wrong it fails and says so.
verify: lint security
	@test -z "$$(gofmt -l . | tee /dev/stderr)" || { echo 'gofmt: the files above are not formatted'; exit 1; }
	go vet ./...
	go test -coverprofile=coverage.out ./...
	@$(MAKE) --no-print-directory cover-check

# What CI runs. When gosec is missing it says so instead of skipping quietly: a gate
# that silently drops a step is not a gate.
security:
	@command -v gosec >/dev/null || { echo 'gosec is not installed (go install github.com/securego/gosec/v2/cmd/gosec@latest); CI runs it'; exit 1; }
	gosec -severity high -confidence medium -quiet ./...

# The same floor CI enforces, measured the same way.
cover-check:
	@total=$$(go tool cover -func=coverage.out | awk '/^total:/ {print substr($$3, 1, length($$3)-1)}'); \
	awk -v t="$$total" -v min="$(COVER_MIN)" 'BEGIN { if (t+0 < min+0) { printf "coverage %.1f%% < %s%%\n", t, min; exit 1 } printf "coverage %.1f%% (floor %s%%)\n", t, min }'

clean:
	rm -rf bin dist coverage.out
