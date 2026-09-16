# Argus

Argus is an adaptive test intelligence and evolution platform. It determines
which tests should run for a software change, identifies stale or missing test
coverage, and produces evidence-backed maintenance proposals.

The repository is at engineering-foundation stage. Its first executable is a
minimal control-plane process that establishes lifecycle and verification
boundaries without introducing product behavior prematurely.

## Start here

- [Product definition](docs/product/product-definition.md)
- [Architecture overview](docs/architecture/overview.md)
- [Proposal decomposition and provenance](docs/proposal.md)
- [Contributor workflow](CONTRIBUTING.md)
- [Engineering standards](docs/development/engineering-standards.md)
- [SQL migration criteria](docs/development/sql-migrations.md)
- [Architecture decisions](docs/decisions/README.md)
- [Implementation milestones](docs/roadmap/milestones.md)
- [Research and market landscape](docs/research/landscape.md)
- [Security policy](SECURITY.md)
- [Code of conduct](CODE_OF_CONDUCT.md)

## Requirements

- Go 1.26.4, as declared by [go.mod](go.mod).
- GNU Make for the repository command surface. The underlying pinned Go
  invocations are visible in the [Makefile](Makefile).

No external service is required for the current scaffold. The first quality-
tool run requires network access to download the versions pinned in the
[Makefile](Makefile); later runs reuse the Go module cache. Vulnerability scans
also require access to the Go vulnerability database unless it is cached.

## Run the control plane

~~~sh
go run ./cmd/control-plane
~~~

The command writes structured lifecycle logs to standard output and waits for
an interrupt or termination signal. Press `Ctrl+C` to request a graceful
shutdown. The current process intentionally exposes no API, storage,
configuration, workers, or test-selection behavior.

## Build and verify

~~~sh
make build
make fmt
make fmt-check
make check
make test
make race
make vuln
make validate
~~~

`make validate` is the required non-mutating acceptance suite. It builds the
command, verifies formatting and linter configuration, runs lint and static
analysis, verifies module tidiness and checksums, runs ordinary and race-enabled
tests without cached results, and scans reachable dependencies for known
vulnerabilities.

The tools are invoked through these pinned commands rather than ambient global
binaries:

~~~sh
go run github.com/golangci/golangci-lint/v2/cmd/golangci-lint@v2.13.2
go run golang.org/x/vuln/cmd/govulncheck@v1.7.0
go run github.com/rhysd/actionlint/cmd/actionlint@v1.7.12
~~~

The build writes the platform-native executable under the ignored `bin`
directory. `make fmt` updates Go source formatting; review its diff before
committing. `make check` includes `govet`, `staticcheck`, and GitHub Actions
workflow validation, so the test targets disable the duplicate implicit
`go test` vet pass.

## Continuous integration

The [validation workflow](.github/workflows/validate.yml) runs `make validate`
on Ubuntu 24.04 and Windows Server 2025 for every pull request and every push to
`main`; it can also be run manually. The Windows job installs the pinned GNU
Make 4.4.1 package because GNU Make is not part of the hosted Windows image.
Both jobs read the exact Go version from `go.mod` and execute the repository's
same checked-in, non-mutating acceptance suite.

The root [.gitattributes](.gitattributes) enforces LF line endings for text
files on every checkout, matching `.editorconfig` and preventing Windows Git
settings from creating formatter-only differences. Windows batch and command
scripts retain CRLF line endings.

The workflow grants only read access to repository contents and does not retain
checkout credentials. Its GitHub-authored actions are pinned to immutable
release commits. CI requires network access for the Go toolchain, pinned quality
tools, vulnerability database, and the Windows GNU Make package.
