package browserlocator

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strings"

	"github.com/stanimirivanov/argus/internal/adaptation"
)

var positional = regexp.MustCompile(`^\s*\.\s*(first|nth)\s*\(`)

type locator struct {
	start int
	end   int
	text  string
	kind  string
	role  string
	value string
	quote string
}

// Diagnose emits a proposal only when the failed locator's prior target and
// the current unique visible target share a stable test ID or role/name pair.
func Diagnose(request Request) (Diagnosis, error) {
	if err := validRequest(request); err != nil {
		return Diagnosis{}, err
	}
	result := Diagnosis{APIVersion: DiagnosisVersion, BaselineID: request.Baseline.Request.AttemptID,
		FailedID: request.Failed.Request.AttemptID, SuiteKey: request.SuiteKey, TestKey: request.TestKey}
	baseline, err := parseSnapshot(request.BaselineData, request.Baseline)
	if err != nil {
		return Diagnosis{}, err
	}
	current, err := parseSnapshot(request.FailedData, request.Failed)
	if err != nil {
		return Diagnosis{}, err
	}
	failure := testResult(request.Failed, request.SuiteKey, request.TestKey).Failure
	if failure == nil || failure.Code != "locator-not-found" {
		result.ReasonCode = "non-locator-failure"

		return result, nil
	}
	if !baseline.Complete || !current.Complete {
		result.ReasonCode = "incomplete-structure"

		return result, nil
	}
	if baseline.URL != current.URL {
		result.ReasonCode = "cross-flow-navigation"

		return result, nil
	}
	old, reason := findSourceLocator(request.Source, failure.Message)
	if reason != "" {
		result.ReasonCode = reason

		return result, nil
	}
	if !inSelectedTest(request.Source, old.start, request.SuiteKey, request.TestKey) {
		result.ReasonCode = "cross-flow-source"

		return result, nil
	}
	if positional.Match(request.Source[old.end:]) {
		result.ReasonCode = "positional-locator"

		return result, nil
	}
	previous := matching(baseline.Nodes, old)
	if len(previous) != 1 || !previous[0].Visible || len(matching(current.Nodes, old)) != 0 {
		result.ReasonCode = "unproved-original-target"

		return result, nil
	}
	replacement, target, reason := equivalent(*previous[0], current.Nodes, old)
	if reason != "" {
		result.ReasonCode = reason

		return result, nil
	}
	proposal := Proposal{APIVersion: ProposalVersion, PolicyVersion: PolicyVersion,
		BaselineID: result.BaselineID, FailedID: result.FailedID,
		BaselineSHA256: digest(request.BaselineData), FailedSHA256: digest(request.FailedData),
		SuiteKey: result.SuiteKey, TestKey: result.TestKey, OriginalTarget: *previous[0], CurrentTarget: target,
		Edit: adaptation.TextEdit{Path: request.SourcePath, BeforeSHA256: digest(request.Source),
			StartByte: old.start, EndByte: old.end, Original: old.text,
			Replacement: replacement, SemanticRole: "browser-locator"}}
	data, err := json.Marshal(proposal)
	if err != nil {
		return Diagnosis{}, fmt.Errorf("encode browser proposal: %w", err)
	}
	proposal.ProposalID = digest(data)
	if err := ValidateProposal(proposal); err != nil {
		return Diagnosis{}, err
	}
	result.ReasonCode = "candidate"
	result.Proposal = &proposal

	return result, nil
}

func inSelectedTest(source []byte, offset int, suiteKey, testKey string) bool {
	prior := string(source[:offset])
	start := -1
	for next := 0; next < len(prior); {
		found := strings.Index(prior[next:], "test(")
		if found < 0 {
			break
		}
		position := next + found
		if codeOffset(source, position) && (position == 0 || (!isIdentifier(source[position-1]) && source[position-1] != '.')) {
			start = position
		}
		next = position + len("test(")
	}
	if start < 0 {
		return false
	}
	header := prior[start:]
	end := strings.Index(header, ")")
	if end < 0 || end > 300 {
		return false
	}
	if strings.Contains(header[end:], "});") {
		return false
	}

	return strings.Contains(header[:end], "[argus:"+suiteKey+":"+testKey+"]")
}

func isIdentifier(value byte) bool {
	return value >= 'a' && value <= 'z' || value >= 'A' && value <= 'Z' ||
		value >= '0' && value <= '9' || value == '_' || value == '$'
}

// codeOffset excludes comments and quoted content. Unsupported JavaScript
// syntax can cause abstention but must not broaden the permitted edit.
func codeOffset(source []byte, offset int) bool {
	state := byte('c')
	for i := 0; i < offset; i++ {
		value := source[i]
		switch state {
		case 'c':
			state, i = scanCodeByte(source, i, offset)
		case 'l':
			if value == '\n' {
				state = 'c'
			}
		case 'b':
			if value == '*' && i+1 < offset && source[i+1] == '/' {
				state = 'c'
				i++
			}
		default:
			if value == '\\' {
				i++
			} else if value == state {
				state = 'c'
			}
		}
	}

	return state == 'c'
}

func scanCodeByte(source []byte, index, limit int) (byte, int) {
	value := source[index]
	if value == '/' && index+1 < limit {
		switch source[index+1] {
		case '/':
			return 'l', index + 1
		case '*':
			return 'b', index + 1
		}
	}
	switch value {
	case '\'', '"', '`':
		return value, index
	default:
		return 'c', index
	}
}

func findSourceLocator(source []byte, message string) (locator, string) {
	var found []locator
	for _, match := range roleCall.FindAllSubmatchIndex(source, -1) {
		value, ok := exactLocator(string(source[match[0]:match[1]]))
		if !ok {
			continue
		}
		value.start, value.end = match[0], match[1]
		if codeOffset(source, value.start) && strings.HasSuffix(string(source[:value.start]), "page.") && failureNamesLocator(message, value) {
			found = append(found, value)
		}
	}
	for _, match := range idCall.FindAllSubmatchIndex(source, -1) {
		value, ok := exactLocator(string(source[match[0]:match[1]]))
		if !ok {
			continue
		}
		value.start, value.end = match[0], match[1]
		if codeOffset(source, value.start) && strings.HasSuffix(string(source[:value.start]), "page.") && failureNamesLocator(message, value) {
			found = append(found, value)
		}
	}
	if !strings.Contains(message, "element(s) not found") {
		return locator{}, "failure-source-mismatch"
	}
	if len(found) != 1 {
		return locator{}, "ambiguous-or-unsupported-source"
	}
	if strings.Count(string(source), found[0].text) != 1 {
		return locator{}, "ambiguous-or-unsupported-source"
	}

	return found[0], ""
}

func exactLocator(text string) (locator, bool) {
	if match := roleCall.FindStringSubmatchIndex(text); match != nil && match[0] == 0 && match[1] == len(text) {
		quote := text[match[2]:match[3]]
		if text[match[5]:match[5]+1] != quote || text[match[6]:match[7]] != text[match[9]:match[9]+1] {
			return locator{}, false
		}

		return locator{text: text, kind: "role", role: text[match[4]:match[5]], value: text[match[8]:match[9]], quote: quote}, true
	}
	if match := idCall.FindStringSubmatchIndex(text); match != nil && match[0] == 0 && match[1] == len(text) {
		quote := text[match[2]:match[3]]
		if text[match[5]:match[5]+1] != quote {
			return locator{}, false
		}

		return locator{text: text, kind: "test-id", value: text[match[4]:match[5]], quote: quote}, true
	}

	return locator{}, false
}

func failureNamesLocator(message string, value locator) bool {
	if value.kind == "role" {
		for _, match := range roleCall.FindAllStringSubmatch(message, -1) {
			if match[2] == value.role && match[4] == value.value {
				return true
			}
		}

		return false
	}
	for _, match := range idCall.FindAllStringSubmatch(message, -1) {
		if match[2] == value.value {
			return true
		}
	}

	return false
}

func matching(nodes []Node, call locator) []*Node {
	var matches []*Node
	for i := range nodes {
		if call.kind == "role" && nodes[i].Role == call.role && nodes[i].Name == call.value ||
			call.kind == "test-id" && nodes[i].TestID == call.value {
			matches = append(matches, &nodes[i])
		}
	}

	return matches
}

func equivalent(before Node, nodes []Node, original locator) (string, Node, string) {
	var candidates []Node
	for _, node := range nodes {
		if original.kind == "role" && before.TestID != "" && node.TestID == before.TestID && node.Role == before.Role && node.Name != "" ||
			original.kind == "test-id" && before.Role != "" && before.Name != "" && node.Role == before.Role && node.Name == before.Name && node.TestID != "" {
			candidates = append(candidates, node)
		}
	}
	if len(candidates) != 1 || !candidates[0].Visible {
		return "", Node{}, "ambiguous-or-hidden-target"
	}
	target := candidates[0]
	var replacement string
	if original.kind == "role" {
		if len(matching(nodes, locator{kind: "role", role: target.Role, value: target.Name})) != 1 {
			return "", Node{}, "ambiguous-or-hidden-target"
		}
		replacement = "getByRole(" + original.quote + target.Role + original.quote + ", { name: " + original.quote + target.Name + original.quote + " })"
	} else {
		if len(matching(nodes, locator{kind: "test-id", value: target.TestID})) != 1 {
			return "", Node{}, "ambiguous-or-hidden-target"
		}
		replacement = "getByTestId(" + original.quote + target.TestID + original.quote + ")"
	}
	if replacement == original.text || strings.ContainsAny(replacement, "\r\n\\") ||
		strings.Contains(target.Name, original.quote) || strings.Contains(target.TestID, original.quote) {
		return "", Node{}, "unsupported-replacement"
	}

	return replacement, target, ""
}
