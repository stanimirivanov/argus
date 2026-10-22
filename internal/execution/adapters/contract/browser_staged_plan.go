package contract

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"slices"

	"github.com/stanimirivanov/argus/internal/catalog"
	"github.com/stanimirivanov/argus/internal/contracts"
	"github.com/stanimirivanov/argus/internal/execution"
	"github.com/stanimirivanov/argus/internal/execution/browser"
	"github.com/stanimirivanov/argus/internal/execution/browserplanning"
	"github.com/stanimirivanov/argus/internal/execution/scheduledplan"
	"github.com/stanimirivanov/argus/internal/selection"
)

// BrowserStageDocuments are independently decoded inputs. CLI boundaries compute
// both raw hashes and the catalog converter's canonical evidence hash themselves.
type BrowserStageDocuments struct {
	Manifest                                                 contracts.BrowserSelectionManifestV1
	Discovery                                                contracts.BrowserDiscoveryV1
	ManifestSHA256, DiscoverySHA256, DiscoveryEvidenceSHA256 string
	Bindings                                                 contracts.BrowserExecutionBindingsV1
}

// BrowserStageSource verifies portfolio bytes and exact complete discovered
// project variants; it rejects missing/extra groups, revisions and environments.
func BrowserStageSource(data []byte, expectedSHA, episodeID string, docs BrowserStageDocuments) (scheduledplan.Schedule, map[string]int, error) {
	sum := sha256.Sum256(data)
	if len(data) > 16<<20 || hex.EncodeToString(sum[:]) != expectedSHA {
		return scheduledplan.Schedule{}, nil, browser.ErrInvalid
	}
	manifest, err := ImportBrowserManifest(docs.Manifest)
	if err != nil {
		return scheduledplan.Schedule{}, nil, err
	}
	bindings, err := ImportBrowserExecutionBindings(docs.Bindings)
	if err != nil {
		return scheduledplan.Schedule{}, nil, err
	}
	if _, err := browserplanning.Build(manifest, docs.ManifestSHA256, bindings); err != nil {
		return scheduledplan.Schedule{}, nil, err
	}
	if err := checkStageDiscovery(docs); err != nil {
		return scheduledplan.Schedule{}, nil, err
	}
	report, err := contracts.DecodePredictivePortfolioV1(data)
	if err != nil {
		return scheduledplan.Schedule{}, nil, err
	}
	episode, costs := browserStageEpisode(report, episodeID)
	if episode.ID == "" || costs.ManifestSHA256 != docs.ManifestSHA256 {
		return scheduledplan.Schedule{}, nil, browser.ErrInvalid
	}
	source := scheduledplan.Schedule{Reference: execution.ScheduleReference{APIVersion: report.APIVersion, SHA256: expectedSHA}, EpisodeID: episodeID, ManifestSHA256: docs.ManifestSHA256}
	costByID := map[string]contracts.PredictiveStageTestCost{}
	for _, cost := range costs.Tests {
		costByID[cost.ID] = cost
	}
	assignments := map[string]bool{}
	for _, binding := range bindings.Groups {
		group, err := browserStageGroup(binding, manifest, episode, costByID, assignments)
		if err != nil {
			return scheduledplan.Schedule{}, nil, err
		}
		source.Groups = append(source.Groups, group)
	}
	if len(assignments) != len(costs.Tests) {
		return scheduledplan.Schedule{}, nil, browser.ErrInvalid
	}
	if _, err := scheduledplan.Build(source, scheduledplan.Shadow); err != nil {
		return scheduledplan.Schedule{}, nil, err
	}
	order, err := apiStageOrder(source, costs)

	return source, order, err
}

func checkStageDiscovery(docs BrowserStageDocuments) error {
	d := docs.Discovery
	if !d.Complete || d.Catalog != docs.Manifest.Catalog || len(docs.DiscoverySHA256) != 64 {
		return browser.ErrInvalid
	}
	if ref := docs.Manifest.Evidence; ref != nil && (ref.Digest != docs.DiscoveryEvidenceSHA256 || ref.ObservationID != d.ObservationID || ref.MappingRevision != d.MappingRevision || ref.CollectorVersion != d.CollectorVersion) {
		return browser.ErrInvalid
	}
	if docs.Manifest.Mode == "targeted" && !d.MappingComplete {
		return browser.ErrInvalid
	}
	observed, err := indexStageDiscovery(d)
	if err != nil {
		return err
	}
	expected := map[string]bool{}
	for _, group := range docs.Bindings.Groups {
		if err := coverDiscoveredGroup(group, docs.Manifest.Decisions, observed, expected); err != nil {
			return err
		}
	}
	if len(observed) != len(expected) {
		return browser.ErrInvalid
	}

	return nil
}

func indexStageDiscovery(d contracts.BrowserDiscoveryV1) (map[string]string, error) {
	observed, suites := map[string]string{}, map[string]bool{}
	for _, suite := range d.Suites {
		scope := browserVariantKey(suite.TestRepository, suite.Revision, suite.SuiteKey, "", "")
		if suites[scope] {
			return nil, browser.ErrInvalid
		}
		suites[scope] = true
		for _, test := range suite.Cases {
			key := browserVariantKey(suite.TestRepository, suite.Revision, suite.SuiteKey, test.TestKey, test.Project)
			if _, exists := observed[key]; exists || len(observed) >= execution.MaxAttemptTests {
				return nil, browser.ErrInvalid
			}
			observed[key] = test.Title
		}
	}

	return observed, nil
}

func coverDiscoveredGroup(group contracts.BrowserExecutionGroupBinding, decisions []contracts.BrowserSelectionDecision, observed map[string]string, expected map[string]bool) error {
	for _, d := range decisions {
		if d.TestRepository != group.TestRepository || d.Revision != group.TestRevision || (len(d.Projects) > 0 && !slices.Contains(d.Projects, group.Project)) {
			continue
		}
		key := browserVariantKey(group.TestRepository, group.TestRevision, d.SuiteKey, d.TestKey, group.Project)
		if title, exists := observed[key]; !exists || title != d.Name {
			return browser.ErrInvalid
		}
		expected[key] = true
	}

	return nil
}

func browserVariantKey(repository contracts.RepositoryReference, revision contracts.RevisionReference, suite, test, project string) string {
	return repository.Provider + "\x00" + repository.Host + "\x00" + repository.ProviderRepositoryID + "\x00" + repository.Owner + "\x00" + repository.Name + "\x00" + revision.Algorithm + "\x00" + revision.Digest + "\x00" + suite + "\x00" + test + "\x00" + project
}

func browserStageGroup(b browserplanning.Binding, manifest browser.Manifest, episode contracts.PredictivePortfolioEpisode, costs map[string]contracts.PredictiveStageTestCost, assigned map[string]bool) (scheduledplan.Selection, error) {
	group := scheduledplan.Selection{Binding: scheduledplan.Binding{GroupKey: b.GroupKey, TestRepository: b.TestRepository, TestRevision: b.TestRevision, Family: catalog.TestFamilyFunctionalUI, Adapter: b.Adapter, Project: b.Project, Environment: b.Environment}, Candidates: []scheduledplan.Test{}, RelevantNow: []string{}}
	members := map[string]bool{}
	for _, d := range manifest.Decisions {
		if d.Repository != b.TestRepository || d.Revision != b.TestRevision || (len(d.Projects) > 0 && !slices.Contains(d.Projects, b.Project)) {
			continue
		}
		id, err := browserPortfolioID(b, d.SuiteKey, d.TestKey)
		if err != nil {
			return scheduledplan.Selection{}, err
		}
		cost, exists := costs[id]
		if !exists || assigned[id] || cost.Environment != b.Environment {
			return scheduledplan.Selection{}, browser.ErrInvalid
		}
		assigned[id], members[id] = true, true
		group.Candidates = append(group.Candidates, scheduledplan.Test{ID: id, SuiteKey: d.SuiteKey, TestKey: d.TestKey, Decision: d.Outcome})
	}
	for _, test := range episode.Selected {
		if members[test.ID] {
			group.RelevantNow = append(group.RelevantNow, test.ID)
		}
	}

	return group, nil
}

func browserPortfolioID(b browserplanning.Binding, suite, test string) (string, error) {
	id := b.TestRepository.Identity
	data, err := json.Marshal(struct {
		Provider, Host, ProviderRepositoryID, Family, Adapter string
		Project, Environment                                  *string
		SuiteKey, TestKey                                     string
	}{string(id.Provider), id.Host, id.ProviderRepositoryID, string(catalog.TestFamilyFunctionalUI), b.Adapter, &b.Project, &b.Environment, suite, test})
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(data)

	return hex.EncodeToString(sum[:]), nil
}

// ExportBrowserStagePlan emits the v2 project-specific partition contract.
func ExportBrowserStagePlan(plan scheduledplan.Plan, discoverySHA string) (contracts.BrowserStagePlanV2, error) {
	doc := contracts.BrowserStagePlanV2{APIVersion: "argus.dev/browser-execution-plan/v2", Schedule: contracts.ManifestDigestReference{APIVersion: plan.Schedule.APIVersion, SHA256: plan.Schedule.SHA256}, EpisodeID: plan.EpisodeID, DiscoverySHA256: discoverySHA,
		Manifest: contracts.ManifestDigestReference{APIVersion: browser.ManifestVersion, SHA256: plan.ManifestSHA256}, Mode: string(plan.Mode), Groups: []contracts.BrowserStageGroupV2{}}
	for _, g := range plan.Groups {
		entry := contracts.BrowserStageGroupV2{BrowserExecutionGroupBinding: contracts.BrowserExecutionGroupBinding{GroupKey: g.GroupKey, TestRepository: repositoryReference(g.TestRepository), TestRevision: revisionReference(g.TestRevision), Adapter: g.Adapter, Project: g.Project, Environment: g.Environment}, Candidates: []contracts.StagedCandidateV2{}, RelevantNow: g.RelevantNow, RemainingLater: g.RemainingLater}
		for _, t := range g.Candidates {
			entry.Candidates = append(entry.Candidates, contracts.StagedCandidateV2{ID: t.ID, SuiteKey: t.SuiteKey, TestKey: t.TestKey, Decision: string(t.Decision)})
		}
		doc.Groups = append(doc.Groups, entry)
	}

	return doc, contracts.ValidateStagedDocument(doc, "browser-execution-plan")
}

// ImportBrowserStagePlan requires independent discovery, source and expected mode.
func ImportBrowserStagePlan(doc contracts.BrowserStagePlanV2, source scheduledplan.Schedule, discoverySHA string, mode scheduledplan.Mode) (scheduledplan.Plan, error) {
	if err := contracts.ValidateStagedDocument(doc, "browser-execution-plan"); err != nil {
		return scheduledplan.Plan{}, err
	}
	if doc.DiscoverySHA256 != discoverySHA {
		return scheduledplan.Plan{}, browser.ErrInvalid
	}
	plan := scheduledplan.Plan{Schedule: execution.ScheduleReference{APIVersion: doc.Schedule.APIVersion, SHA256: doc.Schedule.SHA256}, EpisodeID: doc.EpisodeID, ManifestSHA256: doc.Manifest.SHA256, Mode: scheduledplan.Mode(doc.Mode)}
	for _, g := range doc.Groups {
		group := scheduledplan.Group{Selection: scheduledplan.Selection{Binding: scheduledplan.Binding{GroupKey: g.GroupKey, TestRepository: importExecutionRepository(g.TestRepository), TestRevision: importExecutionRevision(g.TestRevision), Family: catalog.TestFamilyFunctionalUI, Adapter: g.Adapter, Project: g.Project, Environment: g.Environment}, RelevantNow: g.RelevantNow}, RemainingLater: g.RemainingLater}
		for _, t := range g.Candidates {
			group.Candidates = append(group.Candidates, scheduledplan.Test{ID: t.ID, SuiteKey: t.SuiteKey, TestKey: t.TestKey, Decision: selection.Outcome(t.Decision)})
		}
		plan.Groups = append(plan.Groups, group)
	}
	if err := scheduledplan.Validate(plan, source, mode); err != nil {
		return scheduledplan.Plan{}, err
	}

	return scheduledplan.Canonical(plan), nil
}

func browserStageEpisode(report contracts.PredictivePortfolioV1, id string) (contracts.PredictivePortfolioEpisode, contracts.PredictiveStageCostEpisode) {
	var episode contracts.PredictivePortfolioEpisode
	var costs contracts.PredictiveStageCostEpisode
	for _, candidate := range report.Episodes {
		if candidate.ID == id {
			episode = candidate
		}
	}
	for _, candidate := range report.Costs.Episodes {
		if candidate.ID == id {
			costs = candidate
		}
	}

	return episode, costs
}
