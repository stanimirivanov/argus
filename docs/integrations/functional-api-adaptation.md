# Functional API adaptation proposal protocol

## TL;DR

`propose-functional-api-repair` emits one deterministic endpoint-reference
proposal. `validate-functional-api-repair` verifies it with original,
candidate, and negative-control runs in a disposable checkout.
`open-functional-api-repair-pr` then rechecks the immutable source and opens an
idempotent draft GitHub pull request containing only the validated edit. After
the review closes, `capture-functional-api-review-outcome` derives its terminal
disposition and captures any bounded reviewer edits. No command merges or
marks the review ready.

## Boundary and responsibilities

Argus decides whether a contract change is a unique endpoint rename. The
repository-owned adapter decides where that endpoint is represented in the
named test. This split lets Playwright, pytest, Postman, and other integrations
use native parsers without giving each adapter control over adaptation policy.

The command requires:

- a `capability-impact/v1` document with `status: complete`;
- the matching `execution-manifest/v1` document;
- one `RUN_REQUIRED` test identity from that manifest;
- the immutable test repository revision checked out by CI; and
- a literal, reviewed adapter command after `--`.

Manifest and impact change provenance must match exactly. A rename candidate
must be the only removed/added pair with the same method, `operationId`, and
capabilities relevant to the test.

## Adapter request

The adapter reads one
`argus.dev/functional-api-adaptation-request/v1` JSON document from stdin. It
contains the deterministic proposal ID, immutable change and test provenance,
and the only permitted endpoint mapping.

The adapter must inspect the already checked-out test revision and map
`suiteKey` plus `testKey` through repository-owned configuration. It must not
select additional tests or infer another endpoint mapping.

Argus imposes a ten-minute maximum on the proposal adapter and a two-hour
maximum on each validation adapter process; the command's own timeout may be
shorter. Both processes stop when stdout exceeds 8 MiB and expose only the
last 64 KiB of stderr. SIGINT and SIGTERM cancel the active request.

## Adapter result

The adapter writes one
`argus.dev/functional-api-adaptation-result/v1` JSON document to stdout and
diagnostics to stderr. It returns either:

- `candidate`, with exactly one `request-target` text edit; or
- `abstained`, with a stable reason code and bounded human explanation.

The edit uses UTF-8 byte offsets and includes:

- a repository-relative source path;
- the SHA-256 of the complete source file before editing;
- half-open `startByte` and `endByte` offsets;
- the exact original endpoint path;
- the exact replacement endpoint path; and
- semantic role `request-target`.

The adapter should abstain when the test mapping is missing, more than one
request target matches, the source language is unsupported, the preimage is
unavailable, or the change would touch assertions, expected values, fixtures,
or additional files.

## Reference invocation

```sh
go run ./cmd/propose-functional-api-repair \
  -impact ./capability-impact.json \
  -manifest ./execution-manifest.json \
  -test-repository-id R_orders_tests_01 \
  -suite-key orders-api \
  -test-key list-orders \
  -test-revision 0123456789abcdef0123456789abcdef01234567 \
  -- node ./tools/argus-playwright-adaptation-adapter.mjs \
  > adaptation-proposal.json
```

The executable is an integration point; Argus does not ship a universal
Playwright or pytest source rewriter. The repository should pin, review, and
test its adapter using the shared request/result fixtures under
`contracts/fixtures`.

## Proposal output

On success, stdout contains `argus.dev/adaptation-proposal/v1`. The proposal
retains the change, test, adapter, rename, source digest, offsets, and exact
replacement. `PATCH_AND_VALIDATE` means a later isolated workflow may apply
and test the candidate. It does not mean the current command changed source or
that CI may commit or merge it.

## Isolated validation

Prepare a disposable checkout at the exact `test.revision` named by the
proposal. Install dependencies in that checkout, then invoke:

```sh
go run ./cmd/validate-functional-api-repair \
  -proposal ./adaptation-proposal.json \
  -disposable-workspace "$RUNNER_TEMP/orders-tests-validation" \
  -- node ./tools/argus-playwright-validation-adapter.mjs \
  > validation-evidence.json
```

The workspace is temporarily modified. It MUST be disposable and MUST be
discarded after the command regardless of outcome. Do not point the command at
a developer's primary checkout.

Argus verifies the complete-file preimage digest and exact byte span before it
writes. It runs the unchanged source, materializes the exact proposed endpoint,
restores the original, materializes a deterministic invalid endpoint, and
restores the original again. It rejects an adapter that modifies the source
file itself or returns mismatched proposal, phase, test, adapter, or source
identities. The proposed source file and every materialized variant are limited
to 16 MiB.
Each mandatory source check and restoration uses its own non-cancelled
30-second deadline. Restoration is still attempted if a preceding check times
out. A failed restoration publishes no validation evidence; discard the
disposable checkout regardless of outcome.

### Validation adapter protocol

The command invokes the same literal adapter command three times with the
disposable checkout as its working directory. Each invocation reads one
`argus.dev/functional-api-repair-validation-request/v1` document from stdin
and writes one `argus.dev/functional-api-repair-validation-result/v1` document
to stdout.

The request contains one stable suite/test identity, the proposal and
validation identities, the phase, and the SHA-256 of the source bytes currently
materialized by Argus. The adapter MUST run exactly that test and MUST NOT edit
the proposed source file. Diagnostics belong on stderr.

Normalized outcomes are `passed`, `failed`, and `error`. A required failure
must be `failed`; infrastructure or setup `error` does not prove the candidate.
All phases must report the same adapter version.

### Validation evidence

Successful stdout is `argus.dev/validation-evidence/v1` and retains:

- proposal, policy, test, and adapter identity;
- the exact candidate file, byte span, original text, and replacement text;
- original, candidate, negative-control, and restored source SHA-256 values;
- the deterministic negative-control endpoint;
- ordered timestamps and normalized outcome for all three phases; and
- bounded failure codes and messages for original and negative-control runs.

Validation evidence is produced only for `failed → passed → failed`. It is a
precondition for later review automation, not merge authorization.

When a trustworthy phase disproves the candidate, the command still exits
non-zero but writes `argus.dev/validation-rejection/v1` to stdout. The document
records the completed run prefix and one of `original-passed`,
`candidate-failed`, or `negative-control-passed`, plus the verified restoration
digest. Capture stdout on failure if this evidence should be retained. Process,
adapter, correlation, source-integrity, and restoration errors do not produce a
rejection document because they are not candidate-quality evidence.

Persist or retrieve the rejection after applying migrations:

```sh
go run ./cmd/adaptation-evidence ingest-validation-rejection \
  -file ./validation-rejection.json
go run ./cmd/adaptation-evidence get-validation-rejection \
  -validation-id 0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef
```

Persistence admits one immutable document per validation ID. Exact retries
report `created: false`; different runs or diagnostics under the same identity
are conflicts. Successful evidence remains a separate contract and is the only
kind accepted by draft pull-request publication.

## Draft pull-request publication

Publication is an explicit external-write step and requires both prior JSON
documents plus a caller-selected base branch:

```sh
export ARGUS_GITHUB_TOKEN='fine-grained-token'
go run ./cmd/open-functional-api-repair-pr \
  -proposal ./adaptation-proposal.json \
  -validation-evidence ./validation-evidence.json \
  -base-branch main \
  > adaptation-review.json
```

Set `ARGUS_GITHUB_API_URL` and `ARGUS_GITHUB_HOST` for GitHub Enterprise. The
token needs contents and pull-request write permission only in the test
repository. It is never accepted as a command-line flag or included in output.
The API URL cannot contain credentials, a query, or a fragment. Argus rejects
all GitHub API redirects, including same-origin redirects, before a bearer
token can be forwarded; configure the final canonical API endpoint.

Before writing, Argus correlates proposal, validation, test, adapter, policy,
and edit identities. It reloads the source at the proposal's immutable test
revision, verifies the complete-file SHA-256 and byte preimage, reconstructs
the candidate, and requires its digest to equal the validated candidate digest.
The provider adapter also verifies that the current owner/name coordinates
still resolve to the cataloged GitHub repository ID.

The head branch is deterministic:
`argus/endpoint-repair-<first-12-proposal-id-characters>`. Publication creates
or recovers that branch, commits the one validated file with an optimistic blob
precondition, and opens a draft PR. An exact retry returns the same open draft,
including after a prior attempt stopped between branch, commit, and PR creation.
Existing divergent content, a non-draft PR, a different base, or a closed PR is
reported as a conflict and is never overwritten.

`argus.dev/adaptation-review/v1` records the proposal, validation, repository,
base/head revisions, deterministic branches, PR number and URL, draft state,
and publication time. The PR body summarizes the three validation gates and
source digests. This evidence is a human-review handoff, not merge authority.

## Terminal review-outcome capture

After the PR closes or merges, retain an explicit reason from the reviewer
workflow and invoke:

```sh
export ARGUS_GITHUB_TOKEN='fine-grained-token'
go run ./cmd/capture-functional-api-review-outcome \
  -proposal ./adaptation-proposal.json \
  -validation-evidence ./validation-evidence.json \
  -review ./adaptation-review.json \
  -reason-code corrected \
  -reason-note 'Reviewer updated the expected request headers.' \
  > review-outcome.json
```

The command revalidates and correlates all three earlier documents before it
reads provider state. It verifies the stable repository identity and exact PR,
base branch, and generated head branch. An open PR returns a not-final error.

The terminal decision is provider-derived, not supplied by the caller:

| Provider state | Decision | Required reason code |
|:--|:--|:--|
| Merged at the generated head | `accepted-as-proposed` | `approved` |
| Merged after additional commits | `accepted-with-edits` | `corrected` |
| Closed without merge | `rejected` | `incorrect-repair`, `unsafe-repair`, `no-longer-needed`, `superseded`, or `other` |

`other` requires a reason note. A note may accompany any decision but is
bounded to 1,000 characters. A merge remains workflow evidence, not proof that
the repair was correct.

When the final head differs, Argus compares the generated head commit directly
to the final reviewed head. The generated commit must be the merge base and
the final head must be strictly ahead. Every changed file requires a complete
unified patch; unknown file kinds, missing or oversized patches, comparison
truncation, non-linear history, and excess total evidence fail closed. The
resulting `argus.dev/review-outcome/v1` is deterministic, portable evidence for
later evaluation and learning.

## Durable outcome evidence

Apply the current PostgreSQL migration chain, then ingest the captured outcome:

```sh
export ARGUS_DATABASE_URL='postgres://argus_runtime:...@db.example/argus'
go run ./cmd/adaptation-evidence ingest -file ./review-outcome.json
```

The command validates Effect Schema structure, domain invariants, and the
derived outcome identity before opening the database. Persistence creates one
immutable outcome per `reviewId` and stores all reviewer edits atomically. An
exact retry returns `created: false`; another disposition, reason, revision,
or patch for that review returns an outcome conflict. A later observation of
the same terminal evidence is an exact retry, so `observedAt` does not change
semantic identity and the first durable observation remains unchanged.

Retrieve the complete portable document by `outcomeId`:

```sh
go run ./cmd/adaptation-evidence get \
  -outcome-id 0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef
```

Reads use a repeatable-read transaction so the outcome header and reviewer
patches come from one database snapshot. Persistence performs no GitHub calls,
does not update PR state, and does not convert a merge into correctness or
promotion authority.

## Remaining non-goals

This slice intentionally does not:

- persist infrastructure or evidence-integrity failures as candidate labels;
- treat a merge as automatic promotion or correctness authority; or
- train or update an adaptation policy from a single captured outcome.

Aggregate learning queries and governed policy updates remain follow-up work.
The database stores the bounded patches contained by `review-outcome/v1`; it
does not replace source control or an artifact-retention system.
