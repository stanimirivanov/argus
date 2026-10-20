package contract

import (
	"encoding/json"

	"github.com/stanimirivanov/argus/internal/adaptation/browservalidation"
	"github.com/stanimirivanov/argus/internal/contracts"
	"github.com/stanimirivanov/argus/internal/execution/browser"
)

// ExportBrowserLocatorValidationV1 flattens verified browser attempts into
// the versioned public evidence contract without changing domain semantics.
func ExportBrowserLocatorValidationV1(evidence browservalidation.Evidence,
	exportAttempt func(browser.Attempt) (contracts.BrowserExecutionAttemptV1, error)) (contracts.BrowserLocatorValidationV1, error) {
	if err := browservalidation.ValidateEvidence(evidence); err != nil {
		return contracts.BrowserLocatorValidationV1{}, err
	}
	if exportAttempt == nil {
		return contracts.BrowserLocatorValidationV1{}, browser.ErrInvalid
	}
	proposal, err := ExportBrowserLocatorProposalV1(evidence.Proposal)
	if err != nil {
		return contracts.BrowserLocatorValidationV1{}, err
	}
	baseline, err := exportAttempt(evidence.Baseline)
	if err != nil {
		return contracts.BrowserLocatorValidationV1{}, err
	}
	failed, err := exportAttempt(evidence.Failed)
	if err != nil {
		return contracts.BrowserLocatorValidationV1{}, err
	}
	document := contracts.BrowserLocatorValidationV1{APIVersion: evidence.APIVersion, PolicyVersion: evidence.PolicyVersion,
		ValidationID: evidence.ValidationID, Status: string(evidence.Status), Reason: evidence.Reason,
		Proposal: proposal, Baseline: baseline, Failed: failed, Source: contracts.BrowserValidationSource{
			Path: evidence.Source.Path, OriginalSHA256: evidence.Source.OriginalSHA256,
			CandidateSHA256: evidence.Source.CandidateSHA256, NegativeSHA256: evidence.Source.NegativeSHA256,
			RestoredSHA256: evidence.Source.RestoredSHA256, NegativeLocator: evidence.Source.NegativeLocator},
		Runs: make([]contracts.BrowserValidationRun, 0, len(evidence.Runs))}
	for _, run := range evidence.Runs {
		attempt, exportErr := exportAttempt(run.Attempt)
		if exportErr != nil {
			return contracts.BrowserLocatorValidationV1{}, exportErr
		}
		document.Runs = append(document.Runs, contracts.BrowserValidationRun{Phase: string(run.Phase), Repeat: run.Repeat,
			SourceSHA256: run.SourceSHA256, Attempt: attempt})
	}
	data, err := json.Marshal(document)
	if err != nil {
		return contracts.BrowserLocatorValidationV1{}, err
	}
	if err := contracts.ValidateBrowserLocatorValidationV1(data); err != nil {
		return contracts.BrowserLocatorValidationV1{}, err
	}

	return document, nil
}

// ImportBrowserLocatorValidationV1 checks uploaded proof before review access.
func ImportBrowserLocatorValidationV1(document contracts.BrowserLocatorValidationV1,
	importAttempt func(contracts.BrowserExecutionAttemptV1) (browser.Attempt, error)) (browservalidation.Evidence, error) {
	data, err := json.Marshal(document)
	if err != nil {
		return browservalidation.Evidence{}, err
	}
	if err := contracts.ValidateBrowserLocatorValidationV1(data); err != nil {
		return browservalidation.Evidence{}, err
	}
	if importAttempt == nil {
		return browservalidation.Evidence{}, browser.ErrInvalid
	}
	proposal, err := ImportBrowserLocatorProposalV1(document.Proposal)
	if err != nil {
		return browservalidation.Evidence{}, err
	}
	baseline, err := importAttempt(document.Baseline)
	if err != nil {
		return browservalidation.Evidence{}, err
	}
	failed, err := importAttempt(document.Failed)
	if err != nil {
		return browservalidation.Evidence{}, err
	}
	evidence := browservalidation.Evidence{APIVersion: document.APIVersion, PolicyVersion: document.PolicyVersion,
		ValidationID: document.ValidationID, Status: browservalidation.Status(document.Status), Reason: document.Reason,
		Proposal: proposal, Baseline: baseline, Failed: failed,
		Source: browservalidation.Source{Path: document.Source.Path, OriginalSHA256: document.Source.OriginalSHA256,
			CandidateSHA256: document.Source.CandidateSHA256, NegativeSHA256: document.Source.NegativeSHA256,
			RestoredSHA256: document.Source.RestoredSHA256, NegativeLocator: document.Source.NegativeLocator}}
	for _, run := range document.Runs {
		attempt, err := importAttempt(run.Attempt)
		if err != nil {
			return browservalidation.Evidence{}, err
		}
		evidence.Runs = append(evidence.Runs, browservalidation.Run{Phase: browservalidation.Phase(run.Phase),
			Repeat: run.Repeat, SourceSHA256: run.SourceSHA256, Attempt: attempt})
	}
	if err := browservalidation.ValidateEvidence(evidence); err != nil {
		return browservalidation.Evidence{}, err
	}

	return evidence, nil
}
