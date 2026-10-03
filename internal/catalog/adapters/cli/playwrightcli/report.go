// Package playwrightcli validates Playwright discovery reports against a
// repository-declared functional UI suite.
package playwrightcli

import (
	"encoding/json"
	"errors"
	"fmt"
	"path"
	"slices"
	"strings"
	"unicode"

	"github.com/stanimirivanov/argus/internal/catalog/discovery"
)

const (
	maxCases = 10_000
	maxDepth = 64
)

var errInvalidReport = errors.New("invalid Playwright list report")

type playwrightReport struct {
	Suites []playwrightSuite `json:"suites"`
	Errors []json.RawMessage `json:"errors"`
}

type playwrightSuite struct {
	File   string            `json:"file"`
	Suites []playwrightSuite `json:"suites"`
	Specs  []playwrightSpec  `json:"specs"`
}

type playwrightSpec struct {
	Title string           `json:"title"`
	File  string           `json:"file"`
	Tags  []string         `json:"tags"`
	Tests []playwrightTest `json:"tests"`
}

type playwrightTest struct {
	ProjectName string                 `json:"projectName"`
	Annotations []playwrightAnnotation `json:"annotations"`
	Results     []json.RawMessage      `json:"results"`
}

type playwrightAnnotation struct {
	Type        string `json:"type"`
	Description string `json:"description"`
}

// ParseListReport reads the built-in JSON reporter's --list output. It rejects
// executed results, discovery errors, missing stable IDs, and oversized trees.
// Unknown vendor fields are ignored deliberately; Argus consumes only the
// documented discovery fields needed by this adapter.
func ParseListReport(data []byte) ([]discovery.Case, error) {
	var report playwrightReport
	if err := json.Unmarshal(data, &report); err != nil {
		return nil, fmt.Errorf("%w: decode JSON: %w", errInvalidReport, err)
	}
	if len(report.Errors) != 0 || len(report.Suites) == 0 {
		return nil, fmt.Errorf("%w: discovery errors or no suites", errInvalidReport)
	}

	cases := make([]discovery.Case, 0)
	visited := 0
	for _, suite := range report.Suites {
		if err := appendSuite(&cases, suite, 0, &visited); err != nil {
			return nil, err
		}
	}
	if len(cases) == 0 {
		return nil, fmt.Errorf("%w: no tests", errInvalidReport)
	}

	return cases, nil
}

func appendSuite(cases *[]discovery.Case, suite playwrightSuite, depth int, visited *int) error {
	(*visited)++
	if depth > maxDepth || *visited > maxCases {
		return fmt.Errorf("%w: suite tree exceeds bound", errInvalidReport)
	}
	if suite.File != "" && !validReportPath(suite.File) {
		return fmt.Errorf("%w: invalid suite path", errInvalidReport)
	}
	for _, spec := range suite.Specs {
		if err := appendSpec(cases, spec); err != nil {
			return err
		}
	}
	for _, child := range suite.Suites {
		if err := appendSuite(cases, child, depth+1, visited); err != nil {
			return err
		}
	}

	return nil
}

func appendSpec(cases *[]discovery.Case, spec playwrightSpec) error {
	if !safeText(spec.Title, 255) || !validReportPath(spec.File) || len(spec.Tags) > 32 || len(spec.Tests) == 0 {
		return fmt.Errorf("%w: invalid test title, path, or tag count", errInvalidReport)
	}
	for _, tag := range spec.Tags {
		if !safeText(tag, 127) {
			return fmt.Errorf("%w: invalid tag", errInvalidReport)
		}
	}
	for _, test := range spec.Tests {
		if len(*cases) >= maxCases || len(test.Results) != 0 ||
			(test.ProjectName != "" && !safeText(test.ProjectName, 127)) {
			return fmt.Errorf("%w: case bound, executed result, or project name", errInvalidReport)
		}
		key, owners, err := annotations(test.Annotations)
		if err != nil {
			return err
		}
		project := test.ProjectName
		if project == "" {
			project = "default"
		}
		*cases = append(*cases, discovery.Case{
			Key: key, Project: project, File: spec.File, Title: spec.Title,
			Tags: append([]string(nil), spec.Tags...), Owners: owners,
		})
	}

	return nil
}

func annotations(values []playwrightAnnotation) (string, []string, error) {
	if len(values) > 64 {
		return "", nil, fmt.Errorf("%w: too many annotations", errInvalidReport)
	}
	key := ""
	owners := make([]string, 0)
	for _, value := range values {
		switch value.Type {
		case "argus.test-key":
			if key != "" || !safeText(value.Description, 63) {
				return "", nil, fmt.Errorf("%w: missing or duplicate stable test key", errInvalidReport)
			}
			key = value.Description
		case "argus.owner":
			if !safeText(value.Description, 255) || len(owners) >= 10 {
				return "", nil, fmt.Errorf("%w: invalid owner annotation", errInvalidReport)
			}
			owners = append(owners, value.Description)
		}
	}
	if key == "" {
		return "", nil, fmt.Errorf("%w: missing argus.test-key annotation", errInvalidReport)
	}
	slices.Sort(owners)
	owners = slices.Compact(owners)

	return key, owners, nil
}

func validReportPath(value string) bool {
	return safeText(value, 1024) && !strings.ContainsAny(value, "\\:") &&
		!strings.HasPrefix(value, "/") && value != "." && value != ".." &&
		!strings.HasPrefix(value, "../") && path.Clean(value) == value
}

func safeText(value string, limit int) bool {
	return value != "" && len(value) <= limit && strings.IndexFunc(value, unicode.IsControl) == -1
}
