# Argus

Argus is an adaptive test intelligence and evolution platform. It determines
which tests should run for a software change, identifies stale or missing test
coverage, and produces evidence-backed maintenance proposals.

The repository has completed its engineering foundation and now includes a
real repository-descriptor ingestion boundary and a durable evidence catalog.
Effect Schema authors the wire contracts, generated JSON Schemas validate them,
Go converts repository input into catalog state at a verified immutable
revision, and PostgreSQL stores immutable snapshots and impact observations.
Versioned queries enumerate stable tests and expose supported, refuted, stale,
or conflicting capability-to-test relationships without discarding evidence.

## Start here

- [Product definition](docs/product/product-definition.md)
- [Architecture overview](docs/architecture/overview.md)
- [Contract workspace](contracts/README.md)
- [PostgreSQL catalog operations](docs/development/postgresql.md)
- [Proposal decomposition and provenance](docs/proposal.md)
- [Developer quickstart](docs/development/developer-quickstart.md)
- [Dependency and license policy](docs/development/dependency-policy.md)
- [Contributor workflow](CONTRIBUTING.md)
- [Engineering standards](docs/development/engineering-standards.md)
- [SQL migration criteria](docs/development/sql-migrations.md)
- [Architecture decisions](docs/decisions/README.md)
- [Implementation milestones](docs/roadmap/milestones.md)
- [Research and market landscape](docs/research/landscape.md)
- [Security policy](SECURITY.md)
- [Code of conduct](CODE_OF_CONDUCT.md)

## Requirements

- Go 1.26.6, as declared by [go.mod](go.mod).
- Node.js 24.18.0 and pnpm 11.19.0, as declared by [.node-version](.node-version)
  and [package.json](package.json), for Effect Schema authoring and generation.
- GNU Make 4.3 or newer for the repository command surface.
- A supported Git release and either PowerShell 7 on Windows or Bash on Linux.

Run `make doctor` to report the effective toolchain. The complete supported,
best-effort, and out-of-contract environment definitions, installation notes,
and troubleshooting guidance live in the
[developer quickstart](docs/development/developer-quickstart.md). Tool versions
are declared in the root [go.mod](go.mod) and the isolated
[actionlint module](tools/actionlint/go.mod), while their invocations remain
visible in the [Makefile](Makefile).

No external service is required for ordinary builds, contract validation, or
`make validate`. Catalog persistence requires PostgreSQL 17; the dedicated
`make db-validate` target creates and removes random databases on an explicitly
configured loopback test server.

The first quality-tool run requires network access to download the versions
pinned in the Go module and pnpm lock files; later runs reuse local caches.
Vulnerability scans also require access to the Go vulnerability database
unless it is cached.

## Run the control plane

~~~sh
go run ./cmd/control-plane
~~~

The command writes structured lifecycle logs to standard output and waits for
an interrupt or termination signal. Press `Ctrl+C` to request a graceful
shutdown. The current process intentionally exposes no API, storage,
configuration, workers, or test-selection behavior.

## Validate a repository descriptor

~~~sh
go run ./cmd/descriptor \
  -revision 0123456789abcdef0123456789abcdef01234567 \
  contracts/fixtures/repository-descriptor/v1/valid/source-and-test-repositories.json
~~~

The command validates the document against JSON Schema generated from Effect
Schema, applies catalog-domain invariants, and prints a normalized summary. The
revision argument represents trusted ingestion context and is deliberately not
read from the repository-owned document.

## Persist and read catalog snapshots

Apply migrations explicitly before starting a writer. Database URLs are
secrets and are accepted only through environment configuration:

~~~sh
export ARGUS_DATABASE_URL='postgres://argus_migrator:...@db.example/argus'
go run ./cmd/migrate

export ARGUS_DATABASE_URL='postgres://argus_runtime:...@db.example/argus'
go run ./cmd/catalog import \
  -revision 0123456789abcdef0123456789abcdef01234567 \
  contracts/fixtures/repository-descriptor/v1/valid/source-and-test-repositories.json
~~~

`catalog get` retrieves the same immutable snapshot by repository provider,
host, opaque provider ID, revision, and descriptor API version. See the
[PostgreSQL guide](docs/development/postgresql.md) for local setup, grants,
idempotency, recovery, and complete command examples.

## Query catalog tests

List a bounded page of tests from one explicitly identified immutable snapshot:

~~~sh
go run ./cmd/catalog list-tests \
  -provider github \
  -host github.com \
  -repository-id R_orders_source_01 \
  -revision 0123456789abcdef0123456789abcdef01234567 \
  -page-size 50
~~~

The command emits `argus.dev/test-catalog-page/v1`. Each item includes stable
test repository, suite, and test keys together with display metadata and
capability mappings. Pass `-capability create-order` to restrict the page to
tests explicitly mapped to that source capability. If `nextCursor` is not
`null`, pass its value unchanged through `-cursor` to read the next page.

Cursors are opaque, versioned, and bound to the snapshot and capability filter.
A cursor cannot be reused for a different query. Page sizes range from 1 to
200 and may change between pages. The command never resolves an implicit
“latest” snapshot.

## Ingest and query impact evidence

After importing the referenced catalog snapshot, ingest an immutable producer
bundle:

~~~sh
go run ./cmd/catalog import-impact \
  contracts/fixtures/impact-evidence-bundle/v1/valid/orders-api.json
~~~

Query relationships at an explicit, reproducible evaluation instant:

~~~sh
go run ./cmd/catalog list-impact \
  -provider github \
  -host github.com \
  -repository-id R_orders_source_01 \
  -revision 0123456789abcdef0123456789abcdef01234567 \
  -evaluated-at 2026-09-18T05:00:00Z \
  -capability create-order
~~~

`argus.dev/impact-edge-page/v1` returns each visible evidence observation with
its producer revision, method, basis-point confidence, rationale, event time,
expiry, and active/expired state. Argus derives `supported`, `refuted`, `stale`,
or `conflicting` at the supplied time. Confidence remains producer-specific
metadata; ingestion does not combine it into an authority score. Contradictory
active evidence is retained and reported for review.

## Build and verify

~~~sh
make doctor
make build
make generate-contracts
make fmt
make fmt-check
make check
make test
make race
make db-validate
make vuln
make license
make validate
~~~

`make validate` is the required non-mutating acceptance suite. It builds all
commands, proves Effect-to-JSON-Schema regeneration, validates the shared
structural and domain fixture corpus, verifies Go and TypeScript formatting and
static analysis, checks module integrity, runs ordinary and race-enabled tests
without cached results, scans Go and production Node dependencies for known
vulnerabilities, and enforces runtime dependency license policy.

Database-changing pull requests additionally run `make db-validate`. That
target is intentionally separate from `make validate` so normal development
does not silently depend on a local database.

The tools are declared through Go's versioned `tool` directives in the root
module and the isolated actionlint module, protected by their checksum files,
and invoked through these commands rather than ambient global binaries:

~~~sh
go tool golangci-lint
go tool govulncheck
go -C tools/actionlint tool actionlint
go tool go-licenses
~~~

The build writes platform-native executables under the ignored `bin` directory.
`make generate-contracts` updates checked-in JSON Schema and `make fmt` updates
Go and TypeScript formatting; review their diffs before committing. `make
check` includes generation reproducibility, TypeScript type checking, `govet`,
`staticcheck`, and GitHub Actions workflow validation, so the test targets
disable the duplicate implicit `go test` vet pass.

## Continuous integration

The [validation workflow](.github/workflows/validate.yml) runs `make validate`
on Ubuntu 24.04 and Windows Server 2025, plus the PostgreSQL integration suite
against PostgreSQL 17.11 on Ubuntu, for every pull request and every push to
`main`; it can also be run manually. The Windows job installs the pinned GNU
Make 4.4.1 package because GNU Make is not part of the hosted Windows image.
The cross-platform matrix reads the exact Go and Node versions from repository
files, installs pnpm from the exact `packageManager` declaration, restores the
frozen lockfile, and executes the checked-in acceptance suite. The PostgreSQL
job needs only the exact Go toolchain and database image.

The root [.gitattributes](.gitattributes) enforces LF line endings for text
files on every checkout, matching `.editorconfig` and preventing Windows Git
settings from creating formatter-only differences. Windows batch and command
scripts retain CRLF line endings.

The workflow grants only read access to repository contents and does not retain
checkout credentials. GitHub Actions references use major semantic release
tags so routine patch and minor maintenance does not create hash-management
work. CI requires network access for the Go and Node dependency graphs, pinned
quality tools, vulnerability databases, and the Windows GNU Make package.
