package main

import (
	"strings"
)

var requiredPullRequestHeadings = []string{
	"Linked work",
	"Outcome and scope",
	"Design, compatibility, and operations",
	"Verification",
	"Limitations and review",
}

var requiredPullRequestMarkers = []string{
	"Issue:",
	"Milestone:",
	"Problem and resulting behavior:",
	"Boundaries and deliberate exclusions:",
	"Assumptions and unresolved questions:",
	"ADRs:",
	"Contract/API/schema effects:",
	"Security/privacy effects:",
	"Rollout and recovery:",
	"Commands and evidence:",
	"Checks not run, blocking conditions, and residual risk:",
	"Known limitations and follow-up:",
}

func (r *repository) checkTemplates() []violation {
	var violations []violation
	violations = append(violations, r.checkIssueTemplate()...)
	violations = append(violations, r.checkPullRequestTemplate()...)

	return violations
}

func (r *repository) checkIssueTemplate() []violation {
	policy, hasPolicy := r.document("CONTRIBUTING.md")
	template, hasTemplate := r.document(".github/ISSUE_TEMPLATE/work-item.md")
	if !hasPolicy || !hasTemplate {
		path := "CONTRIBUTING.md"
		message := "canonical contribution policy is missing"
		fix := "restore CONTRIBUTING.md with the required issue structure"
		if hasPolicy {
			path = ".github/ISSUE_TEMPLATE/work-item.md"
			message = "implementation issue template is missing"
			fix = "restore the checked-in work-item template"
		}

		return []violation{{path: path, line: 1, rule: "template.issue", message: message, fix: fix, policy: issuePolicy}}
	}

	canonicalBody, found := fencedBlockAfterHeading(policy, "Required issue structure", "markdown")
	if !found {
		return []violation{{
			path:    policy.path,
			line:    1,
			rule:    "template.issue",
			message: "required issue structure has no fenced Markdown body",
			fix:     "restore the canonical `markdown` block under `## Required issue structure`",
			policy:  issuePolicy,
		}}
	}

	canonical := parseMarkdown("canonical-issue.md", canonicalBody)
	requiredHeadings := secondLevelNames(canonical)
	actualHeadings := secondLevelNames(template)
	var violations []violation
	if !equalStrings(requiredHeadings, actualHeadings) {
		violations = append(violations, violation{
			path:    template.path,
			line:    firstSecondLevelLine(template),
			rule:    "template.issue",
			message: "issue sections " + quote(strings.Join(actualHeadings, ", ")) + " do not match canonical sections " + quote(strings.Join(requiredHeadings, ", ")),
			fix:     "make the checked-in issue template use the canonical section names and order",
			policy:  issuePolicy,
		})
	}

	canonicalMilestone := countLinesWithPrefix(canonical.lines, "**Milestone:**")
	actualMilestone := countLinesWithPrefix(template.lines, "**Milestone:**")
	if canonicalMilestone != actualMilestone {
		violations = append(violations, violation{
			path:    template.path,
			line:    1,
			rule:    "template.issue",
			message: "issue template milestone field does not match the canonical issue body",
			fix:     "keep exactly one `**Milestone:** MNN - Outcome` field",
			policy:  issuePolicy,
		})
	}

	canonicalChecks := countCheckboxes(canonical.lines)
	actualChecks := countCheckboxes(template.lines)
	if canonicalChecks != actualChecks {
		violations = append(violations, violation{
			path:    template.path,
			line:    headingLine(template, "Acceptance criteria"),
			rule:    "template.issue",
			message: "issue template has " + decimal(actualChecks) + " acceptance checkboxes; canonical structure has " + decimal(canonicalChecks),
			fix:     "align the acceptance-criteria checklist with the canonical issue body",
			policy:  issuePolicy,
		})
	}

	return violations
}

func (r *repository) checkPullRequestTemplate() []violation {
	template, found := r.document(".github/PULL_REQUEST_TEMPLATE.md")
	if !found {
		return []violation{{
			path:    ".github/PULL_REQUEST_TEMPLATE.md",
			line:    1,
			rule:    "template.pull-request",
			message: "pull request template is missing",
			fix:     "restore the checked-in template covering every required pull request concern",
			policy:  pullRequestPolicy,
		}}
	}

	var violations []violation
	actualHeadings := secondLevelNames(template)
	if !equalStrings(requiredPullRequestHeadings, actualHeadings) {
		violations = append(violations, violation{
			path:    template.path,
			line:    firstSecondLevelLine(template),
			rule:    "template.pull-request",
			message: "pull request template section names or order have drifted from the required review structure",
			fix:     "restore the linked-work, outcome, design/operations, verification, and limitations sections",
			policy:  pullRequestPolicy,
		})
	}

	for _, marker := range requiredPullRequestMarkers {
		if lineContaining(template.lines, marker) > 0 {
			continue
		}
		violations = append(violations, violation{
			path:    template.path,
			line:    1,
			rule:    "template.pull-request",
			message: "pull request template is missing field " + quote(marker),
			fix:     "restore a prompt for this required pull request concern",
			policy:  pullRequestPolicy,
		})
	}

	return violations
}

func fencedBlockAfterHeading(document *markdownDocument, heading, language string) (string, bool) {
	start := headingLine(document, heading)
	if start == 0 {
		return "", false
	}

	opening := "~~~" + language
	inBlock := false
	var body strings.Builder
	for index := start; index < len(document.lines); index++ {
		line := document.lines[index]
		if !inBlock {
			if strings.TrimSpace(line) == opening {
				inBlock = true
			}
			continue
		}
		if strings.TrimSpace(line) == "~~~" {
			return body.String(), true
		}
		body.WriteString(line)
		body.WriteByte('\n')
	}

	return "", false
}

func secondLevelNames(document *markdownDocument) []string {
	names := make([]string, 0, len(document.secondLevels))
	for _, heading := range document.secondLevels {
		names = append(names, heading.text)
	}

	return names
}

func equalStrings(left, right []string) bool {
	if len(left) != len(right) {
		return false
	}
	for index := range left {
		if left[index] != right[index] {
			return false
		}
	}

	return true
}

func countLinesWithPrefix(lines []string, prefix string) int {
	count := 0
	for _, line := range lines {
		if strings.HasPrefix(strings.TrimSpace(line), prefix) {
			count++
		}
	}

	return count
}

func countCheckboxes(lines []string) int {
	count := 0
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "- [ ] ") || strings.HasPrefix(trimmed, "- [x] ") || strings.HasPrefix(trimmed, "- [X] ") {
			count++
		}
	}

	return count
}

func firstSecondLevelLine(document *markdownDocument) int {
	if len(document.secondLevels) == 0 {
		return 1
	}

	return document.secondLevels[0].line
}

func headingLine(document *markdownDocument, name string) int {
	for _, heading := range document.headings {
		if heading.text == name {
			return heading.line
		}
	}

	return 0
}

func lineContaining(lines []string, marker string) int {
	for index, line := range lines {
		if strings.Contains(line, marker) {
			return index + 1
		}
	}

	return 0
}
