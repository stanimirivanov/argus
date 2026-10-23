# M09-PR5 performance selection delivery

## TL;DR

- The owner approved four dependent PRs after the complete prototype exceeded review size.
- Ship contracts first; then selector, evidence adapters and CLI.
- Stop after each linked issue/PR and wait for human approval and merge.

| Slice | Scope | Depends on | Required gate |
|:--|:--|:--|:--|
| M09-PR5a | Effect/JSON Schema and Go wire policy/context/recommendation contracts, conformance and semantic obligations | M09-PR4c | `make generate-contracts`, `make validate` |
| M09-PR5b | Deterministic selector with omissions, complete-impact checks, exact reviewed target/deployment bindings and conservative review | PR5a | Focused domain tests, `make validate` |
| M09-PR5c | Pinned stored-catalog and complete impact conversion, reviewed policy import and validated result/context export | PR5a, PR5b | Fixture/conversion/conformance tests, `make validate` |
| M09-PR5d | Opt-in operator inspection/selection CLI, scoped existing-store reads, cancellation/error output and command/architecture integration | PR5c | CLI/smoke tests, `make validate` |

The shared contracts and selector are consumed by multiple later adapters and M09
request/projection slices. No empty runtime stubs are introduced. Tests ship with
each owning behavior; subsequent work must retain the conservative semantic
obligations in [the contract guide](../../integrations/performance-selection.md).
The original PR5 outcome is complete only after all four slices merge. Predictive
ranking, dispatch, automatic load fallback, new persistence and provider internals
remain outside these slices. Refresh sources and run only affected focused checks
plus the required acceptance gate for each actual delivered tree.
