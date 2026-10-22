package cli

import (
	"time"

	"github.com/stanimirivanov/argus/internal/contracts"
	"github.com/stanimirivanov/argus/internal/predictive"
)

func safetyStamp(value *time.Time) *string {
	if value == nil {
		return nil
	}
	stamp := value.UTC().Format(time.RFC3339Nano)

	return &stamp
}

func exportSafety(report predictive.SafetyReport, inputSHA string) contracts.PredictiveSafetyReportV1 {
	output := contracts.PredictiveSafetyReportV1{APIVersion: predictive.SafetyVersion, InputSHA256: inputSHA, WindowStart: report.Window.Start.UTC().Format(time.RFC3339Nano), WindowEnd: report.Window.End.UTC().Format(time.RFC3339Nano), AsOf: report.Window.AsOf.UTC().Format(time.RFC3339Nano), Groups: []contracts.PredictiveSafetyGroup{}, Cohorts: []contracts.PredictiveSafetyCohort{}, Unavailable: report.Unavailable}
	for _, g := range report.Groups {
		output.Groups = append(output.Groups, exportSafetyGroup(g))
	}
	for _, c := range report.Cohorts {
		output.Cohorts = append(output.Cohorts, contracts.PredictiveSafetyCohort{PredictiveSafetyMetrics: contracts.PredictiveSafetyMetrics{Family: string(c.Family), SelectionRate: c.SelectionRate, Failures: c.Failures, CaughtFailures: c.CaughtFailures, Recall: c.Recall}, Provider: string(c.Repository.Provider), Host: c.Repository.Host, ProviderRepositoryID: c.Repository.ProviderRepositoryID, Groups: c.Groups, CompleteGroups: c.CompleteGroups, Candidates: c.Candidates, Early: c.Early})
	}

	return output
}

func exportSafetyGroup(g predictive.SafetyGroup) contracts.PredictiveSafetyGroup {
	i := g.Input
	value := contracts.PredictiveSafetyGroup{FeatureCutoffAt: i.FeatureCutoffAt.UTC().Format(time.RFC3339Nano), PredictiveSafetyMetrics: contracts.PredictiveSafetyMetrics{Family: string(i.Scope.Family), SelectionRate: g.SelectionRate, Failures: g.Failures, CaughtFailures: g.CaughtFailures, Recall: g.Recall}, RunID: i.RunID, EpisodeID: i.EpisodeID, GroupKey: i.GroupKey, Mode: i.Mode, Schedule: contracts.ManifestDigestReference{APIVersion: i.Schedule.APIVersion, SHA256: i.Schedule.SHA256}, Manifest: contracts.ManifestDigestReference{APIVersion: i.Manifest.APIVersion, SHA256: i.Manifest.SHA256}, PlanSHA256: i.PlanSHA256, PolicyVersion: i.PolicyVersion,
		TestRepository: repositoryRef(i.Scope.Repository), TestRevision: revisionRef(i.Scope.Revision), Adapter: i.Scope.Adapter, Project: i.Scope.Project, Environment: i.Scope.Environment, ModelSHA256: i.ModelSHA256, BaselineECE: i.BaselineECE, State: g.State, CandidateCount: g.CandidateCount, EarlyCount: g.EarlyCount, UnknownLabels: g.UnknownLabels, Brier: g.Brier, ECE: g.ECE, ECEDrift: g.ECEDrift, FirstFailureMS: g.FirstFailureMS, Misses: []contracts.PredictiveSafetyMiss{}, Phases: []contracts.PredictiveSafetyPhase{}, Unavailable: []string{}}
	if i.DiscoverySHA256 != "" {
		digest := i.DiscoverySHA256
		value.DiscoverySHA256 = &digest
	}
	if i.Calibration != nil {
		value.CalibrationCutoffAt = safetyStamp(&i.CalibrationCutoffAt)
		value.Calibration = &contracts.ManifestDigestReference{APIVersion: i.Calibration.APIVersion, SHA256: i.Calibration.SHA256}
	}
	if g.State != "complete" {
		value.Unavailable = append(value.Unavailable, "complete-control-metrics-pending-or-unknown")
	}
	if g.Brier == nil {
		value.Unavailable = append(value.Unavailable, "complete-matched-calibration-labels-unavailable")
	}
	if g.ECEDrift == nil {
		value.Unavailable = append(value.Unavailable, "comparable-baseline-drift-unavailable")
	}
	if g.FirstFailureMS == nil {
		value.Unavailable = append(value.Unavailable, "no-complete-observed-failure-time")
	}
	if g.Recall == nil {
		value.Unavailable = append(value.Unavailable, "recall-undefined-without-complete-failing-controls")
	}
	for _, m := range g.Misses {
		value.Misses = append(value.Misses, contracts.PredictiveSafetyMiss{ID: m.ID, SuiteKey: m.SuiteKey, TestKey: m.TestKey, Reason: m.Reason})
	}
	for _, p := range g.Phases {
		value.Phases = append(value.Phases, contracts.PredictiveSafetyPhase{Stage: string(p.Stage), State: p.State, AttemptID: p.AttemptID, StartedAt: safetyStamp(p.StartedAt), CompletedAt: safetyStamp(p.CompletedAt), DurationMS: p.DurationMS, ElapsedMS: p.ElapsedMS, EstimatedDurationMS: p.EstimatedDurationMS, EstimatedResourceUnitMS: p.EstimatedResourceUnitMS})
	}

	return value
}
