# ADR-0010: Use a bounded process protocol for functional API execution

- Status: Accepted
- Date: 2026-09-19
- Milestone: M05 - Functional API selection
- Deciders: Argus maintainers
- Supersedes: None
- Superseded by: None

## TL;DR

- Invoke functional API adapters as explicit CI-local processes with versioned
  JSON on standard input and output.
- Never take an executable or shell fragment from an execution manifest.
- Bind every attempt to canonical manifest bytes and an immutable test revision.
- Reject missing, duplicate, or unrequested results rather than accepting
  partial evidence.
- Emit normalized attempt evidence before returning a failing CI status for a
  non-passing outcome.

## Context

M05 can now select functional API tests, but a framework-specific runner cannot
be part of the selection domain. Test repositories may use Playwright, pytest,
REST Assured, or another framework, and their setup remains repository-owned.
Argus needs one portable boundary that a CI job can use without granting a
repository-authored manifest authority to choose arbitrary commands.

The first integration must preserve exact test identities, support both the
early selected stage and the later full-suite control, retain failures as data,
and remain usable on Linux and Windows. Remote dispatch, durable attempt
storage, artifact upload, and framework-specific implementation are not yet
available.

## Decision

Argus defines versioned Effect Schema contracts for a functional API adapter
request, adapter result, and normalized execution attempt. The reference
`run-functional-api` command reads and validates an execution manifest, selects
one explicitly named test repository and adapter, and invokes an explicitly
configured process.

The process receives one JSON request on standard input and must write one JSON
result on standard output. Argus passes the executable and argument vector
directly to the operating system without a command shell. The manifest can
select cataloged tests but cannot supply or modify the executable.

Each request includes:

- an attempt identity supplied by the CI system;
- the SHA-256 digest of the canonical execution manifest;
- the selected or full-suite stage;
- stable test-repository identity and an immutable test revision;
- the expected adapter identity; and
- the exact bounded set of suite/test identities.

Adapter output is untrusted. Argus validates the contract, attempt and adapter
identity, timestamps, bounds, test-result semantics, and exact set equality
with the request. Missing, duplicate, or additional results reject the entire
attempt. Test outcomes are normalized to passed, failed, skipped, or error;
attempt outcome is derived with error, failed, incomplete, then passed
precedence.

An adapter returns exit code zero after emitting a structurally valid result,
even when tests failed. The reference runner emits the normalized attempt and
then returns a non-zero status for failed, incomplete, or error outcomes. A
non-zero adapter exit, timeout, malformed document, or oversized output is an
adapter failure and does not fabricate attempt evidence.

The initial runner executes one repository/adapter group per invocation. CI is
responsible for checkout, dependency installation, isolation, choosing the
adapter command, and uploading referenced artifacts. Output is capped at 8 MiB
and runtime at two hours.

## Alternatives considered

### Embed Playwright or another framework in the control plane

- Benefits: One executable could discover and run tests directly.
- Costs and risks: Framework dependencies and repository setup leak into the
  control plane and prevent a general adapter ecosystem.
- Reason not selected: Test frameworks remain behind a portable boundary.

### Store an arbitrary command in the manifest

- Benefits: Minimal CI configuration.
- Costs and risks: Repository-authored catalog data becomes a command-execution
  authority and makes review, quoting, and cross-platform behavior unsafe.
- Reason not selected: The CI workflow must explicitly authorize executables.

### Start with remote control-plane dispatch

- Benefits: Central scheduling and lifecycle management immediately.
- Costs and risks: Requires identity, queue, worker, cancellation, credential,
  and deployment decisions before the adapter semantics are proven.
- Reason not selected: The CI-local protocol establishes the reusable seam
  without prematurely choosing deployment topology.

## Consequences

### Positive

- Framework adapters can evolve independently while sharing conformance
  fixtures and exact correlation rules.
- The same protocol works in common Linux and Windows CI environments.
- Test failures survive as normalized evidence instead of disappearing behind
  a process exit code.
- Manifests cannot inject executable paths or shell fragments.

### Negative

- Adapter authors must translate stable Argus IDs to framework-native filters.
- CI must explicitly provide test checkout revision, adapter command, attempt
  identity, and repository group.
- A process failure before valid output cannot yet be recorded durably as an
  infrastructure attempt.

### Neutral or follow-up

- A later M05 slice will persist attempts, ingest artifact registrations, run
  selected and full-suite controls, and calculate selection misses.
- A matrix planner may automate heterogeneous repository/adapter groups when a
  design-partner manifest demonstrates that need.
- Playwright and other concrete adapters remain repository-owned integrations.

## Compatibility and migration

This change adds three v1 contracts and a new command. Existing manifests and
catalog tables are unchanged. Adapters must reject unsupported request versions
and Argus rejects unsupported result versions. Future semantic changes require
new protocol versions rather than silent field reinterpretation.

## Security and operations

The adapter command comes only from reviewed CI configuration. Argus does not
invoke a shell, evaluate manifest text, fetch artifact URIs, or copy raw adapter
stderr into normalized evidence. CI must run repository code in an isolated,
least-privilege worker and control its network and secrets.

Manifest input is capped at 16 MiB, adapter output at 8 MiB, test results at
10,000, artifact references at 100, individual durations at 24 hours, and the
runner timeout at two hours. Artifact references cannot contain URI user-info.

## Validation

Effect Schema conformance fixtures cover the three contracts. Go tests cover
selected/full-suite planning, exact result correlation, attempt outcome,
manifest-to-attempt CLI behavior, and a real subprocess protocol round trip.
Architecture tests keep process and contract adapters out of execution policy.
