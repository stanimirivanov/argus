package contract

import (
	"fmt"

	"github.com/stanimirivanov/argus/internal/contracts"
	"github.com/stanimirivanov/argus/internal/execution/shadow"
)

// ImportExecutionPlanAttemptBindingsV1 converts structurally valid attempt
// bindings into canonical shadow-evaluation input.
func ImportExecutionPlanAttemptBindingsV1(
	document contracts.ExecutionPlanAttemptBindingsV1,
) (shadow.PlanAttemptBindings, error) {
	if err := contracts.ValidateExecutionPlanAttemptBindingsV1(document); err != nil {
		return shadow.PlanAttemptBindings{}, err
	}
	bindings := shadow.PlanAttemptBindings{
		APIVersion: document.APIVersion,
		Groups:     make([]shadow.GroupAttemptBinding, 0, len(document.Groups)),
	}
	for _, group := range document.Groups {
		bindings.Groups = append(bindings.Groups, shadow.GroupAttemptBinding{
			GroupKey: group.GroupKey, SelectedAttemptID: group.SelectedAttemptID,
			FullSuiteAttemptID: group.FullSuiteAttemptID,
		})
	}
	bindings = shadow.CanonicalPlanAttemptBindings(bindings)
	if err := shadow.ValidatePlanAttemptBindings(bindings); err != nil {
		return shadow.PlanAttemptBindings{}, fmt.Errorf("validate execution plan attempt bindings: %w", err)
	}

	return bindings, nil
}

// ExportSelectionPlanShadowReportV1 converts aggregate plan evidence to its
// public versioned contract.
func ExportSelectionPlanShadowReportV1(
	report shadow.PlanReport,
) (contracts.SelectionPlanShadowReportV1, error) {
	groups := make([]contracts.SelectionPlanShadowGroup, 0, len(report.Groups))
	for _, group := range report.Groups {
		groups = append(groups, contracts.SelectionPlanShadowGroup{
			GroupKey:       group.GroupKey,
			TestRepository: repositoryReference(group.TestRepository),
			TestRevision:   revisionReference(group.TestRevision),
			Adapter: contracts.AdapterIdentity{
				ID: group.AdapterID, Version: group.AdapterVersion,
			},
			SelectedAttemptID: group.SelectedAttemptID, FullSuiteAttemptID: group.FullSuiteAttemptID,
			SelectedTestCount: group.SelectedTestCount, FullSuiteTestCount: group.FullSuiteTestCount,
			SelectedDurationMS: group.SelectedDurationMS, FullSuiteDurationMS: group.FullSuiteDurationMS,
			DurationReductionMS:         group.DurationReductionMS,
			SelectedFailureCount:        group.SelectedFailureCount,
			FullSuiteFailureCount:       group.FullSuiteFailureCount,
			CaughtFullSuiteFailureCount: group.CaughtFullSuiteFailureCount,
			FailureRecallBasisPoints:    group.FailureRecallBasisPoints,
			MissedFailures:              exportFailureMisses(group.MissedFailures),
		})
	}
	document := contracts.SelectionPlanShadowReportV1{
		APIVersion: report.APIVersion,
		Plan: contracts.PlanDigestReference{
			APIVersion: report.Plan.APIVersion, SHA256: report.Plan.SHA256,
		},
		Manifest: contracts.ManifestDigestReference{
			APIVersion: report.Manifest.APIVersion, SHA256: report.Manifest.SHA256,
		},
		GroupCount: report.GroupCount, GroupsWithSelection: report.GroupsWithSelection,
		FullOnlyGroupCount: report.FullOnlyGroupCount,
		SelectedTestCount:  report.SelectedTestCount, FullSuiteTestCount: report.FullSuiteTestCount,
		SelectedDurationMS: report.SelectedDurationMS, FullSuiteDurationMS: report.FullSuiteDurationMS,
		DurationReductionMS:         report.DurationReductionMS,
		SelectedFailureCount:        report.SelectedFailureCount,
		FullSuiteFailureCount:       report.FullSuiteFailureCount,
		CaughtFullSuiteFailureCount: report.CaughtFullSuiteFailureCount,
		FailureRecallBasisPoints:    report.FailureRecallBasisPoints,
		Groups:                      groups,
	}
	if err := contracts.ValidateSelectionPlanShadowReportV1(document); err != nil {
		return contracts.SelectionPlanShadowReportV1{}, fmt.Errorf("export selection plan shadow report: %w", err)
	}

	return document, nil
}
