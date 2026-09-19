// Package selection owns provider- and framework-neutral execution decisions.
package selection

import (
	"cmp"
	"errors"
	"fmt"
	"slices"
	"strings"

	"github.com/stanimirivanov/argus/internal/catalog"
	"github.com/stanimirivanov/argus/internal/change"
)

const (
	// ManifestAPIVersion identifies the first execution-manifest contract.
	ManifestAPIVersion = "argus.dev/execution-manifest/v1"
	// FunctionalAPIPolicyVersion identifies the deterministic initial policy.
	FunctionalAPIPolicyVersion = "argus.dev/selection-policy/functional-api/v1"
	// MaxManifestDecisions bounds one selection result.
	MaxManifestDecisions = 10_000
	// MaxManifestCapabilities bounds affected and per-test mapping evidence.
	MaxManifestCapabilities = 5_000
)

var (
	// ErrInvalid means a selection request or result violates domain invariants.
	ErrInvalid = errors.New("invalid test selection")
	// ErrUnavailable means selection could not produce a trustworthy result.
	ErrUnavailable = errors.New("test selection dependency unavailable")
)

// Mode describes whether direct impact or a conservative fallback selected tests.
type Mode string

const (
	// ModeTargeted selects tests mapped to affected capabilities.
	ModeTargeted Mode = "targeted"
	// ModeFallback requires every candidate because impact evidence is incomplete.
	ModeFallback Mode = "fallback"
)

// Outcome is the policy decision for one candidate test.
type Outcome string

const (
	// OutcomeRunRequired includes the test in the immediate execution stage.
	OutcomeRunRequired Outcome = "RUN_REQUIRED"
	// OutcomeSkipForNow omits the test from this stage but not the full-suite control.
	OutcomeSkipForNow Outcome = "SKIP_FOR_NOW"
)

// ReasonCode is a stable machine-readable selection explanation.
type ReasonCode string

const (
	// ReasonDirectCapabilityImpact identifies an explicit capability match.
	ReasonDirectCapabilityImpact ReasonCode = "DIRECT_CAPABILITY_IMPACT"
	// ReasonConservativeFallback identifies incomplete or unmapped impact.
	ReasonConservativeFallback ReasonCode = "CONSERVATIVE_FALLBACK"
	// ReasonNotAffected identifies an omitted candidate without a capability match.
	ReasonNotAffected ReasonCode = "NOT_AFFECTED_IN_EARLY_STAGE"
)

// RemainingExecution states whether an omitted test has a later control path.
type RemainingExecution string

const (
	// RemainingNone means the test is already required in this manifest.
	RemainingNone RemainingExecution = "none"
	// RemainingFullSuite requires the omitted test in the later full-suite control.
	RemainingFullSuite RemainingExecution = "full-suite"
)

// TestReference identifies one cataloged test and immutable display metadata.
type TestReference struct {
	Repository   catalog.Repository
	SuiteKey     string
	Family       catalog.TestFamily
	Adapter      string
	TestKey      string
	Name         string
	Capabilities []string
}

// Identity returns the stable identity of the referenced test.
func (reference TestReference) Identity() catalog.TestIdentity {
	return catalog.TestIdentity{
		TestRepository: reference.Repository.Identity,
		SuiteKey:       reference.SuiteKey,
		TestKey:        reference.TestKey,
	}
}

// Reason explains one decision and the capabilities supporting it.
type Reason struct {
	Code         ReasonCode
	Capabilities []string
}

// Decision records inclusion or omission for one functional API test.
type Decision struct {
	Test               TestReference
	Outcome            Outcome
	RemainingExecution RemainingExecution
	Reasons            []Reason
}

// Manifest is a deterministic selection result from immutable inputs.
type Manifest struct {
	APIVersion            string
	PolicyVersion         string
	ImpactAPIVersion      string
	ImpactAnalyzerVersion string
	Change                change.Reference
	Catalog               catalog.SnapshotReference
	Family                catalog.TestFamily
	Mode                  Mode
	AffectedCapabilities  []string
	UncoveredCapabilities []string
	Decisions             []Decision
	Warnings              []string
}

// CanonicalManifest deep-copies and orders all set-like fields.
func CanonicalManifest(manifest Manifest) Manifest {
	canonical := manifest
	canonical.Change.ObservedAt = manifest.Change.ObservedAt.UTC()
	canonical.AffectedCapabilities = canonicalStrings(manifest.AffectedCapabilities)
	canonical.UncoveredCapabilities = canonicalStrings(manifest.UncoveredCapabilities)
	canonical.Warnings = canonicalStrings(manifest.Warnings)
	canonical.Decisions = make([]Decision, len(manifest.Decisions))
	for index, decision := range manifest.Decisions {
		canonical.Decisions[index] = decision
		canonical.Decisions[index].Test.Capabilities = canonicalStrings(decision.Test.Capabilities)
		canonical.Decisions[index].Reasons = make([]Reason, len(decision.Reasons))
		for reasonIndex, reason := range decision.Reasons {
			canonical.Decisions[index].Reasons[reasonIndex] = reason
			canonical.Decisions[index].Reasons[reasonIndex].Capabilities = canonicalStrings(reason.Capabilities)
		}
		slices.SortFunc(canonical.Decisions[index].Reasons, func(left, right Reason) int {
			return cmp.Compare(left.Code, right.Code)
		})
	}
	slices.SortFunc(canonical.Decisions, func(left, right Decision) int {
		return catalog.CompareTestIdentities(left.Test.Identity(), right.Test.Identity())
	})

	return canonical
}

// ValidateManifest enforces policy-result invariants independent of transports.
func ValidateManifest(manifest Manifest) error {
	if manifest.APIVersion != ManifestAPIVersion ||
		manifest.PolicyVersion != FunctionalAPIPolicyVersion ||
		manifest.ImpactAPIVersion != change.ImpactAPIVersion ||
		manifest.ImpactAnalyzerVersion != change.OpenAPIAnalyzerVersion {
		return fmt.Errorf("%w: version", ErrInvalid)
	}
	if manifest.Family != catalog.TestFamilyFunctionalAPI ||
		(manifest.Mode != ModeTargeted && manifest.Mode != ModeFallback) {
		return fmt.Errorf("%w: family or mode", ErrInvalid)
	}
	if err := validateChangeReference(manifest.Change); err != nil {
		return err
	}
	if !manifest.Catalog.Key().Valid() {
		return fmt.Errorf("%w: catalog identity", ErrInvalid)
	}
	if manifest.Catalog.Key().Repository != manifest.Change.SourceRepository.Identity ||
		manifest.Catalog.Revision != manifest.Change.BaseRevision {
		return fmt.Errorf("%w: catalog provenance", ErrInvalid)
	}
	if err := validateCapabilitySets(manifest); err != nil {
		return err
	}
	if len(manifest.Decisions) > MaxManifestDecisions {
		return fmt.Errorf("%w: decision bound", ErrInvalid)
	}
	identities := make(map[catalog.TestIdentity]struct{}, len(manifest.Decisions))
	for _, decision := range manifest.Decisions {
		if err := validateDecision(decision, manifest.Mode, manifest.AffectedCapabilities); err != nil {
			return err
		}
		identity := decision.Test.Identity()
		if _, exists := identities[identity]; exists {
			return fmt.Errorf("%w: duplicate test decision", ErrInvalid)
		}
		identities[identity] = struct{}{}
	}

	return nil
}

func validateCapabilitySets(manifest Manifest) error {
	if len(manifest.AffectedCapabilities) > MaxManifestCapabilities ||
		len(manifest.UncoveredCapabilities) > MaxManifestCapabilities ||
		len(manifest.Warnings) > change.MaxImpactWarnings {
		return fmt.Errorf("%w: manifest bounds", ErrInvalid)
	}
	if manifest.Mode == ModeTargeted && len(manifest.AffectedCapabilities) == 0 {
		return fmt.Errorf("%w: targeted manifest without affected capabilities", ErrInvalid)
	}
	affected := make(map[string]struct{}, len(manifest.AffectedCapabilities))
	for _, capability := range manifest.AffectedCapabilities {
		if !catalog.IsLocalKey(capability) {
			return fmt.Errorf("%w: affected capability", ErrInvalid)
		}
		affected[capability] = struct{}{}
	}
	for _, capability := range manifest.UncoveredCapabilities {
		if _, exists := affected[capability]; !exists {
			return fmt.Errorf("%w: uncovered capability", ErrInvalid)
		}
	}
	for _, warning := range manifest.Warnings {
		if strings.TrimSpace(warning) == "" || len(warning) > 512 {
			return fmt.Errorf("%w: warning", ErrInvalid)
		}
	}

	return nil
}

func validateDecision(decision Decision, mode Mode, affected []string) error {
	if !decision.Test.Identity().Valid() || decision.Test.Family != catalog.TestFamilyFunctionalAPI ||
		strings.TrimSpace(decision.Test.Adapter) == "" || strings.TrimSpace(decision.Test.Name) == "" ||
		len(decision.Test.Capabilities) == 0 ||
		len(decision.Test.Capabilities) > MaxManifestCapabilities || len(decision.Reasons) == 0 {
		return fmt.Errorf("%w: test decision", ErrInvalid)
	}
	if err := validateTestCapabilities(decision.Test.Capabilities); err != nil {
		return err
	}
	if err := validateOutcome(decision, mode); err != nil {
		return err
	}
	if err := validateReasons(decision, affected); err != nil {
		return err
	}

	return validateReasonPolicy(decision, mode)
}

func validateTestCapabilities(capabilities []string) error {
	for _, capability := range capabilities {
		if !catalog.IsLocalKey(capability) {
			return fmt.Errorf("%w: test capability", ErrInvalid)
		}
	}

	return nil
}

func validateOutcome(decision Decision, mode Mode) error {
	switch decision.Outcome {
	case OutcomeRunRequired:
		if decision.RemainingExecution != RemainingNone {
			return fmt.Errorf("%w: required test remaining stage", ErrInvalid)
		}
	case OutcomeSkipForNow:
		if mode != ModeTargeted || decision.RemainingExecution != RemainingFullSuite {
			return fmt.Errorf("%w: omitted test remaining stage", ErrInvalid)
		}
	default:
		return fmt.Errorf("%w: outcome", ErrInvalid)
	}

	return nil
}

func validateReasons(decision Decision, affected []string) error {
	for _, reason := range decision.Reasons {
		if reason.Code != ReasonDirectCapabilityImpact &&
			reason.Code != ReasonConservativeFallback &&
			reason.Code != ReasonNotAffected {
			return fmt.Errorf("%w: reason", ErrInvalid)
		}
		if len(reason.Capabilities) > MaxManifestCapabilities {
			return fmt.Errorf("%w: reason capability bound", ErrInvalid)
		}
		for _, capability := range reason.Capabilities {
			if !slices.Contains(affected, capability) ||
				!slices.Contains(decision.Test.Capabilities, capability) {
				return fmt.Errorf("%w: reason capability provenance", ErrInvalid)
			}
		}
	}

	return nil
}

func validateReasonPolicy(decision Decision, mode Mode) error {
	if len(decision.Reasons) != 1 {
		return fmt.Errorf("%w: reason count", ErrInvalid)
	}
	reason := decision.Reasons[0]
	switch {
	case mode == ModeFallback:
		if decision.Outcome != OutcomeRunRequired || reason.Code != ReasonConservativeFallback ||
			len(reason.Capabilities) != 0 {
			return fmt.Errorf("%w: fallback decision", ErrInvalid)
		}
	case decision.Outcome == OutcomeRunRequired:
		if reason.Code != ReasonDirectCapabilityImpact || len(reason.Capabilities) == 0 {
			return fmt.Errorf("%w: direct impact decision", ErrInvalid)
		}
	case decision.Outcome == OutcomeSkipForNow:
		if reason.Code != ReasonNotAffected || len(reason.Capabilities) != 0 {
			return fmt.Errorf("%w: omission decision", ErrInvalid)
		}
	}

	return nil
}

func validateChangeReference(reference change.Reference) error {
	return change.ValidateSet(change.Set{
		APIVersion: change.SetAPIVersion, SourceRepository: reference.SourceRepository,
		PullRequestNumber: reference.PullRequestNumber, BaseRevision: reference.BaseRevision,
		HeadRevision: reference.HeadRevision, ObservedAt: reference.ObservedAt, Trigger: reference.Trigger,
	})
}

func canonicalStrings(values []string) []string {
	result := append([]string{}, values...)
	slices.Sort(result)

	return slices.Compact(result)
}
