VERSION ?= $(shell git describe --always --dirty 2>/dev/null || echo dev)
LDFLAGS := -s -w -X jwlib/internal/cli.Version=$(VERSION)

.PHONY: build test vet install vps clean

# Static linux/amd64 binary: pure-Go SQLite (modernc.org/sqlite), no cgo.
build:
	CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -trimpath -ldflags "$(LDFLAGS)" -o bin/jwlib .

test:
	go test ./...

vet:
	go vet ./...

install: build
	install -m 0755 bin/jwlib $(HOME)/.local/bin/jwlib

vps: build
	scp bin/jwlib VPS:~/.local/bin/jwlib

clean:
	rm -rf bin
