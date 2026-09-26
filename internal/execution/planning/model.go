// Package planning turns one selection manifest into bounded CI execution jobs.
package planning

import (
	"cmp"
	"fmt"
	"slices"
	"strings"

	"github.com/stanimirivanov/argus/internal/catalog"
	"github.com/stanimirivanov/argus/internal/execution"
	"github.com/stanimirivanov/argus/internal/selection"
)

const (
	// BindingsAPIVersion identifies reviewed repository/revision bindings v1.
	BindingsAPIVersion = "argus.dev/functional-api-execution-bindings/v1"
	// PlanAPIVersion identifies the flattened CI execution plan v1.
	PlanAPIVersion = "argus.dev/functional-api-execution-plan/v1"
	// MaxGroups bounds distinct repository/adapter execution groups.
	MaxGroups = selection.MaxManifestDecisions
	// MaxJobs allows one selected and one full-suite job per group.
	MaxJobs = 2 * MaxGroups
)

// Binding supplies reviewed execution configuration that selection must never
// control: a stable group key and immutable test-repository revision.
type Binding struct {
	GroupKey       string
	TestRepository catalog.Repository
	TestRevision   catalog.Revision
	Adapter        string
}

// Bindings supplies reviewed execution configuration. Planning requires its
// group set to match the current manifest exactly.
type Bindings struct {
	APIVersion string
	Groups     []Binding
}

// Job is one repository/adapter/stage invocation suitable for a CI matrix.
// Adapter commands remain reviewed CI configuration and are deliberately not
// part of this value.
type Job struct {
	GroupKey       string
	Stage          execution.Stage
	TestRepository catalog.Repository
	TestRevision   catalog.Revision
	Adapter        string
	TestCount      int
}

// Plan is a deterministic flat job list bound to canonical manifest bytes.
type Plan struct {
	APIVersion string
	Manifest   execution.ManifestReference
	Jobs       []Job
}

// CanonicalBindings deep-copies and orders bindings by stable group key.
func CanonicalBindings(bindings Bindings) Bindings {
	canonical := bindings
	canonical.Groups = append([]Binding{}, bindings.Groups...)
	slices.SortFunc(canonical.Groups, func(left, right Binding) int {
		return cmp.Compare(left.GroupKey, right.GroupKey)
	})

	return canonical
}

// CanonicalPlan deep-copies jobs into stable group and selected-before-control order.
func CanonicalPlan(plan Plan) Plan {
	canonical := plan
	canonical.Jobs = append([]Job{}, plan.Jobs...)
	slices.SortFunc(canonical.Jobs, compareJobs)

	return canonical
}

// ValidateBindings rejects ambiguous, unbounded, or non-executable bindings.
func ValidateBindings(bindings Bindings) error {
	if bindings.APIVersion != BindingsAPIVersion || len(bindings.Groups) > MaxGroups {
		return fmt.Errorf("%w: execution bindings envelope", execution.ErrInvalid)
	}
	keys := make(map[string]struct{}, len(bindings.Groups))
	identities := make(map[groupIdentity]struct{}, len(bindings.Groups))
	for _, binding := range bindings.Groups {
		if !catalog.IsLocalKey(binding.GroupKey) || !catalog.IsLocalKey(binding.Adapter) ||
			validateRepository(binding.TestRepository) != nil || validateRevision(binding.TestRevision) != nil {
			return fmt.Errorf("%w: execution group binding", execution.ErrInvalid)
		}
		if _, exists := keys[binding.GroupKey]; exists {
			return fmt.Errorf("%w: duplicate execution group key", execution.ErrInvalid)
		}
		keys[binding.GroupKey] = struct{}{}
		identity := groupIdentity{Repository: binding.TestRepository.Identity, Adapter: binding.Adapter}
		if _, exists := identities[identity]; exists {
			return fmt.Errorf("%w: duplicate execution group binding", execution.ErrInvalid)
		}
		identities[identity] = struct{}{}
	}

	return nil
}

// ValidatePlan verifies the portable job plan independently of its producer.
func ValidatePlan(plan Plan) error {
	if plan.APIVersion != PlanAPIVersion || execution.ValidateManifestReference(plan.Manifest) != nil ||
		len(plan.Jobs) > MaxJobs {
		return fmt.Errorf("%w: execution plan envelope", execution.ErrInvalid)
	}
	identities := make(map[jobIdentity]struct{}, len(plan.Jobs))
	groups := make(map[string]planGroup, len(plan.Jobs))
	for _, job := range plan.Jobs {
		if err := validatePlanJob(job); err != nil {
			return err
		}
		identity := jobIdentity{GroupKey: job.GroupKey, Stage: job.Stage}
		if _, exists := identities[identity]; exists {
			return fmt.Errorf("%w: duplicate execution plan job", execution.ErrInvalid)
		}
		identities[identity] = struct{}{}
		group, err := includePlanJob(groups[job.GroupKey], job)
		if err != nil {
			return err
		}
		groups[job.GroupKey] = group
	}
	for _, group := range groups {
		if !group.HasFullSuite || (group.HasSelected && group.SelectedTestCount > group.FullSuiteTestCount) {
			return fmt.Errorf("%w: incomplete execution plan group", execution.ErrInvalid)
		}
	}

	return nil
}

func validatePlanJob(job Job) error {
	if !catalog.IsLocalKey(job.GroupKey) || !catalog.IsLocalKey(job.Adapter) ||
		validateRepository(job.TestRepository) != nil || validateRevision(job.TestRevision) != nil ||
		(job.Stage != execution.StageSelected && job.Stage != execution.StageFullSuite) ||
		job.TestCount < 1 || job.TestCount > selection.MaxManifestDecisions {
		return fmt.Errorf("%w: execution plan job", execution.ErrInvalid)
	}

	return nil
}

func includePlanJob(group planGroup, job Job) (planGroup, error) {
	if group.Initialized && (group.Repository != job.TestRepository || group.Revision != job.TestRevision ||
		group.Adapter != job.Adapter) {
		return planGroup{}, fmt.Errorf("%w: inconsistent execution plan group", execution.ErrInvalid)
	}
	group.Initialized = true
	group.Repository = job.TestRepository
	group.Revision = job.TestRevision
	group.Adapter = job.Adapter
	if job.Stage == execution.StageSelected {
		group.SelectedTestCount = job.TestCount
		group.HasSelected = true
	} else {
		group.FullSuiteTestCount = job.TestCount
		group.HasFullSuite = true
	}

	return group, nil
}

type groupIdentity struct {
	Repository catalog.RepositoryIdentity
	Adapter    string
}

type jobIdentity struct {
	GroupKey string
	Stage    execution.Stage
}

type planGroup struct {
	Initialized        bool
	Repository         catalog.Repository
	Revision           catalog.Revision
	Adapter            string
	SelectedTestCount  int
	FullSuiteTestCount int
	HasSelected        bool
	HasFullSuite       bool
}

func validateRepository(repository catalog.Repository) error {
	identity := catalog.TestIdentity{
		TestRepository: repository.Identity, SuiteKey: "validation", TestKey: "validation",
	}
	if !identity.Valid() || strings.TrimSpace(repository.Owner) == "" || len(repository.Owner) > 255 ||
		strings.TrimSpace(repository.Name) == "" || len(repository.Name) > 255 {
		return execution.ErrInvalid
	}

	return nil
}

func validateRevision(revision catalog.Revision) error {
	normalized, err := catalog.NewRevision(revision.Algorithm, revision.Digest)
	if err != nil || normalized != revision {
		return execution.ErrInvalid
	}

	return nil
}

func compareJobs(left, right Job) int {
	if compared := cmp.Compare(left.GroupKey, right.GroupKey); compared != 0 {
		return compared
	}
	if left.Stage == right.Stage {
		return 0
	}
	if left.Stage == execution.StageSelected {
		return -1
	}

	return 1
}
