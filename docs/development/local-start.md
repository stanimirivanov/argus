# Start Argus locally

## TL;DR

- Start the isolated PostgreSQL 17 service with
  `docker compose -f compose.local.yaml up -d --wait` from the repository root.
- Run `go run ./cmd/migrate --local`. For the DBOS evaluation, also run
  `go run ./cmd/migrate --local --dbos-evaluation`.
- Supply a real GitHub read token and webhook secret, then run
  `go run ./cmd/control-plane --local` with `ARGUS_DBOS_EVALUATION=true`.
- `--local` supplies only a loopback database URL, not GitHub credentials.
  Deployed commands still require explicit configuration.
- `Ctrl+C` or SIGTERM gives active webhook handlers up to 10 seconds to finish
  before DBOS and database resources close; longer work still needs redelivery.

## Prerequisites

Use the Go version in [go.mod](../../go.mod) and a Docker-compatible Compose
v2 installation. The local database is optional for ordinary `make verify`
and `make validate`; these checks do not require Docker. See the
[developer quickstart](developer-quickstart.md) for supported toolchains and
the complete contributor workflow.

The checked-in [Compose file](../../compose.local.yaml) starts PostgreSQL
17.11 on `127.0.0.1:55432` with a named development volume. Its fixed
`argus_dev` credentials are **only for a workstation database**. Do not reuse
them in shared, CI, or production environments. Other local processes can
still access the loopback port. If port 55432 is occupied, use a separately
configured PostgreSQL instance and set `ARGUS_DATABASE_URL` explicitly
instead of using `--local`.

## First local start

From the repository root, in PowerShell 7:

~~~powershell
docker compose -f compose.local.yaml up -d --wait
go run ./cmd/migrate --local
go run ./cmd/migrate --local --dbos-evaluation

$env:ARGUS_GITHUB_TOKEN = '<your repository-read token>'
$env:ARGUS_GITHUB_WEBHOOK_SECRET = '<your own random webhook secret>'
$env:ARGUS_DBOS_EVALUATION = 'true'
go run ./cmd/control-plane --local
~~~

The same `docker compose` and `go run` commands work in Bash; set the three
variables with `export` instead. Omit the DBOS migration and flag for the
unchanged default webhook path. Both migrations are explicit and repeatable.
The ordinary migration creates the authoritative catalog schema; the DBOS
command prepares only the experimental `argus_dbos_eval` checkpoint schema.
Normal server startup does not migrate either schema.

The server listens on `127.0.0.1:8080` and accepts signed
`POST /webhooks/github`. A local-only listener needs a controlled forwarding
mechanism if GitHub must deliver a webhook to your workstation. Configure
the GitHub webhook with **the same secret** and give the token read access
to pull requests and repository metadata. Neither value has a checked-in
default. This is a live-provider integration, not an offline demo; without
those credentials, the server correctly refuses startup. See
[ADR-0028](../decisions/0028-evaluate-embedded-dbos-workflows-for-change-processing.md)
for evaluation semantics and limitations.

Use `Ctrl+C` to stop the server. `docker compose -f compose.local.yaml down`
stops the database but preserves its volume. Do not use `down -v` unless you
intentionally want to delete that development database. Never point the local
Compose service or its credentials at shared data.

On `Ctrl+C` or SIGTERM, the listener stops admitting requests and gives active
handlers a bounded 10-second drain. The application context remains live during
that drain, so the signal alone does not cancel an accepted DBOS assessment.
After the drain, DBOS shutdown cancels any remaining in-process work before the
database pool closes. A delivery that did not receive a success response must
be redelivered; this behavior does not prove process-kill recovery or justify
production promotion of the DBOS evaluation.

## Existing or deployed PostgreSQL

Do not pass a database URL as a command argument; it can appear in process
listings. Set `ARGUS_DATABASE_URL` in the environment using the appropriate
privileged identity for each migration, then a separately provisioned runtime
identity for the server:

~~~powershell
$env:ARGUS_DATABASE_URL = 'postgres://<migration-role>:<secret>@<host>/argus?sslmode=verify-full'
go run ./cmd/migrate
go run ./cmd/migrate --dbos-evaluation

$env:ARGUS_DATABASE_URL = 'postgres://<runtime-role>:<secret>@<host>/argus?sslmode=verify-full'
$env:ARGUS_GITHUB_TOKEN = '<your repository-read token>'
$env:ARGUS_GITHUB_WEBHOOK_SECRET = '<your shared webhook secret>'
$env:ARGUS_DBOS_EVALUATION = 'true'
go run ./cmd/control-plane
~~~

`--local` cannot be combined with `ARGUS_DATABASE_URL`, and local server mode
refuses a non-loopback `ARGUS_HTTP_ADDRESS`. In normal mode, a missing database
URL, token, or webhook secret is an error. Production runtime privileges and
migration ownership are described in the [PostgreSQL guide](postgresql.md);
the DBOS evaluation has not met production promotion criteria.

## Troubleshooting

- `ARGUS_DATABASE_URL is required`: either start the Compose service and add `--local`, or configure an existing database URL explicitly. The bare `go run ./cmd/migrate --dbos-evaluation` command intentionally has no implicit production target.
- `connection refused`: confirm `docker compose -f compose.local.yaml ps` shows a healthy PostgreSQL service, and verify port 55432 is free.
- `DBOS evaluation schema` startup failure: prepare the schema with `go run ./cmd/migrate --local --dbos-evaluation` before setting `ARGUS_DBOS_EVALUATION=true`.
- `required environment is missing`: provide a real GitHub token and your webhook secret. The local database default does not bypass webhook authentication.
- A webhook returns an authorization or provider error: check the token's repository access, webhook secret, GitHub API host, and the repository's supported pull-request event. Do not log or paste credentials while diagnosing.
