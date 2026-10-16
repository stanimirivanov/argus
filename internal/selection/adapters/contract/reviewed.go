package contract

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"

	"github.com/stanimirivanov/argus/internal/catalog"
	"github.com/stanimirivanov/argus/internal/contracts"
	"github.com/stanimirivanov/argus/internal/selection"
)

// ImportV4 validates reviewed execution input without erasing policy digests
// or mapping evidence needed by downstream execution correlation.
func ImportV4(document contracts.ExecutionManifestV4) (selection.Manifest, error) {
	if err := contracts.ValidateExecutionManifestV4(document); err != nil {
		return selection.Manifest{}, err
	}
	base := contracts.ExecutionManifestV1{
		APIVersion: document.APIVersion, PolicyVersion: document.PolicyVersion,
		Impact: document.Impact, Change: document.Change, Catalog: document.Catalog,
		Family: document.Family, Mode: document.Mode,
		AffectedCapabilities:  document.AffectedCapabilities,
		UncoveredCapabilities: document.UncoveredCapabilities, Warnings: document.Warnings,
	}
	for _, decision := range document.Decisions {
		converted := contracts.TestDecision{
			TestRepository: decision.TestRepository, Suite: decision.Suite, Test: decision.Test,
			Capabilities: decision.Capabilities, Outcome: decision.Outcome,
			RemainingExecution: decision.RemainingExecution,
		}
		for _, reason := range decision.Reasons {
			converted.Reasons = append(converted.Reasons, contracts.SelectionReason{
				Code: reason.Code, Capabilities: reason.Capabilities,
			})
		}
		base.Decisions = append(base.Decisions, converted)
	}

	return importValidatedManifest(base, &document)
}

// ManifestSHA256V4 binds execution to canonical reviewed manifest bytes.
func ManifestSHA256V4(manifest selection.Manifest) (string, error) {
	document, err := ExportV4(manifest)
	if err != nil {
		return "", fmt.Errorf("canonicalize reviewed execution manifest: %w", err)
	}
	data, err := json.Marshal(document)
	if err != nil {
		return "", fmt.Errorf("encode reviewed execution manifest: %w", err)
	}
	digest := sha256.Sum256(data)

	return hex.EncodeToString(digest[:]), nil
}

// ImportExecutableManifest accepts the two functional API manifest versions
// supported by the planner and runner, rejecting every browser variant.
func ImportExecutableManifest(data []byte) (selection.Manifest, error) {
	var envelope struct {
		APIVersion string `json:"apiVersion"`
	}
	if err := json.Unmarshal(data, &envelope); err != nil {
		return selection.Manifest{}, err
	}
	switch envelope.APIVersion {
	case selection.ManifestAPIVersion:
		document, err := contracts.DecodeExecutionManifestV1(data)
		if err != nil {
			return selection.Manifest{}, err
		}

		return ImportV1(document)
	case selection.ReviewedManifestAPIVersion:
		document, err := contracts.DecodeExecutionManifestV4(data)
		if err != nil {
			return selection.Manifest{}, err
		}

		return ImportV4(document)
	default:
		return selection.Manifest{}, selection.ErrInvalid
	}
}

// ExecutableManifestSHA256 binds a supported manifest to its canonical wire bytes.
func ExecutableManifestSHA256(manifest selection.Manifest) (string, error) {
	switch manifest.APIVersion {
	case selection.ManifestAPIVersion:
		return ManifestSHA256V1(manifest)
	case selection.ReviewedManifestAPIVersion:
		return ManifestSHA256V4(manifest)
	default:
		return "", selection.ErrInvalid
	}
}

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
