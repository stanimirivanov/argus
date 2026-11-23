# ADR-0015: Validate repairs with a restored negative-control workspace

**Status:** Accepted  
**Date:** 2026-09-27

## TL;DR

Argus validates an endpoint repair only in an explicitly supplied disposable
checkout. It verifies the proposal's source digest and byte span, runs the
unchanged test, temporarily applies the exact candidate, and then substitutes
a deterministic invalid endpoint as a negative control. Validation succeeds
only when the original fails, the candidate passes, and the negative control
fails. Original bytes are restored and verified after every modified phase;
restoration failure or adapter source mutation prevents evidence publication.

## Context

An endpoint edit is not trustworthy merely because authoritative contract
evidence suggested it or because the edited test passes. A test can pass after
an accidental oracle weakening, a no-op source edit, a catch-all route, stale
environment state, or an adapter that executed a different test.

ADR-0014 deliberately stopped at a proposal. The next M06 boundary must prove
that the selected test was failing for the stale endpoint, becomes successful
for the exact proposed endpoint, and remains sensitive to the request target.
It must do this without leaving a user's working checkout modified.

## Decision

### Require an explicitly disposable checkout

`validate-functional-api-repair` requires a caller-supplied
`-disposable-workspace`. CI is responsible for checking out the proposal's
immutable test revision and preparing dependencies there. Argus resolves the
root, confines the proposal path below it, follows symlinks only when the final
target remains inside the root, and accepts regular files only.

The workspace adapter verifies the complete-file SHA-256 before each write.
The application service verifies the byte-addressed original text before it
constructs either variant. It writes only the already reviewed request-target
span.

### Execute three ordered phases

The validation policy runs exactly one stable catalog test in this order:

1. `original`: unchanged source must produce normalized outcome `failed`;
2. `candidate`: the proposal's exact replacement must produce `passed`; and
3. `negative-control`: the same span is replaced with
   `/__argus_negative_control__/<proposal-prefix>` and must produce `failed`.

An `error` outcome never satisfies a required failure. The negative endpoint
is derived by Argus, not selected by the adapter. This discriminates a test
that is genuinely sensitive to the request target from one that passes
regardless of the endpoint value.

### Restore after every modified execution

Candidate and negative-control bytes are each materialized from the original
preimage, never from the preceding variant. After execution, Argus checks that
the adapter did not alter the source, restores the original bytes using a
non-cancelled cleanup context, reads the file again, and verifies its original
SHA-256.

If execution, source verification, or restoration fails, Argus returns an
error and publishes no successful validation evidence. The workspace remains
classified as disposable because a process crash or machine loss can bypass
in-process cleanup.

### Keep execution framework-specific but policy-owned

For each phase, Argus sends a versioned request to one explicit reviewed
command with the disposable workspace as its working directory. The adapter
maps the stable suite and test keys to native framework invocation and returns
one normalized result. Argus correlates validation, proposal, phase, test,
adapter, and source-digest identities.

All three results must use the same adapter version and non-overlapping ordered
timestamps. Successful `ValidationEvidence` retains the original, candidate,
negative, and restored source digests plus normalized failure diagnostics. It
does not authorize commit, pull-request creation, merge, or deployment.

## Consequences

- A passing candidate alone cannot advance to review.
- The candidate and negative control are deterministic and replayable from the
  proposal plus original source bytes.
- CI must provide a disposable checkout and a conforming framework adapter.
- A crash can leave only the disposable checkout dirty; callers must discard
  it after the command regardless of outcome.
- Rejected or interrupted validations currently return an error rather than a
  persisted unsuccessful evidence record.
- Evidence persistence, review pull-request creation, and reviewer outcome
  ingestion remain later M06 work.

## Rejected alternatives

### Accept candidate success without reproducing failure

Rejected because an already-passing test does not demonstrate that the repair
addressed observed drift.

### Use only original and candidate runs

Rejected because a catch-all route or weakened test can make both the intended
candidate and unrelated endpoint values pass.

### Patch the user's primary checkout

Rejected because cancellation, process failure, or adapter behavior could
leave unrelated work modified. The command requires a disposable checkout and
still restores it defensively.

### Let the adapter choose the negative control

Rejected because adapters could use inconsistent or trivially passing controls.
The policy derives the invalid endpoint and verifies its exact source digest.
