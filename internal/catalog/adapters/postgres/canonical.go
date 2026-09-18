package postgres

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"

	"github.com/stanimirivanov/argus/internal/catalog"
)

// Fingerprint documents are a private persistence encoding. They preserve the
// original field names and order independently of domain structs or public
// transport contracts, because existing snapshot hashes are immutable data.
type fingerprintSnapshot struct {
	APIVersion   string                  `json:"apiVersion"`
	Repository   fingerprintRepository   `json:"repository"`
	Revision     fingerprintRevision     `json:"revision"`
	Capabilities []fingerprintCapability `json:"capabilities"`
	Components   []fingerprintComponent  `json:"components"`
	TestSuites   []fingerprintTestSuite  `json:"testSuites"`
}

type fingerprintRevision struct {
	Algorithm catalog.RevisionAlgorithm `json:"algorithm"`
	Digest    string                    `json:"digest"`
}

type fingerprintRepositoryIdentity struct {
	Provider             catalog.Provider `json:"provider"`
	Host                 string           `json:"host"`
	ProviderRepositoryID string           `json:"providerRepositoryId"`
}

type fingerprintRepository struct {
	Identity fingerprintRepositoryIdentity `json:"identity"`
	Owner    string                        `json:"owner"`
	Name     string                        `json:"name"`
}

type fingerprintCapability struct {
	Key  string `json:"key"`
	Name string `json:"name"`
}

type fingerprintComponent struct {
	Key          string   `json:"key"`
	Root         string   `json:"root"`
	Capabilities []string `json:"capabilities"`
}

type fingerprintTest struct {
	Key          string   `json:"key"`
	Name         string   `json:"name"`
	Capabilities []string `json:"capabilities"`
}

type fingerprintTestSuite struct {
	Key        string                `json:"key"`
	Repository fingerprintRepository `json:"repository"`
	Family     catalog.TestFamily    `json:"family"`
	Adapter    string                `json:"adapter"`
	Tests      []fingerprintTest     `json:"tests"`
}

func snapshotFingerprint(snapshot catalog.Snapshot) (string, error) {
	encoded, err := json.Marshal(newFingerprintSnapshot(snapshot))
	if err != nil {
		return "", err
	}
	digest := sha256.Sum256(encoded)

	return hex.EncodeToString(digest[:]), nil
}

func newFingerprintSnapshot(snapshot catalog.Snapshot) fingerprintSnapshot {
	var capabilities []fingerprintCapability
	if snapshot.Capabilities != nil {
		capabilities = make([]fingerprintCapability, len(snapshot.Capabilities))
		for index, capability := range snapshot.Capabilities {
			capabilities[index] = fingerprintCapability{Key: capability.Key, Name: capability.Name}
		}
	}

	components := make([]fingerprintComponent, len(snapshot.Components))
	for index, component := range snapshot.Components {
		components[index] = fingerprintComponent{
			Key:          component.Key,
			Root:         component.Root,
			Capabilities: append([]string(nil), component.Capabilities...),
		}
	}

	testSuites := make([]fingerprintTestSuite, len(snapshot.TestSuites))
	for suiteIndex, suite := range snapshot.TestSuites {
		tests := make([]fingerprintTest, len(suite.Tests))
		for testIndex, test := range suite.Tests {
			tests[testIndex] = fingerprintTest{
				Key:          test.Key,
				Name:         test.Name,
				Capabilities: append([]string(nil), test.Capabilities...),
			}
		}
		testSuites[suiteIndex] = fingerprintTestSuite{
			Key:        suite.Key,
			Repository: fingerprintRepositoryFrom(suite.Repository),
			Family:     suite.Family,
			Adapter:    suite.Adapter,
			Tests:      tests,
		}
	}

	return fingerprintSnapshot{
		APIVersion:   snapshot.APIVersion,
		Repository:   fingerprintRepositoryFrom(snapshot.Repository),
		Revision:     fingerprintRevision(snapshot.Revision),
		Capabilities: capabilities,
		Components:   components,
		TestSuites:   testSuites,
	}
}

func fingerprintRepositoryFrom(repository catalog.Repository) fingerprintRepository {
	return fingerprintRepository{
		Identity: fingerprintRepositoryIdentity(repository.Identity),
		Owner:    repository.Owner,
		Name:     repository.Name,
	}
}
