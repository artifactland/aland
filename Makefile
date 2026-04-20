# Lightweight make targets for local dev. Release automation lives in
# .goreleaser.yaml + .github/workflows; see RELEASING.md.

BINARY  := aland
PKG     := ./cmd/aland
OUT     := bin/$(BINARY)
VERSION ?= dev

.PHONY: build test lint vet fmt clean install

build:
	@mkdir -p bin
	go build -ldflags "-X main.Version=$(VERSION)" -o $(OUT) $(PKG)

test:
	go test ./...

vet:
	go vet ./...

fmt:
	gofmt -w -s .

lint: vet
	@echo "vet passed"

install: build
	install -m 0755 $(OUT) $(GOBIN)/$(BINARY)

clean:
	rm -rf bin
