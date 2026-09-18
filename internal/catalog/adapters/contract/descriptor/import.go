// Package descriptor translates the versioned repository-descriptor transport
// contract into validated catalog domain values. JSON paths and transport DTOs
// stop at this boundary.
package descriptor

import (
	"errors"
	"fmt"
	"path"
	"strings"

	"github.com/stanimirivanov/argus/contracts"
	"github.com/stanimirivanov/argus/internal/catalog"
)

// ValidationError identifies a semantic violation at a repository-descriptor
// JSON path.
type ValidationError struct {
	Path    string
	Code    string
	Message string
}

// Error returns a stable, human-readable representation of the violation.
func (err *ValidationError) Error() string {
	return fmt.Sprintf("%s: %s (%s)", err.Path, err.Message, err.Code)
}

// Import converts a structurally validated repository descriptor into catalog
// state and enforces cross-field semantic invariants.
func Import(document contracts.RepositoryDescriptorV1, revision catalog.Revision) (catalog.Snapshot, error) {
	if err := validateRevision(revision); err != nil {
		return catalog.Snapshot{}, fmt.Errorf("validate ingestion revision: %w", err)
	}
	if err := validateRepositoryCoordinates(document); err != nil {
		return catalog.Snapshot{}, err
	}

	capabilities, capabilityKeys, err := importCapabilities(document.Capabilities)
	if err != nil {
		return catalog.Snapshot{}, err
	}

	components, err := importComponents(document.Components, capabilityKeys)
	if err != nil {
		return catalog.Snapshot{}, err
	}

	testSuites, err := importTestSuites(document.TestSuites, capabilityKeys)
	if err != nil {
		return catalog.Snapshot{}, err
	}

	return catalog.Snapshot{
		APIVersion:   document.APIVersion,
		Repository:   importRepository(document.Repository),
		Revision:     revision,
		Capabilities: capabilities,
		Components:   components,
		TestSuites:   testSuites,
	}, nil
}

// A provider identity may acquire new display coordinates over time, but it
// cannot mean two different owner/name pairs inside one immutable observation.
func validateRepositoryCoordinates(document contracts.RepositoryDescriptorV1) error {
	type coordinates struct {
		owner string
		name  string
	}

	repositories := map[repositoryIdentity]coordinates{
		repositoryIdentityFrom(document.Repository): {
			owner: document.Repository.Owner,
			name:  document.Repository.Name,
		},
	}
	for index, suite := range document.TestSuites {
		identity := repositoryIdentityFrom(suite.Repository)
		observed := coordinates{owner: suite.Repository.Owner, name: suite.Repository.Name}
		if existing, ok := repositories[identity]; ok && existing != observed {
			return &ValidationError{
				Path:    fmt.Sprintf("/testSuites/%d/repository", index),
				Code:    "repository-coordinate-conflict",
				Message: "one repository identity has conflicting owner or name coordinates",
			}
		}
		repositories[identity] = observed
	}

	return nil
}

func validateRevision(revision catalog.Revision) error {
	validated, err := catalog.NewRevision(revision.Algorithm, revision.Digest)
	if err != nil {
		return err
	}
	if validated != revision {
		return errors.New("revision is not normalized")
	}

	return nil
}

func importRepository(reference contracts.RepositoryReference) catalog.Repository {
	return catalog.Repository{
		Identity: catalog.RepositoryIdentity{
			Provider:             catalog.Provider(reference.Provider),
			Host:                 reference.Host,
			ProviderRepositoryID: reference.ProviderRepositoryID,
		},
		Owner: reference.Owner,
		Name:  reference.Name,
	}
}

func importCapabilities(
	declarations []contracts.CapabilityDeclaration,
) ([]catalog.Capability, map[string]struct{}, error) {
	capabilities := make([]catalog.Capability, 0, len(declarations))
	keys := make(map[string]struct{}, len(declarations))
	for index, declaration := range declarations {
		keyPath := fmt.Sprintf("/capabilities/%d/key", index)
		if err := addUnique(keys, declaration.Key, keyPath, "duplicate-capability"); err != nil {
			return nil, nil, err
		}

		capabilities = append(capabilities, catalog.Capability{Key: declaration.Key, Name: declaration.Name})
	}

	return capabilities, keys, nil
}

func importComponents(
	declarations []contracts.ComponentDeclaration,
	capabilities map[string]struct{},
) ([]catalog.Component, error) {
	components := make([]catalog.Component, 0, len(declarations))
	keys := make(map[string]struct{}, len(declarations))
	for index, declaration := range declarations {
		basePath := fmt.Sprintf("/components/%d", index)
		if err := addUnique(keys, declaration.Key, basePath+"/key", "duplicate-component"); err != nil {
			return nil, err
		}
		if err := validateComponentRoot(declaration.Root, basePath+"/root"); err != nil {
			return nil, err
		}
		if err := validateCapabilityReferences(
			declaration.Capabilities,
			capabilities,
			basePath+"/capabilities",
		); err != nil {
			return nil, err
		}

		components = append(components, catalog.Component{
			Key:          declaration.Key,
			Root:         declaration.Root,
			Capabilities: append([]string(nil), declaration.Capabilities...),
		})
	}

	return components, nil
}

func importTestSuites(
	declarations []contracts.TestSuiteDeclaration,
	capabilities map[string]struct{},
) ([]catalog.TestSuite, error) {
	suites := make([]catalog.TestSuite, 0, len(declarations))
	suiteKeys := make(map[suiteIdentity]struct{}, len(declarations))
	for suiteIndex, declaration := range declarations {
		basePath := fmt.Sprintf("/testSuites/%d", suiteIndex)
		identity := suiteIdentity{
			Repository: repositoryIdentityFrom(declaration.Repository),
			Key:        declaration.Key,
		}
		if _, ok := suiteKeys[identity]; ok {
			return nil, &ValidationError{
				Path: basePath + "/key",
				Code: "duplicate-test-suite",
				Message: fmt.Sprintf(
					"key %q is declared more than once for this repository",
					declaration.Key,
				),
			}
		}
		suiteKeys[identity] = struct{}{}

		tests, err := importTests(declaration.Tests, capabilities, basePath+"/tests")
		if err != nil {
			return nil, err
		}

		suites = append(suites, catalog.TestSuite{
			Key:        declaration.Key,
			Repository: importRepository(declaration.Repository),
			Family:     catalog.TestFamily(declaration.Family),
			Adapter:    declaration.Adapter,
			Tests:      tests,
		})
	}

	return suites, nil
}

type repositoryIdentity struct {
	Provider             string
	Host                 string
	ProviderRepositoryID string
}

type suiteIdentity struct {
	Repository repositoryIdentity
	Key        string
}

func repositoryIdentityFrom(repository contracts.RepositoryReference) repositoryIdentity {
	return repositoryIdentity{
		Provider:             repository.Provider,
		Host:                 repository.Host,
		ProviderRepositoryID: repository.ProviderRepositoryID,
	}
}

func importTests(
	declarations []contracts.TestDeclaration,
	capabilities map[string]struct{},
	basePath string,
) ([]catalog.Test, error) {
	tests := make([]catalog.Test, 0, len(declarations))
	keys := make(map[string]struct{}, len(declarations))
	for index, declaration := range declarations {
		testPath := fmt.Sprintf("%s/%d", basePath, index)
		if err := addUnique(keys, declaration.Key, testPath+"/key", "duplicate-test"); err != nil {
			return nil, err
		}
		if err := validateCapabilityReferences(
			declaration.Capabilities,
			capabilities,
			testPath+"/capabilities",
		); err != nil {
			return nil, err
		}

		tests = append(tests, catalog.Test{
			Key:          declaration.Key,
			Name:         declaration.Name,
			Capabilities: append([]string(nil), declaration.Capabilities...),
		})
	}

	return tests, nil
}

func validateCapabilityReferences(
	references []string,
	capabilities map[string]struct{},
	basePath string,
) error {
	seen := make(map[string]struct{}, len(references))
	for index, reference := range references {
		referencePath := fmt.Sprintf("%s/%d", basePath, index)
		if _, ok := capabilities[reference]; !ok {
			return &ValidationError{
				Path:    referencePath,
				Code:    "unknown-capability",
				Message: fmt.Sprintf("capability %q is not declared", reference),
			}
		}
		if err := addUnique(seen, reference, referencePath, "duplicate-capability-reference"); err != nil {
			return err
		}
	}

	return nil
}

func validateComponentRoot(root, rootPath string) error {
	cleaned := path.Clean(root)
	hasWindowsVolume := len(root) >= 2 && root[1] == ':'
	if strings.Contains(root, "\\") ||
		strings.HasPrefix(root, "/") ||
		cleaned != root ||
		cleaned == "." ||
		cleaned == ".." ||
		strings.HasPrefix(cleaned, "../") ||
		hasWindowsVolume {
		return &ValidationError{
			Path:    rootPath,
			Code:    "invalid-component-root",
			Message: "root must be a normalized repository-relative path",
		}
	}

	return nil
}

func addUnique(seen map[string]struct{}, key, keyPath, code string) error {
	if _, ok := seen[key]; ok {
		return &ValidationError{
			Path:    keyPath,
			Code:    code,
			Message: fmt.Sprintf("key %q is declared more than once", key),
		}
	}

	seen[key] = struct{}{}

	return nil
}
