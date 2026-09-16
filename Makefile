GOLANGCI_LINT_MODULE := github.com/golangci/golangci-lint/v2/cmd/golangci-lint
GOVULNCHECK_MODULE := golang.org/x/vuln/cmd/govulncheck
GO_LICENSES_MODULE := github.com/google/go-licenses/v2
GO_JSONSCHEMA_MODULE := github.com/atombender/go-jsonschema
GOLANGCI_LINT := go tool golangci-lint
GOVULNCHECK := go tool govulncheck
ACTIONLINT := go -C tools/actionlint tool actionlint
GO_LICENSES := go tool go-licenses

TOOL_PACKAGES := \
	$(GO_JSONSCHEMA_MODULE) \
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

.PHONY: help doctor build generate-contracts fmt fmt-check check test race vuln license validate

help:
	@echo Argus engineering-foundation command surface
	@echo   make doctor  Report the effective local development toolchain
	@echo   make build  Compile all Go packages
	@echo   make generate-contracts  Regenerate checked-in Go and Python contract bindings
	@echo   make fmt    Format Go and Python sources
	@echo   make fmt-check  Verify formatting without changes
	@echo   make check  Run format, lint, static-analysis, and module checks
	@echo   make test   Run ordinary tests without cached results
	@echo   make race   Run all tests with the race detector
	@echo   make vuln   Scan reachable dependencies for known vulnerabilities
	@echo   make license  Enforce runtime and development-tool license policy
	@echo   make validate  Run all non-mutating acceptance checks

doctor:
	@git --version
	@go version
	@go env -json GOVERSION GOOS GOARCH GOTOOLCHAIN CGO_ENABLED
	@uv --version
	@uv run --locked python --version
	@$(MAKE) --version

build:
	go build -o bin/ ./cmd/control-plane

generate-contracts:
	go run ./internal/tools/contractgen -write

fmt:
	$(GOLANGCI_LINT) fmt
	uv run --locked ruff check --fix contracts/generated/python contracts/scripts contracts/tests
	uv run --locked ruff format contracts/generated/python contracts/scripts contracts/tests

fmt-check:
	$(GOLANGCI_LINT) fmt --diff
	uv run --locked ruff check contracts/generated/python contracts/scripts contracts/tests
	uv run --locked ruff format --check contracts/generated/python contracts/scripts contracts/tests

check: fmt-check
	uv lock --check
	uv run --locked ty check --extra-search-path contracts/generated/python
	go run ./internal/tools/contractgen
	$(GOLANGCI_LINT) config verify
	$(GOLANGCI_LINT) run ./...
	$(ACTIONLINT)
	go mod tidy -diff
	go mod verify
	go -C tools/actionlint mod tidy -diff
	go -C tools/actionlint mod verify

test:
	go test -vet=off -count=1 ./...
	uv run --locked python contracts/scripts/validate.py
	uv run --locked python -m unittest discover -s contracts/tests -p "test_*.py"

race:
	go test -vet=off -race -count=1 ./...

vuln:
	$(GOVULNCHECK) ./...
	go -C tools/actionlint tool govulncheck github.com/rhysd/actionlint/cmd/actionlint

license:
	$(GO_LICENSES) check --include_tests ./... --allowed_licenses $(RUNTIME_ALLOWED_LICENSES)
	$(GO_LICENSES) check $(TOOL_PACKAGES) --allowed_licenses $(DEVELOPMENT_ALLOWED_LICENSES) $(TOOL_LICENSE_EXCEPTIONS)
	go -C tools/actionlint tool go-licenses check github.com/rhysd/actionlint/cmd/actionlint --allowed_licenses $(RUNTIME_ALLOWED_LICENSES)

validate: build check test race vuln license
