package discovery

import (
	"cmp"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"path"
	"slices"
	"strings"
	"time"

	"github.com/stanimirivanov/argus/internal/catalog"
)

const (
	// EvidenceAPIVersion identifies immutable Playwright discovery and reviewed UI mappings.
	EvidenceAPIVersion = "argus.dev/browser-discovery/v1"
	maxCases           = 10_000
	maxMappings        = 20_000
)

// ErrInvalidEvidence means browser discovery cannot support a trustworthy decision.
var ErrInvalidEvidence = errors.New("invalid browser discovery evidence")

// SuiteObservation binds collected project variants to one declared UI suite and checkout.
type SuiteObservation struct {
	Repository catalog.Repository
	Revision   catalog.Revision
	SuiteKey   string
	Cases      []Case
}

// MappingKind distinguishes reviewed routes from declared source components.
type MappingKind string

const (
	// MappingRoute is an explicitly reviewed route-to-test relationship.
	MappingRoute MappingKind = "route"
	// MappingComponent is an explicitly reviewed component-to-test relationship.
	MappingComponent MappingKind = "component"
)

// Mapping connects a source root and route or component key to a stable test.
// A route root is reviewed policy, not a path inferred from a Playwright title.
type Mapping struct {
	Kind         MappingKind
	Key          string
	RoutePattern string
	SourceRoot   string
	Test         catalog.TestIdentity
}

// Evidence is one bounded, immutable observation of all browser suites in a
// source snapshot. ObservationID is caller-stable across retries; differing
// content under that ID conflicts. Incomplete evidence can be stored but never
// authorize targeted omissions.
type Evidence struct {
	APIVersion       string
	ObservationID    string
	Catalog          catalog.SnapshotReference
	CollectorVersion string
	CollectedAt      time.Time
	ExpiresAt        time.Time
	Complete         bool
	MappingRevision  catalog.Revision
	MappingComplete  bool
	Suites           []SuiteObservation
	Mappings         []Mapping
}

// CanonicalEvidence orders set-like fields without rewriting their identities.
func CanonicalEvidence(evidence Evidence) Evidence {
	result := evidence
	result.CollectedAt = evidence.CollectedAt.UTC()
	result.ExpiresAt = evidence.ExpiresAt.UTC()
	result.Suites = make([]SuiteObservation, len(evidence.Suites))
	for index, suite := range evidence.Suites {
		result.Suites[index] = suite
		result.Suites[index].Cases = make([]Case, len(suite.Cases))
		for caseIndex, item := range suite.Cases {
			result.Suites[index].Cases[caseIndex] = item
			result.Suites[index].Cases[caseIndex].Tags = canonicalStrings(item.Tags)
			result.Suites[index].Cases[caseIndex].Owners = canonicalStrings(item.Owners)
		}
		slices.SortFunc(result.Suites[index].Cases, func(left, right Case) int {
			if compared := cmp.Compare(left.Key, right.Key); compared != 0 {
				return compared
			}
			return cmp.Compare(left.Project, right.Project)
		})
	}
	slices.SortFunc(result.Suites, func(left, right SuiteObservation) int {
		if compared := compareRepository(left.Repository.Identity, right.Repository.Identity); compared != 0 {
			return compared
		}
		return cmp.Compare(left.SuiteKey, right.SuiteKey)
	})
	result.Mappings = append([]Mapping(nil), evidence.Mappings...)
	slices.SortFunc(result.Mappings, func(left, right Mapping) int {
		if compared := cmp.Compare(left.Kind, right.Kind); compared != 0 {
			return compared
		}
		if compared := cmp.Compare(left.Key, right.Key); compared != 0 {
			return compared
		}
		if compared := cmp.Compare(left.SourceRoot, right.SourceRoot); compared != 0 {
			return compared
		}

		return catalog.CompareTestIdentities(left.Test, right.Test)
	})

	return result
}

// DigestEvidence is the stable SHA-256 used for exact-retry comparison and
// selection provenance. Callers validate the evidence first.
func DigestEvidence(evidence Evidence) (string, error) {
	encoded, err := json.Marshal(CanonicalEvidence(evidence))
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(encoded)

	return hex.EncodeToString(sum[:]), nil
}

// ValidateEvidence checks collector and reviewed mapping claims against the
// immutable descriptor. Incomplete observations remain valid diagnostics; a
// selector must require both completeness flags before omitting any test.
func ValidateEvidence(evidence Evidence, snapshot catalog.Snapshot) error {
	if err := validateEvidenceHeader(evidence, snapshot); err != nil {
		return err
	}
	declared, err := validateObservedSuites(evidence, snapshot)
	if err != nil {
		return err
	}

	return validateReviewedMappings(evidence, snapshot, declared)
}

func validateEvidenceHeader(evidence Evidence, snapshot catalog.Snapshot) error {
	if evidence.APIVersion != EvidenceAPIVersion || !catalog.IsLocalKey(evidence.ObservationID) ||
		evidence.Catalog.Key() != snapshot.Key() || !evidence.Catalog.Key().Valid() ||
		strings.TrimSpace(evidence.CollectorVersion) == "" || len(evidence.CollectorVersion) > 127 ||
		evidence.CollectedAt.IsZero() || !evidence.ExpiresAt.After(evidence.CollectedAt) ||
		!evidence.MappingRevision.Valid() || len(evidence.Suites) > maxCases ||
		len(evidence.Mappings) > maxMappings {
		return fmt.Errorf("%w: header", ErrInvalidEvidence)
	}

	return nil
}

func validateObservedSuites(evidence Evidence, snapshot catalog.Snapshot) (map[catalog.TestIdentity]struct{}, error) {
	declared := make(map[catalog.TestIdentity]struct{})
	suiteKeys := make(map[string]catalog.TestSuite)
	for _, suite := range snapshot.TestSuites {
		if suite.Family != catalog.TestFamilyFunctionalUI || suite.Adapter != "playwright" {
			continue
		}
		key := suiteIdentity(suite.Repository.Identity, suite.Key)
		suiteKeys[key] = suite
		for _, test := range suite.Tests {
			declared[catalog.TestIdentity{TestRepository: suite.Repository.Identity, SuiteKey: suite.Key, TestKey: test.Key}] = struct{}{}
		}
	}
	observedSuites := make(map[string]struct{}, len(evidence.Suites))
	observedTests := make(map[catalog.TestIdentity]struct{})
	totalCases := 0
	for _, suite := range evidence.Suites {
		key := suiteIdentity(suite.Repository.Identity, suite.SuiteKey)
		declaration, exists := suiteKeys[key]
		if !exists || !suite.Revision.Valid() || !suite.Repository.Valid() ||
			declaration.Repository != suite.Repository {
			return nil, fmt.Errorf("%w: suite identity", ErrInvalidEvidence)
		}
		if _, duplicate := observedSuites[key]; duplicate {
			return nil, fmt.Errorf("%w: duplicate suite", ErrInvalidEvidence)
		}
		observedSuites[key] = struct{}{}
		if err := validateSuiteCases(suite, declared, observedTests); err != nil {
			return nil, err
		}
		totalCases += len(suite.Cases)
	}
	if totalCases > maxCases {
		return nil, fmt.Errorf("%w: case bound", ErrInvalidEvidence)
	}
	if evidence.Complete && (len(observedSuites) != len(suiteKeys) || len(observedTests) != len(declared)) {
		return nil, fmt.Errorf("%w: incomplete collection", ErrInvalidEvidence)
	}

	return declared, nil
}

func validateSuiteCases(suite SuiteObservation, declared,
	observedTests map[catalog.TestIdentity]struct{}) error {
	variants := make(map[string]struct{}, len(suite.Cases))
	for _, item := range suite.Cases {
		identity := catalog.TestIdentity{TestRepository: suite.Repository.Identity, SuiteKey: suite.SuiteKey, TestKey: item.Key}
		if _, exists := declared[identity]; !exists || !validCase(item) {
			return fmt.Errorf("%w: case", ErrInvalidEvidence)
		}
		variant := item.Key + "\x00" + item.Project
		if _, duplicate := variants[variant]; duplicate {
			return fmt.Errorf("%w: duplicate project variant", ErrInvalidEvidence)
		}
		variants[variant] = struct{}{}
		observedTests[identity] = struct{}{}
	}

	return nil
}

func validateReviewedMappings(evidence Evidence, snapshot catalog.Snapshot, declared map[catalog.TestIdentity]struct{}) error {
	components := make(map[string]string, len(snapshot.Components))
	for _, component := range snapshot.Components {
		components[component.Key] = component.Root
	}
	routeRoots := make(map[string]Mapping)
	mappedTests := make(map[catalog.TestIdentity]struct{})
	seenMappings := make(map[Mapping]struct{}, len(evidence.Mappings))
	for _, mapping := range evidence.Mappings {
		if _, exists := declared[mapping.Test]; !exists || !validRoot(mapping.SourceRoot) ||
			!catalog.IsLocalKey(mapping.Key) {
			return fmt.Errorf("%w: mapping identity", ErrInvalidEvidence)
		}
		if _, duplicate := seenMappings[mapping]; duplicate {
			return fmt.Errorf("%w: duplicate mapping", ErrInvalidEvidence)
		}
		seenMappings[mapping] = struct{}{}
		if err := validateMappingRoot(mapping, components, routeRoots); err != nil {
			return err
		}
		mappedTests[mapping.Test] = struct{}{}
	}
	if evidence.MappingComplete && (!evidence.Complete || len(mappedTests) != len(declared)) {
		return fmt.Errorf("%w: incomplete mappings", ErrInvalidEvidence)
	}

	return nil
}

func validateMappingRoot(mapping Mapping, components map[string]string, routeRoots map[string]Mapping) error {
	switch mapping.Kind {
	case MappingComponent:
		if mapping.RoutePattern != "" || components[mapping.Key] != mapping.SourceRoot {
			return fmt.Errorf("%w: component root", ErrInvalidEvidence)
		}
	case MappingRoute:
		if !validRoute(mapping.RoutePattern) {
			return fmt.Errorf("%w: route pattern", ErrInvalidEvidence)
		}
		if previous, exists := routeRoots[mapping.Key]; exists &&
			(previous.SourceRoot != mapping.SourceRoot || previous.RoutePattern != mapping.RoutePattern) {
			return fmt.Errorf("%w: contradictory route roots", ErrInvalidEvidence)
		}
		routeRoots[mapping.Key] = mapping
	default:
		return fmt.Errorf("%w: mapping kind", ErrInvalidEvidence)
	}

	return nil
}

func suiteIdentity(repository catalog.RepositoryIdentity, suite string) string {
	return string(repository.Provider) + "\x00" + repository.Host + "\x00" + repository.ProviderRepositoryID + "\x00" + suite
}

func compareRepository(left, right catalog.RepositoryIdentity) int {
	return cmp.Compare(suiteIdentity(left, ""), suiteIdentity(right, ""))
}

func validCase(item Case) bool {
	if !catalog.IsLocalKey(item.Key) || strings.TrimSpace(item.Project) == "" || len(item.Project) > 127 ||
		!validRoot(item.File) || strings.TrimSpace(item.Title) == "" || len(item.Title) > 255 ||
		len(item.Tags) > 64 || len(item.Owners) > 64 {
		return false
	}
	for _, value := range append(append([]string(nil), item.Tags...), item.Owners...) {
		if strings.TrimSpace(value) == "" || len(value) > 127 || strings.ContainsAny(value, "\x00\r\n") {
			return false
		}
	}

	return true
}

func validRoot(value string) bool {
	return value != "" && len(value) <= 1024 && path.Clean(value) == value && value != "." &&
		!strings.HasPrefix(value, "/") && value != ".." && !strings.HasPrefix(value, "../") &&
		!strings.Contains(value, "\\") && !(len(value) >= 2 && value[1] == ':')
}

func validRoute(value string) bool {
	return strings.HasPrefix(value, "/") && len(value) <= 512 && !strings.ContainsAny(value, "?#\\\x00\r\n") &&
		!strings.Contains(value, "//") && path.Clean(value) == value
}

func canonicalStrings(values []string) []string {
	result := append([]string{}, values...)
	slices.Sort(result)
	return slices.Compact(result)
}
