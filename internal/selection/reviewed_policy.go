package selection

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"slices"
	"strings"

	"github.com/stanimirivanov/argus/internal/catalog"
)

const (
	// ReviewedPolicyAPIVersion identifies the trusted CI policy input.
	ReviewedPolicyAPIVersion = "argus.dev/reviewed-selection-policy/v1"
	// HeadInventoryAPIVersion identifies complete test discovery at immutable revisions.
	HeadInventoryAPIVersion = "argus.dev/head-test-inventory/v1"
	// ReviewedManifestAPIVersion identifies policy and evidence-aware API selection.
	ReviewedManifestAPIVersion = "argus.dev/execution-manifest/v4"
	// ReviewedFunctionalAPIPolicyVersion identifies the precedence rules below.
	ReviewedFunctionalAPIPolicyVersion = "argus.dev/selection-policy/functional-api/v2"
	// MaxReviewedRules bounds one reviewed policy document.
	MaxReviewedRules = 10_000
)

// ReviewedPolicy is repository-scoped owner policy from trusted CI input. Digest
// is the SHA-256 of the validated input bytes, not a caller-provided assertion.
type ReviewedPolicy struct {
	APIVersion          string
	SourceRepository    catalog.RepositoryIdentity
	Digest              string
	MustRunCapabilities []string
	MustRunTests        []catalog.TestIdentity
	Pins                []catalog.TestIdentity
	Exclusions          []catalog.TestIdentity
}

// InventoryTest is one observed test at an immutable test-repository revision.
// New and Modified are conservative inclusion signals, not mapping authority.
type InventoryTest struct {
	Test     TestReference
	Revision catalog.Revision
	New      bool
	Modified bool
}

// HeadInventory is the complete discovered test set supplied by trusted CI.
// Missing or incomplete inventory cannot authorize a selective manifest.
type HeadInventory struct {
	APIVersion       string
	SourceRepository catalog.RepositoryIdentity
	Digest           string
	Complete         bool
	Tests            []InventoryTest
}

// ReviewedInputs binds one selection to policy and head-discovery documents.
type ReviewedInputs struct {
	Policy    ReviewedPolicy
	Inventory HeadInventory
}

// DigestBytes returns an immutable lowercase SHA-256 reference for input bytes.
func DigestBytes(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

// ValidateReviewedInputs rejects unscoped policy, incomplete inventory, and
// ambiguous declarations before any test can be omitted.
func ValidateReviewedInputs(inputs ReviewedInputs, source catalog.RepositoryIdentity) error {
	policy, inventory := inputs.Policy, inputs.Inventory
	if policy.APIVersion != ReviewedPolicyAPIVersion || inventory.APIVersion != HeadInventoryAPIVersion ||
		!source.Valid() || policy.SourceRepository != source || inventory.SourceRepository != source ||
		!validDigest(policy.Digest) || !validDigest(inventory.Digest) || !inventory.Complete {
		return fmt.Errorf("%w: reviewed policy or inventory provenance", ErrInvalid)
	}
	if len(policy.MustRunCapabilities)+len(policy.MustRunTests)+len(policy.Pins)+len(policy.Exclusions) > MaxReviewedRules ||
		len(inventory.Tests) > MaxManifestDecisions {
		return fmt.Errorf("%w: reviewed policy bounds", ErrInvalid)
	}
	if !uniqueCapabilities(policy.MustRunCapabilities) || !uniqueIdentities(policy.MustRunTests) ||
		!uniqueIdentities(policy.Pins) || !uniqueIdentities(policy.Exclusions) {
		return fmt.Errorf("%w: duplicate or invalid policy rule", ErrInvalid)
	}
	seen := make(map[catalog.TestIdentity]bool, len(inventory.Tests))
	revisions := make(map[catalog.RepositoryIdentity]catalog.Revision)
	for _, item := range inventory.Tests {
		identity := item.Test.Identity()
		if !identity.Valid() || seen[identity] || !item.Revision.Valid() ||
			item.Test.Family != catalog.TestFamilyFunctionalAPI ||
			strings.TrimSpace(item.Test.Adapter) == "" || strings.TrimSpace(item.Test.Name) == "" ||
			!uniqueCapabilities(item.Test.Capabilities) {
			return fmt.Errorf("%w: invalid head inventory test", ErrInvalid)
		}
		if revision, exists := revisions[identity.TestRepository]; exists && revision != item.Revision {
			return fmt.Errorf("%w: inconsistent test repository revision", ErrInvalid)
		}
		revisions[identity.TestRepository] = item.Revision
		seen[identity] = true
	}
	for _, group := range [][]catalog.TestIdentity{policy.MustRunTests, policy.Pins, policy.Exclusions} {
		for _, identity := range group {
			if !seen[identity] {
				return fmt.Errorf("%w: policy rule names absent head test", ErrInvalid)
			}
		}
	}

	return nil
}

func validDigest(value string) bool {
	if len(value) != 64 {
		return false
	}
	for _, character := range value {
		if !strings.ContainsRune("0123456789abcdef", character) {
			return false
		}
	}

	return true
}

func uniqueCapabilities(values []string) bool {
	seen := make(map[string]bool, len(values))
	for _, value := range values {
		if !catalog.IsLocalKey(value) || seen[value] {
			return false
		}
		seen[value] = true
	}

	return true
}

func uniqueIdentities(values []catalog.TestIdentity) bool {
	seen := make(map[catalog.TestIdentity]bool, len(values))
	for _, value := range values {
		if !value.Valid() || seen[value] {
			return false
		}
		seen[value] = true
	}

	return true
}

// IncludesTest reports whether a stable test identity is present in a rule.
func IncludesTest(identities []catalog.TestIdentity, identity catalog.TestIdentity) bool {
	return slices.Contains(identities, identity)
}
