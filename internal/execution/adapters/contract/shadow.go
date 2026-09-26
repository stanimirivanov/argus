package contract

import (
	"fmt"

	"github.com/stanimirivanov/argus/contracts"
	"github.com/stanimirivanov/argus/internal/execution/shadow"
)

// ExportSelectionShadowReportV1 converts a derived report to its public contract.
func ExportSelectionShadowReportV1(report shadow.Report) (contracts.SelectionShadowReportV1, error) {
	document := contracts.SelectionShadowReportV1{
		APIVersion: report.APIVersion,
		Manifest: contracts.ManifestDigestReference{
			APIVersion: report.Manifest.APIVersion, SHA256: report.Manifest.SHA256,
		},
		TestRepository:    repositoryReference(report.TestRepository),
		TestRevision:      revisionReference(report.TestRevision),
		Adapter:           contracts.AdapterIdentity{ID: report.AdapterID, Version: report.AdapterVersion},
		SelectedAttemptID: report.SelectedAttemptID, FullSuiteAttemptID: report.FullSuiteAttemptID,
		SelectedTestCount: report.SelectedTestCount, FullSuiteTestCount: report.FullSuiteTestCount,
		SelectedDurationMS:          report.SelectedDurationMS,
		FullSuiteDurationMS:         report.FullSuiteDurationMS,
		DurationReductionMS:         report.DurationReductionMS,
		SelectedFailureCount:        report.SelectedFailureCount,
		FullSuiteFailureCount:       report.FullSuiteFailureCount,
		CaughtFullSuiteFailureCount: report.CaughtFullSuiteFailureCount,
		FailureRecallBasisPoints:    report.FailureRecallBasisPoints,
		MissedFailures:              exportFailureMisses(report.MissedFailures),
	}
	if err := contracts.ValidateSelectionShadowReportV1(document); err != nil {
		return contracts.SelectionShadowReportV1{}, fmt.Errorf("export selection shadow report: %w", err)
	}

	return document, nil
}

func exportFailureMisses(values []shadow.FailureMiss) []contracts.ShadowFailureMiss {
	misses := make([]contracts.ShadowFailureMiss, 0, len(values))
	for _, miss := range values {
		var selectedOutcome *string
		if miss.SelectedOutcome != nil {
			value := string(*miss.SelectedOutcome)
			selectedOutcome = &value
		}
		misses = append(misses, contracts.ShadowFailureMiss{
			SuiteKey: miss.SuiteKey, TestKey: miss.TestKey,
			FullSuiteOutcome: string(miss.FullSuiteOutcome), SelectedOutcome: selectedOutcome,
			Reason: string(miss.Reason),
		})
	}

	return misses
}
