# Functional API adaptation proposal protocol

## TL;DR

`propose-functional-api-repair` emits one deterministic endpoint-reference
proposal. `validate-functional-api-repair` then verifies the source preimage
in a disposable checkout and requires three outcomes: original failure,
candidate success, and deterministic negative-control failure. Original bytes
are restored after every modified phase. Neither command commits or opens a
pull request.

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

## Remaining non-goals

This slice intentionally does not:

- open a pull request; or
- persist unsuccessful validation attempts; or
- learn from review outcomes.

Those behaviors require the review outcome and workflow integration planned in
the remaining M06 work.
