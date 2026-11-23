# ADR-0095: Enumerate admitted analysis work

- Status: Proposed
- Date: 2026-11-01
- Milestone: M09 - Perfeng integration
- Deciders: Argus maintainers
- Supersedes: None
- Superseded by: None

## TL;DR

- Read one admitted child under exact scope and issuer with an exclusive cursor.
- Keep terminal, cancelled, deferred and previously analyzed work eligible.
- Reset traversal explicitly to discover admission committed behind the cursor.
- Enumeration is read-only and grants no claim or network authority.

## Decision and consequences

Return existing immutable pending evidence only when retained settlement is
accepted or lifecycle admission. Use provider/principal scope, both request and
attempt issuer, ascending position, LIMIT 1 and the existing operation deadline.
Invalid input fails before storage; failures return no partial work. End/missing
and wrong scope are not found, preserving the existing disclosure boundary.

Statements observe current committed state independently. A later admission ahead
of the cursor may be read next; one behind it requires a subsequent explicit reset
to zero. The caller owns fair rotation and retry policy. This is not a lease or
snapshot across calls: concurrent readers can obtain the same immutable work.

Late/conflicting evidence can follow terminal execution, cancellation cooldown
or a previous analysis publication. Do not filter on those states and do not
inherit NextLifecycle's cancellation scheduling predicate. Consumers independently
revalidate admission before network access and idempotent retention.

Reuse the existing scoped pending-position index and settlement primary key.
No migration, mutable scheduler, activation or new credential authority is added.
Rollback removes only the callable selector; original evidence remains intact.
PR9d3e owns runtime scheduling and rotation.

## Validation

Database tests cover exact scope/issuer, ordered frozen evidence, unsettled and
rejected exclusion, end/reset semantics, delayed admission, terminal/cancelled
work, cooldown independence and existing analysis. Invalid/cancelled inputs fail
without storage access. Focused database acceptance and query-plan inspection
precede any decision to add another index.
