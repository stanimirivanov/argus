# M09-PR9 performance lifecycle delivery

## TL;DR

- Deliver ordered lifecycle and decision evidence in review-sized dependent slices.
- Reuse the pinned public contract and existing admission evidence without raw artifacts.
- Keep cancellation/reruns in PR10 and general operational recovery in M10.

| Slice | Observable scope | Depends on | Gate |
|:--|:--|:--|:--|
| PR9a | Owned lifecycle observations, pinned conversion and deterministic bounded history projection | PR8c2b | Published exchange, ordering, conflict, terminal and late-evidence tests; `make validate` |
| PR9b1 | Owned admission binding and append-only lifecycle schema guards | PR9a | Pinned admission conformance, concurrent schema guards, capacity, upgrade and immutability; `make validate`, `make db-validate` |
| PR9b2 | Atomic append and scoped history/projection reads, preserving authoritative admission/run binding | PR9b1 | Exact replay, rollback, corruption, concurrent append, reopen and scoped projection; `make validate`, `make db-validate` |
| PR9c | Authenticated bounded runtime consumption into durable history, including shutdown and uncertain persistence | PR9b2 | HTTP/storage composition, provider failure, lost acknowledgement and late delivery; `make validate`, `make db-validate` |
| PR9d1 | Strict offline published analysis/v1 artifact validation with independent outcomes | PR9b2 | Pinned examples, quality/SLO/regression distinctions, reference consistency, malformed and contradictory claims; `make validate` |
| PR9d2a | Pin and validate the published request-linked analysis envelope as owned offline claims | PR9d1, shared publication contract/provider support | Exact upstream bytes, closed family, native/context/time consistency, ownership and malformed/version tests; `make validate` |
| PR9d2b1 | Bind authenticated publication context/public Run to authoritative frozen request/admission and retain exact owned bytes/digest | PR9d2a | Wrong full scope/context/run, forged/rejected settlement, known acceptance timing, lifecycle recovery and ownership tests; `make validate` |
| PR9d2b2a | Pin/validate owned native policy claims for frozen-review loading and report conformance | PR9d2b1 | Published bytes, numeric precision, optional sections, ambiguous rules/bounds and malformed/version tests; `make validate` |
| PR9d2b2b | Load protected operator/CI frozen native review bundles with independently authenticated reviewer and exact policy bytes | PR9d2b2a, protected-bundle decision | Full request/admission/native/policy pins, provenance, bounded reads, cancellation, ownership and safe failure tests; `make validate` |
| PR9d2b2c | Verify native resource/policy/baseline and complete report rule/section associations | PR9d2b2b | Wrong native/resource/policy, exact policy bytes, missing rules, baseline and configured-section authority tests; `make validate` |
| PR9d2c | Fetch authorized publication references and bounded exact analysis bytes for verified correlation | PR9d2a, PR9d2b2c | Auth/origin, checksum/size, timeout/cancellation, provider errors and unsupported version tests; `make validate` |
| PR9d3 | Retain correlated analysis decision evidence and opt into bounded runtime consumption | PR9d2c | Replay/conflict, independent outcomes, scoped projection, rollback/reopen and end-to-end evidence; `make validate`, `make db-validate` |

PR9a is a shared foundation for both durable lifecycle consumption and decision
evidence. Its callable projection does not activate polling, receive events or
replace immutable admission settlement. Split later slices further if needed;
analysis requires its published compatible contract before consumption, never
invented fields or a COMPLETED-to-PASS shortcut. The worker remains admission-only
by default; PR9c explicitly opts in to authenticated bounded lifecycle polling.

All observations require independently authenticated provider/principal scope and
the entire original immutable context. Provider revisions order snapshots, not
arrival time. Stale delivery cannot regress state, but contradictory retained
evidence must remain visible as a conflict. Terminal state and run identity are
immutable. Late results remain valid; cancellation intent and elapsed deadlines
cannot fabricate termination. See [ADR-0066](../../decisions/0066-project-performance-lifecycle-by-provider-revision.md).

PR9a has no migration or activation; a revert removes the callable projection.
Later storage/runtime slices must define migration, concurrent admission binding,
retention, authorization and rollback before enabling consumption. Existing
cross-platform CI owns broad verification on the busy contributor host; report
focused local results separately from pending CI and do not weaken its gates.
