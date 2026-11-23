# ADR-0023: Separate GitHub review read and write authority

- Status: Proposed
- Date: 2026-10-02
- Milestone: M10 - Production readiness
- Deciders: Argus maintainers
- Supersedes: None
- Superseded by: None

## TL;DR

- Give immutable source reading, draft-PR publication, and terminal-outcome
  observation separate concrete adapters and credentials.
- Keep publication's guarded provider reads with its write credential because
  idempotent recovery depends on them.
- Prefer `ARGUS_GITHUB_READ_TOKEN` and `ARGUS_GITHUB_WRITE_TOKEN`; retain
  `ARGUS_GITHUB_TOKEN` as a compatibility fallback, not a least-privilege setup.
- Do not change review contracts, evidence meaning, branch naming, or retry
  behavior.

## Context

The review application port previously combined source reads and draft-PR
writes, and one GitHub adapter exposed source reading, mutation, and terminal
observation through the same credential. The command also accepted only one
token. That made a read-only outcome observer carry unnecessary write
authority and prevented a CI publication step from assigning independent
source-read and review-write secrets.

The publication adapter still needs provider reads to verify repository
identity, recover a deterministic branch, inspect the candidate file, and
recognize an exact retry. Those reads are part of the write workflow, not an
independent source-reading authority.

## Decision

The review application service consumes separate `SourceReader` and
`Publisher` ports. The GitHub adapter exposes separate concrete
`SourceReader`, `Publisher`, and `OutcomeObserver` types. Its shared HTTP
mechanics remain private and retain exact-origin redirect rejection, bounded
responses, immutable-revision checks, and existing error semantics. The
read-only concrete adapters expose no mutation method.

The publication command binds the source reader to
`ARGUS_GITHUB_READ_TOKEN` and the publisher to
`ARGUS_GITHUB_WRITE_TOKEN`. Outcome capture binds only the read token. Each
unset scoped token falls back to `ARGUS_GITHUB_TOKEN` so existing jobs keep
working during migration. A single fallback token does not provide separate
authority and should be replaced with independently scoped secrets.

## Alternatives considered

### Keep one all-purpose adapter and token

- Benefits: No configuration or code change.
- Costs and risks: Outcome observation retains write authority; the review
  service cannot express its two distinct provider needs.
- Reason not selected: It makes least-privilege deployment impossible without
  duplicating an all-purpose adapter elsewhere.

### Require distinct tokens immediately

- Benefits: Enforces separate secrets for every new publication invocation.
- Costs and risks: Breaks existing CI jobs before their secret distribution is
  updated, even though review behavior and contracts are otherwise unchanged.
- Reason not selected: A documented compatibility fallback permits a staged
  migration without preserving the all-purpose adapter in code.

## Consequences

### Positive

- Read-only review observation can run without a write credential.
- Source and publication credentials can be scoped, rotated, and audited
  independently.
- The application service expresses its read/write boundary directly.

### Negative

- Publication now constructs two provider clients and operators may manage two
  secrets instead of one.
- A deployment using the legacy fallback still has its former broad authority.

### Neutral or follow-up

- GitHub permission grants remain external configuration; narrow Go method
  sets do not themselves prove a token has least privilege.
- Removal of the legacy token fallback requires a separately announced
  operational compatibility change.

## Compatibility and migration

Public proposal, validation, review, and outcome documents are unchanged.
Successful publication and exact retry behavior are unchanged. Existing
`ARGUS_GITHUB_TOKEN` jobs keep working. To migrate, provision a read credential
for source and outcome operations and a write credential for publication,
then set both scoped environment variables and remove the legacy secret.

## Security and operations

The scoped read-token value is bound only to the source reader and observer;
the scoped write-token value is bound only to the publisher. The legacy
fallback may bind one secret to both roles. All clients keep the existing
exact-origin redirect policy and never print tokens. The publisher's
read-after-write calls use its write credential because they are necessary to
recover partial publication safely.

## Validation

Exercise different read and write tokens against independent test origins,
reject mutation attempts through read-only handlers, preserve exact-retry and
conflict tests, verify token-precedence rules, and run `make validate`.
