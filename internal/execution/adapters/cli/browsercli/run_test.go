package browsercli

import (
	"testing"

	"github.com/stanimirivanov/argus/internal/contracts"
)

func TestManifestDecisionSemantics(t *testing.T) {
	decision := contracts.BrowserSelectionDecision{TestRepository: contracts.RepositoryReference{Provider: "github", Host: "github.com", ProviderRepositoryID: "tests-1"}, SuiteKey: "shop", TestKey: "checkout", Outcome: "RUN_REQUIRED", RemainingExecution: "none"}
	document := contracts.BrowserSelectionManifestV1{Mode: "fallback", Decisions: []contracts.BrowserSelectionDecision{decision}}
	if err := validateManifestDecisions(document); err != nil {
		t.Fatal(err)
	}
	cases := []struct {
		name   string
		change func(*contracts.BrowserSelectionManifestV1)
	}{
		{"fallback skip", func(d *contracts.BrowserSelectionManifestV1) {
			d.Decisions[0].Outcome = "SKIP_FOR_NOW"
			d.Decisions[0].RemainingExecution = "full-suite"
		}},
		{"duplicate", func(d *contracts.BrowserSelectionManifestV1) { d.Decisions = append(d.Decisions, d.Decisions[0]) }},
		{"wrong obligation", func(d *contracts.BrowserSelectionManifestV1) { d.Decisions[0].RemainingExecution = "full-suite" }},
		{"targeted missing evidence", func(d *contracts.BrowserSelectionManifestV1) { d.Mode = "targeted" }},
	}
	for _, item := range cases {
		t.Run(item.name, func(t *testing.T) {
			copy := document
			copy.Decisions = append([]contracts.BrowserSelectionDecision{}, document.Decisions...)
			item.change(&copy)
			if err := validateManifestDecisions(copy); err == nil {
				t.Fatal("expected invalid manifest")
			}
		})
	}
}
