# ADR-0076: Compose cancellation after admission recovery

- Status: Proposed
- Date: 2026-10-28
- Milestone: M09 - Perfeng integration
- Deciders: Argus maintainers
- Supersedes: None
- Superseded by: None

## TL;DR

- Default-off cancellation runs serially after admission recovery and before lifecycle.
- Keep independent cursors and durable per-request cooldown; cancel and join on shutdown.
- Valid retry hints pause all provider stages in this process, including unresolved recovery.

## Decision and consequences

PR10d2c adds explicit CANCELLATION_ENABLED and CANCELLATION_INTERVAL settings under
ARGUS_PERFORMANCE_DISPATCH_. Dispatch remains the outer opt-in. The cancellation
interval defaults to 5s and accepts 1s..1m whole seconds; disabled cancellation
ignores its interval. Lifecycle opt-in remains independent.

Each iteration attempts one fenced dispatch/recovery, one due cancellation and
optionally one lifecycle poll. Prior-claim cancellation recovers by GET at a higher
fence; the local first-send cutoff still blocks unclaimed cancelled work. Only
retained admission becomes selectable. Absence/timeouts create no execution right.

When cancellation is enabled, valid HTTP 429/503 RetryAfter hints from any stage
stop later stages and pause subsequent iterations using the trusted process clock.
This also covers failed durable hint extensions and unresolved admission. Cancel
errors skip same-iteration lifecycle work. Independent cursors preserve fairness;
no provider I/O runs concurrently in this worker. Log only fixed diagnostics.

No wire, migration or dependency changes. Apply migration 21 before activation.
Stop/join the worker before closing storage. Durable admitted cooldown survives
restart; process-wide pacing does not. Before restarting after unresolved rate
limits or failed hint extension, operators must honor the provider wait window.
Disable the flag and stop older pollers on rollback, retaining evidence/cooldown.
This is no fleet throttle or distributed cancellation lease; M10 owns those concerns.
