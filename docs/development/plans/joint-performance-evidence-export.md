# Joint performance evidence export

## TL;DR

- Joint M09-PR11 exports the already published projection contract for an authorized child.
- Merge snapshot, public conversion and protected command in that order.
- No slice submits work, changes storage or publishes to a network service.

## Review slices and invariants

1. **Consistent snapshot:** read immutable intent/child, authoritative admission,
   verified lifecycle and compact analysis in one bounded read-only repeatable-read
   transaction. Every failure returns zero evidence. Existing reads keep their
   behavior; private query helpers allow the same checks inside this snapshot.
2. **Public conversion:** bind independently approved intent/request pins and
   original provenance; emit the closed public family with a complete 8 MiB bound.
   Retain independent outcomes, late publications and report conflicts. This and
   the snapshot are foundations for the CLI and real joint acceptance verifier.
3. **Protected command:** authorize requested scope/position before opening a
   dedicated restricted database connection. Load independently issued protected
   host policy, perform no writes and emit only a validated complete document.

The contract is joint PR10; the real flow remains joint PR8 after all three
slices merge. Policy issuance and database grants belong to the trusted host.
Database identity checks cannot authenticate policy authorship. No input claims,
positions or hashes grant authority by themselves. No database migrations or
new dependencies are required. Revert the command before its foundations; retain
all existing evidence. Each slice ships focused tests, and database boundaries
require disposable PostgreSQL acceptance alongside repository validation.
