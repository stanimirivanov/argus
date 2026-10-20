// Package predictive owns immutable, framework-neutral evidence for offline selection evaluation.
package predictive

import (
	"errors"
	"fmt"
	"slices"
	"time"

	"github.com/stanimirivanov/argus/internal/catalog"
	"github.com/stanimirivanov/argus/internal/change"
	"github.com/stanimirivanov/argus/internal/execution"
	"github.com/stanimirivanov/argus/internal/selection"
)

// ObservationVersion is the first immutable offline observation contract.
const ObservationVersion = "argus.dev/predictive-observation/v1"

// ErrInvalid means evidence cannot safely enter predictive evaluation.
var ErrInvalid = errors.New("invalid predictive observation")

// Candidate is a decision-time test feature, before its control outcome is known.
type Candidate struct {
	Repository   catalog.Repository
	Revision     catalog.Revision
	SuiteKey     string
	TestKey      string
	Adapter      string
	Outcome      selection.Outcome
	Reasons      []string
	Capabilities []string
	MappingKeys  []string
	Projects     []string
}

// Control is one complete, explicitly named full-suite group attempt.
type Control struct {
	AttemptID   string
	Manifest    execution.ManifestReference
	Stage       execution.Stage
	Repository  catalog.Repository
	Revision    catalog.Revision
	Adapter     string
	Project     string
	Environment string
	StartedAt   time.Time
	CompletedAt time.Time
	Results     []execution.TestResult
}

// Input binds an immutable change and manifest to one control group.
type Input struct {
	Change          change.Set
	ManifestChange  change.Reference
	Manifest        execution.ManifestReference
	FeatureCutoffAt time.Time
	Family          catalog.TestFamily
	Candidates      []Candidate
	Control         Control
}

// FileFeature describes a changed path without retaining raw patch text.
type FileFeature struct {
	Path        string
	Kind        change.Kind
	Additions   int
	Deletions   int
	PatchStatus change.PatchStatus
}

// TestObservation keeps pre-control decision features separate from the label.
type TestObservation struct {
	SuiteKey     string
	TestKey      string
	Decision     selection.Outcome
	Reasons      []string
	Capabilities []string
	MappingKeys  []string
	Outcome      execution.TestOutcome
	DurationMS   int64
}

// Observation excludes raw patches, failures, and artifacts; its labels remain
// separate from decision-time features for later leakage-safe replay.
type Observation struct {
	APIVersion         string
	Change             change.Reference
	Files              []FileFeature
	FilesTruncated     bool
	Manifest           execution.ManifestReference
	FeatureCutoffAt    time.Time
	Family             catalog.TestFamily
	Repository         catalog.Repository
	Revision           catalog.Revision
	Adapter            string
	Project            string
	Environment        string
	ControlAttemptID   string
	ControlStartedAt   time.Time
	ControlCompletedAt time.Time
	Tests              []TestObservation
}

// Build rejects mismatched provenance and incomplete control membership before
// emitting any observation. Each observation covers one executable group.
func Build(input Input) (Observation, error) {
	if err := validateInput(input); err != nil {
		return Observation{}, err
	}
	control := input.Control
	observation := Observation{APIVersion: ObservationVersion, Change: input.Change.Reference(),
		Manifest: input.Manifest, FeatureCutoffAt: input.FeatureCutoffAt.UTC(), Family: input.Family,
		Repository: control.Repository, Revision: control.Revision, Adapter: control.Adapter,
		Project: control.Project, Environment: control.Environment, ControlAttemptID: control.AttemptID,
		ControlStartedAt: control.StartedAt.UTC(), ControlCompletedAt: control.CompletedAt.UTC(),
		Files: make([]FileFeature, 0, len(input.Change.Files)), FilesTruncated: input.Change.FilesTruncated}
	for _, file := range input.Change.Files {
		observation.Files = append(observation.Files, FileFeature{Path: file.Path, Kind: file.Kind, Additions: file.Additions, Deletions: file.Deletions, PatchStatus: file.PatchStatus})
	}
	slices.SortFunc(observation.Files, func(a, b FileFeature) int { return compare(a.Path, b.Path) })
	tests, err := buildTests(input)
	if err != nil {
		return Observation{}, err
	}
	observation.Tests = tests

	return observation, nil
}

func validateInput(input Input) error {
	control := input.Control
	if change.ValidateSet(input.Change) != nil || input.ManifestChange != input.Change.Reference() ||
		input.FeatureCutoffAt.IsZero() || input.FeatureCutoffAt.Before(input.Change.ObservedAt) ||
		input.FeatureCutoffAt.After(control.StartedAt) || control.StartedAt.IsZero() ||
		control.CompletedAt.IsZero() || control.CompletedAt.Before(control.StartedAt) ||
		execution.ValidateAttemptID(control.AttemptID) != nil || control.Stage != execution.StageFullSuite ||
		!control.Repository.Valid() || !control.Revision.Valid() || !catalog.IsLocalKey(control.Adapter) ||
		control.Manifest != input.Manifest || len(input.Candidates) == 0 || len(input.Candidates) > execution.MaxAttemptTests {
		return ErrInvalid
	}
	if input.Family != catalog.TestFamilyFunctionalAPI && input.Family != catalog.TestFamilyFunctionalUI {
		return fmt.Errorf("%w: unsupported family", ErrInvalid)
	}
	if input.Family == catalog.TestFamilyFunctionalAPI && (control.Project != "" || control.Environment != "") {
		return fmt.Errorf("%w: API group coordinates", ErrInvalid)
	}
	if input.Family == catalog.TestFamilyFunctionalUI && (!catalog.IsLocalKey(control.Project) || !catalog.IsLocalKey(control.Environment)) {
		return fmt.Errorf("%w: browser group coordinates", ErrInvalid)
	}

	return nil
}

func buildTests(input Input) ([]TestObservation, error) {
	control := input.Control
	results, err := indexedResults(control.Results)
	if err != nil {
		return nil, err
	}
	seen := make(map[string]bool, len(input.Candidates))
	tests := make([]TestObservation, 0, len(input.Candidates))
	for _, candidate := range input.Candidates {
		if candidate.Repository.Identity != control.Repository.Identity || candidate.Adapter != control.Adapter {
			continue
		}
		if candidate.Repository != control.Repository || candidate.Revision != control.Revision {
			return nil, fmt.Errorf("%w: test group revision", ErrInvalid)
		}
		if input.Family == catalog.TestFamilyFunctionalUI && len(candidate.Projects) > 0 && !slices.Contains(candidate.Projects, control.Project) {
			continue
		}
		key := candidate.SuiteKey + "\x00" + candidate.TestKey
		result, exists := results[key]
		if !exists || seen[key] || result.Duration < 0 || result.Duration > execution.MaxTestDuration {
			return nil, fmt.Errorf("%w: control membership", ErrInvalid)
		}
		seen[key] = true
		tests = append(tests, TestObservation{SuiteKey: candidate.SuiteKey,
			TestKey: candidate.TestKey, Decision: candidate.Outcome,
			Reasons: sorted(candidate.Reasons), Capabilities: sorted(candidate.Capabilities), MappingKeys: sorted(candidate.MappingKeys),
			Outcome: result.Outcome, DurationMS: result.Duration.Milliseconds()})
	}
	if len(tests) == 0 || len(seen) != len(results) {
		return nil, fmt.Errorf("%w: incomplete control coverage", ErrInvalid)
	}
	slices.SortFunc(tests, func(a, b TestObservation) int {
		if n := compare(a.SuiteKey, b.SuiteKey); n != 0 {
			return n
		}
		return compare(a.TestKey, b.TestKey)
	})

	return tests, nil
}

func indexedResults(values []execution.TestResult) (map[string]execution.TestResult, error) {
	results := make(map[string]execution.TestResult, len(values))
	for _, result := range values {
		key := result.SuiteKey + "\x00" + result.TestKey
		if _, exists := results[key]; exists || result.Outcome == execution.TestSkipped || result.Outcome == execution.TestError {
			return nil, fmt.Errorf("%w: incomplete control result", ErrInvalid)
		}
		results[key] = result
	}

	return results, nil
}

func sorted(values []string) []string {
	copyOf := append([]string{}, values...)
	slices.Sort(copyOf)

	return copyOf
}

func compare(a, b string) int {
	if a < b {
		return -1
	}
	if a > b {
		return 1
	}

	return 0
}
