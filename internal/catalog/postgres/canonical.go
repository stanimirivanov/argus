package postgres

import (
	"cmp"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"slices"
	"strings"

	"github.com/stanimirivanov/argus/internal/catalog"
)

func canonicalSnapshot(snapshot catalog.Snapshot) catalog.Snapshot {
	canonical := snapshot
	canonical.Capabilities = append([]catalog.Capability(nil), snapshot.Capabilities...)
	canonical.Components = cloneComponents(snapshot.Components)
	canonical.TestSuites = cloneTestSuites(snapshot.TestSuites)

	slices.SortFunc(canonical.Capabilities, func(left, right catalog.Capability) int {
		return cmp.Compare(left.Key, right.Key)
	})
	slices.SortFunc(canonical.Components, func(left, right catalog.Component) int {
		return cmp.Compare(left.Key, right.Key)
	})
	slices.SortFunc(canonical.TestSuites, compareTestSuites)

	return canonical
}

func cloneComponents(components []catalog.Component) []catalog.Component {
	cloned := make([]catalog.Component, len(components))
	for index, component := range components {
		cloned[index] = component
		cloned[index].Capabilities = append([]string(nil), component.Capabilities...)
		slices.Sort(cloned[index].Capabilities)
	}

	return cloned
}

func cloneTestSuites(suites []catalog.TestSuite) []catalog.TestSuite {
	cloned := make([]catalog.TestSuite, len(suites))
	for suiteIndex, suite := range suites {
		cloned[suiteIndex] = suite
		cloned[suiteIndex].Tests = make([]catalog.Test, len(suite.Tests))
		for testIndex, test := range suite.Tests {
			cloned[suiteIndex].Tests[testIndex] = test
			cloned[suiteIndex].Tests[testIndex].Capabilities = append(
				[]string(nil),
				test.Capabilities...,
			)
			slices.Sort(cloned[suiteIndex].Tests[testIndex].Capabilities)
		}
		slices.SortFunc(cloned[suiteIndex].Tests, func(left, right catalog.Test) int {
			return cmp.Compare(left.Key, right.Key)
		})
	}

	return cloned
}

func compareTestSuites(left, right catalog.TestSuite) int {
	if compared := cmp.Compare(repositorySortKey(left.Repository.Identity), repositorySortKey(right.Repository.Identity)); compared != 0 {
		return compared
	}

	return cmp.Compare(left.Key, right.Key)
}

func repositorySortKey(identity catalog.RepositoryIdentity) string {
	return strings.Join(
		[]string{string(identity.Provider), identity.Host, identity.ProviderRepositoryID},
		"\x00",
	)
}

func snapshotFingerprint(snapshot catalog.Snapshot) (string, error) {
	encoded, err := json.Marshal(snapshot)
	if err != nil {
		return "", err
	}
	digest := sha256.Sum256(encoded)

	return hex.EncodeToString(digest[:]), nil
}
