package catalogcli

import "github.com/stanimirivanov/argus/internal/catalog"

// Output DTOs keep the command's JSON boundary independent from catalog domain
// values. Changing the domain representation therefore cannot silently change
// command output or persistence fingerprints.
type snapshotOutput struct {
	APIVersion   string             `json:"apiVersion"`
	Repository   repositoryOutput   `json:"repository"`
	Revision     revisionOutput     `json:"revision"`
	Capabilities []capabilityOutput `json:"capabilities"`
	Components   []componentOutput  `json:"components"`
	TestSuites   []testSuiteOutput  `json:"testSuites"`
}

type revisionOutput struct {
	Algorithm catalog.RevisionAlgorithm `json:"algorithm"`
	Digest    string                    `json:"digest"`
}

type repositoryIdentityOutput struct {
	Provider             catalog.Provider `json:"provider"`
	Host                 string           `json:"host"`
	ProviderRepositoryID string           `json:"providerRepositoryId"`
}

type repositoryOutput struct {
	Identity repositoryIdentityOutput `json:"identity"`
	Owner    string                   `json:"owner"`
	Name     string                   `json:"name"`
}

type capabilityOutput struct {
	Key  string `json:"key"`
	Name string `json:"name"`
}

type componentOutput struct {
	Key          string   `json:"key"`
	Root         string   `json:"root"`
	Capabilities []string `json:"capabilities"`
}

type testOutput struct {
	Key          string   `json:"key"`
	Name         string   `json:"name"`
	Capabilities []string `json:"capabilities"`
}

type testSuiteOutput struct {
	Key        string             `json:"key"`
	Repository repositoryOutput   `json:"repository"`
	Family     catalog.TestFamily `json:"family"`
	Adapter    string             `json:"adapter"`
	Tests      []testOutput       `json:"tests"`
}

func newSnapshotOutput(snapshot catalog.Snapshot) snapshotOutput {
	capabilities := make([]capabilityOutput, len(snapshot.Capabilities))
	for index, capability := range snapshot.Capabilities {
		capabilities[index] = capabilityOutput{Key: capability.Key, Name: capability.Name}
	}

	components := make([]componentOutput, len(snapshot.Components))
	for index, component := range snapshot.Components {
		components[index] = componentOutput{
			Key:          component.Key,
			Root:         component.Root,
			Capabilities: append([]string(nil), component.Capabilities...),
		}
	}

	testSuites := make([]testSuiteOutput, len(snapshot.TestSuites))
	for suiteIndex, suite := range snapshot.TestSuites {
		tests := make([]testOutput, len(suite.Tests))
		for testIndex, test := range suite.Tests {
			tests[testIndex] = testOutput{
				Key:          test.Key,
				Name:         test.Name,
				Capabilities: append([]string(nil), test.Capabilities...),
			}
		}
		testSuites[suiteIndex] = testSuiteOutput{
			Key:        suite.Key,
			Repository: newRepositoryOutput(suite.Repository),
			Family:     suite.Family,
			Adapter:    suite.Adapter,
			Tests:      tests,
		}
	}

	return snapshotOutput{
		APIVersion:   snapshot.APIVersion,
		Repository:   newRepositoryOutput(snapshot.Repository),
		Revision:     revisionOutput(snapshot.Revision),
		Capabilities: capabilities,
		Components:   components,
		TestSuites:   testSuites,
	}
}

func newRepositoryOutput(repository catalog.Repository) repositoryOutput {
	return repositoryOutput{
		Identity: repositoryIdentityOutput(repository.Identity),
		Owner:    repository.Owner,
		Name:     repository.Name,
	}
}
