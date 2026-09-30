package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCheckRepositoryAcceptsConsistentPolicy(t *testing.T) {
	t.Parallel()

	root := validRepository(t)
	violations, err := checkRepository(root)
	if err != nil {
		t.Fatalf("check repository: %v", err)
	}
	if len(violations) != 0 {
		t.Fatalf("expected no violations, got:\n%s", formatViolations(violations))
	}
}

func TestCheckRepositoryReportsBrokenInternalLinksAndAnchors(t *testing.T) {
	t.Parallel()

	root := validRepository(t)
	writeFixture(t, root, "docs/guide.md", "# Guide\n\n[missing](missing.md)\n[anchor](../CONTRIBUTING.md#not-a-heading)\n")

	violations := mustCheck(t, root)
	assertRuleCount(t, violations, "docs.internal-link", 2)
	assertViolationContains(t, violations, "docs.internal-link", "does not resolve")
	assertViolationContains(t, violations, "docs.internal-link", "heading anchor")
}

func TestCheckRepositoryAcceptsReferenceDefinitionWithAngleBracketsAndTitle(t *testing.T) {
	t.Parallel()

	root := validRepository(t)
	writeFixture(t, root, "docs/guide.md", "# Guide\n\n## Target section\n")
	writeFixture(t, root, "docs/index.md", "# Index\n\n[guide]: <guide.md#target-section> \"Guide title\"\n")

	violations := mustCheck(t, root)
	if len(violations) != 0 {
		t.Fatalf("expected valid reference definition, got:\n%s", formatViolations(violations))
	}
}

func TestCheckRepositoryAcceptsRootRelativeMarkdownLinks(t *testing.T) {
	t.Parallel()

	root := validRepository(t)
	writeFixture(t, root, "docs/guide.md", "# Guide\n\n## Target section\n")
	writeFixture(t, root, "docs/index.md", "# Index\n\n[guide](/docs/guide.md#target-section)\n")

	violations := mustCheck(t, root)
	if len(violations) != 0 {
		t.Fatalf("expected valid root-relative link, got:\n%s", formatViolations(violations))
	}
}

func TestCheckRepositoryReportsBrokenReferenceDefinitionsOutsideCodeFences(t *testing.T) {
	t.Parallel()

	root := validRepository(t)
	writeFixture(t, root, "docs/guide.md", "# Guide\n\n## Existing section\n")
	writeFixture(t, root, "docs/index.md", `# Index

[missing]: missing.md "Missing document"
[anchor]: guide.md#absent-section 'Missing section'

~~~markdown
[example-only]: absent-example.md
~~~
`)

	violations := mustCheck(t, root)
	assertRuleCount(t, violations, "docs.internal-link", 2)
	assertViolationContains(t, violations, "docs.internal-link", "missing.md")
	assertViolationContains(t, violations, "docs.internal-link", "absent-section")
}

func TestCheckRepositoryRequiresTLDRForLongDocuments(t *testing.T) {
	t.Parallel()

	root := validRepository(t)
	writeFixture(t, root, "docs/long.md", "# Long guide\n\n## One\n\n## Two\n\n## Three\n\n## Four\n\n## Five\n\n## Six\n")

	violations := mustCheck(t, root)
	assertRuleCount(t, violations, "docs.tldr", 1)
	assertViolationContains(t, violations, "docs.tldr", "first second-level section")
}

func TestCheckRepositoryReportsADRIndexAndMetadataDrift(t *testing.T) {
	t.Parallel()

	root := validRepository(t)
	adr := strings.ReplaceAll(validADR, "- Status: Accepted", "- Status: Unknown")
	adr = strings.ReplaceAll(adr, "# ADR-0001", "# ADR-0002")
	writeFixture(t, root, "docs/decisions/0001-use-a-policy.md", adr)

	violations := mustCheck(t, root)
	assertViolationContains(t, violations, "adr.number", "does not match filename")
	assertViolationContains(t, violations, "adr.status", "not a supported")
	assertViolationContains(t, violations, "adr.index", "does not match ADR metadata")
}

func TestCheckRepositoryReportsIssueAndPullRequestTemplateDrift(t *testing.T) {
	t.Parallel()

	root := validRepository(t)
	issue := strings.Replace(validIssueTemplate, "## Scope", "## Included", 1)
	writeFixture(t, root, ".github/ISSUE_TEMPLATE/work-item.md", issue)
	pullRequest := strings.Replace(validPullRequestTemplate, "- Issue:\n", "", 1)
	writeFixture(t, root, ".github/PULL_REQUEST_TEMPLATE.md", pullRequest)

	violations := mustCheck(t, root)
	assertRuleCount(t, violations, "template.issue", 1)
	assertViolationContains(t, violations, "template.pull-request", "Issue:")
}

func validRepository(t *testing.T) string {
	t.Helper()

	root := t.TempDir()
	writeFixture(t, root, "CONTRIBUTING.md", validContributing)
	writeFixture(t, root, ".github/ISSUE_TEMPLATE/work-item.md", validIssueTemplate)
	writeFixture(t, root, ".github/PULL_REQUEST_TEMPLATE.md", validPullRequestTemplate)
	writeFixture(t, root, "docs/decisions/README.md", validADRIndex)
	writeFixture(t, root, "docs/decisions/0001-use-a-policy.md", validADR)

	return root
}

func writeFixture(t *testing.T, root, relativePath, content string) {
	t.Helper()

	path := filepath.Join(root, filepath.FromSlash(relativePath))
	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		t.Fatalf("create fixture directory: %v", err)
	}
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("write fixture %s: %v", relativePath, err)
	}
}

func mustCheck(t *testing.T, root string) []violation {
	t.Helper()

	violations, err := checkRepository(root)
	if err != nil {
		t.Fatalf("check repository: %v", err)
	}

	return violations
}

func assertRuleCount(t *testing.T, violations []violation, rule string, expected int) {
	t.Helper()

	actual := 0
	for _, violation := range violations {
		if violation.rule == rule {
			actual++
		}
	}
	if actual != expected {
		t.Fatalf("expected %d %s violations, got %d:\n%s", expected, rule, actual, formatViolations(violations))
	}
}

func assertViolationContains(t *testing.T, violations []violation, rule, fragment string) {
	t.Helper()

	for _, violation := range violations {
		if violation.rule == rule && strings.Contains(violation.String(), fragment) {
			return
		}
	}
	t.Fatalf("expected %s violation containing %q, got:\n%s", rule, fragment, formatViolations(violations))
}

func formatViolations(violations []violation) string {
	lines := make([]string, 0, len(violations))
	for _, violation := range violations {
		lines = append(lines, violation.String())
	}

	return strings.Join(lines, "\n")
}

const validContributing = `# Contributing

## TL;DR

- Follow the policy.

## Required issue structure

~~~markdown
**Milestone:** MNN - Outcome

## Goal

Describe the result.

## Scope

- Included behavior.

## Design decisions

- Important decisions.

## Acceptance criteria

- [ ] Observable behavior.
- [ ] Failure behavior.
- [ ] Documentation.

## Out of scope

- Deferred work.
~~~

## Pull request description

- Required evidence.
`

const validIssueTemplate = `---
name: Work item
---

**Milestone:** MNN - Outcome

## Goal

Describe the result.

## Scope

- Included behavior.

## Design decisions

- Important decisions.

## Acceptance criteria

- [ ] Observable behavior.
- [ ] Failure behavior.
- [ ] Documentation.

## Out of scope

- Deferred work.
`

const validPullRequestTemplate = `## Linked work

- Issue:
- Milestone:

## Outcome and scope

Problem and resulting behavior:

Boundaries and deliberate exclusions:

Assumptions and unresolved questions:

## Design, compatibility, and operations

- ADRs:
- Contract/API/schema effects:
- Security/privacy effects:
- Rollout and recovery:

## Verification

Commands and evidence:

Checks not run, blocking conditions, and residual risk:

## Limitations and review

Known limitations and follow-up:
`

const validADRIndex = `# Architecture decision records

## TL;DR

- Decisions are indexed.

## Naming and lifecycle

Follow the lifecycle.

## Index

| ADR | Decision | Status | Date |
|:--|:--|:--|:--|
| [ADR-0001](0001-use-a-policy.md) | Use a policy | Accepted | 2026-09-29 |
`

const validADR = `# ADR-0001: Use a policy

- Status: Accepted
- Date: 2026-09-29
- Milestone: M01 - Foundation
- Deciders: Maintainers
- Supersedes: None
- Superseded by: None

## TL;DR

- Use the policy.

## Context

The repository needs a policy.
`
