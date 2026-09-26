# Functional API adapter protocol

## TL;DR

- `run-functional-api` consumes an execution manifest and runs one explicit
  test-repository/adapter group.
- `plan-functional-api` first turns heterogeneous manifest groups and reviewed
  immutable-revision bindings into a deterministic flat CI matrix.
- The adapter reads `argus.dev/functional-api-adapter-request/v1` from stdin and
  writes `argus.dev/functional-api-adapter-result/v1` to stdout.
- Adapter commands are CI configuration, never manifest content, and run
  without a command shell.
- Emit valid result JSON and exit zero even for test failures; Argus emits the
  normalized attempt and then fails the CI step.
- Use `-stage selected` for early feedback and `-stage full-suite` for the
  authoritative control.

## Responsibilities

Argus owns selection, exact request/result correlation, normalization, and the
portable protocol. A test repository owns dependency installation, framework
configuration, secrets, environment preparation, and the adapter that maps
stable catalog IDs to native framework filters.

The reference runner does not clone repositories, install dependencies, upload
artifacts, or persist results. Run it after the test repository is checked out
at the exact revision supplied with `-test-revision`.

For a heterogeneous manifest, run `plan-functional-api` first. The planner
requires an exact reviewed binding for every repository/adapter group and
always emits the later full-suite job. It does not choose commands or acquire
credentials. See the
[GitHub Actions functional API guide](github-actions-functional-api.md) for the
complete trust boundary and matrix handoff.

## Adapter request

The runner supplies one JSON document on stdin. Its `tests` array is the entire
authorized set for this invocation. An adapter MUST NOT discover additional
tests and silently add them to its response.

~~~json
{
  "apiVersion": "argus.dev/functional-api-adapter-request/v1",
  "attemptId": "github-123456-1",
  "manifest": {
    "apiVersion": "argus.dev/execution-manifest/v1",
    "sha256": "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"
  },
  "stage": "selected",
  "testRepository": {
    "provider": "github",
    "host": "github.com",
    "providerRepositoryId": "tests-1",
    "owner": "example",
    "name": "orders-tests"
  },
  "testRevision": {
    "algorithm": "git-sha1",
    "digest": "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
  },
  "adapter": "playwright",
  "tests": [
    {
      "suiteKey": "orders-api",
      "testKey": "create-order-test",
      "name": "Create order"
    }
  ]
}
~~~

The adapter must map `suiteKey` and `testKey` through repository-owned,
reviewed configuration. Display names are diagnostic and MUST NOT be used as
stable selectors.

## Adapter result

The adapter writes exactly one result document on stdout. It MUST include one
result for every requested test, no extra results, and the unchanged attempt
and adapter identities. Write diagnostic logs to stderr so stdout remains a
machine-readable protocol channel.

~~~json
{
  "apiVersion": "argus.dev/functional-api-adapter-result/v1",
  "attemptId": "github-123456-1",
  "adapter": { "id": "playwright", "version": "1.58.2" },
  "startedAt": "2026-09-19T10:00:00Z",
  "completedAt": "2026-09-19T10:00:01Z",
  "results": [
    {
      "suiteKey": "orders-api",
      "testKey": "create-order-test",
      "outcome": "passed",
      "durationMs": 850,
      "failure": null
    }
  ],
  "artifacts": []
}
~~~

Valid outcomes are `passed`, `failed`, `skipped`, and `error`. Failed and error
results require a bounded stable failure code and diagnostic message; passed
and skipped results use `failure: null`. Do not put secrets, full environment
dumps, or unbounded stack traces in the message.

The adapter exits zero after it writes a valid result, including when a test
failed. A non-zero adapter exit means the protocol failed and prevents Argus
from claiming normalized test evidence. After receiving valid output, Argus
returns a non-zero status for failed, incomplete, or error attempts.

## Reference CI invocation

The adapter executable and arguments appear after `--` and are passed directly
to the operating system. They are not evaluated by a shell.

~~~sh
go run ./cmd/run-functional-api \
  -manifest ./execution-manifest.json \
  -stage selected \
  -attempt-id "github-${GITHUB_RUN_ID}-${GITHUB_RUN_ATTEMPT}" \
  -test-provider github \
  -test-host github.com \
  -test-repository-id tests-1 \
  -test-revision aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa \
  -adapter playwright \
  -timeout 30m \
  -- node ./tools/argus-playwright-adapter.mjs \
  > ./execution-attempt.json
~~~

Run the same manifest with `-stage full-suite` in the later control job. The
selected stage includes only `RUN_REQUIRED` decisions; the full-suite stage
includes every matching candidate, including early `SKIP_FOR_NOW` decisions.

Upload `execution-attempt.json` even when the step fails so later ingestion can
retain test failures. Artifact references in the result are metadata only;
the CI workflow remains responsible for uploading bytes and retaining the
referenced checksum.

After artifact upload, ingest the attempt document through the control-plane
store. Exact retries are safe; reusing an attempt ID for different normalized
content is rejected:

~~~sh
go run ./cmd/execution-evidence ingest -file ./execution-attempt.json
~~~

Once both stages are present, name the exact pair to compare:

~~~sh
go run ./cmd/execution-evidence shadow-report \
  -selected-attempt "github-${GITHUB_RUN_ID}-selected" \
  -full-suite-attempt "github-${GITHUB_RUN_ID}-full"
~~~

The pair must share the manifest digest, stable test-repository identity,
immutable test revision, and adapter ID/version. The full-suite result set must
contain every selected test. Argus deliberately does not choose a “latest”
control because concurrent reruns would make that comparison non-reproducible.
The report distinguishes a failing control test that was omitted
(`not-selected`) from one that ran early but did not fail then
(`not-reproduced`).

A planned group with no selected tests has only a full-suite attempt. The v1
pairwise report cannot compare that group; retain its full-suite evidence as
authoritative until aggregate plan-level evaluation is added.

## Bounds and failure behavior

- Manifest input is limited to 16 MiB.
- Adapter stdout is limited to 8 MiB.
- One request is limited to 10,000 tests and 100 artifact references.
- Individual reported test durations are limited to 24 hours.
- Attempt timestamps must be UTC and have at most microsecond precision so
  durable storage never silently rounds immutable evidence.
- CLI timeout must be greater than zero and no more than two hours.
- Missing, duplicate, or unrequested test results reject the whole response.
- Artifact URIs must be absolute and cannot contain embedded user credentials.
- Malformed or trailing JSON, version mismatches, identity mismatches, and
  non-canonical revisions fail closed.

The shared fixtures under `contracts/fixtures/functional-api-adapter-*` are the
starting conformance corpus for adapter implementations.
