// Package discovery checks observed test-runner inventory against a declared
// catalog snapshot without granting observed metadata selection authority.
package discovery

import (
	"errors"
	"fmt"
	"slices"

	"github.com/stanimirivanov/argus/internal/catalog"
)

// ErrMismatch means the observed inventory cannot prove the declared suite is current.
var ErrMismatch = errors.New("playwright inventory does not match declared suite")

// Case is one Playwright project variant of a repository-declared test.
// Key is an explicit Argus annotation, not Playwright's generated session ID.
type Case struct {
	Key     string
	Project string
	File    string
	Title   string
	Tags    []string
	Owners  []string
}

// Inventory contains only validated local discovery data. It is not durable
// selection evidence and does not verify the revision of the checkout.
type Inventory struct {
	Suite    catalog.TestSuite
	Cases    []Case
	TestKeys []string
	Projects []string
}

// Reconcile requires exact coverage of one declared Playwright UI suite.
// Each declared key must occur in at least one project, and a project may
// contain a key only once. Tags and owners remain observational metadata.
func Reconcile(snapshot catalog.Snapshot, suiteKey string, cases []Case) (Inventory, error) {
	var suite *catalog.TestSuite
	for index := range snapshot.TestSuites {
		if snapshot.TestSuites[index].Key == suiteKey {
			suite = &snapshot.TestSuites[index]
			break
		}
	}
	if suite == nil || suite.Family != catalog.TestFamilyFunctionalUI || suite.Adapter != "playwright" {
		return Inventory{}, fmt.Errorf("%w: unknown or unsupported suite %q", ErrMismatch, suiteKey)
	}

	declared := make(map[string]struct{}, len(suite.Tests))
	for _, test := range suite.Tests {
		declared[test.Key] = struct{}{}
	}
	observed := make(map[string]struct{}, len(cases))
	observedKeys := make(map[string]struct{}, len(cases))
	projects := make(map[string]struct{})
	for _, test := range cases {
		if _, exists := declared[test.Key]; !exists {
			return Inventory{}, fmt.Errorf("%w: undeclared key %q", ErrMismatch, test.Key)
		}
		if test.Project == "" || test.File == "" || test.Title == "" {
			return Inventory{}, fmt.Errorf("%w: incomplete case %q", ErrMismatch, test.Key)
		}
		variant := test.Key + "\x00" + test.Project
		if _, exists := observed[variant]; exists {
			return Inventory{}, fmt.Errorf("%w: duplicate key %q in project %q", ErrMismatch, test.Key, test.Project)
		}
		observed[variant] = struct{}{}
		observedKeys[test.Key] = struct{}{}
		projects[test.Project] = struct{}{}
	}

	keys := make([]string, 0, len(declared))
	for key := range declared {
		if _, found := observedKeys[key]; !found {
			return Inventory{}, fmt.Errorf("%w: missing declared key %q", ErrMismatch, key)
		}
		keys = append(keys, key)
	}
	slices.Sort(keys)
	projectNames := make([]string, 0, len(projects))
	for project := range projects {
		projectNames = append(projectNames, project)
	}
	slices.Sort(projectNames)
	orderedCases := append([]Case(nil), cases...)
	slices.SortFunc(orderedCases, func(left, right Case) int {
		if cmp := compare(left.Key, right.Key); cmp != 0 {
			return cmp
		}
		return compare(left.Project, right.Project)
	})

	return Inventory{Suite: *suite, Cases: orderedCases, TestKeys: keys, Projects: projectNames}, nil
}

func compare(left, right string) int {
	switch {
	case left < right:
		return -1
	case left > right:
		return 1
	default:
		return 0
	}
}
