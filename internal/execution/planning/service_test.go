package planning

import (
	"errors"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/stanimirivanov/argus/internal/catalog"
	"github.com/stanimirivanov/argus/internal/change"
	"github.com/stanimirivanov/argus/internal/execution"
	"github.com/stanimirivanov/argus/internal/selection"
)

func TestBuildCreatesDeterministicHeterogeneousStageJobs(t *testing.T) {
	t.Parallel()
	manifest := planningManifest()
	bindings := planningBindings()
	bindings.Groups[0], bindings.Groups[1] = bindings.Groups[1], bindings.Groups[0]

	plan, err := Build(manifest, manifestDigest(), bindings)
	if err != nil {
		t.Fatalf("build execution plan: %v", err)
	}
	if len(plan.Jobs) != 3 {
		t.Fatalf("job count = %d, want 3", len(plan.Jobs))
	}
	want := []struct {
		key   string
		stage execution.Stage
		count int
	}{
		{key: "orders-playwright", stage: execution.StageSelected, count: 1},
		{key: "orders-playwright", stage: execution.StageFullSuite, count: 2},
		{key: "payments-pytest", stage: execution.StageFullSuite, count: 1},
	}
	for index, job := range plan.Jobs {
		if job.GroupKey != want[index].key || job.Stage != want[index].stage ||
			job.TestCount != want[index].count {
			t.Fatalf("job %d = %+v, want %+v", index, job, want[index])
		}
	}
	if plan.Jobs[2].TestRevision.Digest != strings.Repeat("c", 40) {
		t.Fatalf("payments revision = %q", plan.Jobs[2].TestRevision.Digest)
	}
}

func TestBuildRejectsMissingAndUnexpectedBindings(t *testing.T) {
	t.Parallel()
	manifest := planningManifest()
	bindings := planningBindings()
	bindings.Groups = bindings.Groups[:1]
	if _, err := Build(manifest, manifestDigest(), bindings); !errors.Is(err, execution.ErrInvalid) {
		t.Fatalf("missing binding error = %v", err)
	}

	bindings = planningBindings()
	bindings.Groups[0].TestRepository.Name = "renamed-without-new-manifest"
	if _, err := Build(manifest, manifestDigest(), bindings); !errors.Is(err, execution.ErrInvalid) {
		t.Fatalf("coordinate mismatch error = %v", err)
	}
}

func TestCanonicalBindingsDoesNotMutateInput(t *testing.T) {
	t.Parallel()
	bindings := planningBindings()
	original := append([]Binding{}, bindings.Groups...)
	_ = CanonicalBindings(bindings)
	if !reflect.DeepEqual(bindings.Groups, original) {
		t.Fatal("canonical bindings mutated caller-owned slice")
	}
}

func TestValidateBindingsRejectsAmbiguousGroups(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name   string
		mutate func(*Bindings)
	}{
		{
			name: "duplicate group key",
			mutate: func(bindings *Bindings) {
				bindings.Groups[1].GroupKey = bindings.Groups[0].GroupKey
			},
		},
		{
			name: "duplicate repository adapter identity",
			mutate: func(bindings *Bindings) {
				bindings.Groups[1].TestRepository = bindings.Groups[0].TestRepository
				bindings.Groups[1].Adapter = bindings.Groups[0].Adapter
			},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			bindings := planningBindings()
			test.mutate(&bindings)
			if err := ValidateBindings(bindings); !errors.Is(err, execution.ErrInvalid) {
				t.Fatalf("ambiguous bindings error = %v", err)
			}
		})
	}
}

func TestValidatePlanRejectsSelectedJobWithoutControl(t *testing.T) {
	t.Parallel()
	binding := planningBindings().Groups[0]
	plan := Plan{
		APIVersion: PlanAPIVersion,
		Manifest: execution.ManifestReference{
			APIVersion: selection.ManifestAPIVersion, SHA256: manifestDigest(),
		},
		Jobs: []Job{newJob(binding, execution.StageSelected, 1)},
	}
	if err := ValidatePlan(plan); !errors.Is(err, execution.ErrInvalid) {
		t.Fatalf("selected-only plan error = %v", err)
	}
}

func planningBindings() Bindings {
	return Bindings{
		APIVersion: BindingsAPIVersion,
		Groups: []Binding{
			{
				GroupKey: "orders-playwright", TestRepository: ordersRepository(),
				TestRevision: catalog.Revision{
					Algorithm: catalog.RevisionGitSHA1, Digest: strings.Repeat("b", 40),
				},
				Adapter: "playwright",
			},
			{
				GroupKey: "payments-pytest", TestRepository: paymentsRepository(),
				TestRevision: catalog.Revision{
					Algorithm: catalog.RevisionGitSHA1, Digest: strings.Repeat("c", 40),
				},
				Adapter: "pytest",
			},
		},
	}
}

func planningManifest() selection.Manifest {
	source := catalog.Repository{
		Identity: catalog.RepositoryIdentity{
			Provider: catalog.ProviderGitHub, Host: "github.com", ProviderRepositoryID: "source-42",
		},
		Owner: "example", Name: "shop",
	}
	base := catalog.Revision{
		Algorithm: catalog.RevisionGitSHA1, Digest: strings.Repeat("0", 40),
	}

	return selection.Manifest{
		APIVersion: selection.ManifestAPIVersion, PolicyVersion: selection.FunctionalAPIPolicyVersion,
		ImpactAPIVersion: change.ImpactAPIVersion, ImpactAnalyzerVersion: change.OpenAPIAnalyzerVersion,
		Change: change.Reference{
			SourceRepository: source, PullRequestNumber: 42, BaseRevision: base,
			HeadRevision: catalog.Revision{
				Algorithm: catalog.RevisionGitSHA1, Digest: strings.Repeat("1", 40),
			},
			ObservedAt: time.Date(2026, 9, 26, 8, 0, 0, 0, time.UTC),
			Trigger: change.Trigger{
				Provider: catalog.ProviderGitHub, DeliveryID: "delivery-42",
				Event: "pull_request", Action: "synchronize",
			},
		},
		Catalog: catalog.SnapshotReference{
			SourceRepository: source, Revision: base,
			DescriptorAPIVersion: "argus.dev/repository-descriptor/v1",
		},
		Family: catalog.TestFamilyFunctionalAPI, Mode: selection.ModeTargeted,
		AffectedCapabilities: []string{"create-order"},
		Decisions: []selection.Decision{
			planningDecision(ordersRepository(), "orders-api", "playwright", "create", selection.OutcomeRunRequired),
			planningDecision(ordersRepository(), "orders-api", "playwright", "list", selection.OutcomeSkipForNow),
			planningDecision(paymentsRepository(), "payments-api", "pytest", "refund", selection.OutcomeSkipForNow),
		},
	}
}

func planningDecision(
	repository catalog.Repository,
	suiteKey string,
	adapter string,
	testKey string,
	outcome selection.Outcome,
) selection.Decision {
	remaining := selection.RemainingFullSuite
	reason := selection.Reason{Code: selection.ReasonNotAffected}
	if outcome == selection.OutcomeRunRequired {
		remaining = selection.RemainingNone
		reason = selection.Reason{
			Code: selection.ReasonDirectCapabilityImpact, Capabilities: []string{"create-order"},
		}
	}

	return selection.Decision{
		Test: selection.TestReference{
			Repository: repository, SuiteKey: suiteKey,
			Family: catalog.TestFamilyFunctionalAPI, Adapter: adapter,
			TestKey: testKey, Name: testKey, Capabilities: []string{"create-order"},
		},
		Outcome: outcome, RemainingExecution: remaining, Reasons: []selection.Reason{reason},
	}
}

func ordersRepository() catalog.Repository {
	return catalog.Repository{
		Identity: catalog.RepositoryIdentity{
			Provider: catalog.ProviderGitHub, Host: "github.com", ProviderRepositoryID: "tests-orders",
		},
		Owner: "example", Name: "orders-tests",
	}
}

func paymentsRepository() catalog.Repository {
	return catalog.Repository{
		Identity: catalog.RepositoryIdentity{
			Provider: catalog.ProviderGitHub, Host: "github.com", ProviderRepositoryID: "tests-payments",
		},
		Owner: "example", Name: "payments-tests",
	}
}

func manifestDigest() string { return strings.Repeat("a", 64) }
