package browservalidation

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"time"

	"github.com/stanimirivanov/argus/internal/adaptation"
	"github.com/stanimirivanov/argus/internal/adaptation/browserlocator"
	"github.com/stanimirivanov/argus/internal/execution"
	"github.com/stanimirivanov/argus/internal/execution/browser"
)

const cleanupTimeout = 30 * time.Second

// Workspace guards one explicitly disposable checkout at the failed attempt's revision.
type Workspace interface {
	Read(context.Context, string) ([]byte, error)
	WriteIfUnchanged(context.Context, string, string, []byte) error
	Restore(context.Context, string, []byte) error
}

// Service coordinates the existing browser adapter and guarded source workspace.
type Service struct {
	workspace Workspace
	adapter   browser.Adapter
}

// NewService binds a disposable source workspace to a browser execution adapter.
func NewService(workspace Workspace, adapter browser.Adapter) *Service {
	return &Service{workspace: workspace, adapter: adapter}
}

// Validate runs the unchanged test twice, the proposed edit twice, and a
// synthetic impossible locator twice. The source is restored after every run.
func (service *Service) Validate(ctx context.Context, proposal browserlocator.Proposal, baseline, failed browser.Attempt) (Evidence, error) {
	if service == nil || service.workspace == nil || service.adapter == nil {
		return Evidence{}, adaptation.ErrUnavailable
	}
	if browserlocator.ValidateProposal(proposal) != nil || browser.ValidateAttempt(baseline) != nil ||
		browser.ValidateAttempt(failed) != nil || baseline.Request.AttemptID != proposal.BaselineID ||
		failed.Request.AttemptID != proposal.FailedID || !sameContext(baseline.Request, failed.Request, proposal) ||
		baseline.Result.Results[0].Outcome != execution.TestPassed ||
		failed.Result.Results[0].Outcome != execution.TestFailed ||
		failed.Result.Results[0].Failure == nil || failed.Result.Results[0].Failure.Code != "locator-not-found" ||
		!hasSnapshot(baseline, proposal.BaselineSHA256) || !hasSnapshot(failed, proposal.FailedSHA256) {
		return Evidence{}, fmt.Errorf("%w: browser proposal attempt binding", adaptation.ErrInvalid)
	}
	if err := service.adapter.VerifyArtifacts(ctx, baseline.Result); err != nil {
		return Evidence{}, fmt.Errorf("verify baseline artifacts: %w", err)
	}
	if err := service.adapter.VerifyArtifacts(ctx, failed.Result); err != nil {
		return Evidence{}, fmt.Errorf("verify failed artifacts: %w", err)
	}
	material, err := service.prepare(ctx, proposal)
	if err != nil {
		return Evidence{}, err
	}
	material.adapterVersion = failed.Result.AdapterVersion
	evidence := Evidence{APIVersion: EvidenceVersion, PolicyVersion: PolicyVersion, ValidationID: material.id,
		Proposal: proposal, Baseline: baseline, Failed: failed, Source: material.source, Runs: []Run{}}
	for _, phase := range material.phases() {
		observed, runErr := service.runPhase(ctx, failed.Request, material, phase, &evidence)
		if runErr != nil {
			return Evidence{}, runErr
		}
		if observed != phase.expected {
			return service.reject(proposal.Edit.Path, phase.reason, evidence)
		}
	}
	evidence.Source.RestoredSHA256, err = service.restored(proposal.Edit.Path, material.source.OriginalSHA256)
	if err != nil {
		return Evidence{}, err
	}
	evidence.Status = Validated
	if err := ValidateEvidence(evidence); err != nil {
		return Evidence{}, err
	}

	return evidence, nil
}

type materialized struct {
	original, candidate, negative []byte
	source                        Source
	id                            string
	adapterVersion                string
}

type phaseSpec struct {
	phase    Phase
	data     []byte
	sha      string
	expected execution.TestOutcome
	reason   string
}

func (service *Service) prepare(ctx context.Context, proposal browserlocator.Proposal) (materialized, error) {
	original, err := service.workspace.Read(ctx, proposal.Edit.Path)
	if err != nil {
		return materialized{}, err
	}
	edit := proposal.Edit
	if len(original) > adaptation.MaxSourceBytes || digest(original) != edit.BeforeSHA256 ||
		edit.StartByte < 0 || edit.EndByte > len(original) || edit.StartByte >= edit.EndByte ||
		!bytes.Equal(original[edit.StartByte:edit.EndByte], []byte(edit.Original)) {
		return materialized{}, fmt.Errorf("%w: browser source preimage", adaptation.ErrInvalid)
	}
	control, err := browserlocator.NegativeControlLocator(proposal)
	if err != nil {
		return materialized{}, err
	}
	candidate := replace(original, edit.StartByte, edit.EndByte, edit.Replacement)
	negative := replace(original, edit.StartByte, edit.EndByte, control)
	if len(candidate) > adaptation.MaxSourceBytes || len(negative) > adaptation.MaxSourceBytes {
		return materialized{}, fmt.Errorf("%w: browser variant size", adaptation.ErrInvalid)
	}
	source := Source{Path: edit.Path, OriginalSHA256: digest(original), CandidateSHA256: digest(candidate),
		NegativeSHA256: digest(negative), NegativeLocator: control}

	return materialized{original: original, candidate: candidate, negative: negative, source: source,
		id: DeriveID(proposal.ProposalID, proposal.FailedID, source.CandidateSHA256, source.NegativeSHA256)}, nil
}

func (material materialized) phases() []phaseSpec {
	return []phaseSpec{
		{Original, material.original, material.source.OriginalSHA256, execution.TestFailed, "original-passed"},
		{Candidate, material.candidate, material.source.CandidateSHA256, execution.TestPassed, "candidate-locator-failed"},
		{NegativeControl, material.negative, material.source.NegativeSHA256, execution.TestFailed, "negative-control-passed"},
	}
}

func (service *Service) runPhase(ctx context.Context, template browser.Request, material materialized, phase phaseSpec, evidence *Evidence) (execution.TestOutcome, error) {
	var observed execution.TestOutcome
	for repeat := 1; repeat <= 2; repeat++ {
		attempt, err := service.execute(ctx, template, material, phase, repeat)
		if err != nil {
			return "", err
		}
		result := attempt.Result.Results[0]
		if result.Outcome != execution.TestPassed && result.Outcome != execution.TestFailed {
			return "", fmt.Errorf("%w: browser run did not complete", adaptation.ErrUnavailable)
		}
		if result.Outcome == execution.TestFailed && (result.Failure == nil || result.Failure.Code != "locator-not-found") {
			return "", fmt.Errorf("%w: browser product or infrastructure failure", adaptation.ErrUnavailable)
		}
		if len(evidence.Runs) > 0 && attempt.Result.StartedAt.Before(evidence.Runs[len(evidence.Runs)-1].Attempt.Result.CompletedAt) {
			return "", fmt.Errorf("%w: overlapping browser runs", adaptation.ErrInvalid)
		}
		if repeat == 2 && result.Outcome != observed {
			return "", fmt.Errorf("%w: browser repeat disagreement", adaptation.ErrUnavailable)
		}
		observed = result.Outcome
		evidence.Runs = append(evidence.Runs, Run{Phase: phase.phase, Repeat: repeat, SourceSHA256: phase.sha, Attempt: attempt})
	}

	return observed, nil
}

func (service *Service) reject(path, reason string, evidence Evidence) (Evidence, error) {
	restored, err := service.restored(path, evidence.Source.OriginalSHA256)
	if err != nil {
		return Evidence{}, err
	}
	evidence.Source.RestoredSHA256 = restored
	evidence.Status, evidence.Reason = Rejected, reason
	if err := ValidateEvidence(evidence); err != nil {
		return Evidence{}, err
	}

	return Evidence{}, &RejectedError{Evidence: evidence}
}

func (service *Service) execute(ctx context.Context, template browser.Request, material materialized, phase phaseSpec, repeat int) (browser.Attempt, error) {
	mutated := phase.phase != Original
	if mutated {
		if err := service.workspace.WriteIfUnchanged(ctx, material.source.Path, material.source.OriginalSHA256, phase.data); err != nil {
			return browser.Attempt{}, fmt.Errorf("materialize browser %s: %w", phase.phase, err)
		}
	}
	request := template
	request.AttemptID = attemptID(material.id, phase.phase, repeat)
	result, runErr := service.adapter.Execute(ctx, request)
	checkCtx, stop := context.WithTimeout(context.WithoutCancel(ctx), cleanupTimeout)
	current, checkErr := service.workspace.Read(checkCtx, material.source.Path)
	stop()
	unchanged := checkErr == nil && digest(current) == phase.sha
	if mutated || !unchanged {
		restoreCtx, stopRestore := context.WithTimeout(context.WithoutCancel(ctx), cleanupTimeout)
		restoreErr := service.workspace.Restore(restoreCtx, material.source.Path, material.original)
		stopRestore()
		if restoreErr != nil {
			return browser.Attempt{}, fmt.Errorf("restore browser source: %w", restoreErr)
		}
	}
	if _, err := service.restored(material.source.Path, material.source.OriginalSHA256); err != nil {
		return browser.Attempt{}, err
	}
	if checkErr != nil {
		return browser.Attempt{}, fmt.Errorf("verify browser source: %w", checkErr)
	}
	if !unchanged {
		return browser.Attempt{}, fmt.Errorf("%w: browser adapter mutated source", adaptation.ErrInvalid)
	}
	if runErr != nil {
		return browser.Attempt{}, runErr
	}
	if err := browser.ValidateResult(request, result); err != nil {
		return browser.Attempt{}, err
	}
	if result.AdapterVersion != material.adapterVersion {
		return browser.Attempt{}, fmt.Errorf("%w: browser adapter drift", adaptation.ErrInvalid)
	}
	if err := service.adapter.VerifyArtifacts(ctx, result); err != nil {
		return browser.Attempt{}, err
	}

	return browser.CanonicalAttempt(browser.Attempt{APIVersion: browser.AttemptVersion, Request: request,
		Result: result, Outcome: execution.DeriveAttemptOutcome(result.Results)}), nil
}

func (service *Service) restored(path, sha string) (string, error) {
	ctx, stop := context.WithTimeout(context.Background(), cleanupTimeout)
	defer stop()
	current, err := service.workspace.Read(ctx, path)
	if err != nil {
		return "", err
	}
	if digest(current) != sha {
		return "", fmt.Errorf("%w: browser source restoration", adaptation.ErrInvalid)
	}

	return sha, nil
}

func hasSnapshot(attempt browser.Attempt, sha string) bool {
	for _, artifact := range attempt.Result.Artifacts {
		if artifact.Kind == "accessibility" && artifact.SHA256 == sha &&
			artifact.SuiteKey == attempt.Request.Tests[0].SuiteKey && artifact.TestKey == attempt.Request.Tests[0].TestKey {
			return true
		}
	}

	return false
}

func replace(original []byte, start, end int, value string) []byte {
	result := make([]byte, 0, len(original)-(end-start)+len(value))
	result = append(result, original[:start]...)
	result = append(result, value...)
	return append(result, original[end:]...)
}

func digest(data []byte) string { sum := sha256.Sum256(data); return hex.EncodeToString(sum[:]) }
