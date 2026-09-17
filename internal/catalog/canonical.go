package catalog

import (
	"cmp"
	"slices"
)

// CanonicalSnapshot returns a deep copy whose semantically unordered
// collections use stable identity order. It defines exact-retry equivalence
// for persistence adapters and never mutates the caller's snapshot.
func CanonicalSnapshot(snapshot Snapshot) Snapshot {
	canonical := snapshot
	canonical.Capabilities = append([]Capability(nil), snapshot.Capabilities...)
	canonical.Components = canonicalComponents(snapshot.Components)
	canonical.TestSuites = canonicalTestSuites(snapshot.TestSuites)

	slices.SortFunc(canonical.Capabilities, func(left, right Capability) int {
		return cmp.Compare(left.Key, right.Key)
	})
	slices.SortFunc(canonical.Components, func(left, right Component) int {
		return cmp.Compare(left.Key, right.Key)
	})
	slices.SortFunc(canonical.TestSuites, compareTestSuites)

	return canonical
}

func canonicalComponents(components []Component) []Component {
	canonical := make([]Component, len(components))
	for index, component := range components {
		canonical[index] = component
		canonical[index].Capabilities = append([]string(nil), component.Capabilities...)
		slices.Sort(canonical[index].Capabilities)
	}

	return canonical
}

func canonicalTestSuites(suites []TestSuite) []TestSuite {
	canonical := make([]TestSuite, len(suites))
	for suiteIndex, suite := range suites {
		canonical[suiteIndex] = suite
		canonical[suiteIndex].Tests = make([]Test, len(suite.Tests))
		for testIndex, test := range suite.Tests {
			canonical[suiteIndex].Tests[testIndex] = test
			canonical[suiteIndex].Tests[testIndex].Capabilities = append(
				[]string(nil),
				test.Capabilities...,
			)
			slices.Sort(canonical[suiteIndex].Tests[testIndex].Capabilities)
		}
		slices.SortFunc(canonical[suiteIndex].Tests, func(left, right Test) int {
			return cmp.Compare(left.Key, right.Key)
		})
	}

	return canonical
}

func compareTestSuites(left, right TestSuite) int {
	if compared := compareRepositoryIdentities(left.Repository.Identity, right.Repository.Identity); compared != 0 {
		return compared
	}

	return cmp.Compare(left.Key, right.Key)
}

func compareRepositoryIdentities(left, right RepositoryIdentity) int {
	if compared := cmp.Compare(left.Provider, right.Provider); compared != 0 {
		return compared
	}
	if compared := cmp.Compare(left.Host, right.Host); compared != 0 {
		return compared
	}

	return cmp.Compare(left.ProviderRepositoryID, right.ProviderRepositoryID)
}
