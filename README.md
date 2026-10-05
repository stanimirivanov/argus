# Argus

Argus selects tests for a software change, explains the evidence behind the
selection, and proposes constrained, reviewable maintenance when tests become
stale. The current end-to-end slice covers functional API cataloging, change
impact, selection, execution evidence, and validated repair. Functional UI
selection and catalog conformance are available; browser execution and repair
are planned.

## Start locally

Use the exact Go version in [go.mod](go.mod). From the repository root, start
the database and apply the Argus and optional DBOS evaluation migrations:

~~~powershell
docker compose -f compose.local.yaml up -d --wait
go run ./cmd/migrate --local
go run ./cmd/migrate --local --dbos-evaluation
~~~

To run the opt-in DBOS webhook path, supply your own GitHub read token and
webhook secret, then start the control plane:

~~~powershell
$env:ARGUS_GITHUB_TOKEN = '<your repository-read token>'
$env:ARGUS_GITHUB_WEBHOOK_SECRET = '<your own random webhook secret>'
$env:ARGUS_DBOS_EVALUATION = 'true'
go run ./cmd/control-plane --local
~~~

Omit the DBOS migration and flag to use the default path. `--local` supplies
only the loopback development database; it does not supply real GitHub
credentials. The server listens on `127.0.0.1:8080` and accepts signed
`POST /webhooks/github`. [Local startup and troubleshooting](docs/development/local-start.md)
explains Compose, credentials, existing PostgreSQL, and the evaluation's
limits. Normal builds and validation need no database or credentials.

## Find the right guide

The [documentation map](docs/README.md) routes each task to its canonical
source. Common entry points:

- [Developer quickstart](docs/development/developer-quickstart.md): supported tools, setup, and verification.
- [Product definition](docs/product/product-definition.md) and [architecture overview](docs/architecture/overview.md): behavior and boundaries.
- [PostgreSQL guide](docs/development/postgresql.md): catalog migrations, imports, queries, roles, and recovery.
- [Functional UI selection](docs/integrations/functional-ui-selection.md): Playwright catalog checking and browser manifests.
- [Functional API adapter protocol](docs/integrations/functional-api-adapters.md), [CI integration](docs/integrations/github-actions-functional-api.md), and [adaptation protocol](docs/integrations/functional-api-adaptation.md): execution, evidence, and repair.
- [Contract workspace](contracts/README.md) and [decisions](docs/decisions/README.md): versioned formats and technical choices.
- [Contributor workflow](CONTRIBUTING.md), [milestones](docs/roadmap/milestones.md), and [security policy](SECURITY.md).

## Build and verify

~~~sh
make doctor       # report the effective toolchain
make bootstrap    # resolve pinned dependencies once
make verify       # fast build, checks, and ordinary tests
make fmt          # format before review
make validate     # complete non-mutating acceptance checks
make db-validate  # PostgreSQL integration; requires a disposable test server
~~~

See the [Makefile](Makefile) for focused targets and the
[developer quickstart](docs/development/developer-quickstart.md) for tool
versions and platform requirements. Every executable supports standalone
`--help` and `--version` without credentials. Direct-database commands remain
administrative or transitional boundaries, not the ordinary CI integration
pattern.
