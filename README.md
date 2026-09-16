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
- GNU Make for the convenience targets below. The underlying Go commands are
  the portable interface.

No external service or network access is required for the current scaffold.

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
make check
make test
~~~

The corresponding Go commands are:

~~~sh
go build -o bin/ ./cmd/control-plane
go fmt ./...
go vet ./...
go test ./...
~~~

The build writes the platform-native executable under the ignored `bin`
directory. `make fmt` updates Go source formatting; review its diff before
committing.
Pinned third-party quality tools, race tests, vulnerability checks, and CI are
separate M01 work items.
