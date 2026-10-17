package functionalapi

import (
	"testing"

	"github.com/stanimirivanov/argus/internal/catalog"
	"github.com/stanimirivanov/argus/internal/change"
	"github.com/stanimirivanov/argus/internal/selection"
)

func TestAssessMaintenanceKeepsUncertaintySeparateFromPatchEligibility(t *testing.T) {
	t.Parallel()
	impact := validImpact()
	test := validTest()
	manifest := selection.Manifest{
		APIVersion:       selection.ManifestAPIVersion,
		PolicyVersion:    selection.FunctionalAPIPolicyVersion,
		ImpactAPIVersion: impact.APIVersion, ImpactAnalyzerVersion: impact.AnalyzerVersion,
		Change: impact.Change,
		Catalog: catalog.SnapshotReference{
			SourceRepository: impact.Change.SourceRepository, Revision: impact.Change.BaseRevision,
			DescriptorAPIVersion: "argus.dev/repository-descriptor/v1",
		},
		Family: catalog.TestFamilyFunctionalAPI, Mode: selection.ModeTargeted,
		AffectedCapabilities: []string{"list-orders"},
		Decisions: []selection.Decision{{
			Test: selection.TestReference{
				Repository: test.Repository, SuiteKey: test.SuiteKey, TestKey: test.TestKey,
				Name: test.Name, Adapter: test.Adapter, Family: catalog.TestFamilyFunctionalAPI,
				Capabilities: test.Capabilities,
			},
			Outcome: selection.OutcomeRunRequired, RemainingExecution: selection.RemainingNone,
			Reasons: []selection.Reason{{Code: selection.ReasonDirectCapabilityImpact, Capabilities: []string{"list-orders"}}},
		}},
	}
	base := Observation{RepositoryID: test.Repository.Identity.ProviderRepositoryID,
		SuiteKey: test.SuiteKey, TestKey: test.TestKey, AttemptID: "attempt-1"}
	partial := validImpact()
	partial.Status = change.ImpactPartial
	partial.Warnings = []string{"unavailable source evidence"}
	cases := []struct {
		name         string
		impact       change.CapabilityImpact
		observations []Observation
		want         MaintenanceState
	}{
		{"passed", impact, []Observation{{RepositoryID: base.RepositoryID, SuiteKey: base.SuiteKey,
			TestKey: base.TestKey, AttemptID: base.AttemptID, Outcome: "passed"}}, MaintenanceValid},
		{"failed", impact, []Observation{{RepositoryID: base.RepositoryID, SuiteKey: base.SuiteKey,
			TestKey: base.TestKey, AttemptID: base.AttemptID, Outcome: "failed"}}, MaintenanceInvalidated},
		{"missing", impact, nil, MaintenanceUncertain},
		{"conflicting", impact, []Observation{{RepositoryID: base.RepositoryID, SuiteKey: base.SuiteKey,
			TestKey: base.TestKey, AttemptID: "attempt-1", Outcome: "passed"},
			{RepositoryID: base.RepositoryID, SuiteKey: base.SuiteKey,
				TestKey: base.TestKey, AttemptID: "attempt-2", Outcome: "failed"}}, MaintenanceUncertain},
		{"partial impact", partial, []Observation{{RepositoryID: base.RepositoryID,
			SuiteKey: base.SuiteKey, TestKey: base.TestKey, AttemptID: base.AttemptID,
			Outcome: "failed"}}, MaintenanceUncertain},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			report, err := AssessMaintenance(tc.impact, manifest, tc.observations)
			if err != nil || len(report.Items) != 1 || report.Items[0].State != tc.want {
				t.Fatalf("assessment = %+v, %v", report, err)
			}
		})
	}
	manifest.UncoveredCapabilities = []string{"list-orders"}
	report, err := AssessMaintenance(impact, manifest, nil)
	if err != nil || len(report.Items) != 2 || report.Items[1].State != MaintenanceUncovered {
		t.Fatalf("uncovered assessment = %+v, %v", report, err)
	}
}
