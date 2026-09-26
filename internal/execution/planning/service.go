package planning

import (
	"fmt"

	"github.com/stanimirivanov/argus/internal/catalog"
	"github.com/stanimirivanov/argus/internal/execution"
	"github.com/stanimirivanov/argus/internal/selection"
)

// Build converts every heterogeneous repository/adapter group into runnable
// selected and full-suite CI jobs using reviewed immutable revision bindings.
func Build(manifest selection.Manifest, manifestSHA256 string, bindings Bindings) (Plan, error) {
	manifest = selection.CanonicalManifest(manifest)
	if err := selection.ValidateManifest(manifest); err != nil {
		return Plan{}, fmt.Errorf("%w: manifest", execution.ErrInvalid)
	}
	bindings = CanonicalBindings(bindings)
	if err := ValidateBindings(bindings); err != nil {
		return Plan{}, err
	}
	manifestReference := execution.ManifestReference{
		APIVersion: manifest.APIVersion, SHA256: manifestSHA256,
	}
	if execution.ValidateManifestReference(manifestReference) != nil {
		return Plan{}, fmt.Errorf("%w: execution plan manifest digest", execution.ErrInvalid)
	}

	groups, err := collectManifestGroups(manifest)
	if err != nil {
		return Plan{}, err
	}
	boundGroups := make(map[groupIdentity]Binding, len(bindings.Groups))
	for _, binding := range bindings.Groups {
		identity := groupIdentity{Repository: binding.TestRepository.Identity, Adapter: binding.Adapter}
		manifestGroup, exists := groups[identity]
		if !exists || manifestGroup.Repository != binding.TestRepository {
			return Plan{}, fmt.Errorf("%w: unexpected execution group binding", execution.ErrInvalid)
		}
		boundGroups[identity] = binding
	}
	if len(boundGroups) != len(groups) {
		return Plan{}, fmt.Errorf("%w: missing execution group binding", execution.ErrInvalid)
	}

	plan := Plan{APIVersion: PlanAPIVersion, Manifest: manifestReference, Jobs: make([]Job, 0, 2*len(groups))}
	for identity, group := range groups {
		binding := boundGroups[identity]
		if group.SelectedTestCount > 0 {
			plan.Jobs = append(plan.Jobs, newJob(binding, execution.StageSelected, group.SelectedTestCount))
		}
		plan.Jobs = append(plan.Jobs, newJob(binding, execution.StageFullSuite, group.FullSuiteTestCount))
	}
	plan = CanonicalPlan(plan)
	if err := ValidatePlan(plan); err != nil {
		return Plan{}, err
	}

	return plan, nil
}

type manifestGroup struct {
	Repository         catalog.Repository
	SelectedTestCount  int
	FullSuiteTestCount int
}

func collectManifestGroups(manifest selection.Manifest) (map[groupIdentity]manifestGroup, error) {
	groups := make(map[groupIdentity]manifestGroup)
	for _, decision := range manifest.Decisions {
		if !catalog.IsLocalKey(decision.Test.Adapter) || validateRepository(decision.Test.Repository) != nil {
			return nil, fmt.Errorf("%w: non-executable manifest group", execution.ErrInvalid)
		}
		identity := groupIdentity{
			Repository: decision.Test.Repository.Identity, Adapter: decision.Test.Adapter,
		}
		group, exists := groups[identity]
		if exists && group.Repository != decision.Test.Repository {
			return nil, fmt.Errorf("%w: conflicting repository coordinates", execution.ErrInvalid)
		}
		group.Repository = decision.Test.Repository
		group.FullSuiteTestCount++
		if decision.Outcome == selection.OutcomeRunRequired {
			group.SelectedTestCount++
		}
		groups[identity] = group
	}

	return groups, nil
}

func newJob(binding Binding, stage execution.Stage, testCount int) Job {
	return Job{
		GroupKey: binding.GroupKey, Stage: stage,
		TestRepository: binding.TestRepository, TestRevision: binding.TestRevision,
		Adapter: binding.Adapter, TestCount: testCount,
	}
}
