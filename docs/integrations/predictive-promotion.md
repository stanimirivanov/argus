# Owner-reviewed staged promotion

## TL;DR

- `predictive-promotion` rederives replay and prospective shadow evidence before
  deciding whether one exact portfolio can use staged mode.
- Every repository/family requires explicit owner thresholds and artifact pins;
  no policy, no failing controls or insufficient evidence means shadow fallback.
- Re-evaluate for every planning invocation using trusted current time and all
  declared evidence. The gate has no background monitor or persisted approval.
- Both planners retain mandatory remaining/full execution. Promotion is not
  release authority, and M10 deployment/alerting remains separate.

## Reviewed policy

Supply `argus.dev/predictive-promotion-policy/v1` with these required fields:

| Fields | Meaning |
|:--|:--|
| `repository`, `family` | Immutable provider/host/repository ID and functional-api or functional-ui; display names do not grant scope. |
| `reviewId`, `reviewedBy`, `reviewedAt`, `expiresAt` | Authenticated owner review metadata and validity interval. |
| `scheduleSha256` | Exact bytes produced by predictive-portfolio, including its trailing newline. Regeneration uses the original dataset, constraints and budget. |
| `modelSha256` | Canonical JSON hash of the calibration report's Go model DTO, the same identity retained by the safety report. |
| `calibrationSha256` | Exact predictive-calibration output bytes, including the trailing newline. CI must retain and authenticate that artifact; reserialized copies do not transfer authority. |
| `maxEvidenceAgeSeconds` | Maximum age of the last completed historical control and prospective as-of snapshot, at most one year. |
| `validForSeconds` | Decision lifetime, at most one day, also capped by policy/evidence expiry. |
| `minReplayEpisodes`, `minShadowRuns`, `minFailures` | Positive scoped minimums. The failure minimum applies separately to replay and shadow; zero failures cannot prove recall. |
| `maxMissRate`, `maxEce`, `maxEceDrift` | Values in [0,1]. Drift uses absolute signed ECE difference, so either direction beyond the limit denies. |
| `maxStageDurationMs`, `maxStageResourceUnitMs` | Positive estimated stage duration and resource-unit-millisecond bounds. |

There is deliberately no checked-in production policy or default risk envelope.
Owners must review values for their own repositories and evidence. The contracts
under `contracts/source/predictive-promotion-policy-v1.ts` define the exact shape.

## Evidence and decision

~~~sh
go run ./cmd/predictive-promotion -index replay/index.json -constraints constraints.json -budget-ms 600000 -shadow-index shadow/index.json -policy owner-policy.json -as-of "$NOW" > promotion.json
~~~

`NOW` is the trusted CI timestamp used for this planning operation, in RFC3339
format. Use the [chronological archive](predictive-replay.md), exact original
[reviewed constraints](predictive-cost.md), and an explicit
[safety archive](predictive-safety.md). The gate rereads original observations,
plans, manifests/discovery, bindings, calibration and attempts; it never accepts
edited safety report scalars. Input limits and path confinement are unchanged.

Replay freezes the portfolio before reading held-out labels for evaluation.
Local cohort support and the existing abstention gate must permit a portfolio;
fallback/over-budget replay cannot establish predictive promotion. Scoped replay
metrics count distinct held-out episodes and use all matching test probabilities
for decile ECE. Costs conservatively bound the complete replay portfolio, including
other scopes in that portfolio; shared setup is never credited as measured savings.

Prospective evidence requires new independent shadow attempts after both the
replay holdout end and latest historical control completion. Historical control
IDs cannot be reused as either shadow phase. Original candidate decisions may be
re-executed prospectively, but their old labels are not new live results. Every
matching group, including pending/incomplete ones, participates: a good group
cannot conceal another matching group's missing evidence. Counts are distinct
run IDs, not duplicate project groups. Group ECE/drift and each nonempty phase's
estimated duration/resource work must satisfy policy; no averaging hides a breach.
The exact complete portfolio must match in replay and shadow.

The decision pins the raw policy, canonical replay (including observation
digests), raw shadow index and exact portfolio/model/calibration identities.
It records scope, evaluation/expiry, replay/shadow counts, empirical miss rates,
reason codes and `remainingRequired: true`. Insufficient evidence or a threshold
breach emits `mode: shadow`; malformed source evidence produces an error with no
output. A new evaluation has no memory of an earlier approval.

These rates describe observed exposure only. They are not confidence bounds,
independent-sample claims or a guarantee of future behavior. Stage cost is an
estimate, not monetary spend. Unsupported telemetry cannot strengthen authority.

## Planner handoff and fallback

Both `plan-functional-api` and `plan-browser` use the existing staged arguments
plus this handoff when requesting `-mode staged`:

~~~sh
-promotion-policy owner-policy.json -promotion-decision promotion.json -promotion-sha256 "$TRUSTED_DECISION_SHA256" -promotion-as-of "$NOW"
~~~

Compute `TRUSTED_DECISION_SHA256` from the successful gate output inside the
trusted CI job. Authenticate the owner policy before invoking the gate; review
labels and SHA values supplied by an untrusted pull request are not authority.
Do not accept partial output after a failed command, reuse a cached decision or
let a pull request choose an old `NOW`. All new evidence must be included in the
current explicit archive; this file-based gate cannot discover omitted runs.

The planner checks the exact decision and policy bytes, identical evaluation
time, both expiries, model/calibration/schedule identities, empirical counts and
every planned repository/family. Missing configuration, stale/denied decisions,
digest mismatch or wrong scope produces a shadow plan. Unreadable or malformed
provided documents fail without producing a plan. The default remains shadow.
The runner's mode must follow the emitted plan, not the requested mode: after
fallback run `relevant-now` and `shadow-control`, not `remaining-later`.

**Compatibility change:** `-mode staged` alone now returns shadow. Existing v1
paths and v2 runner formats remain valid; trusted CI must continue enforcing exact
plans, provenance and required pass/capacity behavior. No policy can drop the
remaining-later complement or declare a partial result a release gate.

## Rollout, rollback and verification

Deploy the new gate and planners together. Keep shadow enabled until local owners
have sufficient measured evidence and reviewed policy. Removing the authority
flags, allowing policy expiry, or re-evaluating breached evidence restores shadow;
retain prior immutable inputs and decisions for audit. No migration is required.

Run `make generate-contracts`, `make predictive-validate` and `make validate`.
See [ADR-0051](../decisions/0051-gate-staged-promotion-by-owner-policy.md) for
the authority boundary and compatibility decision.
