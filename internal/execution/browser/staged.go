package browser

import (
	"context"
	"fmt"
	"slices"

	"github.com/stanimirivanov/argus/internal/execution"
	"github.com/stanimirivanov/argus/internal/execution/scheduledplan"
)

const (
	// StagedRequestVersion identifies complete-inventory staged browser input.
	StagedRequestVersion = "argus.dev/browser-adapter-request/v2"
	// StagedAttemptVersion identifies immutable staged browser evidence.
	StagedAttemptVersion = "argus.dev/browser-execution-attempt/v2"
)

// Staging pins schedule, scope and the independent complete discovery bytes.
type Staging struct {
	execution.Staging
	DiscoverySHA256 string
}

func legacyStageRequest(request Request) Request {
	request.APIVersion, request.Stage, request.Staging, request.Candidates = RequestVersion, execution.StageSelected, nil, nil
	return request
}

// ValidateStagedRequest requires exact requested membership within the complete
// independent inventory. V1 consumers deliberately reject these new fields.
func ValidateStagedRequest(request Request) error {
	if request.APIVersion != StagedRequestVersion || request.Staging == nil ||
		execution.ValidateStaging(&request.Staging.Staging, request.Stage) != nil ||
		request.Staging.Environment != request.Environment || !digestValid(request.Staging.DiscoverySHA256) {
		return ErrInvalid
	}
	legacy := legacyStageRequest(request)
	if err := ValidateRequest(legacy); err != nil {
		return err
	}
	legacy.Tests = request.Candidates
	if err := ValidateRequest(legacy); err != nil {
		return err
	}
	inventory := map[[2]string]Test{}
	for _, test := range request.Candidates {
		inventory[[2]string{test.SuiteKey, test.TestKey}] = test
	}
	for _, test := range request.Tests {
		if inventory[[2]string{test.SuiteKey, test.TestKey}] != test {
			return ErrInvalid
		}
	}
	if request.Stage == execution.StageShadowControl && len(request.Tests) != len(request.Candidates) {
		return ErrInvalid
	}

	return nil
}

// ValidateRecordedAttempt dispatches explicit versions without widening v1.
func ValidateRecordedAttempt(attempt Attempt) error {
	if attempt.APIVersion == AttemptVersion {
		return ValidateAttempt(attempt)
	}
	if attempt.APIVersion != StagedAttemptVersion || ValidateStagedRequest(attempt.Request) != nil ||
		attempt.Outcome != execution.DeriveAttemptOutcome(attempt.Result.Results) {
		return ErrInvalid
	}

	return ValidateResult(legacyStageRequest(attempt.Request), attempt.Result)
}

// StageOptions binds one invocation to independently verified stage inputs.
// Mode expresses required passes, not production or release authorization.
type StageOptions struct {
	Plan                scheduledplan.Plan
	Source              scheduledplan.Schedule
	DiscoverySHA256     string
	Mode                scheduledplan.Mode
	GroupKey, AttemptID string
	Stage               execution.Stage
}

// ExecuteStage runs one exact nonempty pass and verifies all required artifact
// bytes before returning evidence. Empty memberships create no invocation.
func (service *Service) ExecuteStage(ctx context.Context, manifest Manifest, options StageOptions) (*Attempt, error) {
	if service == nil || service.adapter == nil || validateManifest(manifest) != nil || execution.ValidateAttemptID(options.AttemptID) != nil || !digestValid(options.DiscoverySHA256) {
		return nil, ErrInvalid
	}
	tests, err := scheduledplan.TestsForStage(options.Plan, options.Source, options.Mode, options.GroupKey, options.Stage)
	if err != nil {
		return nil, err
	}
	var group scheduledplan.Group
	for _, candidate := range options.Plan.Groups {
		if candidate.GroupKey == options.GroupKey {
			group = candidate
		}
	}
	request, err := stagedRequest(manifest, group, tests, options)
	if err != nil {
		return nil, err
	}
	check := request
	if len(tests) == 0 {
		check.Tests = request.Candidates
	}
	if err := ValidateStagedRequest(check); err != nil {
		return nil, err
	}
	if len(tests) == 0 {
		return nil, nil
	}
	result, err := service.adapter.Execute(ctx, request)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", execution.ErrUnavailable, err)
	}
	if err := ValidateResult(legacyStageRequest(request), result); err != nil {
		return nil, err
	}
	if err := service.adapter.VerifyArtifacts(ctx, result); err != nil {
		return nil, err
	}
	attempt := CanonicalAttempt(Attempt{APIVersion: StagedAttemptVersion, Request: request, Result: result, Outcome: execution.DeriveAttemptOutcome(result.Results)})

	return &attempt, ValidateRecordedAttempt(attempt)
}

func stagedRequest(manifest Manifest, group scheduledplan.Group, tests []scheduledplan.Test, options StageOptions) (Request, error) {
	request := Request{APIVersion: StagedRequestVersion, AttemptID: options.AttemptID, Manifest: execution.ManifestReference{APIVersion: manifest.APIVersion, SHA256: options.Source.ManifestSHA256},
		Stage: options.Stage, TestRepository: group.TestRepository, TestRevision: group.TestRevision, Adapter: group.Adapter, Project: group.Project, Environment: group.Environment,
		Staging: &Staging{Staging: execution.Staging{Schedule: options.Source.Reference, EpisodeID: options.Source.EpisodeID, GroupKey: group.GroupKey, Environment: group.Environment, Mode: string(options.Mode)}, DiscoverySHA256: options.DiscoverySHA256}}
	byID := map[string]Test{}
	for _, candidate := range group.Candidates {
		found := false
		for _, decision := range manifest.Decisions {
			if decision.Repository == group.TestRepository && decision.Revision == group.TestRevision && decision.SuiteKey == candidate.SuiteKey && decision.TestKey == candidate.TestKey && decision.Outcome == candidate.Decision &&
				(len(decision.Projects) == 0 || slices.Contains(decision.Projects, group.Project)) {
				test := Test{SuiteKey: candidate.SuiteKey, TestKey: candidate.TestKey, Name: decision.Name}
				request.Candidates = append(request.Candidates, test)
				byID[candidate.ID] = test
				found = true

				break
			}
		}
		if !found {
			return Request{}, ErrInvalid
		}
	}
	for _, test := range tests {
		request.Tests = append(request.Tests, byID[test.ID])
	}

	return request, nil
}
