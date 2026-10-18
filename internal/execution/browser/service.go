package browser

import (
	"context"
	"fmt"
	"slices"
	"strings"

	"github.com/stanimirivanov/argus/internal/catalog"
	"github.com/stanimirivanov/argus/internal/execution"
	"github.com/stanimirivanov/argus/internal/selection"
)

// Adapter executes and verifies one browser request outside the use case.
type Adapter interface {
	Execute(context.Context, Request) (Result, error)
	VerifyArtifacts(context.Context, Result) error
}

// Manifest is the browser execution view of a validated selection manifest.
type Manifest struct {
	APIVersion string
	Decisions  []Decision
}

// Decision is the execution view of one validated browser selection decision.
type Decision struct {
	Repository catalog.Repository
	SuiteKey   string
	TestKey    string
	Name       string
	Revision   catalog.Revision
	Projects   []string
	Outcome    selection.Outcome
}

// Options chooses one immutable repository, project, environment, and stage.
type Options struct {
	AttemptID      string
	ManifestSHA256 string
	Stage          execution.Stage
	TestRepository catalog.RepositoryIdentity
	TestRevision   catalog.Revision
	Adapter        string
	Project        string
	Environment    string
}

// Service correlates browser manifest decisions with adapter evidence.
type Service struct{ adapter Adapter }

// NewService constructs the browser execution use case.
func NewService(adapter Adapter) *Service { return &Service{adapter: adapter} }

// Execute runs only the selected tests for one explicit browser group.
func (service *Service) Execute(ctx context.Context, manifest Manifest, options Options) (Attempt, error) {
	if service == nil || service.adapter == nil || validateManifest(manifest) != nil {
		return Attempt{}, ErrInvalid
	}
	request := Request{APIVersion: RequestVersion, AttemptID: options.AttemptID,
		Manifest: execution.ManifestReference{APIVersion: manifest.APIVersion, SHA256: options.ManifestSHA256},
		Stage:    options.Stage, TestRevision: options.TestRevision, Adapter: options.Adapter, Project: options.Project, Environment: options.Environment, Tests: []Test{}}
	for _, decision := range manifest.Decisions {
		if decision.Repository.Identity != options.TestRepository {
			continue
		}
		if decision.Revision != options.TestRevision {
			return Attempt{}, fmt.Errorf("%w: test revision differs from manifest binding", ErrInvalid)
		}
		if options.Stage == execution.StageSelected && decision.Outcome != selection.OutcomeRunRequired {
			continue
		}
		if len(decision.Projects) > 0 && !slices.Contains(decision.Projects, options.Project) {
			continue
		}
		if request.TestRepository.Identity == (catalog.RepositoryIdentity{}) {
			request.TestRepository = decision.Repository
		} else if request.TestRepository != decision.Repository {
			return Attempt{}, ErrInvalid
		}
		request.Tests = append(request.Tests, Test{SuiteKey: decision.SuiteKey, TestKey: decision.TestKey, Name: decision.Name})
	}
	if err := ValidateRequest(request); err != nil {
		return Attempt{}, err
	}
	result, err := service.adapter.Execute(ctx, request)
	if err != nil {
		return Attempt{}, fmt.Errorf("%w: %w", execution.ErrUnavailable, err)
	}
	if err := ValidateResult(request, result); err != nil {
		return Attempt{}, err
	}
	if err := service.adapter.VerifyArtifacts(ctx, result); err != nil {
		return Attempt{}, err
	}
	attempt := CanonicalAttempt(Attempt{APIVersion: AttemptVersion, Request: request, Result: result, Outcome: execution.DeriveAttemptOutcome(result.Results)})

	return attempt, nil
}

func validateManifest(manifest Manifest) error {
	if manifest.APIVersion != ManifestVersion || len(manifest.Decisions) == 0 || len(manifest.Decisions) > execution.MaxAttemptTests {
		return ErrInvalid
	}
	seen := map[string]bool{}
	for _, decision := range manifest.Decisions {
		id := decision.Repository.Identity
		key := string(id.Provider) + "\x00" + id.Host + "\x00" + id.ProviderRepositoryID + "\x00" + decision.SuiteKey + "\x00" + decision.TestKey
		if !decision.Repository.Valid() || !decision.Revision.Valid() || !catalog.IsLocalKey(decision.SuiteKey) || !catalog.IsLocalKey(decision.TestKey) ||
			strings.TrimSpace(decision.Name) == "" || len(decision.Name) > 255 || seen[key] ||
			(decision.Outcome != selection.OutcomeRunRequired && decision.Outcome != selection.OutcomeSkipForNow) {
			return ErrInvalid
		}
		seen[key] = true
		projects := map[string]bool{}
		for _, project := range decision.Projects {
			if !catalog.IsLocalKey(project) || projects[project] {
				return ErrInvalid
			}
			projects[project] = true
		}
	}

	return nil
}
