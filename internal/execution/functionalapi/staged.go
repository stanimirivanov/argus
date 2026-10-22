package functionalapi

import (
	"context"
	"fmt"

	"github.com/stanimirivanov/argus/internal/execution"
	"github.com/stanimirivanov/argus/internal/execution/scheduledplan"
	"github.com/stanimirivanov/argus/internal/selection"
)

// StagedRequestAPIVersion identifies explicit schedule-correlated adapter input.
const StagedRequestAPIVersion = "argus.dev/functional-api-adapter-request/v2"

// ValidateStagedRequest shares result-independent test invariants with v1 while
// requiring the v2 schedule/mode/scope envelope.
func ValidateStagedRequest(request Request) error {
	if request.APIVersion != StagedRequestAPIVersion || execution.ValidateStaging(request.Staging, request.Stage) != nil {
		return fmt.Errorf("%w: staged adapter request", execution.ErrInvalid)
	}
	request.APIVersion, request.Stage, request.Staging = AdapterRequestAPIVersion, execution.StageSelected, nil

	return ValidateRequest(request)
}

// ExecuteStage invokes exactly one nonempty stage after independent source
// correlation. Empty stages return nil evidence without invoking the adapter.
// Each retry must use a distinct attempt ID unless replaying identical evidence.
func (service *Service) ExecuteStage(ctx context.Context, manifest selection.Manifest, plan scheduledplan.Plan,
	source scheduledplan.Schedule, mode scheduledplan.Mode, groupKey, attemptID string, stage execution.Stage) (*execution.Attempt, error) {
	if service == nil || service.adapter == nil || selection.ValidateManifest(manifest) != nil ||
		execution.ValidateAttemptID(attemptID) != nil {
		return nil, execution.ErrInvalid
	}
	tests, err := scheduledplan.TestsForStage(plan, source, mode, groupKey, stage)
	if err != nil {
		return nil, err
	}
	if len(tests) == 0 {
		return nil, nil
	}
	var group scheduledplan.Group
	for _, candidate := range plan.Groups {
		if candidate.GroupKey == groupKey {
			group = candidate
		}
	}
	request := Request{APIVersion: StagedRequestAPIVersion, AttemptID: attemptID,
		Manifest: execution.ManifestReference{APIVersion: manifest.APIVersion, SHA256: source.ManifestSHA256},
		Stage:    stage, TestRepository: group.TestRepository, TestRevision: group.TestRevision, Adapter: group.Adapter,
		Staging: &execution.Staging{Schedule: source.Reference, EpisodeID: source.EpisodeID, GroupKey: groupKey,
			Environment: group.Environment, Mode: string(mode)}}
	for _, test := range tests {
		found := false
		for _, decision := range manifest.Decisions {
			if decision.Test.Repository == group.TestRepository && decision.Test.Adapter == group.Adapter &&
				decision.Test.SuiteKey == test.SuiteKey && decision.Test.TestKey == test.TestKey && decision.Outcome == test.Decision {
				request.Tests = append(request.Tests, Test{SuiteKey: test.SuiteKey, TestKey: test.TestKey, Name: decision.Test.Name})
				found = true
				break
			}
		}
		if !found {
			return nil, fmt.Errorf("%w: stage candidate missing from manifest", execution.ErrInvalid)
		}
	}
	if err := ValidateStagedRequest(request); err != nil {
		return nil, err
	}
	result, err := service.adapter.Execute(ctx, CanonicalRequest(request))
	if err != nil {
		return nil, fmt.Errorf("%w: %w", execution.ErrUnavailable, err)
	}
	result = CanonicalAdapterResult(result)
	if err := correlate(request, result); err != nil {
		return nil, err
	}
	attempt := buildAttempt(request, result)
	attempt.APIVersion, attempt.Staging = execution.StagedAttemptAPIVersion, request.Staging
	if err := execution.ValidateRecordedAttempt(attempt); err != nil {
		return nil, err
	}

	return &attempt, nil
}
