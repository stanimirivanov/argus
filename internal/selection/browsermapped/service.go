// Package browsermapped selects stable Playwright tests from verified changed
// paths, immutable discovery, and reviewed route/component mappings.
package browsermapped

import (
	"cmp"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/stanimirivanov/argus/internal/catalog"
	"github.com/stanimirivanov/argus/internal/catalog/discovery"
	"github.com/stanimirivanov/argus/internal/selection"
)

const (
	// ManifestAPIVersion identifies the first discovery-bound browser selection.
	ManifestAPIVersion = "argus.dev/browser-selection-manifest/v1"
	// PolicyVersion identifies exact route/component mapping with conservative fallback.
	PolicyVersion = "argus.dev/selection-policy/browser-mapped/v1"
)

var (
	// ErrInvalid means the caller supplied inconsistent selection bindings.
	ErrInvalid = errors.New("invalid browser mapped selection")
	// ErrUnavailable means durable evidence cannot support a trustworthy result.
	ErrUnavailable = errors.New("browser mapped selection unavailable")
)

// SuiteBinding pins the intended test checkout for one browser suite. A
// collected revision cannot appoint itself the revision CI will execute.
type SuiteBinding struct {
	Repository catalog.RepositoryIdentity
	SuiteKey   string
	Revision   catalog.Revision
}

// Request contains all immutable inputs needed for one browser decision.
type Request struct {
	Impact      selection.BrowserPathProjection
	Catalog     catalog.Snapshot
	Evidence    *discovery.Evidence
	Bindings    []SuiteBinding
	EvaluatedAt time.Time
}

// EvidenceReference records exactly which discovery and reviewed mappings
// authorized a targeted decision.
type EvidenceReference struct {
	ObservationID    string
	Digest           string
	CollectorVersion string
	MappingRevision  catalog.Revision
}

// Decision contains one stable test, its project variants, and the rule that
// requires or defers it. Omitted tests retain a full-suite obligation.
type Decision struct {
	Repository         catalog.Repository
	SuiteKey           string
	TestKey            string
	Name               string
	Revision           catalog.Revision
	Projects           []string
	Outcome            selection.Outcome
	RemainingExecution selection.RemainingExecution
	Reason             string
	MappingKeys        []string
}

// Manifest is a browser-owned selection result, separate from functional API
// execution manifests and from the earlier advisory UI v2/v3 policies.
type Manifest struct {
	APIVersion    string
	PolicyVersion string
	Impact        selection.BrowserPathProjection
	Catalog       catalog.SnapshotReference
	Evidence      *EvidenceReference
	EvaluatedAt   time.Time
	Mode          selection.Mode
	Decisions     []Decision
	Warnings      []string
}

// Select requires every declared Playwright suite to have an explicit CI
// checkout binding. It widens to all candidates for stale, incomplete, or
// unmapped evidence; malformed durable evidence is rejected.
func Select(request Request) (Manifest, error) {
	if err := selection.ValidateBrowserPathProjection(request.Impact); err != nil || !request.Catalog.Key().Valid() ||
		request.Catalog.Repository.Identity != request.Impact.Change.SourceRepository.Identity ||
		request.Catalog.Revision != request.Impact.Change.BaseRevision || request.EvaluatedAt.IsZero() {
		return Manifest{}, ErrInvalid
	}
	suites, bindings, err := indexSuites(request.Catalog, request.Bindings)
	if err != nil {
		return Manifest{}, err
	}
	manifest := Manifest{APIVersion: ManifestAPIVersion, PolicyVersion: PolicyVersion,
		Impact: request.Impact, Catalog: catalog.SnapshotReference{SourceRepository: request.Catalog.Repository,
			Revision: request.Catalog.Revision, DescriptorAPIVersion: request.Catalog.APIVersion},
		EvaluatedAt: request.EvaluatedAt.UTC(), Mode: selection.ModeFallback}
	if len(suites) == 0 {
		return Manifest{}, fmt.Errorf("%w: no Playwright UI suites", ErrInvalid)
	}
	eligible, matches, projects, err := evaluateDiscovery(request, bindings, &manifest)
	if err != nil {
		return Manifest{}, err
	}
	if eligible {
		manifest.Mode = selection.ModeTargeted
	}
	manifest.Decisions = buildDecisions(suites, bindings, matches, projects, eligible)
	slices.SortFunc(manifest.Decisions, func(left, right Decision) int {
		return catalog.CompareTestIdentities(catalog.TestIdentity{TestRepository: left.Repository.Identity,
			SuiteKey: left.SuiteKey, TestKey: left.TestKey}, catalog.TestIdentity{TestRepository: right.Repository.Identity,
			SuiteKey: right.SuiteKey, TestKey: right.TestKey})
	})
	manifest.Warnings = sortedUnique(manifest.Warnings)

	return manifest, nil
}

func evaluateDiscovery(request Request, bindings map[string]catalog.Revision, manifest *Manifest) (
	bool, map[catalog.TestIdentity][]string, map[catalog.TestIdentity][]string, error,
) {
	matches := map[catalog.TestIdentity][]string{}
	projects := map[catalog.TestIdentity][]string{}
	if request.Evidence == nil {
		manifest.Warnings = append(manifest.Warnings, "browser discovery is missing; all UI tests are required")
		return false, matches, projects, nil
	}
	evidence := discovery.CanonicalEvidence(*request.Evidence)
	if err := discovery.ValidateEvidence(evidence, request.Catalog); err != nil {
		return false, nil, nil, fmt.Errorf("%w: %w", ErrUnavailable, err)
	}
	digest, err := discovery.DigestEvidence(evidence)
	if err != nil {
		return false, nil, nil, fmt.Errorf("%w: evidence digest: %w", ErrUnavailable, err)
	}
	manifest.Evidence = &EvidenceReference{ObservationID: evidence.ObservationID, Digest: digest,
		CollectorVersion: evidence.CollectorVersion, MappingRevision: evidence.MappingRevision}
	eligible := evidence.Complete && evidence.MappingComplete && evidence.MappingRevision == request.Catalog.Revision &&
		!request.EvaluatedAt.Before(evidence.CollectedAt) && request.EvaluatedAt.Before(evidence.ExpiresAt) &&
		sameRevisions(evidence.Suites, bindings)
	if !eligible {
		manifest.Warnings = append(manifest.Warnings, "browser discovery or reviewed mappings are incomplete, stale, or revision-mismatched; all UI tests are required")
	}
	for _, suite := range evidence.Suites {
		for _, item := range suite.Cases {
			id := catalog.TestIdentity{TestRepository: suite.Repository.Identity, SuiteKey: suite.SuiteKey, TestKey: item.Key}
			projects[id] = append(projects[id], item.Project)
		}
	}
	if !request.Impact.Complete {
		manifest.Warnings = append(manifest.Warnings, "changed-file evidence is incomplete; all UI tests are required")
		return false, matches, projects, nil
	}
	covered := projectMappedPaths(request.Impact.Paths, evidence.Mappings, matches)
	if !covered {
		manifest.Warnings = append(manifest.Warnings, "a changed path has no reviewed browser mapping; all UI tests are required")
	}

	return eligible && covered, matches, projects, nil
}

func projectMappedPaths(paths []string, mappings []discovery.Mapping, matches map[catalog.TestIdentity][]string) bool {
	covered := true
	for _, filePath := range paths {
		matchedPath := false
		for _, mapping := range mappings {
			if filePath != mapping.SourceRoot && !strings.HasPrefix(filePath, mapping.SourceRoot+"/") {
				continue
			}
			matchedPath = true
			matches[mapping.Test] = append(matches[mapping.Test], string(mapping.Kind)+":"+mapping.Key)
		}
		if !matchedPath {
			covered = false
		}
	}

	return covered
}

func buildDecisions(suites []catalog.TestSuite, bindings map[string]catalog.Revision,
	matches, projects map[catalog.TestIdentity][]string, eligible bool) []Decision {
	decisions := make([]Decision, 0)
	for _, suite := range suites {
		revision := bindings[suiteIdentity(suite.Repository.Identity, suite.Key)]
		for _, test := range suite.Tests {
			id := catalog.TestIdentity{TestRepository: suite.Repository.Identity, SuiteKey: suite.Key, TestKey: test.Key}
			decision := Decision{Repository: suite.Repository, SuiteKey: suite.Key, TestKey: test.Key,
				Name: test.Name, Revision: revision, Projects: sortedUnique(projects[id]), MappingKeys: sortedUnique(matches[id])}
			switch {
			case !eligible:
				decision.Projects, decision.MappingKeys = []string{}, []string{}
				decision.Outcome, decision.RemainingExecution, decision.Reason =
					selection.OutcomeRunRequired, selection.RemainingNone, "CONSERVATIVE_FALLBACK"
			case len(decision.MappingKeys) > 0:
				decision.Outcome, decision.RemainingExecution, decision.Reason =
					selection.OutcomeRunRequired, selection.RemainingNone, "DIRECT_UI_MAPPING"
			default:
				decision.Outcome, decision.RemainingExecution, decision.Reason =
					selection.OutcomeSkipForNow, selection.RemainingFullSuite, "NOT_AFFECTED_IN_EARLY_STAGE"
			}
			decisions = append(decisions, decision)
		}
	}

	return decisions
}

func indexSuites(snapshot catalog.Snapshot, requested []SuiteBinding) ([]catalog.TestSuite, map[string]catalog.Revision, error) {
	suites := make([]catalog.TestSuite, 0)
	for _, suite := range snapshot.TestSuites {
		if suite.Family == catalog.TestFamilyFunctionalUI && suite.Adapter == "playwright" {
			suites = append(suites, suite)
		}
	}
	bindings := make(map[string]catalog.Revision, len(requested))
	for _, binding := range requested {
		if !binding.Repository.Valid() || !catalog.IsLocalKey(binding.SuiteKey) || !binding.Revision.Valid() {
			return nil, nil, ErrInvalid
		}
		key := suiteIdentity(binding.Repository, binding.SuiteKey)
		if _, exists := bindings[key]; exists {
			return nil, nil, fmt.Errorf("%w: duplicate suite binding", ErrInvalid)
		}
		bindings[key] = binding.Revision
	}
	if len(bindings) != len(suites) {
		return nil, nil, fmt.Errorf("%w: suite binding coverage", ErrInvalid)
	}
	for _, suite := range suites {
		if _, exists := bindings[suiteIdentity(suite.Repository.Identity, suite.Key)]; !exists {
			return nil, nil, fmt.Errorf("%w: unknown suite binding", ErrInvalid)
		}
	}
	slices.SortFunc(suites, func(left, right catalog.TestSuite) int {
		return cmp.Compare(suiteIdentity(left.Repository.Identity, left.Key), suiteIdentity(right.Repository.Identity, right.Key))
	})

	return suites, bindings, nil
}

func sameRevisions(suites []discovery.SuiteObservation, bindings map[string]catalog.Revision) bool {
	if len(suites) != len(bindings) {
		return false
	}
	for _, suite := range suites {
		if bindings[suiteIdentity(suite.Repository.Identity, suite.SuiteKey)] != suite.Revision {
			return false
		}
	}

	return true
}

func suiteIdentity(repository catalog.RepositoryIdentity, suite string) string {
	return string(repository.Provider) + "\x00" + repository.Host + "\x00" + repository.ProviderRepositoryID + "\x00" + suite
}

func sortedUnique(values []string) []string {
	result := append([]string{}, values...)
	slices.Sort(result)
	return slices.Compact(result)
}
