# Argus

Argus is an adaptive test intelligence and evolution platform. It determines
which tests should run for a software change, identifies stale or missing test
coverage, and produces evidence-backed maintenance proposals.

The repository has completed its engineering foundation and now includes a
real repository-descriptor ingestion boundary and a durable evidence catalog.
Effect Schema authors the wire contracts, generated JSON Schemas validate them,
Go converts repository input into catalog state at a verified immutable
revision, and PostgreSQL stores immutable snapshots and impact observations.
The control plane also accepts signed GitHub pull-request webhooks, resolves a
stable base/head comparison, retains bounded changed-file evidence, compares
changed OpenAPI 3 documents at both immutable revisions, and durably maps
changed operations to explicitly declared capabilities.
Versioned queries enumerate stable tests and expose supported, refuted, stale,
or conflicting capability-to-test relationships without discarding evidence.

## Start here

- [Product definition](docs/product/product-definition.md)
- [Architecture overview](docs/architecture/overview.md)
- [Contract workspace](contracts/README.md)
- [Functional API adapter protocol](docs/integrations/functional-api-adapters.md)
- [Functional API adaptation protocol](docs/integrations/functional-api-adaptation.md)
- [GitHub Actions functional API integration](docs/integrations/github-actions-functional-api.md)
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
export ARGUS_DATABASE_URL='postgres://argus_runtime:...@db.example/argus'
export ARGUS_GITHUB_TOKEN='fine-grained-token'
export ARGUS_GITHUB_WEBHOOK_SECRET='shared-webhook-secret'
go run ./cmd/control-plane
~~~

Apply migrations first. The command listens on `127.0.0.1:8080` by default and
accepts `POST /webhooks/github`. It verifies `X-Hub-Signature-256` over the raw
body, accepts supported `pull_request` actions, resolves files through the
GitHub API, and returns `argus.dev/change-set/v1`. A new delivery returns `201`;
an exact retry returns the original result with `200` and does not call GitHub
again. Reusing a delivery ID with different signed content returns `409`.
Before success, the workflow also stores an
`argus.dev/capability-impact/v1` assessment. OpenAPI operations opt into
mapping with `x-argus-capabilities: [capability-key]`; unmapped operations and
partial analysis remain explicit evidence rather than being discarded.

Set `ARGUS_HTTP_ADDRESS` to change the listener. GitHub Enterprise deployments
also set `ARGUS_GITHUB_API_URL` and `ARGUS_GITHUB_HOST`. The token needs only
read access to pull requests and repository metadata. Request bodies, tokens,
and webhook secrets are never logged. Press `Ctrl+C` for graceful shutdown.
This endpoint is provider-authenticated ingestion, not the general Argus API;
catalog queries remain local until the M10 identity boundary exists.

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

## Inspect change impact

After a webhook succeeds, inspect its persisted semantic assessment by verified
delivery identity:

~~~sh
go run ./cmd/catalog get-change-impact \
  -provider github \
  -delivery-id 01234567-89ab-cdef-0123-456789abcdef
~~~

The result includes document-level semantic and breaking-change counts, changed
HTTP operations, explicit capability mappings, potentially-breaking markers,
and partial-analysis warnings. It is the explainability input for M05 selection,
not yet an execution decision.

## Select functional API tests

Generate a deterministic execution manifest after the matching approved
base-revision catalog and change impact have been stored:

~~~sh
export ARGUS_DATABASE_URL='postgres://argus_runtime:...@db.example/argus'
go run ./cmd/select \
  -provider github \
  -delivery-id 01234567-89ab-cdef-0123-456789abcdef
~~~

The command emits `argus.dev/execution-manifest/v1`. With complete mapped
impact, tests sharing an affected capability are `RUN_REQUIRED`; other
functional API candidates are `SKIP_FOR_NOW` in the early stage and explicitly
required in a later full-suite control. Partial, empty, or unmapped impact
switches to fallback mode and requires every functional API candidate. The
manifest reports affected capabilities without a mapped test.

## Plan heterogeneous functional API execution

Bind every repository/adapter group in the manifest to a reviewed immutable
test revision, then generate the flat CI job matrix:

~~~sh
go run ./cmd/plan-functional-api \
  -manifest ./execution-manifest.json \
  -bindings ./.argus/functional-api-execution-bindings.json \
  > ./execution-plan.json
~~~

The planner rejects missing, extra, duplicate, or coordinate-mismatched
bindings. It emits a selected job only for groups with required tests and a
full-suite job for every group. Commands and credentials remain reviewed CI
configuration rather than manifest or plan content. See the
[GitHub Actions functional API guide](docs/integrations/github-actions-functional-api.md)
for immutable checkout, matrix execution, attempt upload, trusted ingestion,
and shadow-report wiring.

## Run a functional API manifest group

After checking out a cataloged test repository at an immutable revision, run
one explicit repository/adapter group through a CI-local adapter:

~~~sh
go run ./cmd/run-functional-api \
  -manifest ./execution-manifest.json \
  -stage selected \
  -attempt-id github-123456-1 \
  -test-repository-id tests-1 \
  -test-revision aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa \
  -adapter playwright \
  -- node ./tools/argus-playwright-adapter.mjs \
  > ./execution-attempt.json
~~~

The adapter reads a versioned JSON request on stdin and writes normalized test
results on stdout. Argus rejects missing or additional results, binds the
attempt to the canonical manifest and test revision, and emits
`argus.dev/execution-attempt/v1`. Use `-stage full-suite` for the later control.
A non-passing test result is emitted as evidence before the command returns a
non-zero CI status. See the
[functional API adapter protocol](docs/integrations/functional-api-adapters.md)
for conformance and security requirements.

## Propose a constrained functional API repair

For one required test and one complete OpenAPI impact, ask a reviewed
framework adapter to locate the request target and emit a reviewable proposal:

~~~sh
go run ./cmd/propose-functional-api-repair \
  -impact ./capability-impact.json \
  -manifest ./execution-manifest.json \
  -test-repository-id tests-1 \
  -suite-key orders-api \
  -test-key list-orders \
  -test-revision aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa \
  -- node ./tools/argus-playwright-adaptation-adapter.mjs \
  > ./adaptation-proposal.json
~~~

Argus proceeds only for a unique removed/added operation pair with the same
HTTP method, `operationId`, and capability mapping. The adapter may return one
exact request-target edit or abstain. This command never modifies source; its
`PATCH_AND_VALIDATE` output is input to the later isolated validation stage.
See the [functional API adaptation protocol](docs/integrations/functional-api-adaptation.md).

Validate the proposal in an explicitly disposable checkout:

~~~sh
go run ./cmd/validate-functional-api-repair \
  -proposal ./adaptation-proposal.json \
  -disposable-workspace "$RUNNER_TEMP/orders-tests-validation" \
  -- node ./tools/argus-playwright-validation-adapter.mjs \
  > ./validation-evidence.json
~~~

Argus verifies and temporarily materializes only the proposed source span. It
requires the original test to fail, the candidate to pass, and a deterministic
invalid-endpoint control to fail. Original bytes are restored and verified
after each modified run. The checkout must still be discarded after validation.

Publish the correlated proposal and successful validation proof as a draft
GitHub pull request:

~~~sh
export ARGUS_GITHUB_TOKEN='fine-grained-token'
go run ./cmd/open-functional-api-repair-pr \
  -proposal ./adaptation-proposal.json \
  -validation-evidence ./validation-evidence.json \
  -base-branch main \
  > ./adaptation-review.json
~~~

The token is read only from the environment and needs repository contents and
pull-request write access in the test repository. Argus verifies the immutable
source preimage again, creates a deterministic `argus/endpoint-repair-*`
branch, commits only the validated file, and opens a draft PR containing the
validation summary. Exact retries return the same open draft; a divergent
branch or PR is rejected. The command never marks a PR ready, merges it, or
deletes provider state.

After that pull request reaches a terminal state, capture the review result as
portable learning evidence:

~~~sh
export ARGUS_GITHUB_TOKEN='fine-grained-token'
go run ./cmd/capture-functional-api-review-outcome \
  -proposal ./adaptation-proposal.json \
  -validation-evidence ./validation-evidence.json \
  -review ./adaptation-review.json \
  -reason-code corrected \
  -reason-note 'Reviewer updated the expected request headers.' \
  > ./review-outcome.json
~~~

Argus derives `accepted-as-proposed`, `accepted-with-edits`, or `rejected`
from the closed GitHub pull request and requires a compatible explicit reason
code. When the final head differs from the generated head, it retains the
complete bounded patch between those two commits. Open reviews, divergent
history, missing patches, and truncated comparisons fail closed. The command
does not infer correctness from merge state.

After applying migrations, persist and retrieve that immutable outcome:

~~~sh
export ARGUS_DATABASE_URL='postgres://argus_runtime:...@db.example/argus'
go run ./cmd/adaptation-evidence ingest -file ./review-outcome.json
go run ./cmd/adaptation-evidence get \
  -outcome-id 0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef
~~~

An exact semantic retry reports `created: false`, even when it was observed
again later. A different outcome, reason, or final edit under the same review
identity is an immutable conflict. Reads reconstruct the header and complete
reviewer edits from one repeatable-read database snapshot.

Persist each normalized attempt after the CI job has uploaded any referenced
artifact bytes:

~~~sh
export ARGUS_DATABASE_URL='postgres://argus_runtime:...@db.example/argus'
go run ./cmd/execution-evidence ingest -file ./selected-attempt.json
go run ./cmd/execution-evidence ingest -file ./full-suite-attempt.json
~~~

An exact retry reports `created: false`; different content under the same
attempt ID is rejected. Produce a deterministic comparison by naming both
attempts explicitly:

~~~sh
go run ./cmd/execution-evidence shadow-report \
  -selected-attempt github-123456-selected \
  -full-suite-attempt github-123456-full
~~~

The versioned report contains duration reduction, failure recall in basis
points, and each full-suite failure that selection missed. Argus rejects pairs
from different manifests, repositories, revisions, or adapter versions, and a
full-suite attempt that is not a superset of the selected set. Artifact
references are metadata only; ingestion does not upload or verify the external
objects.

For a heterogeneous plan, bind every planned group to its explicit stored
attempt IDs and produce one complete safety report:

~~~sh
go run ./cmd/execution-evidence plan-shadow-report \
  -plan ./execution-plan.json \
  -attempt-bindings ./attempt-bindings.json
~~~

`argus.dev/execution-plan-attempt-bindings/v1` uses `null` for the selected
attempt of a full-only group and always requires a full-suite attempt. The
aggregate report checks every attempt against the plan and counts a full-only
failure as a `not-selected` miss. Its duration fields sum normalized per-test
durations rather than parallel CI wall-clock time. Pairwise reports remain
available for focused diagnosis.

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
