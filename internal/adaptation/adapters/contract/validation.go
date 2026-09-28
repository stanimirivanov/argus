package contract

import (
	"fmt"
	"time"

	"github.com/stanimirivanov/argus/contracts"
	"github.com/stanimirivanov/argus/internal/adaptation"
)

// ExportValidationRequestV1 converts a validated phase request to adapter JSON.
func ExportValidationRequestV1(
	request adaptation.ValidationRequest,
) (contracts.FunctionalAPIRepairValidationRequestV1, error) {
	request.Test = adaptation.CanonicalTestReference(request.Test)
	if err := adaptation.ValidateValidationRequest(request); err != nil {
		return contracts.FunctionalAPIRepairValidationRequestV1{}, err
	}
	document := contracts.FunctionalAPIRepairValidationRequestV1{
		APIVersion: request.APIVersion, ValidationID: request.ValidationID,
		ProposalID: request.ProposalID, Phase: string(request.Phase), Test: exportTest(request.Test),
		SourceSHA256: request.SourceSHA256,
	}
	if err := contracts.ValidateFunctionalAPIRepairValidationRequestV1(document); err != nil {
		return contracts.FunctionalAPIRepairValidationRequestV1{}, fmt.Errorf("export validation request: %w", err)
	}

	return document, nil
}

// ImportValidationResultV1 converts structurally valid, untrusted phase output.
func ImportValidationResultV1(
	document contracts.FunctionalAPIRepairValidationResultV1,
) (adaptation.ValidationAdapterResult, error) {
	startedAt, err := time.Parse(time.RFC3339Nano, document.StartedAt)
	if err != nil {
		return adaptation.ValidationAdapterResult{}, fmt.Errorf("parse validation start time: %w", err)
	}
	completedAt, err := time.Parse(time.RFC3339Nano, document.CompletedAt)
	if err != nil {
		return adaptation.ValidationAdapterResult{}, fmt.Errorf("parse validation completion time: %w", err)
	}
	result := adaptation.ValidationAdapterResult{
		APIVersion: document.APIVersion, ValidationID: document.ValidationID,
		ProposalID: document.ProposalID, Phase: adaptation.ValidationPhase(document.Phase),
		SuiteKey: document.SuiteKey, TestKey: document.TestKey,
		AdapterID: document.Adapter.ID, AdapterVersion: document.Adapter.Version,
		SourceSHA256: document.SourceSHA256, StartedAt: startedAt, CompletedAt: completedAt,
		Outcome: adaptation.ValidationOutcome(document.Outcome),
	}
	if document.Failure != nil {
		result.Failure = &adaptation.ValidationFailure{
			Code: document.Failure.Code, Message: document.Failure.Message,
		}
	}
	if err := adaptation.ValidateValidationAdapterResult(result); err != nil {
		return adaptation.ValidationAdapterResult{}, fmt.Errorf("validate validation result semantics: %w", err)
	}

	return result, nil
}

// ExportValidationEvidenceV1 converts a successful domain proof to its public contract.
func ExportValidationEvidenceV1(
	evidence adaptation.ValidationEvidence,
) (contracts.ValidationEvidenceV1, error) {
	evidence = adaptation.CanonicalValidationEvidence(evidence)
	if err := adaptation.ValidateValidationEvidence(evidence); err != nil {
		return contracts.ValidationEvidenceV1{}, err
	}
	runs := make([]contracts.AdaptationValidationRun, 0, len(evidence.Runs))
	for _, run := range evidence.Runs {
		runs = append(runs, contracts.AdaptationValidationRun{
			Phase: string(run.Phase), SourceSHA256: run.SourceSHA256,
			StartedAt:   run.StartedAt.Format(time.RFC3339Nano),
			CompletedAt: run.CompletedAt.Format(time.RFC3339Nano),
			Outcome:     string(run.Outcome), Failure: exportValidationFailure(run.Failure),
		})
	}
	document := contracts.ValidationEvidenceV1{
		APIVersion: evidence.APIVersion, PolicyVersion: evidence.PolicyVersion,
		ValidationID: evidence.ValidationID, ProposalID: evidence.ProposalID,
		ProposalPolicyVersion: evidence.ProposalPolicyVersion, Test: exportTest(evidence.Test),
		Adapter: contracts.AdapterIdentity{ID: evidence.AdapterID, Version: evidence.AdapterVersion},
		Edit: contracts.AdaptationTextEdit{
			Path: evidence.Edit.Path, BeforeSHA256: evidence.Edit.BeforeSHA256,
			StartByte: evidence.Edit.StartByte, EndByte: evidence.Edit.EndByte,
			Original: evidence.Edit.Original, Replacement: evidence.Edit.Replacement,
			SemanticRole: evidence.Edit.SemanticRole,
		},
		Source: contracts.AdaptationValidationSource{
			Path: evidence.Source.Path, OriginalSHA256: evidence.Source.OriginalSHA256,
			CandidateSHA256:     evidence.Source.CandidateSHA256,
			NegativeSHA256:      evidence.Source.NegativeSHA256,
			RestoredSHA256:      evidence.Source.RestoredSHA256,
			NegativeControlPath: evidence.Source.NegativeControlPath,
		},
		Runs: runs,
	}
	if err := contracts.ValidateValidationEvidenceV1(document); err != nil {
		return contracts.ValidationEvidenceV1{}, fmt.Errorf("export validation evidence: %w", err)
	}

	return document, nil
}

// ImportValidationEvidenceV1 converts a structurally valid successful proof.
func ImportValidationEvidenceV1(
	document contracts.ValidationEvidenceV1,
) (adaptation.ValidationEvidence, error) {
	runs := make([]adaptation.ValidationRun, 0, len(document.Runs))
	for _, documentRun := range document.Runs {
		startedAt, err := time.Parse(time.RFC3339Nano, documentRun.StartedAt)
		if err != nil {
			return adaptation.ValidationEvidence{}, fmt.Errorf("parse validation run start: %w", err)
		}
		completedAt, err := time.Parse(time.RFC3339Nano, documentRun.CompletedAt)
		if err != nil {
			return adaptation.ValidationEvidence{}, fmt.Errorf("parse validation run completion: %w", err)
		}
		run := adaptation.ValidationRun{
			Phase: adaptation.ValidationPhase(documentRun.Phase), SourceSHA256: documentRun.SourceSHA256,
			StartedAt: startedAt, CompletedAt: completedAt,
			Outcome: adaptation.ValidationOutcome(documentRun.Outcome),
		}
		if documentRun.Failure != nil {
			run.Failure = &adaptation.ValidationFailure{
				Code: documentRun.Failure.Code, Message: documentRun.Failure.Message,
			}
		}
		runs = append(runs, run)
	}
	evidence := adaptation.ValidationEvidence{
		APIVersion: document.APIVersion, PolicyVersion: document.PolicyVersion,
		ValidationID: document.ValidationID, ProposalID: document.ProposalID,
		ProposalPolicyVersion: document.ProposalPolicyVersion, Test: importTest(document.Test),
		AdapterID: document.Adapter.ID, AdapterVersion: document.Adapter.Version,
		Edit: adaptation.TextEdit{
			Path: document.Edit.Path, BeforeSHA256: document.Edit.BeforeSHA256,
			StartByte: document.Edit.StartByte, EndByte: document.Edit.EndByte,
			Original: document.Edit.Original, Replacement: document.Edit.Replacement,
			SemanticRole: document.Edit.SemanticRole,
		},
		Source: adaptation.ValidationSourceEvidence{
			Path: document.Source.Path, OriginalSHA256: document.Source.OriginalSHA256,
			CandidateSHA256: document.Source.CandidateSHA256, NegativeSHA256: document.Source.NegativeSHA256,
			RestoredSHA256:      document.Source.RestoredSHA256,
			NegativeControlPath: document.Source.NegativeControlPath,
		},
		Runs: runs,
	}
	evidence = adaptation.CanonicalValidationEvidence(evidence)
	if err := adaptation.ValidateValidationEvidence(evidence); err != nil {
		return adaptation.ValidationEvidence{}, fmt.Errorf("validate imported validation evidence: %w", err)
	}

	return evidence, nil
}

// ExportValidationRejectionV1 converts trustworthy domain rejection evidence
// to its portable contract without treating it as successful validation.
func ExportValidationRejectionV1(
	evidence adaptation.ValidationRejectionEvidence,
) (contracts.ValidationRejectionV1, error) {
	evidence = adaptation.CanonicalValidationRejectionEvidence(evidence)
	if err := adaptation.ValidateValidationRejectionEvidence(evidence); err != nil {
		return contracts.ValidationRejectionV1{}, err
	}
	document := contracts.ValidationRejectionV1{
		APIVersion: evidence.APIVersion, PolicyVersion: evidence.PolicyVersion,
		ValidationID: evidence.ValidationID, ProposalID: evidence.ProposalID,
		ProposalPolicyVersion: evidence.ProposalPolicyVersion, Test: exportTest(evidence.Test),
		Adapter: contracts.AdapterIdentity{ID: evidence.AdapterID, Version: evidence.AdapterVersion},
		Edit:    exportValidationEdit(evidence.Edit), Source: exportValidationSource(evidence.Source),
		Rejection: contracts.AdaptationValidationRejection{
			Phase: string(evidence.RejectedPhase), Reason: string(evidence.Reason),
			ExpectedOutcome: string(evidence.ExpectedOutcome), ActualOutcome: string(evidence.ActualOutcome),
		},
		Runs: exportValidationRuns(evidence.Runs),
	}
	if err := contracts.ValidateValidationRejectionV1(document); err != nil {
		return contracts.ValidationRejectionV1{}, fmt.Errorf("export validation rejection: %w", err)
	}

	return document, nil
}

// ImportValidationRejectionV1 converts structurally valid negative evidence
// into the canonical domain representation used by durable ingestion.
func ImportValidationRejectionV1(
	document contracts.ValidationRejectionV1,
) (adaptation.ValidationRejectionEvidence, error) {
	if err := contracts.ValidateValidationRejectionV1(document); err != nil {
		return adaptation.ValidationRejectionEvidence{}, err
	}
	runs, err := importValidationRuns(document.Runs)
	if err != nil {
		return adaptation.ValidationRejectionEvidence{}, err
	}
	evidence := adaptation.ValidationRejectionEvidence{
		APIVersion: document.APIVersion, PolicyVersion: document.PolicyVersion,
		ValidationID: document.ValidationID, ProposalID: document.ProposalID,
		ProposalPolicyVersion: document.ProposalPolicyVersion, Test: importTest(document.Test),
		AdapterID: document.Adapter.ID, AdapterVersion: document.Adapter.Version,
		Edit: adaptation.TextEdit{
			Path: document.Edit.Path, BeforeSHA256: document.Edit.BeforeSHA256,
			StartByte: document.Edit.StartByte, EndByte: document.Edit.EndByte,
			Original: document.Edit.Original, Replacement: document.Edit.Replacement,
			SemanticRole: document.Edit.SemanticRole,
		},
		Source: adaptation.ValidationSourceEvidence{
			Path: document.Source.Path, OriginalSHA256: document.Source.OriginalSHA256,
			CandidateSHA256: document.Source.CandidateSHA256, NegativeSHA256: document.Source.NegativeSHA256,
			RestoredSHA256:      document.Source.RestoredSHA256,
			NegativeControlPath: document.Source.NegativeControlPath,
		},
		RejectedPhase:   adaptation.ValidationPhase(document.Rejection.Phase),
		Reason:          adaptation.ValidationRejectionReason(document.Rejection.Reason),
		ExpectedOutcome: adaptation.ValidationOutcome(document.Rejection.ExpectedOutcome),
		ActualOutcome:   adaptation.ValidationOutcome(document.Rejection.ActualOutcome), Runs: runs,
	}
	evidence = adaptation.CanonicalValidationRejectionEvidence(evidence)
	if err := adaptation.ValidateValidationRejectionEvidence(evidence); err != nil {
		return adaptation.ValidationRejectionEvidence{}, fmt.Errorf("validate imported validation rejection: %w", err)
	}

	return evidence, nil
}

func exportValidationEdit(edit adaptation.TextEdit) contracts.AdaptationTextEdit {
	return contracts.AdaptationTextEdit{
		Path: edit.Path, BeforeSHA256: edit.BeforeSHA256, StartByte: edit.StartByte, EndByte: edit.EndByte,
		Original: edit.Original, Replacement: edit.Replacement, SemanticRole: edit.SemanticRole,
	}
}

func exportValidationSource(source adaptation.ValidationSourceEvidence) contracts.AdaptationValidationSource {
	return contracts.AdaptationValidationSource{
		Path: source.Path, OriginalSHA256: source.OriginalSHA256,
		CandidateSHA256: source.CandidateSHA256, NegativeSHA256: source.NegativeSHA256,
		RestoredSHA256: source.RestoredSHA256, NegativeControlPath: source.NegativeControlPath,
	}
}

func exportValidationRuns(runs []adaptation.ValidationRun) []contracts.AdaptationValidationRun {
	documents := make([]contracts.AdaptationValidationRun, 0, len(runs))
	for _, run := range runs {
		documents = append(documents, contracts.AdaptationValidationRun{
			Phase: string(run.Phase), SourceSHA256: run.SourceSHA256,
			StartedAt: run.StartedAt.Format(time.RFC3339Nano), CompletedAt: run.CompletedAt.Format(time.RFC3339Nano),
			Outcome: string(run.Outcome), Failure: exportValidationFailure(run.Failure),
		})
	}

	return documents
}

func importValidationRuns(documents []contracts.AdaptationValidationRun) ([]adaptation.ValidationRun, error) {
	runs := make([]adaptation.ValidationRun, 0, len(documents))
	for _, document := range documents {
		startedAt, err := time.Parse(time.RFC3339Nano, document.StartedAt)
		if err != nil {
			return nil, fmt.Errorf("parse validation rejection run start: %w", err)
		}
		completedAt, err := time.Parse(time.RFC3339Nano, document.CompletedAt)
		if err != nil {
			return nil, fmt.Errorf("parse validation rejection run completion: %w", err)
		}
		run := adaptation.ValidationRun{
			Phase: adaptation.ValidationPhase(document.Phase), SourceSHA256: document.SourceSHA256,
			StartedAt: startedAt, CompletedAt: completedAt, Outcome: adaptation.ValidationOutcome(document.Outcome),
		}
		if document.Failure != nil {
			run.Failure = &adaptation.ValidationFailure{Code: document.Failure.Code, Message: document.Failure.Message}
		}
		runs = append(runs, run)
	}

	return runs, nil
}

func exportValidationFailure(failure *adaptation.ValidationFailure) *contracts.NormalizedFailure {
	if failure == nil {
		return nil
	}

	return &contracts.NormalizedFailure{Code: failure.Code, Message: failure.Message}
}
