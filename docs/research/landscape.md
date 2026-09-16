# Test intelligence research and market landscape

**Status:** Non-normative evidence base
**Research snapshot:** 15 September 2026

## TL;DR

- Deterministic, dynamic, and predictive selection methods are complementary;
  none is a sufficient universal solution.
- Complex models must beat interpretable baselines on chronological local data
  before they influence enforcement.
- Commercial products validate demand for predictive selection and self-healing
  but commonly separate those capabilities or constrain them by ecosystem.
- Argus differentiates through cross-repository impact, one decision surface
  for run and repair, evidence-backed adaptation, abstention, and portable
  customer-controlled operation.
- Product and vendor claims change. Refresh this snapshot before build-versus-
  buy, procurement, licensing, or competitive decisions.

## Purpose and use

This document preserves the evidence and competitive reasoning behind the
[product definition](../product/product-definition.md). It is not a product
contract and does not override ADRs or milestones. Claims are a dated snapshot;
contributors MUST verify current vendor scope, licensing, and product behavior
before relying on them for a decision.

## Research findings

Regression test selection is established but not solved. A
[systematic literature review](https://doi.org/10.1145/3057269) found substantial
variation in techniques, cost models, coverage criteria, and evaluation
quality. Google's study of
[transition-based test selection](https://research.google/pubs/assessing-transition-based-test-selection-algorithms-at-google/)
reported that simple recent-history heuristics underperformed expectations and
described selection as an open problem. These findings support benchmarking
interpretable baselines rather than assuming a complex model will win.

[Meta's published PTS work](https://arxiv.org/abs/1810.05286) demonstrated
material infrastructure savings with high retained failure detection in its
environment. Those results establish feasibility, not portable targets. Argus
must measure its own repositories, test families, costs, and failure patterns.

Static, dynamic, and predictive evidence have different failure modes:

| Evidence | Strength | Limitation |
|:--|:--|:--|
| Static dependencies and contracts | Available before execution and explainable | Reflection, generated behavior, configuration, and remote systems may be missed |
| Dynamic coverage and traces | Observe real runtime relationships | Unexecuted paths and rare conditions remain invisible |
| Historical outcomes | Rank beyond known dependency edges | Probabilistic, biased by past execution, and weak during cold start or architecture change |
| Explicit ownership/mappings | High-authority local intent | Can become stale and requires governance |
| Reviewer confirmation | Captures local semantic knowledge | Sparse and potentially biased or inconsistent |

Argus should combine these sources, retain contradictions, and use conservative
fallbacks. Evaluation needs chronological splits, full/remaining-suite controls,
per-family reporting, and total analysis/execution/review cost.

## Open-source reference points

| Project or capability | Demonstrated approach | Relevant lesson for Argus |
|:--|:--|:--|
| [STARTS](https://github.com/TestingResearchIllinois/starts) | Static class-level regression selection for Maven/Java | Useful deterministic reference; too ecosystem-specific for the platform core |
| Ekstazi research family | Dynamic test dependencies on code and resources | Motivates instrumentation adapters and stale-edge handling |
| [Healenium](https://github.com/healenium/healenium) | Runtime locator healing for Selenium/Appium with reports | Repository patches, oracle protection, and negative controls are needed beyond runtime substitution |
| [Schemathesis](https://github.com/schemathesis/schemathesis) | Schema-driven API generation and workflows | Potential engine for uncovered API behavior and negative cases |
| [EvoMaster](https://github.com/WebFuzzing/EvoMaster/blob/master/docs/blackbox.md) | Search/fuzzing-based REST and GraphQL generation | Potential later generation engine behind Argus policy |
| [Pact](https://docs.pact.io/) | Executable consumer-driven contracts | Strong source of consumer-provider impact edges and intent |
| [Playwright locator guidance](https://playwright.dev/docs/locators) | User-facing roles, names, and test IDs as resilient locators | Supports constrained UI candidate evidence, not blind healing |
| [k6 ecosystem](https://grafana.com/docs/k6/latest/testing-guides/api-load-testing/) | API load execution and generated starting points | Execution mechanics do not establish workload intent or a valid baseline; Perfeng owns this boundary |

These projects are candidates for integration or design reference, not selected
dependencies.

## Commercial reference points

The following offerings demonstrated relevant capabilities at the snapshot
date. The table records comparison dimensions, not verified procurement advice.

| Offering | Demonstrated capability | Argus opportunity or boundary |
|:--|:--|:--|
| [Develocity Predictive Test Selection](https://docs.gradle.com/develocity/predictive-test-selection/) | Learned selection, profiles, explanations, and remaining tests in JVM build ecosystems | Heterogeneous cross-repository suites plus maintenance decisions |
| [Launchable](https://help.launchableinc.com/features/predictive-test-selection/) | History-based ML selection and suite insights | Cross-layer impact evidence and validated maintenance |
| [Harness Test Intelligence](https://developer.harness.io/docs/continuous-integration/use-ci/run-tests/ti-overview/) | Change/call-graph selection integrated with CI | Initial Argus focus on expensive non-unit and cross-repository suites |
| [Microsoft Azure Pipelines TIA](https://learn.microsoft.com/en-us/azure/devops/pipelines/test/test-impact-analysis) | Impacted-test selection, fallback, and periodic full runs | Polyglot distributed systems and provider-independent policy |
| [Tricentis SeaLights](https://documentation.tricentis.com/sealights/en/content/sealights/imported_resources/best_practices_for_implementing_software_quality_intelligence_with_sealights-compressed.pdf) | TIA across multiple functional and manual test categories | Openness, customer-controlled deployment, run-and-repair coupling, and explicit evidence |
| [Katalon self-healing](https://docs.katalon.com/katalon-studio/maintain-tests/self-healing-tests-in-katalon-studio) | Locator healing using structural, accessibility, visual, and model signals | Adaptation beyond locators with repository patches and validation gates |
| [mabl auto-heal](https://help.mabl.com/hc/en-us/articles/19078583792404-How-auto-heal-works) | History/confidence-based browser healing and review | Assertion-sensitive policy, rollback, and delayed correctness evidence |
| [Grafana k6 Studio](https://grafana.com/docs/k6/latest/k6-studio/get-started/create-an-http-test/) | Recording, generation, correlation, validation, and execution workflows | Argus supplies cross-family impact decisions; Perfeng owns performance execution and analysis |

## Competitive thesis

Argus does not need day-one feature parity with every offering. Its credible
combination is:

1. one decision surface for execution and maintenance, including the state
   “impacted but stale”;
2. cross-layer, cross-repository relationships between source, contracts,
   capabilities, tests, fixtures, environments, and evidence;
3. observable validation—original failure, repaired success, negative control,
   invariant checks, and repetitions—before confidence claims;
4. uncertainty-aware abstention and broader-run fallback;
5. versioned policy and portable adapters that allow customer-controlled
   execution and evidence;
6. preservation of each test's authoritative basis, including contracts,
   requirements, SLOs, and human intent;
7. counterfactual or fault-seeded validation where a green run is insufficient;
8. portfolio scheduling under time, resource, device, and environment
   constraints rather than isolated binary classification;
9. learning from reviewer edits and structured reasons rather than merge alone;
   and
10. promotion of autonomy only inside a measured per-repository and per-family
    safety envelope.

Argus MAY complement an existing selection vendor by consuming its proposed
test set, applying local policy, and handling cross-repository maintenance. A
build-versus-integrate decision must compare total cost, data control,
explainability, coverage, lock-in, and operational burden.

## Research questions

The roadmap should create evidence for these questions:

1. Does a cross-layer graph improve failure recall over file distance, recent
   history, explicit mappings, or same-process coverage?
2. Can selection and stale-test detection share evidence without suppressing
   broken tests?
3. Which evidence predicts that a locator or API repair preserves intent?
4. Does fault seeding materially reduce false healing at acceptable cost?
5. How should scheduling balance failure probability, diversity, criticality,
   duration, and environment startup?
6. Can reviewer edits predict when Argus should abstain?
7. How quickly should dynamic relationships expire in distributed systems?
8. Can privacy-safe runtime summaries improve relevance without merely
   reproducing current behavior?

## Evaluation baselines

Any predictive or optimization work MUST compare against, as applicable:

- run all;
- explicit repository/capability mappings;
- static or coverage-based TIA;
- recent failure and flake history;
- file or dependency distance;
- duration-aware ordering; and
- random selection with the same budget.

Historical replay MUST precede prospective shadow mode. Results MUST report
misses, abstentions, cold-start behavior, drift, and cost—not only savings.

## Refresh policy and sources

Refresh this document before a build-versus-buy decision and at least when a
milestone depends on a named external capability. Record the review date and
material scope or licensing changes. Prefer primary research papers, official
documentation, and directly inspected source repositories.

Primary research and taxonomy sources from the proposal:

- [ISTQB Certified Tester Foundation Level Syllabus v4.0.1](https://istqb.org/wp-content/uploads/2024/11/ISTQB_CTFL_Syllabus_v4.0.1.pdf)
- [Predictive Test Selection](https://arxiv.org/abs/1810.05286)
- [Assessing Transition-based Test Selection Algorithms at Google](https://research.google/pubs/assessing-transition-based-test-selection-algorithms-at-google/)
- [Effective Regression Test Case Selection: A Systematic Literature Review](https://doi.org/10.1145/3057269)
- [An Extensive Study of Static Regression Test Selection](https://www.cs.cornell.edu/~legunsen/pubs/LegunsenETAL16StaticRTSStudy.pdf)

The official project and vendor links in the tables above form the dated
commercial/open-source source list. Their inclusion is not an endorsement.
