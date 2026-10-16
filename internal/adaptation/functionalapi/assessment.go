package functionalapi

import (
	"fmt"
	"slices"

	"github.com/stanimirivanov/argus/internal/change"
	"github.com/stanimirivanov/argus/internal/selection"
)

// MaintenanceState is an assessment of evidence, never authorization to patch.
type MaintenanceState string

// Maintenance states distinguish observed behavior from coverage and uncertainty.
const (
	MaintenanceValid       MaintenanceState = "VALID"
	MaintenanceInvalidated MaintenanceState = "INVALIDATED"
	MaintenanceUncovered   MaintenanceState = "UNCOVERED"
	MaintenanceUncertain   MaintenanceState = "UNCERTAIN"
)

// Observation contains one normalized execution result correlated by the caller
// to the exact manifest, repository revision, and test identity.
type Observation struct {
	RepositoryID string `json:"repositoryId"`
	SuiteKey     string `json:"suiteKey"`
	TestKey      string `json:"testKey"`
	AttemptID    string `json:"attemptId"`
	Outcome      string `json:"outcome"`
}

// MaintenanceItem separates a test assessment from an uncovered capability.
type MaintenanceItem struct {
	RepositoryID string           `json:"repositoryId"`
	SuiteKey     string           `json:"suiteKey"`
	TestKey      string           `json:"testKey"`
	Capability   string           `json:"capability"`
	State        MaintenanceState `json:"state"`
	Reason       string           `json:"reason"`
	AttemptIDs   []string         `json:"attemptIds"`
}

// MaintenanceReport retains the immutable selection and change provenance.
type MaintenanceReport struct {
	APIVersion            string            `json:"apiVersion"`
	ImpactAPIVersion      string            `json:"impactApiVersion"`
	ImpactAnalyzerVersion string            `json:"impactAnalyzerVersion"`
	SourceRepositoryID    string            `json:"sourceRepositoryId"`
	BaseRevision          string            `json:"baseRevision"`
	HeadRevision          string            `json:"headRevision"`
	PolicyDigest          string            `json:"policyDigest"`
	InventoryDigest       string            `json:"inventoryDigest"`
	Items                 []MaintenanceItem `json:"items"`
}

// AssessMaintenance classifies complete execution evidence conservatively.
// Missing, partial, conflicting, or unsupported evidence is uncertain.
func AssessMaintenance(impact change.CapabilityImpact, manifest selection.Manifest, observations []Observation) (MaintenanceReport, error) {
	if err := change.ValidateCapabilityImpact(impact); err != nil {
		return MaintenanceReport{}, err
	}
	if err := selection.ValidateManifest(manifest); err != nil {
		return MaintenanceReport{}, err
	}
	manifest = selection.CanonicalManifest(manifest)
	if manifest.Change != impact.Change || manifest.ImpactAPIVersion != impact.APIVersion ||
		manifest.ImpactAnalyzerVersion != impact.AnalyzerVersion {
		return MaintenanceReport{}, fmt.Errorf("maintenance assessment provenance mismatch")
	}
	report := MaintenanceReport{
		APIVersion: "argus.dev/maintenance-assessment/v1", ImpactAPIVersion: impact.APIVersion,
		ImpactAnalyzerVersion: impact.AnalyzerVersion, PolicyDigest: manifest.PolicyDigest,
		SourceRepositoryID: impact.Change.SourceRepository.Identity.ProviderRepositoryID,
		BaseRevision:       impact.Change.BaseRevision.Digest, HeadRevision: impact.Change.HeadRevision.Digest,
		InventoryDigest: manifest.InventoryDigest, Items: []MaintenanceItem{},
	}
	for _, decision := range manifest.Decisions {
		report.Items = append(report.Items, assessDecision(impact, manifest.Mode, decision, observations))
	}
	for _, capability := range manifest.UncoveredCapabilities {
		report.Items = append(report.Items, MaintenanceItem{
			Capability: capability, State: MaintenanceUncovered, Reason: "no-mapped-test",
			AttemptIDs: []string{},
		})
	}

	return report, nil
}

func assessDecision(impact change.CapabilityImpact, mode selection.Mode, decision selection.Decision, observations []Observation) MaintenanceItem {
	item := MaintenanceItem{
		RepositoryID: decision.Test.Repository.Identity.ProviderRepositoryID,
		SuiteKey:     decision.Test.SuiteKey, TestKey: decision.Test.TestKey,
		AttemptIDs: []string{},
	}
	matched := matchingObservations(item, observations)
	for _, observation := range matched {
		item.AttemptIDs = append(item.AttemptIDs, observation.AttemptID)
	}
	slices.Sort(item.AttemptIDs)
	item.AttemptIDs = slices.Compact(item.AttemptIDs)
	item.State, item.Reason = classifyObservation(impact, mode, decision, matched)

	return item
}

func matchingObservations(item MaintenanceItem, observations []Observation) []Observation {
	matched := make([]Observation, 0)
	for _, observation := range observations {
		if observation.RepositoryID == item.RepositoryID && observation.SuiteKey == item.SuiteKey && observation.TestKey == item.TestKey {
			matched = append(matched, observation)
		}
	}

	return matched
}

func classifyObservation(impact change.CapabilityImpact, mode selection.Mode, decision selection.Decision, matched []Observation) (MaintenanceState, string) {
	if impact.Status != change.ImpactComplete || mode != selection.ModeTargeted {
		return MaintenanceUncertain, "impact-incomplete"
	}
	if len(matched) == 0 {
		return MaintenanceUncertain, "execution-evidence-missing"
	}
	for _, observation := range matched[1:] {
		if observation.Outcome != matched[0].Outcome {
			return MaintenanceUncertain, "conflicting-execution-evidence"
		}
	}
	switch matched[0].Outcome {
	case "passed":
		return MaintenanceValid, "test-passed"
	case "failed":
		if _, err := DetectEndpointRename(impact, decision.Test.Capabilities); err == nil {
			return MaintenanceInvalidated, "authoritative-endpoint-rename-and-test-failed"
		}
		return MaintenanceUncertain, "failure-without-supported-rename"
	default:
		return MaintenanceUncertain, "non-assertion-result"
	}
}
