package change

import (
	"cmp"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/stanimirivanov/argus/internal/catalog"
)

const (
	// ImpactAPIVersion is the supported change-derived capability-impact major.
	ImpactAPIVersion = "argus.dev/capability-impact/v1"
	// OpenAPIAnalyzerVersion identifies Argus's interpretation of libopenapi
	// output and x-argus-capabilities mappings.
	OpenAPIAnalyzerVersion = "argus-openapi/v1+libopenapi/v0.38.7"
	// MaxImpactDocuments bounds contract discovery for one change set.
	MaxImpactDocuments = 50
	// MaxImpactWarnings bounds partial-analysis explanations.
	MaxImpactWarnings = 100
	// MaxImpactOperations bounds semantic evidence retained for one document.
	MaxImpactOperations = 5000
	// MaxOperationCapabilities bounds explicit mappings for one operation.
	MaxOperationCapabilities = 50
)

// ImpactStatus states whether all candidate evidence was understood.
type ImpactStatus string

const (
	// ImpactComplete means every bounded OpenAPI candidate was analyzed.
	ImpactComplete ImpactStatus = "complete"
	// ImpactPartial means omitted or unsupported evidence requires conservative
	// downstream behavior.
	ImpactPartial ImpactStatus = "partial"
)

// SemanticChangeKind describes how an OpenAPI document or operation changed.
type SemanticChangeKind string

const (
	// SemanticAdded means the item exists only at the head revision.
	SemanticAdded SemanticChangeKind = "added"
	// SemanticModified means the item's semantic fingerprint changed.
	SemanticModified SemanticChangeKind = "modified"
	// SemanticRemoved means the item exists only at the base revision.
	SemanticRemoved SemanticChangeKind = "removed"
)

// Reference identifies the immutable change without carrying its file bundle.
type Reference struct {
	SourceRepository  catalog.Repository
	PullRequestNumber int
	BaseRevision      catalog.Revision
	HeadRevision      catalog.Revision
	ObservedAt        time.Time
	Trigger           Trigger
}

// Reference returns the immutable identity and provider trigger for a set.
func (set Set) Reference() Reference {
	return Reference{
		SourceRepository:  set.SourceRepository,
		PullRequestNumber: set.PullRequestNumber,
		BaseRevision:      set.BaseRevision,
		HeadRevision:      set.HeadRevision,
		ObservedAt:        set.ObservedAt.UTC(),
		Trigger:           set.Trigger,
	}
}

// CapabilityImpact is the immutable, explainable result of semantic API
// analysis for one change delivery.
type CapabilityImpact struct {
	APIVersion      string
	AnalyzerVersion string
	Change          Reference
	Status          ImpactStatus
	Documents       []DocumentImpact
	Warnings        []string
}

// DocumentImpact summarizes one OpenAPI document and its changed operations.
type DocumentImpact struct {
	Path            string
	PreviousPath    *string
	Kind            SemanticChangeKind
	TotalChanges    int
	BreakingChanges int
	Operations      []OperationImpact
}

// OperationImpact maps one changed HTTP operation to explicit repository-
// scoped capabilities. An empty Capabilities list is retained as an unmapped
// change rather than silently ignored.
type OperationImpact struct {
	Method         string
	Path           string
	OperationID    *string
	Kind           SemanticChangeKind
	Capabilities   []string
	PotentialBreak bool
}

// CanonicalCapabilityImpact deep-copies and orders all set-like fields.
func CanonicalCapabilityImpact(impact CapabilityImpact) CapabilityImpact {
	canonical := impact
	canonical.Change.ObservedAt = impact.Change.ObservedAt.UTC()
	canonical.Warnings = append([]string(nil), impact.Warnings...)
	slices.Sort(canonical.Warnings)
	canonical.Warnings = slices.Compact(canonical.Warnings)
	canonical.Documents = make([]DocumentImpact, len(impact.Documents))
	for documentIndex, document := range impact.Documents {
		canonical.Documents[documentIndex] = document
		canonical.Documents[documentIndex].PreviousPath = cloneOptionalString(document.PreviousPath)
		canonical.Documents[documentIndex].Operations = make([]OperationImpact, len(document.Operations))
		for operationIndex, operation := range document.Operations {
			canonical.Documents[documentIndex].Operations[operationIndex] = operation
			canonical.Documents[documentIndex].Operations[operationIndex].OperationID = cloneOptionalString(operation.OperationID)
			canonical.Documents[documentIndex].Operations[operationIndex].Capabilities = append(
				[]string(nil),
				operation.Capabilities...,
			)
			slices.Sort(canonical.Documents[documentIndex].Operations[operationIndex].Capabilities)
			canonical.Documents[documentIndex].Operations[operationIndex].Capabilities = slices.Compact(
				canonical.Documents[documentIndex].Operations[operationIndex].Capabilities,
			)
		}
		slices.SortFunc(canonical.Documents[documentIndex].Operations, compareOperationImpacts)
	}
	slices.SortFunc(canonical.Documents, func(left, right DocumentImpact) int {
		if compared := cmp.Compare(left.Path, right.Path); compared != 0 {
			return compared
		}
		return cmp.Compare(optionalString(left.PreviousPath), optionalString(right.PreviousPath))
	})

	return canonical
}

// ValidateCapabilityImpact enforces invariants independent of transports,
// parsers, and persistence.
func ValidateCapabilityImpact(impact CapabilityImpact) error {
	if impact.APIVersion != ImpactAPIVersion || impact.AnalyzerVersion != OpenAPIAnalyzerVersion {
		return fmt.Errorf("%w: unsupported capability-impact version", ErrInvalid)
	}
	if err := validateReference(impact.Change); err != nil {
		return err
	}
	if impact.Status != ImpactComplete && impact.Status != ImpactPartial {
		return fmt.Errorf("%w: impact status", ErrInvalid)
	}
	if len(impact.Documents) > MaxImpactDocuments || len(impact.Warnings) > MaxImpactWarnings {
		return fmt.Errorf("%w: capability-impact bounds", ErrInvalid)
	}
	if impact.Status == ImpactComplete && len(impact.Warnings) != 0 {
		return fmt.Errorf("%w: complete impact contains warnings", ErrInvalid)
	}
	for _, warning := range impact.Warnings {
		if strings.TrimSpace(warning) == "" || len(warning) > 512 {
			return fmt.Errorf("%w: impact warning", ErrInvalid)
		}
	}

	return validateDocumentImpacts(impact.Documents)
}

func validateDocumentImpacts(documents []DocumentImpact) error {
	documentPaths := make(map[string]struct{}, len(documents))
	for _, document := range documents {
		if !validRepositoryPath(document.Path) || document.TotalChanges < 1 ||
			document.BreakingChanges < 0 || document.BreakingChanges > document.TotalChanges ||
			len(document.Operations) > MaxImpactOperations {
			return fmt.Errorf("%w: OpenAPI document impact", ErrInvalid)
		}
		if _, exists := documentPaths[document.Path]; exists {
			return fmt.Errorf("%w: duplicate OpenAPI document", ErrInvalid)
		}
		documentPaths[document.Path] = struct{}{}
		if err := validateDocumentKind(document); err != nil {
			return err
		}
		if err := validateOperationImpacts(document.Operations); err != nil {
			return err
		}
	}

	return nil
}

func validateOperationImpacts(operations []OperationImpact) error {
	identities := make(map[string]struct{}, len(operations))
	for _, operation := range operations {
		if err := validateOperationImpact(operation); err != nil {
			return err
		}
		key := operation.Method + " " + operation.Path
		if _, exists := identities[key]; exists {
			return fmt.Errorf("%w: duplicate impacted operation", ErrInvalid)
		}
		identities[key] = struct{}{}
	}

	return nil
}

func validateReference(reference Reference) error {
	return ValidateSet(Set{
		APIVersion:        SetAPIVersion,
		SourceRepository:  reference.SourceRepository,
		PullRequestNumber: reference.PullRequestNumber,
		BaseRevision:      reference.BaseRevision,
		HeadRevision:      reference.HeadRevision,
		ObservedAt:        reference.ObservedAt,
		Trigger:           reference.Trigger,
	})
}

func validateDocumentKind(document DocumentImpact) error {
	switch document.Kind {
	case SemanticAdded:
		if document.PreviousPath != nil || document.BreakingChanges != 0 {
			return fmt.Errorf("%w: added OpenAPI document", ErrInvalid)
		}
	case SemanticModified:
		if document.PreviousPath != nil &&
			(!validRepositoryPath(*document.PreviousPath) || *document.PreviousPath == document.Path) {
			return fmt.Errorf("%w: renamed OpenAPI document", ErrInvalid)
		}
	case SemanticRemoved:
		if document.PreviousPath != nil || document.BreakingChanges != document.TotalChanges {
			return fmt.Errorf("%w: removed OpenAPI document", ErrInvalid)
		}
	default:
		return fmt.Errorf("%w: OpenAPI document change kind", ErrInvalid)
	}

	return nil
}

func validateOperationImpact(operation OperationImpact) error {
	method := strings.ToUpper(operation.Method)
	if operation.Method != method || !validHTTPMethod(method) || !strings.HasPrefix(operation.Path, "/") ||
		len(operation.Path) > 2048 {
		return fmt.Errorf("%w: impacted operation identity", ErrInvalid)
	}
	if operation.Kind != SemanticAdded && operation.Kind != SemanticModified && operation.Kind != SemanticRemoved {
		return fmt.Errorf("%w: impacted operation kind", ErrInvalid)
	}
	if operation.OperationID != nil && (strings.TrimSpace(*operation.OperationID) == "" || len(*operation.OperationID) > 255) {
		return fmt.Errorf("%w: operation ID", ErrInvalid)
	}
	if len(operation.Capabilities) > MaxOperationCapabilities {
		return fmt.Errorf("%w: operation capability bounds", ErrInvalid)
	}
	seen := make(map[string]struct{}, len(operation.Capabilities))
	for _, capability := range operation.Capabilities {
		if !catalog.IsLocalKey(capability) {
			return fmt.Errorf("%w: capability key", ErrInvalid)
		}
		if _, exists := seen[capability]; exists {
			return fmt.Errorf("%w: duplicate operation capability", ErrInvalid)
		}
		seen[capability] = struct{}{}
	}

	return nil
}

func validHTTPMethod(method string) bool {
	switch method {
	case "GET", "PUT", "POST", "DELETE", "OPTIONS", "HEAD", "PATCH", "TRACE", "QUERY":
		return true
	default:
		return false
	}
}

func compareOperationImpacts(left, right OperationImpact) int {
	if compared := cmp.Compare(left.Path, right.Path); compared != 0 {
		return compared
	}
	return cmp.Compare(left.Method, right.Method)
}

func cloneOptionalString(value *string) *string {
	if value == nil {
		return nil
	}
	cloned := *value

	return &cloned
}

func optionalString(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}
