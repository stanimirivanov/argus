package contract

import (
	"github.com/stanimirivanov/argus/internal/catalog"
	"github.com/stanimirivanov/argus/internal/contracts"
	"github.com/stanimirivanov/argus/internal/selection"
)

// ImportReviewedInputs validates trusted CI documents and binds their exact
// bytes to the resulting selection. Scope and completeness remain domain rules.
func ImportReviewedInputs(policyData, inventoryData []byte) (selection.ReviewedInputs, error) {
	policy, err := contracts.DecodeReviewedSelectionPolicyV1(policyData)
	if err != nil {
		return selection.ReviewedInputs{}, err
	}
	inventory, err := contracts.DecodeHeadTestInventoryV1(inventoryData)
	if err != nil {
		return selection.ReviewedInputs{}, err
	}
	result := selection.ReviewedInputs{
		Policy: selection.ReviewedPolicy{
			APIVersion: policy.APIVersion, SourceRepository: importIdentity(policy.SourceRepository),
			Digest: selection.DigestBytes(policyData), MustRunCapabilities: append([]string(nil), policy.MustRunCapabilities...),
			MustRunTests: importIdentities(policy.MustRunTests), Pins: importIdentities(policy.Pins),
			Exclusions: importIdentities(policy.Exclusions),
		},
		Inventory: selection.HeadInventory{
			APIVersion: inventory.APIVersion, SourceRepository: importIdentity(inventory.SourceRepository),
			Digest: selection.DigestBytes(inventoryData), Complete: inventory.Complete,
		},
	}
	for _, item := range inventory.Tests {
		result.Inventory.Tests = append(result.Inventory.Tests, selection.InventoryTest{
			Test: selection.TestReference{
				Repository: importRepository(item.TestRepository), SuiteKey: item.SuiteKey,
				Family: catalog.TestFamilyFunctionalAPI, Adapter: item.Adapter, TestKey: item.TestKey,
				Name: item.Name, Capabilities: append([]string(nil), item.Capabilities...),
			},
			Revision: importRevision(item.Revision), New: item.New, Modified: item.Modified,
		})
	}

	return result, nil
}

func importIdentity(reference contracts.RepositoryIdentityReference) catalog.RepositoryIdentity {
	return catalog.RepositoryIdentity{
		Provider: catalog.Provider(reference.Provider), Host: reference.Host,
		ProviderRepositoryID: reference.ProviderRepositoryID,
	}
}

func importIdentities(documents []contracts.ImpactTestIdentity) []catalog.TestIdentity {
	result := make([]catalog.TestIdentity, 0, len(documents))
	for _, document := range documents {
		result = append(result, catalog.TestIdentity{
			TestRepository: importIdentity(document.TestRepository), SuiteKey: document.SuiteKey, TestKey: document.TestKey,
		})
	}

	return result
}

// ExportV4 publishes the reviewed API selection without changing v1-v3 JSON.
func ExportV4(manifest selection.Manifest) (contracts.ExecutionManifestV4, error) {
	if manifest.APIVersion != selection.ReviewedManifestAPIVersion {
		return contracts.ExecutionManifestV4{}, selection.ErrInvalid
	}
	base, err := exportManifest(manifest)
	if err != nil {
		return contracts.ExecutionManifestV4{}, err
	}
	result := contracts.ExecutionManifestV4{
		APIVersion: base.APIVersion, PolicyVersion: base.PolicyVersion,
		PolicyDigest: manifest.PolicyDigest, InventoryDigest: manifest.InventoryDigest,
		Impact: base.Impact, Change: base.Change, Catalog: base.Catalog,
		Family: base.Family, Mode: base.Mode, AffectedCapabilities: base.AffectedCapabilities,
		UncoveredCapabilities: base.UncoveredCapabilities, Warnings: base.Warnings,
		Decisions: make([]contracts.ReviewedTestDecision, 0, len(base.Decisions)),
	}
	canonical := selection.CanonicalManifest(manifest)
	for index, decision := range base.Decisions {
		reviewed := contracts.ReviewedTestDecision{
			TestRepository: decision.TestRepository, Suite: decision.Suite, Test: decision.Test,
			Capabilities: decision.Capabilities, Outcome: decision.Outcome,
			RemainingExecution: decision.RemainingExecution,
		}
		for _, reason := range canonical.Decisions[index].Reasons {
			reviewed.Reasons = append(reviewed.Reasons, contracts.ReviewedSelectionReason{
				Code: string(reason.Code), Capabilities: append([]string{}, reason.Capabilities...),
				EvidenceKeys: append([]string{}, reason.EvidenceKeys...),
			})
		}
		result.Decisions = append(result.Decisions, reviewed)
	}
	if err := contracts.ValidateExecutionManifestV4(result); err != nil {
		return contracts.ExecutionManifestV4{}, err
	}

	return result, nil
}
