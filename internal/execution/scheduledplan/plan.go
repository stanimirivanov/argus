// Package scheduledplan defines runner-neutral staged partitions and correlation.
package scheduledplan

import (
	"cmp"
	"fmt"
	"regexp"
	"slices"

	"github.com/stanimirivanov/argus/internal/catalog"
	"github.com/stanimirivanov/argus/internal/execution"
	"github.com/stanimirivanov/argus/internal/selection"
)

// Mode defines required passes, without granting permission to execute them.
type Mode string

const (
	// Shadow pairs relevant-now with an independent complete control pass.
	Shadow Mode = "shadow"
	// Staged pairs relevant-now with its mandatory remaining-later complement.
	Staged Mode = "staged"
)

var digestPattern = regexp.MustCompile("^[0-9a-f]{64}$")

// Binding pins one independently reviewed execution scope.
type Binding struct {
	GroupKey                      string
	TestRepository                catalog.Repository
	TestRevision                  catalog.Revision
	Family                        catalog.TestFamily
	Adapter, Project, Environment string
}

// Test maps a portfolio ID to its stable group-local runner identity.
// Decision is the explicit original must-run/skip policy; its zero value is invalid.
type Test struct {
	ID, SuiteKey, TestKey string
	Decision              selection.Outcome
}

// Selection retains the complete independent candidate inventory and early order.
type Selection struct {
	Binding
	Candidates  []Test
	RelevantNow []string
}

// Schedule is projected from a verified portfolio episode and independent
// manifest/inventory/bindings. Callers must not derive missing candidates or
// bindings from an untrusted Plan they are trying to validate.
type Schedule struct {
	Reference                 execution.ScheduleReference
	EpisodeID, ManifestSHA256 string
	Groups                    []Selection
}

// Group retains the exact partition even when shadow control covers the later set.
type Group struct {
	Selection
	RemainingLater []string
}

// Plan is an owned canonical projection. It is not a v1 runner wire document.
type Plan struct {
	Schedule                  execution.ScheduleReference
	EpisodeID, ManifestSHA256 string
	Mode                      Mode
	Groups                    []Group
}

// Build validates the complete source, preserves early order, and computes the
// exact later complement. Empty phases are valid and require no runner invocation.
func Build(source Schedule, mode Mode) (Plan, error) {
	if execution.ValidateScheduleReference(source.Reference) != nil ||
		!catalog.IsLocalKey(source.EpisodeID) || !digestPattern.MatchString(source.ManifestSHA256) ||
		(mode != Shadow && mode != Staged) || len(source.Groups) == 0 || len(source.Groups) > execution.MaxAttemptTests {
		return Plan{}, fmt.Errorf("%w: staged plan source", execution.ErrInvalid)
	}
	plan := Plan{Schedule: source.Reference, EpisodeID: source.EpisodeID, ManifestSHA256: source.ManifestSHA256,
		Mode: mode, Groups: make([]Group, 0, len(source.Groups))}
	keys, identities, ids := map[string]bool{}, map[groupIdentity]bool{}, map[string]bool{}
	total := 0
	for _, input := range source.Groups {
		identity := identityOf(input.Binding)
		if !validBinding(input.Binding) || keys[input.GroupKey] || identities[identity] {
			return Plan{}, fmt.Errorf("%w: ambiguous staged scope", execution.ErrInvalid)
		}
		keys[input.GroupKey], identities[identity] = true, true

		if len(input.Candidates) == 0 || len(input.Candidates) > execution.MaxAttemptTests-total {
			return Plan{}, fmt.Errorf("%w: staged candidate limit", execution.ErrInvalid)
		}
		total += len(input.Candidates)
		group, err := partition(input, ids)
		if err != nil {
			return Plan{}, err
		}
		plan.Groups = append(plan.Groups, group)
	}

	return Canonical(plan), nil
}

func partition(input Selection, ids map[string]bool) (Group, error) {
	candidates, runnerIDs := map[string]Test{}, map[[2]string]bool{}
	for _, test := range input.Candidates {
		identity := [2]string{test.SuiteKey, test.TestKey}
		if !validTest(test) || ids[test.ID] || runnerIDs[identity] {
			return Group{}, fmt.Errorf("%w: duplicate or invalid staged candidate", execution.ErrInvalid)
		}
		ids[test.ID], runnerIDs[identity] = true, true
		candidates[test.ID] = test
	}
	if len(input.RelevantNow) > len(input.Candidates) {
		return Group{}, fmt.Errorf("%w: early candidate limit", execution.ErrInvalid)
	}
	early := map[string]bool{}
	for _, id := range input.RelevantNow {
		if _, exists := candidates[id]; !exists || early[id] {
			return Group{}, fmt.Errorf("%w: unknown or duplicate early test", execution.ErrInvalid)
		}
		early[id] = true
	}
	group := Group{Selection: input, RemainingLater: []string{}}
	for _, test := range input.Candidates {
		if test.Decision == selection.OutcomeRunRequired && !early[test.ID] {
			return Group{}, fmt.Errorf("%w: required test deferred", execution.ErrInvalid)
		}
		if !early[test.ID] {
			group.RemainingLater = append(group.RemainingLater, test.ID)
		}
	}
	slices.Sort(group.RemainingLater)

	return group, nil
}

// Canonical deep-copies groups and memberships; early order remains schedule order.
func Canonical(plan Plan) Plan {
	plan.Groups = slices.Clone(plan.Groups)
	for i := range plan.Groups {
		group := &plan.Groups[i]
		group.Candidates = slices.Clone(group.Candidates)
		slices.SortFunc(group.Candidates, func(a, b Test) int { return cmp.Compare(a.ID, b.ID) })
		group.RelevantNow = append([]string{}, group.RelevantNow...)
		group.RemainingLater = append([]string{}, group.RemainingLater...)
	}
	slices.SortFunc(plan.Groups, func(a, b Group) int { return cmp.Compare(a.GroupKey, b.GroupKey) })

	return plan
}

// Validate correlates every immutable field and exact partition with independently
// supplied source and expected mode. A plan cannot authorize its own mode downgrade.
func Validate(plan Plan, source Schedule, mode Mode) error {
	expected, err := Build(source, mode)
	if err != nil {
		return err
	}
	if plan.Schedule != expected.Schedule || plan.EpisodeID != expected.EpisodeID ||
		plan.ManifestSHA256 != expected.ManifestSHA256 || plan.Mode != expected.Mode || len(plan.Groups) != len(expected.Groups) {
		return fmt.Errorf("%w: staged plan correlation", execution.ErrInvalid)
	}
	total := 0
	for _, group := range plan.Groups {
		if len(group.Candidates) > execution.MaxAttemptTests-total ||
			len(group.RelevantNow) > len(group.Candidates) || len(group.RemainingLater) > len(group.Candidates) {
			return fmt.Errorf("%w: staged plan limit", execution.ErrInvalid)
		}
		total += len(group.Candidates)
	}
	actual := Canonical(plan)
	for i, group := range actual.Groups {
		want := expected.Groups[i]
		if group.Binding != want.Binding || !slices.Equal(group.Candidates, want.Candidates) ||
			!slices.Equal(group.RelevantNow, want.RelevantNow) || !slices.Equal(group.RemainingLater, want.RemainingLater) {
			return fmt.Errorf("%w: staged group partition or scope mismatch", execution.ErrInvalid)
		}
	}

	return nil
}

// TestsForStage returns an owned, ordered exact test list only after correlation.
// Shadow uses the full control instead of a separate remaining-later invocation.
// Execution and promotion authorization remain the caller's responsibility.
func TestsForStage(plan Plan, source Schedule, mode Mode, groupKey string, stage execution.Stage) ([]Test, error) {
	if err := Validate(plan, source, mode); err != nil {
		return nil, err
	}
	if execution.ValidateScheduledStage(stage) != nil ||
		(mode == Shadow && stage == execution.StageRemainingLater) || (mode == Staged && stage == execution.StageShadowControl) {
		return nil, fmt.Errorf("%w: stage incompatible with mode", execution.ErrInvalid)
	}
	for _, group := range plan.Groups {
		if group.GroupKey != groupKey {
			continue
		}
		candidates := map[string]Test{}
		for _, test := range group.Candidates {
			candidates[test.ID] = test
		}
		ids := group.RelevantNow
		if stage == execution.StageRemainingLater {
			ids = group.RemainingLater
		}
		if stage == execution.StageShadowControl {
			tests := slices.Clone(group.Candidates)
			slices.SortFunc(tests, func(a, b Test) int { return cmp.Compare(a.ID, b.ID) })
			return tests, nil
		}
		tests := make([]Test, 0, len(ids))
		for _, id := range ids {
			tests = append(tests, candidates[id])
		}

		return tests, nil
	}

	return nil, fmt.Errorf("%w: unknown staged group", execution.ErrInvalid)
}

type groupIdentity struct {
	Repository                    catalog.RepositoryIdentity
	Family                        catalog.TestFamily
	Adapter, Project, Environment string
}

func identityOf(binding Binding) groupIdentity {
	return groupIdentity{Repository: binding.TestRepository.Identity, Family: binding.Family,
		Adapter: binding.Adapter, Project: binding.Project, Environment: binding.Environment}
}

func validBinding(binding Binding) bool {
	projectValid := binding.Family == catalog.TestFamilyFunctionalAPI && binding.Project == "" ||
		binding.Family == catalog.TestFamilyFunctionalUI && catalog.IsLocalKey(binding.Project)

	return projectValid && catalog.IsLocalKey(binding.GroupKey) && catalog.IsLocalKey(binding.Adapter) &&
		catalog.IsLocalKey(binding.Environment) && execution.ValidateTestRepository(binding.TestRepository) == nil &&
		execution.ValidateTestRevision(binding.TestRevision) == nil
}

func validTest(test Test) bool {
	return digestPattern.MatchString(test.ID) && catalog.IsLocalKey(test.SuiteKey) &&
		catalog.IsLocalKey(test.TestKey) &&
		(test.Decision == selection.OutcomeRunRequired || test.Decision == selection.OutcomeSkipForNow)
}
