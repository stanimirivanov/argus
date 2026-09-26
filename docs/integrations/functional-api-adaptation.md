# Functional API adaptation proposal protocol

## TL;DR

`propose-functional-api-repair` combines complete OpenAPI impact, an execution
manifest, one immutable functional API test, and a reviewed framework adapter.
It emits a single deterministic endpoint-reference proposal or fails closed.
It does not modify source. The adapter must locate exactly one request-target
syntax node and may abstain when it cannot do so safely.

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

## Output and non-goals

On success, stdout contains `argus.dev/adaptation-proposal/v1`. The proposal
retains the change, test, adapter, rename, source digest, offsets, and exact
replacement. `PATCH_AND_VALIDATE` means a later isolated workflow may apply
and test the candidate. It does not mean the current command changed source or
that CI may commit or merge it.

This slice intentionally does not:

- apply the edit;
- reproduce the original failure;
- execute the repaired test or a negative control;
- open a pull request; or
- learn from review outcomes.

Those behaviors require the validation evidence and review outcome contracts
planned in the next M06 delivery slice.
