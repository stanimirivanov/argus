# ADR-0099: Authorize protected performance evidence export

- Status: Proposed
- Date: 2026-11-04
- Milestone: M09 - Perfeng integration
- Deciders: Argus maintainers
- Supersedes: None
- Superseded by: None

## TL;DR

- Authorize exact scope/child and immutable pins from independently issued host policy.
- Require a dedicated restricted login and one bounded read-only snapshot.
- Emit the closed public allowlist only after all retained evidence validates.

## Decision and consequences

Joint PR11 delivers snapshot, encoding and protected-command slices in that order.
The host authenticates issuance of private canonical approval configuration outside
subject input control. It binds scope/position, intent/request pins and the expected
analysis reviewer. Arguments and body claims grant no access. Linux no-follow
opens verify protected ancestors, regular/private/single-link files and bounded reads.

The database connection uses an explicit dedicated setting. Administrative,
member, owner, schema-create and catalog table/column-write authority is refused;
only the five retained evidence tables require SELECT. Read-only session and
repeatable-read transaction settings reinforce, rather than replace, those grants.
No grants, migrations, external lookups or repairs occur during export.

A consistent snapshot revalidates original intent, admission, lifecycle and report
history. Conversion preserves original pins and independent outcomes, including
empty history, late evidence and conflicts, and enforces the complete public byte
budget. Failure returns no public document. Destination I/O may still be partial:
protected CI consumers must discard output on nonzero exit and publish only after
success. The [contract guide](../integrations/performance-decision-projection.md)
defines disclosure and output bounds; the dependent command supplies provisioning
and recovery guidance. Revert command before foundations;
retain all data. This is no public HTTP service or execution/approval authority.
