package contract

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"slices"

	"github.com/stanimirivanov/argus/internal/catalog"
	"github.com/stanimirivanov/argus/internal/contracts"
	"github.com/stanimirivanov/argus/internal/execution"
	"github.com/stanimirivanov/argus/internal/execution/scheduledplan"
	"github.com/stanimirivanov/argus/internal/selection"
)

// APIStageSource verifies full artifact bytes and projects a single portfolio
// episode onto an independently supplied manifest and reviewed bindings.
func APIStageSource(data []byte, expectedSHA, episodeID string, manifest selection.Manifest, manifestSHA string,
	bindings contracts.FunctionalAPIStageBindingsV2) (scheduledplan.Schedule, map[string]int, error) {
	sum := sha256.Sum256(data)
	if len(data) > 16<<20 || hex.EncodeToString(sum[:]) != expectedSHA || selection.ValidateManifest(manifest) != nil {
		return scheduledplan.Schedule{}, nil, fmt.Errorf("%w: schedule bytes or manifest", execution.ErrInvalid)
	}
	if err := contracts.ValidateStagedDocument(bindings, "functional-api-execution-bindings"); err != nil {
		return scheduledplan.Schedule{}, nil, err
	}
	report, err := contracts.DecodePredictivePortfolioV1(data)
	if err != nil {
		return scheduledplan.Schedule{}, nil, err
	}
	var episode contracts.PredictivePortfolioEpisode
	var costs contracts.PredictiveStageCostEpisode
	for _, candidate := range report.Episodes {
		if candidate.ID == episodeID {
			episode = candidate
		}
	}
	for _, candidate := range report.Costs.Episodes {
		if candidate.ID == episodeID {
			costs = candidate
		}
	}
	if episode.ID == "" || costs.ManifestSHA256 != manifestSHA {
		return scheduledplan.Schedule{}, nil, fmt.Errorf("%w: schedule episode/manifest", execution.ErrInvalid)
	}
	source := scheduledplan.Schedule{Reference: execution.ScheduleReference{APIVersion: report.APIVersion, SHA256: expectedSHA}, EpisodeID: episodeID, ManifestSHA256: manifestSHA}
	assignments, costByID := map[string]bool{}, map[string]contracts.PredictiveStageTestCost{}
	for _, cost := range costs.Tests {
		costByID[cost.ID] = cost
	}

	for _, binding := range bindings.Groups {
		group, err := projectAPIStageGroup(binding, manifest, costByID, episode, assignments)
		if err != nil {
			return scheduledplan.Schedule{}, nil, err
		}
		source.Groups = append(source.Groups, group)
	}
	if len(assignments) != len(costs.Tests) || len(assignments) != len(manifest.Decisions) {
		return scheduledplan.Schedule{}, nil, fmt.Errorf("%w: incomplete schedule/manifest coverage", execution.ErrInvalid)
	}
	if _, err := scheduledplan.Build(source, scheduledplan.Shadow); err != nil {
		return scheduledplan.Schedule{}, nil, err
	}
	order, err := apiStageOrder(source, costs)

	return source, order, err
}

// The archived API observation identity predates reviewed environments: both
// project/environment are null. Execution environments are pinned separately.
func apiPortfolioTestID(repository catalog.RepositoryIdentity, adapter, suite, test string) (string, error) {
	data, err := json.Marshal(struct {
		Provider             string
		Host                 string
		ProviderRepositoryID string
		Family               string
		Adapter              string
		Project              *string
		Environment          *string
		SuiteKey             string
		TestKey              string
	}{string(repository.Provider), repository.Host, repository.ProviderRepositoryID, string(catalog.TestFamilyFunctionalAPI), adapter, nil, nil, suite, test})
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(data)

	return hex.EncodeToString(sum[:]), nil
}

func importStageRepository(value contracts.RepositoryReference) catalog.Repository {
	return catalog.Repository{Identity: catalog.RepositoryIdentity{Provider: catalog.Provider(value.Provider), Host: value.Host, ProviderRepositoryID: value.ProviderRepositoryID}, Owner: value.Owner, Name: value.Name}
}

// The current CLI dispatches whole repository/adapter groups. Reject cross-group
// dependencies instead of pretending a CI matrix can enforce interleaved tests.
func apiStageOrder(source scheduledplan.Schedule, costs contracts.PredictiveStageCostEpisode) (map[string]int, error) {
	tests, err := apiStageGraph(source, costs)
	if err != nil {
		return nil, err
	}
	ids := make([]string, 0, len(tests))
	for id := range tests {
		ids = append(ids, id)
	}
	slices.Sort(ids)
	order, visiting := map[string]int{}, map[string]bool{}
	var visit func(string) error
	visit = func(id string) error {
		if _, exists := order[id]; exists {
			return nil
		}
		if visiting[id] {
			return fmt.Errorf("%w: stage dependency cycle", execution.ErrInvalid)
		}
		visiting[id] = true
		dependencies := slices.Clone(tests[id].Dependencies)
		slices.Sort(dependencies)
		for _, dependency := range dependencies {
			if err := visit(dependency); err != nil {
				return err
			}
		}
		order[id] = len(order)
		delete(visiting, id)

		return nil
	}
	for _, id := range ids {
		if err := visit(id); err != nil {
			return nil, err
		}
	}

	return order, nil
}

// ExportAPIStagePlan encodes an owned, independently correlated v2 plan.
func ExportAPIStagePlan(plan scheduledplan.Plan, manifestVersion string) (contracts.FunctionalAPIStagePlanV2, error) {
	doc := contracts.FunctionalAPIStagePlanV2{APIVersion: "argus.dev/functional-api-execution-plan/v2", Schedule: contracts.ManifestDigestReference{APIVersion: plan.Schedule.APIVersion, SHA256: plan.Schedule.SHA256},
		EpisodeID: plan.EpisodeID, Manifest: contracts.ManifestDigestReference{APIVersion: manifestVersion, SHA256: plan.ManifestSHA256}, Mode: string(plan.Mode), Groups: []contracts.FunctionalAPIStageGroupV2{}}
	for _, group := range plan.Groups {
		entry := contracts.FunctionalAPIStageGroupV2{FunctionalAPIStageBindingV2: contracts.FunctionalAPIStageBindingV2{GroupKey: group.GroupKey, TestRepository: repositoryReference(group.TestRepository),
			TestRevision: revisionReference(group.TestRevision), Adapter: group.Adapter, Environment: group.Environment}, Candidates: []contracts.StagedCandidateV2{}, RelevantNow: group.RelevantNow, RemainingLater: group.RemainingLater}
		for _, test := range group.Candidates {
			entry.Candidates = append(entry.Candidates, contracts.StagedCandidateV2{ID: test.ID, SuiteKey: test.SuiteKey, TestKey: test.TestKey, Decision: string(test.Decision)})
		}
		doc.Groups = append(doc.Groups, entry)
	}

	return doc, contracts.ValidateStagedDocument(doc, "functional-api-execution-plan")
}

// ImportAPIStagePlan rejects omission/substitution against the independent source.
func ImportAPIStagePlan(doc contracts.FunctionalAPIStagePlanV2, source scheduledplan.Schedule, mode scheduledplan.Mode, manifestVersion string) (scheduledplan.Plan, error) {
	if err := contracts.ValidateStagedDocument(doc, "functional-api-execution-plan"); err != nil {
		return scheduledplan.Plan{}, err
	}
	if doc.Manifest.APIVersion != manifestVersion {
		return scheduledplan.Plan{}, execution.ErrInvalid
	}
	plan := scheduledplan.Plan{Schedule: execution.ScheduleReference{APIVersion: doc.Schedule.APIVersion, SHA256: doc.Schedule.SHA256}, EpisodeID: doc.EpisodeID, ManifestSHA256: doc.Manifest.SHA256, Mode: scheduledplan.Mode(doc.Mode)}
	for _, entry := range doc.Groups {
		group := scheduledplan.Group{Selection: scheduledplan.Selection{Binding: scheduledplan.Binding{GroupKey: entry.GroupKey, TestRepository: importStageRepository(entry.TestRepository),
			TestRevision: catalog.Revision{Algorithm: catalog.RevisionAlgorithm(entry.TestRevision.Algorithm), Digest: entry.TestRevision.Digest}, Family: catalog.TestFamilyFunctionalAPI, Adapter: entry.Adapter, Environment: entry.Environment}, RelevantNow: entry.RelevantNow}, RemainingLater: entry.RemainingLater}
		for _, test := range entry.Candidates {
			group.Candidates = append(group.Candidates, scheduledplan.Test{ID: test.ID, SuiteKey: test.SuiteKey, TestKey: test.TestKey, Decision: selection.Outcome(test.Decision)})
		}
		plan.Groups = append(plan.Groups, group)
	}
	if err := scheduledplan.Validate(plan, source, mode); err != nil {
		return scheduledplan.Plan{}, err
	}

	return scheduledplan.Canonical(plan), nil
}

func projectAPIStageGroup(binding contracts.FunctionalAPIStageBindingV2, manifest selection.Manifest, costByID map[string]contracts.PredictiveStageTestCost,
	episode contracts.PredictivePortfolioEpisode, assignments map[string]bool) (scheduledplan.Selection, error) {
	repository := importStageRepository(binding.TestRepository)
	group := scheduledplan.Selection{Binding: scheduledplan.Binding{GroupKey: binding.GroupKey, TestRepository: repository,
		TestRevision: catalog.Revision{Algorithm: catalog.RevisionAlgorithm(binding.TestRevision.Algorithm), Digest: binding.TestRevision.Digest},
		Family:       catalog.TestFamilyFunctionalAPI, Adapter: binding.Adapter, Environment: binding.Environment}, Candidates: []scheduledplan.Test{}, RelevantNow: []string{}}
	members := map[string]bool{}
	for _, decision := range manifest.Decisions {
		if decision.Test.Repository != repository || decision.Test.Adapter != binding.Adapter {
			continue
		}
		id, err := apiPortfolioTestID(repository.Identity, binding.Adapter, decision.Test.SuiteKey, decision.Test.TestKey)
		if err != nil {
			return scheduledplan.Selection{}, err
		}
		cost, exists := costByID[id]
		if !exists || assignments[id] || cost.Environment != binding.Environment {
			return scheduledplan.Selection{}, fmt.Errorf("%w: schedule candidate/binding", execution.ErrInvalid)
		}
		assignments[id], members[id] = true, true
		group.Candidates = append(group.Candidates, scheduledplan.Test{ID: id, SuiteKey: decision.Test.SuiteKey, TestKey: decision.Test.TestKey, Decision: decision.Outcome})
	}
	for _, chosen := range episode.Selected {
		if members[chosen.ID] {
			group.RelevantNow = append(group.RelevantNow, chosen.ID)
		}
	}

	return group, nil
}

func apiStageGraph(source scheduledplan.Schedule, costs contracts.PredictiveStageCostEpisode) (map[string]contracts.PredictiveStageTestCost, error) {
	owners, tests, capacities := map[string]string{}, map[string]contracts.PredictiveStageTestCost{}, map[string]int64{}
	for _, group := range source.Groups {
		for _, test := range group.Candidates {
			owners[test.ID] = group.GroupKey
		}
	}
	for _, env := range costs.Environments {
		capacities[env.Key] = env.CapacityUnits
	}
	for _, test := range costs.Tests {
		if test.ResourceUnits > capacities[test.Environment] {
			return nil, fmt.Errorf("%w: stage resource capacity", execution.ErrInvalid)
		}
		tests[test.ID] = test
		for _, dependency := range test.Dependencies {
			if owners[dependency] == "" || owners[dependency] != owners[test.ID] {
				return nil, fmt.Errorf("%w: unsupported cross-group or unknown dependency", execution.ErrInvalid)
			}
		}
	}

	return tests, nil
}
