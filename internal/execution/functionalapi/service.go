// Package functionalapi executes one bounded group from a functional API manifest.
package functionalapi

import (
	"context"
	"fmt"

	"github.com/stanimirivanov/argus/internal/catalog"
	"github.com/stanimirivanov/argus/internal/execution"
	"github.com/stanimirivanov/argus/internal/selection"
)

// Adapter executes an exact request and returns normalized, untrusted output.
type Adapter interface {
	Execute(context.Context, execution.Request) (execution.AdapterResult, error)
}

// Options identifies one explicit repository/adapter execution group.
type Options struct {
	AttemptID      string
	ManifestSHA256 string
	Stage          execution.Stage
	TestRepository catalog.RepositoryIdentity
	TestRevision   catalog.Revision
	Adapter        string
}

// Service correlates manifest decisions with one adapter invocation.
type Service struct {
	adapter Adapter
}

// NewService constructs the functional API execution use case.
func NewService(adapter Adapter) *Service {
	return &Service{adapter: adapter}
}

// Execute invokes the configured adapter for one exact manifest group and
// returns normalized attempt evidence. The adapter cannot expand the test set.
func (service *Service) Execute(
	ctx context.Context,
	manifest selection.Manifest,
	options Options,
) (execution.Attempt, error) {
	if service == nil || service.adapter == nil {
		return execution.Attempt{}, execution.ErrInvalid
	}
	manifest = selection.CanonicalManifest(manifest)
	if err := selection.ValidateManifest(manifest); err != nil {
		return execution.Attempt{}, fmt.Errorf("%w: manifest", execution.ErrInvalid)
	}
	request, err := buildRequest(manifest, options)
	if err != nil {
		return execution.Attempt{}, err
	}
	result, err := service.adapter.Execute(ctx, request)
	if err != nil {
		return execution.Attempt{}, fmt.Errorf("%w: %w", execution.ErrUnavailable, err)
	}
	result = execution.CanonicalAdapterResult(result)
	if err := correlate(request, result); err != nil {
		return execution.Attempt{}, err
	}

	return buildAttempt(request, result), nil
}

func buildRequest(manifest selection.Manifest, options Options) (execution.Request, error) {
	request := execution.Request{
		APIVersion: execution.FunctionalAPIAdapterRequestAPIVersion,
		AttemptID:  options.AttemptID,
		Manifest: execution.ManifestReference{
			APIVersion: manifest.APIVersion, SHA256: options.ManifestSHA256,
		},
		Stage: options.Stage, TestRevision: options.TestRevision, Adapter: options.Adapter,
	}
	for _, decision := range manifest.Decisions {
		if decision.Test.Repository.Identity != options.TestRepository ||
			decision.Test.Adapter != options.Adapter || !included(decision, options.Stage) {
			continue
		}
		if request.TestRepository.Identity == (catalog.RepositoryIdentity{}) {
			request.TestRepository = decision.Test.Repository
		} else if request.TestRepository != decision.Test.Repository {
			return execution.Request{}, fmt.Errorf("%w: conflicting repository coordinates", execution.ErrInvalid)
		}
		request.Tests = append(request.Tests, execution.Test{
			SuiteKey: decision.Test.SuiteKey, TestKey: decision.Test.TestKey, Name: decision.Test.Name,
		})
	}
	request = execution.CanonicalRequest(request)
	if err := execution.ValidateRequest(request); err != nil {
		return execution.Request{}, err
	}

	return request, nil
}

func included(decision selection.Decision, stage execution.Stage) bool {
	return stage == execution.StageFullSuite ||
		(stage == execution.StageSelected && decision.Outcome == selection.OutcomeRunRequired)
}

func correlate(request execution.Request, result execution.AdapterResult) error {
	if err := execution.ValidateAdapterResult(result); err != nil {
		return err
	}
	if result.AttemptID != request.AttemptID || result.AdapterID != request.Adapter ||
		len(result.Results) != len(request.Tests) {
		return fmt.Errorf("%w: adapter result correlation", execution.ErrInvalid)
	}
	expected := make(map[catalog.TestIdentity]struct{}, len(request.Tests))
	for _, test := range request.Tests {
		expected[test.Identity(request.TestRepository.Identity)] = struct{}{}
	}
	for _, result := range result.Results {
		identity := result.Identity(request.TestRepository.Identity)
		if _, exists := expected[identity]; !exists {
			return fmt.Errorf("%w: unexpected adapter test result", execution.ErrInvalid)
		}
		delete(expected, identity)
	}
	if len(expected) != 0 {
		return fmt.Errorf("%w: missing adapter test result", execution.ErrInvalid)
	}

	return nil
}

func buildAttempt(request execution.Request, result execution.AdapterResult) execution.Attempt {
	return execution.Attempt{
		APIVersion: execution.AttemptAPIVersion, AttemptID: request.AttemptID,
		Manifest: request.Manifest, Stage: request.Stage,
		TestRepository: request.TestRepository, TestRevision: request.TestRevision,
		AdapterID: result.AdapterID, AdapterVersion: result.AdapterVersion,
		StartedAt: result.StartedAt, CompletedAt: result.CompletedAt,
		Outcome: execution.DeriveAttemptOutcome(result.Results),
		Results: result.Results, Artifacts: result.Artifacts,
	}
}
