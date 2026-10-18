// Package browserplanning turns reviewed browser bindings into deterministic CI jobs.
package browserplanning

import (
	"cmp"
	"fmt"
	"regexp"
	"slices"

	"github.com/stanimirivanov/argus/internal/catalog"
	"github.com/stanimirivanov/argus/internal/execution"
	"github.com/stanimirivanov/argus/internal/execution/browser"
	"github.com/stanimirivanov/argus/internal/selection"
)

const (
	// BindingsVersion identifies reviewed browser execution coordinates.
	BindingsVersion = "argus.dev/browser-execution-bindings/v1"
	// PlanVersion identifies the browser CI job plan.
	PlanVersion = "argus.dev/browser-execution-plan/v1"
	// MaxGroups bounds the reviewed browser execution matrix.
	MaxGroups = 10000
)

var digestPattern = regexp.MustCompile(`^[0-9a-f]{64}$`)
var projectPattern = regexp.MustCompile(`^[a-z][a-z0-9._-]{0,62}$`)

// Binding is reviewed CI authority for a checkout, project, and environment.
// Executable commands and credentials never flow from the selection manifest.
type Binding struct {
	GroupKey       string
	TestRepository catalog.Repository
	TestRevision   catalog.Revision
	Adapter        string
	Project        string
	Environment    string
}

// Bindings contains all reviewed groups for one browser selection manifest.
type Bindings struct {
	APIVersion string
	Groups     []Binding
}

// Job runs one browser group at the selected or full-suite stage.
type Job struct {
	Binding
	Stage     execution.Stage
	TestCount int
}

// Plan is a deterministic set of browser CI jobs bound to manifest bytes.
type Plan struct {
	APIVersion string
	Manifest   execution.ManifestReference
	Jobs       []Job
}

type groupIdentity struct {
	Repository  catalog.RepositoryIdentity
	Adapter     string
	Project     string
	Environment string
}

type projectHint struct {
	Decision int
	Project  string
}

// CanonicalPlan orders jobs by group and selected-before-control stage.
func CanonicalPlan(plan Plan) Plan {
	plan.Jobs = append([]Job{}, plan.Jobs...)
	slices.SortFunc(plan.Jobs, func(a, b Job) int {
		if n := cmp.Compare(a.GroupKey, b.GroupKey); n != 0 {
			return n
		}
		if a.Stage == b.Stage {
			return 0
		}
		if a.Stage == execution.StageSelected {
			return -1
		}

		return 1
	})

	return plan
}

// ValidatePlan requires one control per group and at most one selected job.
func ValidatePlan(plan Plan) error {
	if plan.APIVersion != PlanVersion || plan.Manifest.APIVersion != browser.ManifestVersion || !digestPattern.MatchString(plan.Manifest.SHA256) || len(plan.Jobs) == 0 || len(plan.Jobs) > 2*MaxGroups {
		return browser.ErrInvalid
	}
	groups, err := indexPlanJobs(plan.Jobs)
	if err != nil {
		return err
	}
	for _, job := range plan.Jobs {
		full, ok := groups[job.GroupKey]
		if !ok || full.Binding != job.Binding || (job.Stage == execution.StageSelected && job.TestCount > full.TestCount) {
			return browser.ErrInvalid
		}
	}

	return nil
}

func indexPlanJobs(jobs []Job) (map[string]Job, error) {
	groups := map[string]Job{}
	selected := map[string]bool{}
	identities := map[groupIdentity]bool{}
	for _, job := range jobs {
		if validateBinding(job.Binding) != nil || job.TestCount < 1 || job.TestCount > execution.MaxAttemptTests || (job.Stage != execution.StageSelected && job.Stage != execution.StageFullSuite) {
			return nil, browser.ErrInvalid
		}
		if job.Stage == execution.StageSelected {
			if selected[job.GroupKey] {
				return nil, browser.ErrInvalid
			}
			selected[job.GroupKey] = true
		} else {
			if _, ok := groups[job.GroupKey]; ok {
				return nil, browser.ErrInvalid
			}
			identity := identityOf(job.Binding)
			if identities[identity] {
				return nil, browser.ErrInvalid
			}
			identities[identity] = true
			groups[job.GroupKey] = job
		}
	}
	if len(groups) > MaxGroups {
		return nil, browser.ErrInvalid
	}

	return groups, nil
}

func identityOf(b Binding) groupIdentity {
	return groupIdentity{Repository: b.TestRepository.Identity, Adapter: b.Adapter, Project: b.Project, Environment: b.Environment}
}

func validateBinding(b Binding) error {
	if !catalog.IsLocalKey(b.GroupKey) || b.Adapter != "playwright" || !projectPattern.MatchString(b.Project) || !projectPattern.MatchString(b.Environment) || execution.ValidateTestRepository(b.TestRepository) != nil || execution.ValidateTestRevision(b.TestRevision) != nil {
		return browser.ErrInvalid
	}
	return nil
}

// Build expands every applicable browser project/environment group into a
// selected job when needed and a mandatory full-suite control job.
func Build(manifest browser.Manifest, digest string, bindings Bindings) (Plan, error) {
	if browser.ValidateManifest(manifest) != nil || bindings.APIVersion != BindingsVersion || len(bindings.Groups) == 0 || len(bindings.Groups) > MaxGroups {
		return Plan{}, browser.ErrInvalid
	}
	reference := execution.ManifestReference{APIVersion: manifest.APIVersion, SHA256: digest}
	if !digestPattern.MatchString(digest) {
		return Plan{}, browser.ErrInvalid
	}
	seenKey := map[string]bool{}
	seenIdentity := map[groupIdentity]bool{}
	covered := make([]bool, len(manifest.Decisions))
	projectCoverage := map[projectHint]bool{}
	plan := Plan{APIVersion: PlanVersion, Manifest: reference, Jobs: make([]Job, 0, 2*len(bindings.Groups))}
	for _, b := range bindings.Groups {
		if validateBinding(b) != nil || seenKey[b.GroupKey] {
			return Plan{}, browser.ErrInvalid
		}
		seenKey[b.GroupKey] = true
		id := identityOf(b)
		if seenIdentity[id] {
			return Plan{}, browser.ErrInvalid
		}
		seenIdentity[id] = true
		selected, full, err := countGroup(b, manifest.Decisions, covered, projectCoverage)
		if err != nil {
			return Plan{}, err
		}
		if full == 0 {
			return Plan{}, fmt.Errorf("%w: empty reviewed browser group", browser.ErrInvalid)
		}
		if selected > 0 {
			plan.Jobs = append(plan.Jobs, Job{Binding: b, Stage: execution.StageSelected, TestCount: selected})
		}
		plan.Jobs = append(plan.Jobs, Job{Binding: b, Stage: execution.StageFullSuite, TestCount: full})
	}
	if err := requireCoverage(manifest.Decisions, covered, projectCoverage); err != nil {
		return Plan{}, err
	}
	plan = CanonicalPlan(plan)
	if err := ValidatePlan(plan); err != nil {
		return Plan{}, err
	}

	return plan, nil
}

func countGroup(b Binding, decisions []browser.Decision, covered []bool, projects map[projectHint]bool) (int, int, error) {
	var selected, full int
	for i, d := range decisions {
		if d.Repository.Identity != b.TestRepository.Identity {
			continue
		}
		if d.Repository != b.TestRepository || d.Revision != b.TestRevision {
			return 0, 0, fmt.Errorf("%w: browser group coordinates", browser.ErrInvalid)
		}
		if len(d.Projects) > 0 && !slices.Contains(d.Projects, b.Project) {
			continue
		}
		covered[i] = true
		projects[projectHint{Decision: i, Project: b.Project}] = true
		full++
		if d.Outcome == selection.OutcomeRunRequired {
			selected++
		}
	}

	return selected, full, nil
}

func requireCoverage(decisions []browser.Decision, covered []bool, projects map[projectHint]bool) error {
	for i, d := range decisions {
		if !covered[i] {
			return fmt.Errorf("%w: missing browser binding", browser.ErrInvalid)
		}
		for _, project := range d.Projects {
			if !projects[projectHint{Decision: i, Project: project}] {
				return fmt.Errorf("%w: missing browser project binding", browser.ErrInvalid)
			}
		}
	}

	return nil
}
