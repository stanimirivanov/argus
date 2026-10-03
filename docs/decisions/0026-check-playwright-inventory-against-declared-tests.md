# ADR-0026: Check Playwright inventory against declared tests

- Status: Proposed
- Date: 2026-10-03
- Milestone: M07 - UI test intelligence
- Deciders: Argus maintainers
- Supersedes: None
- Superseded by: None

## TL;DR

- Use Playwright's built-in JSON reporter in `--list` mode for local inventory.
- Require a declaration-time `argus.test-key` annotation; generated Playwright
  IDs, names, and source lines are not stable Argus catalog identities.
- Compare every observed key with one declared `functional-ui`/`playwright`
  suite, allowing one key in multiple projects but no duplicate per project.
- Keep the checker local and non-authoritative for selection until immutable
  discovery provenance and lifecycle are designed.

## Context

The repository descriptor already declares stable suite-scoped test keys and
capability mappings. The first browser selector trusts those declarations but
cannot tell whether Playwright still collects the tests. Playwright exposes
projects, tags, and annotations through its JSON reporter. Its generated test
ID depends on test file, title, and project and is session-scoped, so it is not
a replacement for a repository-declared stable key.

## Decision

A local CI validator consumes the complete, unfiltered JSON report from
`playwright test --list --reporter=json` and one validated repository descriptor.
Each collected case must have exactly one `argus.test-key` annotation equal to
a declared test key. Optional `argus.owner` annotations and Playwright tags are
reported as observed metadata, not treated as capability or ownership policy.
The validator requires every declared key to appear at least once, rejects
unknown keys and duplicate key/project variants, and rejects discovery errors,
executed results, malformed paths, and bounded-input violations.

The command does not execute Playwright, persist an observation, mutate a
descriptor, or feed selection. A caller supplies the source revision needed to
validate the descriptor, but the command cannot authenticate the file's Git
revision or the test checkout. CI must pin and isolate both checkouts and use
an unfiltered list invocation. No browser-selection policy or manifest changes.

## Alternatives considered

### Use Playwright's generated test ID as the catalog key

- Benefits: No repository annotations.
- Costs and risks: Renaming a title or file, or adding a project, changes the
  runner-derived identity even when the test intent is unchanged.
- Reason not selected: Argus needs a deliberately stable suite-scoped key.

### Parse test source files

- Benefits: No Playwright list command.
- Costs and risks: Misses dynamic declarations, fixtures, config, and project
  expansion, and duplicates Playwright collection semantics.
- Reason not selected: Use the runner's discovery output as the observation.

### Persist discovery and immediately narrow UI selection

- Benefits: One step closer to automated browser execution.
- Costs and risks: A local JSON file is not revision-authenticated durable
  evidence; filtered lists could silently omit tests.
- Reason not selected: Conformance is useful now; selection authority requires
  a separate trust and completeness design.

## Consequences

### Positive

- CI can detect drift between declared browser tests and collected tests.
- Stable Argus keys survive ordinary title, file, and project changes.
- Projects, tags, and optional owners become inspectable without a new runtime
  dependency in Argus.

### Negative

- Test authors must add one stable annotation per test.
- The validator's local output is not reusable as trusted persisted evidence.

### Neutral or follow-up

- Future browser execution must define a separate adapter and evidence
  protocol; this checker alone grants no execution authority.
- A future complete, revision-verified discovery snapshot may join the catalog
  through a versioned contract and reviewed persistence semantics.

## Compatibility and migration

Repository-descriptor v1 and execution-manifest v2 do not change. Existing
Playwright suites opt into the checker by adding annotations matching their
declared keys. No migration or database change is required.

## Security and operations

Playwright test collection evaluates repository code even though browser tests
are not executed. Run it only in an isolated, controlled checkout with the
appropriate CI authority. The checker bounds both input files, ignores
unneeded vendor fields, rejects control characters in displayed metadata, and
prints no report until all keys reconcile. The supplied source revision is a
label from the caller, not verification of the local file.

## Validation

Go tests cover valid multi-project inventory, stable annotations, tags and
owners, missing/unknown/duplicate keys, executed results, discovery errors,
unsafe paths, and command-level descriptor reconciliation. Repository
architecture and validation checks enforce the new package boundary.
