# M01–M07 implementation completion plan

## TL;DR

I recommend **12 remaining PRs to complete the intended M01–M07 scope**, building on the existing implementation—not restarting those milestones.

The main gaps are broader change impact, configurable selection policy, a runnable reference API integration, field-rename adaptation, and the browser execution/repair lifecycle. Each PR below delivers usable behavior with its contracts, persistence where needed, tests, and documentation.

This plan follows the [canonical roadmap](../../roadmap/milestones.md) and does not infer formal GitHub milestone closure.

## **Current milestone assessment**

| Milestone | Existing foundation | Remaining scope |
| ----- | ----- | ----- |
| **M01 - Engineering foundation** | Harness, CI, architecture enforcement, dependency and security checks | Retain these gates; no new scaffolding PR |
| **M02 - Contracts and identity** | Effect-authored contracts, stable identities, strict conversion and fixtures | Evolve contracts alongside consuming features; correct the roadmap’s stale Effect v3 wording |
| **M03 - Repository catalog** | Immutable snapshots, persistence, queries, provenance, conflicts and expiry | Reuse this foundation; real design-partner acceptance still needs demonstrable evidence |
| **M04 - Change impact** | Trusted ingestion and semantic OpenAPI impact | Broader source/configuration/test-change coverage and composable impact evidence |
| **M05 - Functional API selection** | Selection, process protocol, execution persistence, planning and shadow reports | Configurable policy, mapping-evidence precedence, and a shipped reference adapter/integration |
| **M06 - Validated adaptation** | Endpoint proposals, three-phase validation, draft publication and review evidence | Broader classification, working reference transforms, and compatible field-renaming support |
| **M07 - UI test intelligence** | Local Playwright inventory checking and conservative UI manifests | Durable discovery, route mapping, execution, shadow evaluation and locator repair |

Two important distinctions: the current selector implements **direct impact plus fallback**, not the complete configurable policy described in M05; and browser manifests currently **cannot be executed by the functional API runner**.

## **Common implementation rules**

Every PR includes its own tests, documentation, compatibility treatment and operational notes. No separate “contracts first,” “tests later,” or documentation-only closeout PRs.

Every PR runs `make validate`; development uses `make fmt` and `make verify`. Additional gates appear below:

* `make db-validate` for persistence changes.  
* `make dbos-evaluate` when changing the durable ingestion/assessment path.  
* **Proposed:** `make api-validate`, introduced in PR 3 for the complete reference API workflow.  
* **Proposed:** `make ui-validate`, introduced in PR 8 for controlled browser integration tests.

New TypeScript workspaces must join the existing formatting, type-checking, test, dependency and license gates. Integration tests own their infrastructure and synthetic credentials.

No new public HTTP API, repository split, production DBOS adoption, predictive model, or Perfeng executor is required for this sequence.

## **Implementation sequence**

### **PR 1: Persist explainable impact beyond OpenAPI**

**Milestone:** M04 - Change impact  
**Dependencies:** Existing foundation.

* **Objective & Scope:** Add bounded impact analysis for source, configuration and test-file changes using explicit component mappings. Combine it with OpenAPI evidence while retaining unknown, unsupported and incomplete coverage as reasons for broader execution—not evidence of no impact.  
* **Target Layers / Files:** [Change capability](../../../internal/change), [PostgreSQL adapter](../../../internal/postgres), [contract workspace](../../../contracts), and affected ingestion/query composition. Include the execution plan and roadmap corrections in this substantive change.  
* **Contracts & Boundaries:** Version producer-specific evidence and completeness semantics. Preserve published OpenAPI evidence and old webhook identities; do not reinterpret existing stored assessments or make directory proximity sufficient proof.  
* **Harness Gate:** `make validate`, `make db-validate`, `make dbos-evaluate`; mixed-change, rename, unmapped-file, truncation, exact-retry, conflict and restart tests.

### **PR 2: Apply reviewed selection policy and mapping precedence**

**Milestone:** M05 - Functional API selection  
**Dependencies:** PR 1.

* **Objective & Scope:** Implement repository-scoped must-run rules, explicit pins, exclusions and conservative fallback, including deterministic treatment of conflicting or stale mapping evidence. Explain which rule and evidence determined every inclusion or omission.  
* **Target Layers / Files:** [Selection capability](../../../internal/selection), [catalog evidence queries](../../../internal/catalog/impact), and [contract workspace](../../../contracts).  
* **Contracts & Boundaries:** Bind decisions to immutable policy and evidence revisions. Introduce new contract versions where necessary; preserve existing manifests. Exclusions cannot silently defeat mandatory coverage, and missing head inventory must not hide new tests.  
* **Harness Gate:** `make validate`; table-driven precedence tests covering must-run/exclusion conflicts, changed and new tests, stale evidence, unavailable catalogs, deterministic output and mandatory full-suite obligations.

### **PR 3: Ship a runnable functional API reference integration**

**Milestone:** M05 - Functional API selection  
**Dependencies:** Existing execution protocol; integrate PR 2’s policy when merged.

* **Objective & Scope:** Ship one supported Playwright API-test adapter with stable discovery and execution—not just example commands referring to an absent adapter. Demonstrate source change → selection → selected/full execution → durable evidence → complete shadow report using separate source and test repositories.  
* **Target Layers / Files:** New reference adapter and two-repository fixtures; [execution capability](../../../internal/execution), [integration guides](../../../docs/integrations), root verification configuration and CI.  
* **Contracts & Boundaries:** Consume existing functional API protocols where sufficient. Discovery reports observed inventory against declared identities; it cannot silently create authoritative capability mappings. Reviewed CI configuration continues to choose executable commands and immutable revisions.  
* **Harness Gate:** `make validate`, `make db-validate`, and new `make api-validate`; real framework execution against a local fixture service, including a deliberately missed failure, full-only group, malformed output and provider-independent cleanup.

### **PR 4: Classify maintenance needs and deliver reference endpoint repair**

**Milestone:** M06 - Validated adaptation  
**Dependencies:** PR 3.

* **Objective & Scope:** Produce explicit valid, invalidated, uncovered or uncertain maintenance assessments and connect eligible endpoint renames to a working syntax-aware reference transform. Exercise the already implemented validation and review lifecycle rather than building a second one.  
* **Target Layers / Files:** [Adaptation capability](../../../internal/adaptation), the reference adapter, [contract workspace](../../../contracts), and API integration fixtures.  
* **Contracts & Boundaries:** Keep maintenance classification distinct from permission to patch. Preserve endpoint-repair contracts where possible; ambiguous mappings, unsupported syntax and protected-oracle edits produce abstention.  
* **Harness Gate:** `make validate`, `make api-validate`; actual original failure, exact repair success, negative-control failure, restoration, draft-publication retry and terminal-review correlation using a mock GitHub server.

### **PR 5: Generate authoritative compatible-field repair proposals**

**Milestone:** M06 - Validated adaptation  
**Dependencies:** PR 4.

* **Objective & Scope:** Add proposal generation for narrowly supported request-field and response-field renames. Require an explicit authoritative rename relationship and compatible schema semantics; matching a removed field with an added field is insufficient.  
* **Target Layers / Files:** [OpenAPI analysis](../../../internal/change/adapters/openapi), family-owned [adaptation policy](../../../internal/adaptation), reference transforms and contract fixtures.  
* **Contracts & Boundaries:** Version field identity, compatibility proof, edit role and proposal evidence. Permit only supported reference changes that preserve expected values and oracle intent; reject type, requiredness, flow, fixture-value and ambiguous changes.  
* **Harness Gate:** `make validate`, plus `make db-validate` if evidence persistence changes; positive/negative schema fixtures, syntax-aware edit tests, immutable preimage checks and unchanged endpoint-proposal compatibility.

This is independently useful as a **reviewable proposal capability**; it does not claim that an unvalidated candidate is safe to publish automatically.

### **PR 6: Validate and publish field repairs with discriminating evidence**

**Milestone:** M06 - Validated adaptation  
**Dependencies:** PR 5.

* **Objective & Scope:** Complete field repair through repeated candidate execution, class-appropriate negative controls, source restoration, draft publication and captured review outcomes. Extend the reference campaign to demonstrate accepted repairs, trustworthy rejections and false-healing detection.  
* **Target Layers / Files:** [Validation and review capabilities](../../../internal/adaptation), [adaptation persistence](../../../internal/postgres), reference adapter and API integration fixtures.  
* **Contracts & Boundaries:** Introduce validation policy/evidence versions for field repairs and repeated runs without changing the meaning of old endpoint evidence. Preserve human edits and rejection reasons; merge alone remains insufficient proof of correctness.  
* **Harness Gate:** `make validate`, `make db-validate`, `make api-validate`; request/response counterexamples, repeat disagreement, infrastructure failure, cancelled restoration, unchanged assertions and conflicting outcome retries.

### **PR 7: Persist browser inventory and explicit UI mappings**

**Milestone:** M07 - UI test intelligence  
**Dependencies:** PRs 1–3; can proceed alongside PRs 4–6.

* **Objective & Scope:** Turn the existing local Playwright inventory check into immutable, queryable discovery evidence. Add explicit route/component-to-test mapping and require trustworthy inventory and mapping coverage before targeted UI omissions.  
* **Target Layers / Files:** [Catalog discovery](../../../internal/catalog/discovery), [selection](../../../internal/selection), [PostgreSQL adapter](../../../internal/postgres), and browser contracts.  
* **Contracts & Boundaries:** Bind discovery to immutable test revision, collector version and project variants. Keep stable test identity separate from runner-generated IDs; observed routes and tags do not acquire policy authority automatically.  
* **Harness Gate:** `make validate`, `make db-validate`; duplicate variants, revision mismatch, stale inventory, contradictory routes, incomplete collection, retry/conflict and restart tests.

### **PR 8: Execute browser manifests and retain browser evidence**

**Milestone:** M07 - UI test intelligence  
**Dependencies:** PR 7.

* **Objective & Scope:** Add a browser-owned execution protocol and working Playwright adapter that executes the selected tests for an explicit project/environment. Persist normalized attempts and integrity-bound references to traces, screenshots and accessibility evidence.  
* **Target Layers / Files:** [Execution capability](../../../internal/execution), reference Playwright adapter, [PostgreSQL adapter](../../../internal/postgres), browser fixtures and CI.  
* **Contracts & Boundaries:** Define browser request/result and attempt compatibility explicitly; never disguise UI manifests as functional API v1. Preserve project/environment dimensions without changing stable test keys. Artifact registration is not proof that external bytes exist.  
* **Harness Gate:** `make validate`, `make db-validate`, and new `make ui-validate`; real browser execution, exact result-set correlation, project isolation, timeout/cancellation, missing artifacts, checksum errors and cleanup.

### **PR 9: Plan browser CI execution and measure selection misses**

**Milestone:** M07 - UI test intelligence  
**Dependencies:** PR 8.

* **Objective & Scope:** Produce reviewed selected/full-suite browser execution plans and complete shadow reports across repository/project/environment groups. Supply a working CI integration that preserves full-suite authority and retains failures even when early jobs fail.  
* **Target Layers / Files:** [Execution planning and shadow evaluation](../../../internal/execution), contract conversions, reference workflows and [integration guides](../../../docs/integrations).  
* **Contracts & Boundaries:** Reuse shared planning/reporting concepts only where semantics match. Version browser-specific bindings and comparisons; require explicit compatible control attempts rather than selecting the latest result.  
* **Harness Gate:** `make validate`, `make ui-validate`; full-only groups, missing controls, cross-project mismatches, reruns, known selection misses and workflow linting.

### **PR 10: Diagnose locator failures and generate constrained candidates**

**Milestone:** M07 - UI test intelligence  
**Dependencies:** PR 8.

* **Objective & Scope:** Separate supported locator failures from application, assertion, navigation and infrastructure failures. Generate a candidate only when fresh structural evidence identifies one equivalent target through role, accessible name, test ID or an explicit rename.  
* **Target Layers / Files:** Browser-owned policy within [adaptation](../../../internal/adaptation), the Playwright adapter, browser proposal contracts and controlled application fixtures.  
* **Contracts & Boundaries:** Define diagnosis and locator-proposal evidence, source preimages and permissible edit roles. Reject ambiguous targets, broadened selectors, arbitrary `first`/`nth` substitutions, cross-flow changes and assertion edits. No model dependency is required.  
* **Harness Gate:** `make validate`, `make ui-validate`; unique and ambiguous targets, hidden/duplicate elements, stale snapshots, actual product regressions, unsupported syntax and protected-oracle counterexamples.

### **PR 11: Validate locator repairs against flakiness and false healing**

**Milestone:** M07 - UI test intelligence  
**Dependencies:** PR 10.

* **Objective & Scope:** Validate locator candidates through reproduced original failure, repeated repaired success and a discriminating negative control. Retain successful and trustworthy rejected evidence while restoring the disposable source under cancellation and failure.  
* **Target Layers / Files:** [Adaptation validation](../../../internal/adaptation/validation), browser workspace/process adapters, [PostgreSQL adapter](../../../internal/postgres), and browser evidence contracts.  
* **Contracts & Boundaries:** Keep browser validation rules family-owned and reuse proven workspace mechanics. Record execution environment, repetition policy, source digests and structural/visual evidence; do not silently broaden the existing endpoint-validation contract.  
* **Harness Gate:** `make validate`, `make db-validate`, `make ui-validate`; flaky candidates, negative-control passes, product defects, adapter tampering, cancellation, restoration failure and immutable evidence retries.

### **PR 12: Publish evidence-backed UI repair reviews**

**Milestone:** M07 - UI test intelligence  
**Dependencies:** PRs 9 and 11.

* **Objective & Scope:** Complete the browser workflow by publishing idempotent draft repair PRs and recording final reviewer decisions and edits. Demonstrate selection, execution, locator diagnosis, validation and review together, including cases where Argus correctly refuses repair.  
* **Target Layers / Files:** [Review and outcome capabilities](../../../internal/adaptation), GitHub adapters, [PostgreSQL adapter](../../../internal/postgres), reference workflows and browser acceptance fixtures.  
* **Contracts & Boundaries:** Add browser-compatible review/outcome contracts and the smallest shared publication seam justified by this second consumer. Preserve read/write credential separation, exact validated patches, evidence access controls and human authority; no auto-merge or visual-baseline approval.  
* **Harness Gate:** `make validate`, `make db-validate`, `make api-validate`, `make ui-validate`; mocked GitHub publication/recovery, divergent branches, bounded reviewer edits, evidence redaction and the complete successful/refused UI repair scenarios.

## **Dependency and parallel-work summary**

| Track | Sequence | Parallel opportunity |
| ----- | ----- | ----- |
| Impact and policy | PR 1 → PR 2 | PR 3 can start from the existing execution foundation |
| Functional API adaptation | PR 3 → PR 4 → PR 5 → PR 6 | Can overlap browser work after shared adapter infrastructure settles |
| Browser selection and execution | PRs 1–3 → PR 7 → PR 8 → PR 9 | PR 9 and PR 10 can proceed concurrently after PR 8 |
| Browser repair | PR 8 → PR 10 → PR 11 | Joins PR 9 at PR 12 |

Numbering gives a recommended merge order; dependencies determine what can actually run concurrently. Coordinate changes to shared contracts and allocate migration/ADR numbers from refreshed state before merging.

Start with PR 1; after it merges, PR 2 and PR 3 may proceed in parallel.

