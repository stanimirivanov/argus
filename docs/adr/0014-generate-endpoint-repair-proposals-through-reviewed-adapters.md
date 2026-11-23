# ADR-0014: Generate endpoint repair proposals through reviewed adapters

**Status:** Accepted  
**Date:** 2026-09-26

## TL;DR

Argus may propose a functional API endpoint-path edit only when complete
OpenAPI impact contains exactly one removed/added operation pair with the same
HTTP method, non-empty `operationId`, and capability mapping. A reviewed,
repository-owned adapter locates the framework-specific request target. Argus
accepts only one byte-addressed old-path-to-new-path replacement and emits an
immutable `PATCH_AND_VALIDATE` proposal. This stage never edits a checkout and
never treats the candidate as correct merely because the mapping exists.

## Context

M05 can select and execute functional API tests, but Argus cannot safely infer
where arbitrary Playwright, pytest, or other framework source stores a request
target. Text search in the control plane could rewrite assertions, fixtures,
comments, or unrelated URLs. Conversely, delegating the entire repair decision
to a framework adapter would hide the evidence and policy that make an
adaptation reviewable.

The first M06 slice needs a useful narrow repair path while retaining two
separate responsibilities:

- Argus owns whether source change evidence is strong enough to consider a
  repair and which semantic change is allowed.
- Repository-owned framework code owns how a stable catalog test maps to its
  source and which syntax node represents the request target.

## Decision

### Require unique, complete contract evidence

The endpoint-rename policy consumes a validated capability impact and a
cataloged test at an immutable test revision. It proceeds only when:

- impact status is `complete`;
- one removed and one added operation occur in the same changed document;
- both operations have the same HTTP method and non-empty `operationId`;
- both operations have the same non-empty explicit capability mapping;
- that mapping intersects the candidate test's capabilities; and
- exactly one pair satisfies all conditions.

Partial, missing, differently mapped, or ambiguous evidence produces no
proposal. The retained `operationId` proves contract-declared identity, not
behavioral equivalence. Later validation must still reproduce the original
failure and test the candidate against negative controls.

### Keep source discovery at a reviewed adapter boundary

The CLI sends one versioned request to an explicitly configured process. The
command and arguments come from reviewed CI configuration, are passed directly
to the operating system without a shell, and are not taken from change or
manifest content. The adapter receives the immutable test identity, revision,
change provenance, and allowed endpoint rename.

The adapter may return one candidate edit or explicitly abstain. Candidate
output names one repository-relative file, its pre-edit SHA-256, byte offsets,
the original and replacement strings, and semantic role `request-target`.
Output is bounded and schema-validated.

### Re-enforce policy after adapter output

Adapter output remains untrusted protocol input. Argus correlates proposal and
adapter identities and accepts only an edit whose original is the old endpoint,
replacement is the new endpoint, and semantic role is `request-target`.
Assertion or expected-value changes, extra edits, absolute/traversing paths,
malformed spans, and stale or uncorrelated results are rejected.

The emitted `AdaptationProposal` is classified `INVALIDATED` with decision
`PATCH_AND_VALIDATE`. It is evidence for the next stage, not authorization to
write, commit, open a pull request, merge, or deploy.

## Consequences

- The first adaptation path is deterministic and auditable across frameworks.
- Framework knowledge stays in repository-owned adapters rather than leaking
  into the control-plane domain.
- No useful proposal is possible until a test repository supplies a conforming
  adapter that can prove a unique request-target occurrence.
- Endpoint moves combined with an operation identity change or ambiguous
  mappings deliberately abstain.
- This slice does not yet apply edits, run original/candidate/negative-control
  validation, persist evidence, or create review pull requests.

## Rejected alternatives

### Search and replace endpoint strings in the control plane

Rejected because an identical literal can appear in assertions, fixtures,
unrelated tests, comments, and multiple request sites. Language-agnostic text
search cannot preserve intent.

### Put all rename inference in the framework adapter

Rejected because adapters would implement divergent evidence thresholds and
the public proposal would not explain why an edit was permitted.

### Mutate the checkout during proposal generation

Rejected because proposal generation lacks failure reproduction, candidate
execution, negative controls, and reviewed branch policy. Mutation belongs to
the later validation workflow.
