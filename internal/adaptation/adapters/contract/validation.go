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

func exportValidationFailure(failure *adaptation.ValidationFailure) *contracts.NormalizedFailure {
	if failure == nil {
		return nil
	}

	return &contracts.NormalizedFailure{Code: failure.Code, Message: failure.Message}
}
