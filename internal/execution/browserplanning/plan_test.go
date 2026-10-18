package browserplanning_test

import (
	"strings"
	"testing"

	"github.com/stanimirivanov/argus/internal/catalog"
	"github.com/stanimirivanov/argus/internal/execution"
	executioncontract "github.com/stanimirivanov/argus/internal/execution/adapters/contract"
	"github.com/stanimirivanov/argus/internal/execution/browser"
	"github.com/stanimirivanov/argus/internal/execution/browserplanning"
	"github.com/stanimirivanov/argus/internal/selection"
)

func fixture() (browser.Manifest, browserplanning.Bindings) {
	repo := catalog.Repository{Identity: catalog.RepositoryIdentity{Provider: catalog.ProviderGitHub, Host: "github.com", ProviderRepositoryID: "tests-1"}, Owner: "example", Name: "browser-tests"}
	revision := catalog.Revision{Algorithm: catalog.RevisionGitSHA1, Digest: strings.Repeat("a", 40)}
	manifest := browser.Manifest{APIVersion: browser.ManifestVersion, Decisions: []browser.Decision{
		{Repository: repo, SuiteKey: "shop", TestKey: "checkout", Name: "checkout", Revision: revision, Projects: []string{"desktop"}, Outcome: selection.OutcomeRunRequired},
		{Repository: repo, SuiteKey: "shop", TestKey: "receipt", Name: "receipt", Revision: revision, Projects: []string{"desktop", "mobile"}, Outcome: selection.OutcomeSkipForNow},
	}}
	bindings := browserplanning.Bindings{APIVersion: browserplanning.BindingsVersion, Groups: []browserplanning.Binding{
		{GroupKey: "desktop-ci", TestRepository: repo, TestRevision: revision, Adapter: "playwright", Project: "desktop", Environment: "ci"},
		{GroupKey: "mobile-ci", TestRepository: repo, TestRevision: revision, Adapter: "playwright", Project: "mobile", Environment: "ci"},
	}}

	return manifest, bindings
}

func TestBuildBrowserPlanKeepsFullOnlyProject(t *testing.T) {
	manifest, bindings := fixture()
	plan, err := browserplanning.Build(manifest, strings.Repeat("b", 64), bindings)
	if err != nil {
		t.Fatal(err)
	}
	if len(plan.Jobs) != 3 || plan.Jobs[0].GroupKey != "desktop-ci" || plan.Jobs[0].Stage != execution.StageSelected || plan.Jobs[0].TestCount != 1 || plan.Jobs[1].Stage != execution.StageFullSuite || plan.Jobs[1].TestCount != 2 || plan.Jobs[2].GroupKey != "mobile-ci" || plan.Jobs[2].Stage != execution.StageFullSuite || plan.Jobs[2].TestCount != 1 {
		t.Fatalf("unexpected browser jobs: %+v", plan.Jobs)
	}
	if _, err := executioncontract.ExportBrowserExecutionPlan(plan); err != nil {
		t.Fatalf("browser plan contract: %v", err)
	}
}
func TestBuildRejectsMissingProjectAndDuplicateGroups(t *testing.T) {
	manifest, bindings := fixture()
	bindings.Groups = bindings.Groups[:1]
	if _, err := browserplanning.Build(manifest, strings.Repeat("b", 64), bindings); err == nil {
		t.Fatal("missing mobile project binding accepted")
	}
	_, bindings = fixture()
	bindings.Groups = append(bindings.Groups, bindings.Groups[0])
	if _, err := browserplanning.Build(manifest, strings.Repeat("b", 64), bindings); err == nil {
		t.Fatal("duplicate group accepted")
	}
}
