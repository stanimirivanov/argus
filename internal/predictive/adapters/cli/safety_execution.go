package cli

import (
	"encoding/json"

	"github.com/stanimirivanov/argus/internal/catalog"
	discoverycontract "github.com/stanimirivanov/argus/internal/catalog/adapters/contract/browser"
	"github.com/stanimirivanov/argus/internal/contracts"
	executioncontract "github.com/stanimirivanov/argus/internal/execution/adapters/contract"
	"github.com/stanimirivanov/argus/internal/execution/browser"
	"github.com/stanimirivanov/argus/internal/execution/scheduledplan"
	"github.com/stanimirivanov/argus/internal/predictive"
	selectioncontract "github.com/stanimirivanov/argus/internal/selection/adapters/contract"
)

func (a *safetyArchive) safetyPlan(run contracts.PredictiveSafetyRun, schedule, manifest, bindings, planBytes []byte) (scheduledplan.Plan, string, error) {
	var header struct {
		APIVersion string `json:"apiVersion"`
	}
	if err := json.Unmarshal(manifest, &header); err != nil {
		return scheduledplan.Plan{}, "", err
	}
	if header.APIVersion == browser.ManifestVersion {
		return a.safetyBrowserPlan(run, schedule, manifest, bindings, planBytes)
	}
	if run.Discovery != nil {
		return scheduledplan.Plan{}, "", predictive.ErrInvalid
	}
	doc, err := selectioncontract.ImportExecutableManifest(manifest)
	if err != nil {
		return scheduledplan.Plan{}, "", err
	}
	manifestSHA, err := selectioncontract.ExecutableManifestSHA256(doc)
	if err != nil {
		return scheduledplan.Plan{}, "", err
	}
	var reviewed contracts.FunctionalAPIStageBindingsV2
	if err := contracts.DecodeStagedDocument(bindings, "functional-api-execution-bindings", &reviewed); err != nil {
		return scheduledplan.Plan{}, "", err
	}
	source, _, err := executioncontract.APIStageSource(schedule, run.Schedule.SHA256, run.EpisodeID, doc, manifestSHA, reviewed)
	if err != nil {
		return scheduledplan.Plan{}, "", err
	}
	var plan contracts.FunctionalAPIStagePlanV2
	if err := contracts.DecodeStagedDocument(planBytes, "functional-api-execution-plan", &plan); err != nil {
		return scheduledplan.Plan{}, "", err
	}
	value, err := executioncontract.ImportAPIStagePlan(plan, source, scheduledplan.Mode(run.Mode), doc.APIVersion)

	return value, doc.PolicyVersion, err
}

func (a *safetyArchive) safetyBrowserPlan(run contracts.PredictiveSafetyRun, schedule, manifest, bindings, planBytes []byte) (scheduledplan.Plan, string, error) {
	if run.Discovery == nil {
		return scheduledplan.Plan{}, "", predictive.ErrInvalid
	}
	doc, err := contracts.DecodeBrowserSelectionManifestV1(manifest)
	if err != nil {
		return scheduledplan.Plan{}, "", err
	}
	reviewed, err := contracts.DecodeBrowserExecutionBindingsV1(bindings)
	if err != nil {
		return scheduledplan.Plan{}, "", err
	}
	data, err := a.read(*run.Discovery)
	if err != nil {
		return scheduledplan.Plan{}, "", err
	}
	discovery, err := contracts.DecodeBrowserDiscoveryV1(data)
	if err != nil {
		return scheduledplan.Plan{}, "", err
	}
	digest, err := discoverycontract.Digest(discovery)
	if err != nil {
		return scheduledplan.Plan{}, "", err
	}
	source, _, err := executioncontract.BrowserStageSource(schedule, run.Schedule.SHA256, run.EpisodeID, executioncontract.BrowserStageDocuments{Manifest: doc, ManifestSHA256: run.Manifest.SHA256, Bindings: reviewed, Discovery: discovery, DiscoverySHA256: run.Discovery.SHA256, DiscoveryEvidenceSHA256: digest})
	if err != nil {
		return scheduledplan.Plan{}, "", err
	}
	var plan contracts.BrowserStagePlanV2
	if err := contracts.DecodeStagedDocument(planBytes, "browser-execution-plan", &plan); err != nil {
		return scheduledplan.Plan{}, "", err
	}
	value, err := executioncontract.ImportBrowserStagePlan(plan, source, run.Discovery.SHA256, scheduledplan.Mode(run.Mode))

	return value, doc.PolicyVersion, err
}

func decodeSafetyAttempt(data []byte) (predictive.SafetyAttempt, error) {
	var header struct {
		APIVersion string `json:"apiVersion"`
	}
	if err := json.Unmarshal(data, &header); err != nil {
		return predictive.SafetyAttempt{}, err
	}
	if header.APIVersion == browser.StagedAttemptVersion {
		a, err := executioncontract.DecodeRecordedBrowserAttempt(data)
		if err != nil {
			return predictive.SafetyAttempt{}, err
		}
		r := a.Request
		inventory := []predictive.SafetyNamedTest{}
		for _, c := range r.Candidates {
			inventory = append(inventory, predictive.SafetyNamedTest{SuiteKey: c.SuiteKey, TestKey: c.TestKey, Name: c.Name})
		}

		return predictive.SafetyAttempt{Inventory: inventory, DiscoverySHA256: r.Staging.DiscoverySHA256, ID: r.AttemptID, AdapterVersion: a.Result.AdapterVersion, Scope: predictive.SafetyScope{Repository: r.TestRepository, Revision: r.TestRevision, Family: catalog.TestFamilyFunctionalUI, Adapter: r.Adapter, Project: r.Project, Environment: r.Environment}, Manifest: r.Manifest, Staging: r.Staging.Staging, Stage: r.Stage, StartedAt: a.Result.StartedAt, CompletedAt: a.Result.CompletedAt, Results: a.Result.Results}, nil
	}
	a, err := executioncontract.DecodeRecordedAttempt(data)
	if err != nil {
		return predictive.SafetyAttempt{}, err
	}
	if a.Staging == nil {
		return predictive.SafetyAttempt{}, predictive.ErrInvalid
	}

	return predictive.SafetyAttempt{ID: a.AttemptID, AdapterVersion: a.AdapterVersion, Scope: predictive.SafetyScope{Repository: a.TestRepository, Revision: a.TestRevision, Family: catalog.TestFamilyFunctionalAPI, Adapter: a.AdapterID, Environment: a.Staging.Environment}, Manifest: a.Manifest, Staging: *a.Staging, Stage: a.Stage, StartedAt: a.StartedAt, CompletedAt: a.CompletedAt, Results: a.Results}, nil
}

func bindSafetyNames(inputs []predictive.SafetyInput, manifest []byte) error {
	if len(inputs) == 0 || inputs[0].Scope.Family != catalog.TestFamilyFunctionalUI {
		return nil
	}
	// The complete manifest was schema/domain checked before this projection.
	var doc contracts.BrowserSelectionManifestV1
	if err := json.Unmarshal(manifest, &doc); err != nil {
		return err
	}
	names := map[string]string{}
	for _, d := range doc.Decisions {
		names[safetyNameKey(repository(d.TestRepository).Identity, d.SuiteKey, d.TestKey)] = d.Name
	}
	for i := range inputs {
		for j := range inputs[i].Candidates {
			c := &inputs[i].Candidates[j]
			c.Name = names[safetyNameKey(inputs[i].Scope.Repository.Identity, c.SuiteKey, c.TestKey)]
			if c.Name == "" {
				return predictive.ErrInvalid
			}
		}
	}

	return nil
}
func safetyNameKey(repo catalog.RepositoryIdentity, suite, test string) string {
	return string(repo.Provider) + "\x00" + repo.Host + "\x00" + repo.ProviderRepositoryID + "\x00" + suite + "\x00" + test
}
