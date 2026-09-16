GOLANGCI_LINT_VERSION := v2.13.2
GOVULNCHECK_VERSION := v1.7.0
ACTIONLINT_VERSION := v1.7.12
GOLANGCI_LINT_MODULE := github.com/golangci/golangci-lint/v2/cmd/golangci-lint
GOVULNCHECK_MODULE := golang.org/x/vuln/cmd/govulncheck
ACTIONLINT_MODULE := github.com/rhysd/actionlint/cmd/actionlint
GOLANGCI_LINT := go run $(GOLANGCI_LINT_MODULE)@$(GOLANGCI_LINT_VERSION)
GOVULNCHECK := go run $(GOVULNCHECK_MODULE)@$(GOVULNCHECK_VERSION)
ACTIONLINT := go run $(ACTIONLINT_MODULE)@$(ACTIONLINT_VERSION)

.PHONY: help doctor build fmt fmt-check check test race vuln validate

help:
	@echo Argus engineering-foundation command surface
	@echo   make doctor  Report the effective local development toolchain
	@echo   make build  Compile all Go packages
	@echo   make fmt    Format Go sources and imports
	@echo   make fmt-check  Verify formatting without changes
	@echo   make check  Run format, lint, static-analysis, and module checks
	@echo   make test   Run ordinary tests without cached results
	@echo   make race   Run all tests with the race detector
	@echo   make vuln   Scan reachable dependencies for known vulnerabilities
	@echo   make validate  Run all non-mutating acceptance checks

doctor:
	@git --version
	@go version
	@go env -json GOVERSION GOOS GOARCH GOTOOLCHAIN CGO_ENABLED
	@$(MAKE) --version

build:
	go build -o bin/ ./cmd/control-plane

fmt:
	$(GOLANGCI_LINT) fmt

fmt-check:
	$(GOLANGCI_LINT) fmt --diff

check: fmt-check
	$(GOLANGCI_LINT) config verify
	$(GOLANGCI_LINT) run ./...
	$(ACTIONLINT)
	go mod tidy -diff
	go mod verify

test:
	go test -vet=off -count=1 ./...

race:
	go test -vet=off -race -count=1 ./...

vuln:
	$(GOVULNCHECK) ./...

validate: build check test race vuln
