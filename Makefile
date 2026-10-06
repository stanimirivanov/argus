GOLANGCI_LINT_MODULE := github.com/golangci/golangci-lint/v2/cmd/golangci-lint
GOVULNCHECK_MODULE := golang.org/x/vuln/cmd/govulncheck
GO_LICENSES_MODULE := github.com/google/go-licenses/v2
QUALITY_TOOLS_DIRECTORY := tools/quality
QUALITY_TOOL := go tool -modfile="$(QUALITY_TOOLS_DIRECTORY)/go.mod"
GOLANGCI_LINT := $(QUALITY_TOOL) golangci-lint
GOVULNCHECK := $(QUALITY_TOOL) govulncheck
GO_LICENSES := $(QUALITY_TOOL) go-licenses
QUALITY_GOVULNCHECK := go -C $(QUALITY_TOOLS_DIRECTORY) tool govulncheck
QUALITY_GO_LICENSES := go -C $(QUALITY_TOOLS_DIRECTORY) tool go-licenses
ACTIONLINT := go -C tools/actionlint tool actionlint

TOOL_PACKAGES := \
	$(GOLANGCI_LINT_MODULE) \
	$(GOVULNCHECK_MODULE) \
	$(GO_LICENSES_MODULE)

RUNTIME_ALLOWED_LICENSES := Apache-2.0,BSD-2-Clause,BSD-3-Clause,ISC,MIT,PostgreSQL
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

.PHONY: help bootstrap doctor build binaries generate-contracts fmt fmt-check docs-check architecture-check check test verify race fuzz-smoke db-validate vuln license supply-chain validate

help:
	@echo Argus engineering-foundation command surface
	@echo   make bootstrap  Resolve all pinned repository dependencies
	@echo   make doctor  Report the effective local development toolchain
	@echo   make build  Compile all Go commands without writing repository artifacts
	@echo   make binaries  Build runnable Go commands under bin
	@echo   make generate-contracts  Regenerate JSON Schema from Effect Schema
	@echo   make fmt    Format Go and TypeScript sources
	@echo   make fmt-check  Verify formatting without changes
	@echo   make docs-check  Verify repository-local documentation policy and links
	@echo   make architecture-check  Verify the complete production dependency matrix
	@echo   make check  Run format, lint, static-analysis, and module checks
	@echo   make test   Run ordinary tests without cached results
	@echo   make verify  Run the fast network-independent development feedback loop
	@echo   make race   Run all tests with the race detector
	@echo   make fuzz-smoke  Run bounded native fuzzing of untrusted Go inputs
	@echo   make db-validate  Run PostgreSQL integration tests against a disposable local server
	@echo   make vuln   Scan reachable dependencies for known vulnerabilities
	@echo   make license  Enforce runtime and development-tool license policy
	@echo   make supply-chain  Run vulnerability and license checks
	@echo   make validate  Run all non-mutating acceptance checks

bootstrap:
	go mod download
	go -C $(QUALITY_TOOLS_DIRECTORY) mod download
	go -C tools/actionlint mod download
	pnpm install --frozen-lockfile

doctor:
	@git --version
	@go version
	@go env -json GOVERSION GOOS GOARCH GOTOOLCHAIN CGO_ENABLED
	@node --version
	@pnpm --version
	@$(MAKE) --version

build:
	go build ./cmd/...

binaries:
	go build -o bin/ ./cmd/...

generate-contracts:
	pnpm contracts:generate

fmt:
	$(GOLANGCI_LINT) fmt
	pnpm format

fmt-check:
	$(GOLANGCI_LINT) fmt --diff
	pnpm format:check

docs-check:
	go run ./tools/repo-check

architecture-check:
	go test -vet=off -count=1 ./internal/architecture

check: fmt-check docs-check architecture-check
	pnpm contracts:check
	pnpm typecheck
	$(GOLANGCI_LINT) config verify
	$(GOLANGCI_LINT) run ./...
	$(GOLANGCI_LINT) run --build-tags=dbose2e ./cmd/control-plane
	$(ACTIONLINT)
	go mod tidy -diff
	go mod verify
	go -C $(QUALITY_TOOLS_DIRECTORY) mod tidy -diff
	go -C $(QUALITY_TOOLS_DIRECTORY) mod verify
	go -C tools/actionlint mod tidy -diff
	go -C tools/actionlint mod verify

test:
	go test -vet=off -count=1 ./...
	pnpm test

verify: build check test

race:
	go test -vet=off -race -count=1 ./...

# Ordinary go test already executes each fuzz target's seed corpus. This
# separate, bounded campaign is a T3 developer sensor, not a CI merge gate.
fuzz-smoke:
	go test -run '^$$' -fuzz '^FuzzTestCatalogCursor$$' -fuzztime=2s ./internal/catalog/testquery
	go test -run '^$$' -fuzz '^FuzzImpactEdgeCursor$$' -fuzztime=2s ./internal/catalog/impact
	go test -run '^$$' -fuzz '^FuzzWebhookDecode$$' -fuzztime=2s ./internal/change/adapters/github
	go test -run '^$$' -fuzz '^FuzzOpenAPIDocumentParsing$$' -fuzztime=2s ./internal/change/adapters/openapi
	go test -run '^$$' -fuzz '^FuzzProcessResultDecoding$$' -fuzztime=2s ./internal/contracts
	go test -run '^$$' -fuzz '^FuzzSourcePath$$' -fuzztime=2s ./internal/adaptation

db-validate:
	go test -vet=off -tags=integration -race -count=1 -timeout=5m ./internal/postgres

# Include the isolated DBOS end-to-end test dependency graph in supply-chain
# scans without requiring a container runtime to execute that test.
vuln license: export GOFLAGS := -tags=dbose2e

vuln:
	$(GOVULNCHECK) ./...
	$(QUALITY_GOVULNCHECK) $(TOOL_PACKAGES)
	go -C tools/actionlint tool govulncheck github.com/rhysd/actionlint/cmd/actionlint
	pnpm audit --prod --audit-level high

license:
	$(GO_LICENSES) check --include_tests ./... --allowed_licenses $(RUNTIME_ALLOWED_LICENSES)
	$(QUALITY_GO_LICENSES) check $(TOOL_PACKAGES) --allowed_licenses $(DEVELOPMENT_ALLOWED_LICENSES) $(TOOL_LICENSE_EXCEPTIONS)
	go -C tools/actionlint tool go-licenses check github.com/rhysd/actionlint/cmd/actionlint --allowed_licenses $(RUNTIME_ALLOWED_LICENSES)
	pnpm licenses:check

supply-chain: vuln license

validate: verify race supply-chain
