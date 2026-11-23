# Argus domain language

Terms for change intelligence, test decisions, and reviewed test maintenance.

## Language

### Change and impact

**Change Set**
An immutable comparison of verified base and head revisions for a repository, with normalized change evidence.

**Repository Descriptor**
A versioned declaration of a repository's components, capabilities, test suites, and stable test identities.

**Capability**
A named behavior or contract surface that changes can affect and tests can exercise.

**Impact Evidence**
A producer-attributed observation about a change and its affected capability or coverage relationship. Provenance, confidence, conflicts, and expiry remain visible.

**Coverage Gap**
An affected capability for which the applicable catalog has no mapped test.

### Selection and execution

**Test Impact Analysis**
Determination of which capabilities and tests a change may affect.

**Execution Decision**
A policy-bound choice about which tests run, when they run, and why they are included or omitted.

**Execution Manifest**
A versioned record of the selection decision, its reasons, and its pinned inputs.

**Execution Plan**
The reviewed execution bindings and stages for a manifest, including which tests an attempt is expected to run.

**Execution Attempt**
One observed execution of the selected plan or stage. A retry has its own attempt identity and retains the prior outcome.

**Early-Stage Omission**
A test left out of the early stage under policy. A later control can still require that test.

**Missed Failure**
A failure in a comparable control run that the earlier selection failed to reveal.

**Predictive Observation**
An immutable pairing of decision-time features with separately recorded later outcomes for replay or evaluation.

**Shadow Evaluation**
A comparison of candidate selection against a compatible control using the original decision inputs. Promotion requires a separate reviewed policy decision.

### Adaptation and evidence handoff

**Adaptation Proposal**
An unapplied candidate test repair tied to original evidence, intended behavior, and its allowed change class.

**Validation Evidence**
Observations from controlled checks of the original and proposed test behavior, including relevant discriminating cases.

**Review Outcome**
The attributable human disposition of a proposal and any edits made during review.

**Change Evidence Artifact**
The planned, versioned Argus publication for one verified Change Set, including its scoped impact, selection reasons, and exact attempt outcomes.

**Performance Decision Projection**
The authorized Argus view of one admitted performance request and its retained evidence. Perfeng owns the meaning of the reported quality, SLO, and regression outcomes.
