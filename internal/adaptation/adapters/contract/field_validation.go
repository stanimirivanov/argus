package contract

import (
	"fmt"
	"time"

	"github.com/stanimirivanov/argus/internal/adaptation"
	"github.com/stanimirivanov/argus/internal/adaptation/fieldvalidation"
	"github.com/stanimirivanov/argus/internal/contracts"
)

// ExportFieldValidationRequestV1 encodes one correlated phase request.
func ExportFieldValidationRequestV1(request fieldvalidation.Request) (contracts.FieldValidationRequestV1, error) {
	request.Test = adaptation.CanonicalTestReference(request.Test)
	if err := fieldvalidation.ValidateRequest(request); err != nil {
		return contracts.FieldValidationRequestV1{}, err
	}
	doc := contracts.FieldValidationRequestV1{APIVersion: request.APIVersion, ValidationID: request.ValidationID,
		ProposalID: request.ProposalID, Phase: string(request.Phase), Repeat: request.Repeat,
		Test: exportTest(request.Test), SourceSHA256: request.SourceSHA256}

	return doc, contracts.ValidateFieldValidationRequestV1(doc)
}

// ImportFieldValidationResultV1 validates untrusted framework output.
func ImportFieldValidationResultV1(doc contracts.FieldValidationResultV1) (fieldvalidation.Result, error) {
	started, err := time.Parse(time.RFC3339Nano, doc.StartedAt)
	if err != nil {
		return fieldvalidation.Result{}, err
	}
	completed, err := time.Parse(time.RFC3339Nano, doc.CompletedAt)
	if err != nil {
		return fieldvalidation.Result{}, err
	}
	result := fieldvalidation.Result{APIVersion: doc.APIVersion, ValidationID: doc.ValidationID, ProposalID: doc.ProposalID,
		Phase: fieldvalidation.Phase(doc.Phase), Repeat: doc.Repeat, SuiteKey: doc.SuiteKey, TestKey: doc.TestKey,
		AdapterID: doc.Adapter.ID, AdapterVersion: doc.Adapter.Version, SourceSHA256: doc.SourceSHA256,
		StartedAt: started, CompletedAt: completed, Outcome: adaptation.ValidationOutcome(doc.Outcome)}
	if doc.Failure != nil {
		result.Failure = &adaptation.ValidationFailure{Code: doc.Failure.Code, Message: doc.Failure.Message}
	}
	if err := fieldvalidation.ValidateResult(result); err != nil {
		return fieldvalidation.Result{}, err
	}

	return result, nil
}

// ExportFieldValidationEvidenceV1 writes only a complete successful proof.
func ExportFieldValidationEvidenceV1(evidence fieldvalidation.Evidence) (contracts.FieldValidationEvidenceV1, error) {
	if err := fieldvalidation.ValidateEvidence(evidence); err != nil {
		return contracts.FieldValidationEvidenceV1{}, err
	}
	doc := exportFieldValidationBase(evidence)

	return doc, contracts.ValidateFieldValidationEvidenceV1(doc)
}

// ImportFieldValidationEvidenceV1 verifies structural and domain proof invariants.
func ImportFieldValidationEvidenceV1(doc contracts.FieldValidationEvidenceV1) (fieldvalidation.Evidence, error) {
	if err := contracts.ValidateFieldValidationEvidenceV1(doc); err != nil {
		return fieldvalidation.Evidence{}, err
	}
	evidence, err := importFieldValidationBase(doc)
	if err != nil {
		return fieldvalidation.Evidence{}, err
	}
	if err := fieldvalidation.ValidateEvidence(evidence); err != nil {
		return fieldvalidation.Evidence{}, err
	}

	return evidence, nil
}

// ExportFieldValidationRejectionV1 retains a trustworthy rejected prefix.
func ExportFieldValidationRejectionV1(rejection fieldvalidation.Rejection) (contracts.FieldValidationRejectionV1, error) {
	if err := fieldvalidation.ValidateRejection(rejection); err != nil {
		return contracts.FieldValidationRejectionV1{}, err
	}
	doc := contracts.FieldValidationRejectionV1{FieldValidationEvidenceV1: exportFieldValidationBase(rejection.Evidence),
		Rejection: contracts.AdaptationValidationRejection{Phase: string(rejection.RejectedPhase), Reason: rejection.Reason,
			ExpectedOutcome: string(rejection.ExpectedOutcome), ActualOutcome: string(rejection.ActualOutcome)}}

	return doc, contracts.ValidateFieldValidationRejectionV1(doc)
}
func exportFieldValidationBase(evidence fieldvalidation.Evidence) contracts.FieldValidationEvidenceV1 {
	runs := make([]contracts.FieldValidationRunV1, 0, len(evidence.Runs))
	for _, run := range evidence.Runs {
		runs = append(runs, contracts.FieldValidationRunV1{Phase: string(run.Phase), Repeat: run.Repeat,
			SourceSHA256: run.SourceSHA256, StartedAt: run.StartedAt.UTC().Format(time.RFC3339Nano),
			CompletedAt: run.CompletedAt.UTC().Format(time.RFC3339Nano), Outcome: string(run.Outcome), Failure: exportValidationFailure(run.Failure)})
	}

	return contracts.FieldValidationEvidenceV1{APIVersion: evidence.APIVersion, PolicyVersion: evidence.PolicyVersion,
		ValidationID: evidence.ValidationID, ProposalID: evidence.ProposalID, ProposalPolicyVersion: evidence.ProposalPolicyVersion,
		Test: exportTest(evidence.Test), Adapter: contracts.AdapterIdentity{ID: evidence.AdapterID, Version: evidence.AdapterVersion},
		Edit: contracts.AdaptationTextEdit{Path: evidence.Edit.Path, BeforeSHA256: evidence.Edit.BeforeSHA256,
			StartByte: evidence.Edit.StartByte, EndByte: evidence.Edit.EndByte, Original: evidence.Edit.Original,
			Replacement: evidence.Edit.Replacement, SemanticRole: evidence.Edit.SemanticRole},
		Source: contracts.FieldValidationSourceV1{Path: evidence.Source.Path, OriginalSHA256: evidence.Source.OriginalSHA256,
			CandidateSHA256: evidence.Source.CandidateSHA256, NegativeSHA256: evidence.Source.NegativeSHA256,
			RestoredSHA256: evidence.Source.RestoredSHA256, NegativeControlField: evidence.Source.NegativeControlField}, Runs: runs}
}
func importFieldValidationBase(doc contracts.FieldValidationEvidenceV1) (fieldvalidation.Evidence, error) {
	evidence := fieldvalidation.Evidence{APIVersion: doc.APIVersion, PolicyVersion: doc.PolicyVersion,
		ValidationID: doc.ValidationID, ProposalID: doc.ProposalID, ProposalPolicyVersion: doc.ProposalPolicyVersion,
		Test: adaptation.CanonicalTestReference(importTest(doc.Test)), AdapterID: doc.Adapter.ID, AdapterVersion: doc.Adapter.Version,
		Edit: adaptation.TextEdit{Path: doc.Edit.Path, BeforeSHA256: doc.Edit.BeforeSHA256, StartByte: doc.Edit.StartByte,
			EndByte: doc.Edit.EndByte, Original: doc.Edit.Original, Replacement: doc.Edit.Replacement, SemanticRole: doc.Edit.SemanticRole},
		Source: fieldvalidation.Source{Path: doc.Source.Path, OriginalSHA256: doc.Source.OriginalSHA256,
			CandidateSHA256: doc.Source.CandidateSHA256, NegativeSHA256: doc.Source.NegativeSHA256,
			RestoredSHA256: doc.Source.RestoredSHA256, NegativeControlField: doc.Source.NegativeControlField}}
	for _, run := range doc.Runs {
		started, err := time.Parse(time.RFC3339Nano, run.StartedAt)
		if err != nil {
			return fieldvalidation.Evidence{}, fmt.Errorf("parse field run start: %w", err)
		}
		completed, err := time.Parse(time.RFC3339Nano, run.CompletedAt)
		if err != nil {
			return fieldvalidation.Evidence{}, fmt.Errorf("parse field run completion: %w", err)
		}
		value := fieldvalidation.Run{Phase: fieldvalidation.Phase(run.Phase), Repeat: run.Repeat, SourceSHA256: run.SourceSHA256,
			StartedAt: started, CompletedAt: completed, Outcome: adaptation.ValidationOutcome(run.Outcome)}
		if run.Failure != nil {
			value.Failure = &adaptation.ValidationFailure{Code: run.Failure.Code, Message: run.Failure.Message}
		}
		evidence.Runs = append(evidence.Runs, value)
	}

	return evidence, nil
}
