GOLANGCI_LINT_MODULE := github.com/golangci/golangci-lint/v2/cmd/golangci-lint
GOVULNCHECK_MODULE := golang.org/x/vuln/cmd/govulncheck
GO_LICENSES_MODULE := github.com/google/go-licenses/v2
GOLANGCI_LINT := go tool golangci-lint
GOVULNCHECK := go tool govulncheck
ACTIONLINT := go -C tools/actionlint tool actionlint
GO_LICENSES := go tool go-licenses

TOOL_PACKAGES := \
	$(GOLANGCI_LINT_MODULE) \
	$(GOVULNCHECK_MODULE) \
	$(GO_LICENSES_MODULE)

RUNTIME_ALLOWED_LICENSES := Apache-2.0,BSD-2-Clause,BSD-3-Clause,ISC,MIT
DEVELOPMENT_ALLOWED_LICENSES := $(RUNTIME_ALLOWED_LICENSES),MPL-2.0

# These packages are reachable only from repository development tools. Their
# reviewed exceptions and non-distribution constraint are documented in the
# dependency policy; application and test dependencies receive no exceptions.
TOOL_LICENSE_EXCEPTIONS := \
	--ignore github.com/OpenPeeDeeP/depguard/v2 \
	--ignore github.com/alecthomas/chroma/v2 \
	--ignore github.com/ashanbrown/forbidigo/v2 \
	--ignore github.com/ashanbrown/makezero/v2 \
	--ignore github.com/denis-tingaikin/go-header \
	--ignore github.com/firefart/nonamedreturns \
	--ignore github.com/golangci/gofmt \
	--ignore github.com/golangci/golangci-lint/v2 \
	--ignore github.com/ldez/structtags \
	--ignore github.com/leonklingele/grouper \
	--ignore github.com/xen0n/gosmopolitan

.PHONY: help doctor build generate-contracts fmt fmt-check check test race db-validate vuln license validate

help:
	@echo Argus engineering-foundation command surface
	@echo   make doctor  Report the effective local development toolchain
	@echo   make build  Compile all Go packages
	@echo   make generate-contracts  Regenerate JSON Schema from Effect Schema
	@echo   make fmt    Format Go and TypeScript sources
	@echo   make fmt-check  Verify formatting without changes
	@echo   make check  Run format, lint, static-analysis, and module checks
	@echo   make test   Run ordinary tests without cached results
	@echo   make race   Run all tests with the race detector
	@echo   make db-validate  Run PostgreSQL integration tests against a disposable local server
	@echo   make vuln   Scan reachable dependencies for known vulnerabilities
	@echo   make license  Enforce runtime and development-tool license policy
	@echo   make validate  Run all non-mutating acceptance checks

doctor:
	@git --version
	@go version
	@go env -json GOVERSION GOOS GOARCH GOTOOLCHAIN CGO_ENABLED
	@node --version
	@pnpm --version
	@$(MAKE) --version

build:
	go build -o bin/ ./cmd/...

generate-contracts:
	pnpm contracts:generate

fmt:
	$(GOLANGCI_LINT) fmt
	pnpm format

fmt-check:
	$(GOLANGCI_LINT) fmt --diff
	pnpm format:check

check: fmt-check
	pnpm contracts:check
	pnpm typecheck
	$(GOLANGCI_LINT) config verify
	$(GOLANGCI_LINT) run ./...
	$(ACTIONLINT)
	go mod tidy -diff
	go mod verify
	go -C tools/actionlint mod tidy -diff
	go -C tools/actionlint mod verify

test:
	go test -vet=off -count=1 ./...
	pnpm test

race:
	go test -vet=off -race -count=1 ./...

db-validate:
	go test -vet=off -tags=integration -race -count=1 -timeout=5m ./internal/catalog/adapters/postgres

vuln:
	$(GOVULNCHECK) ./...
	go -C tools/actionlint tool govulncheck github.com/rhysd/actionlint/cmd/actionlint
	pnpm audit --prod --audit-level high

license:
	$(GO_LICENSES) check --include_tests ./... --allowed_licenses $(RUNTIME_ALLOWED_LICENSES)
	$(GO_LICENSES) check $(TOOL_PACKAGES) --allowed_licenses $(DEVELOPMENT_ALLOWED_LICENSES) $(TOOL_LICENSE_EXCEPTIONS)
	go -C tools/actionlint tool go-licenses check github.com/rhysd/actionlint/cmd/actionlint --allowed_licenses $(RUNTIME_ALLOWED_LICENSES)
	pnpm licenses:check

validate: build check test race vuln license
