# Capture predictive observations

## TL;DR

- Use the exact change set, execution manifest, and completed full-suite attempt from one CI decision.
- Run `predictive-evidence -change change.json -manifest manifest.json -control full-attempt.json`.
- Archive the emitted JSON alongside the three inputs under your CI retention and access policy.

The command emits one JSON envelope for one executable group. Functional API groups are keyed by test repository and adapter. Browser groups additionally retain project and environment. For a plan with several groups, invoke the command once per full-suite attempt using the same change and manifest files.

The envelope includes decision-time change and selection features, then the full-control test outcomes and durations as separate labels. It excludes raw patches, failure messages, and artifact references. `sha256` covers the canonical `observation` JSON object, without the envelope field. Repeated invocations with identical inputs emit identical bytes.

`featureCutoffAt` is the change observation time for functional API manifests and the explicit evaluation time for browser manifests. The functional API value is a conservative feature cutoff, not a claimed exact decision timestamp.

The command rejects missing or mismatched manifest references, group membership, revisions, future-dated decisions, skipped/error control results, and oversized inputs. It writes no partial observation when validation fails. An existing attempt by itself is insufficient: the original change and manifest must be retained. Browser manifest digest identity follows the runner's exact-byte rule; functional API identity follows its canonical contract rule.

This archive is for M08 offline evaluation. It does not change CI selection or release authority. Source CI owns input authenticity, access controls, and retention; changed paths and repository identities can still be sensitive even though raw patch text is omitted.
