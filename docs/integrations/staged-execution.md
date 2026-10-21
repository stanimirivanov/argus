# Shared staged-execution invariants

## TL;DR

- M08-PR9 supplies the common domain boundary consumed by the functional API and browser staged protocols in PR10/PR11.
- Build a schedule projection from a verified portfolio episode plus independent manifest, candidate inventory and reviewed execution bindings.
- Validate a plan against that source and the caller's expected mode before requesting exact stage test lists.
- Existing v1 selected/full-suite runners retain their existing authority and validation rules.

## Source and correlation boundary

The `internal/execution/scheduledplan` package accepts a `Schedule` containing the portfolio reference, episode ID, manifest SHA-256 and complete group selections. The reference identifies `argus.dev/predictive-portfolio/v1` bytes. Its shape validator does not load or authenticate an artifact: the future transport adapters must compute and verify SHA-256 over the complete bounded artifact, select its exact episode, and match the manifest and independent candidate inventory before constructing this input.

Each binding pins repository identity/display coordinates, immutable test revision, family, adapter, project and environment. Functional API groups have an empty project; browser groups require an explicit project. Both require a reviewed environment. Group keys and repository/family/adapter/project/environment scopes are unique; aliases cannot introduce a second revision for the same scope. Candidate IDs are globally unique and stable suite/test identities are unique inside their group. Counts use the existing execution candidate bound. Each candidate carries an explicit original RUN_REQUIRED or SKIP_FOR_NOW decision; missing or unknown decisions are invalid.

`Build` preserves the source's relevant-now order, rejects missing hard-required tests, and derives the exact remaining-later complement. Groups, candidate inventories and complements have deterministic ordering. The candidate inventory must include every test independently of selection: reconstructing it from the plan being checked would hide omissions.

`Validate(plan, source, expectedMode)` checks the reference, episode, manifest, mode, all bindings, complete inventories and exact memberships/order. A self-declared mode cannot downgrade a caller's required shadow control. `TestsForStage` performs that same correlation before returning an owned test list. `Canonical` and plan construction detach slices from caller-owned inputs.

## Required passes

| Mode | Passes | Membership |
|:--|:--|:--|
| `staged` | `relevant-now`, then `remaining-later` | Disjoint sets whose union is the complete candidate inventory in each group |
| `shadow` | `relevant-now`, then independent `shadow-control` | The control contains every candidate, including early tests |

Shadow plans still retain the exact remaining-later complement for omission/evaluation evidence; it is covered by the full control rather than dispatched separately. Requesting remaining-later in shadow mode or shadow-control in staged mode is rejected. Empty early or later memberships are valid; subsequent runners must skip empty invocations while retaining the membership evidence. A full-control fallback can place every candidate in relevant-now and leave the later set empty.

The mode argument defines semantics only. Production adoption, policy approval, execution ordering between jobs, retries, normalized attempts and persistence belong to subsequent slices. A probabilistic early subset alone grants no release authority.

## Compatibility and verification

The new vocabulary is `relevant-now`, `remaining-later` and `shadow-control`. It is not an alias for v1 `selected` or `full-suite`. Existing v1 attempts and API/browser plans reject all three values; each runner needs its new opt-in protocol before consuming a staged plan.

Run `go test -vet=off -count=1 ./internal/execution/...`, `make architecture-check` and `make validate`. Regression tests cover exact partitions, hard requirements, aliases/duplicates/omissions, order and ownership, digest/episode/manifest mismatches, repository/revision/project/environment isolation, mode downgrade and v1 compatibility.

See [ADR-0047](../decisions/0047-define-shared-staged-execution-invariants.md). The foundation adds no stored data or dispatch path; reverting it is sufficient rollback before runner adoption.
