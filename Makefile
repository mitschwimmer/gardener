GO ?= go
GOFMT ?= $(shell $(GO) env GOROOT)/bin/gofmt
GOVULNCHECK_VERSION := v1.8.0
GOVULNCHECK := $(CURDIR)/.tools/bin/govulncheck

.PHONY: help fmt fmt-check deps vet test test-integration build check tools vuln ci run

help:
	@echo 'make fmt    Format Go sources'
	@echo 'make test   Run the fast unit-test loop'
	@echo 'make test-integration  Test against an explicitly supplied disposable Incus socket'
	@echo 'make check  Check formatting/modules, vet, race-test, and build'
	@echo 'make ci     Run all checks, including the vulnerability scan'
	@echo 'make run    Run the CLI scaffold'

fmt:
	$(GOFMT) -w cmd internal

fmt-check:
	@unformatted="$$($(GOFMT) -l cmd internal)" || exit $$?; \
	if [ -n "$$unformatted" ]; then echo "$$unformatted"; echo 'Run make fmt'; exit 1; fi

deps:
	$(GO) mod tidy -diff
	$(GO) mod verify

vet:
	$(GO) vet ./...

test:
	$(GO) test ./...

test-integration:
	$(GO) test -tags integration -race -count=1 -timeout=2m -v ./internal/incusconn

build:
	mkdir -p bin
	$(GO) build -o bin/garden ./cmd/garden

check: fmt-check deps vet
	$(GO) test -race ./...
	$(MAKE) build

tools:
	mkdir -p .tools/bin
	GOBIN=$(CURDIR)/.tools/bin $(GO) install golang.org/x/vuln/cmd/govulncheck@$(GOVULNCHECK_VERSION)

vuln: tools
	$(GOVULNCHECK) ./...

ci: check vuln

run:
	$(GO) run ./cmd/garden
