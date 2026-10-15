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
	// CompositeImpactAPIVersion adds explicit changed-file mapping evidence.
	CompositeImpactAPIVersion = "argus.dev/capability-impact/v2"
	// CompositeAnalyzerVersion binds the file-root and semantic API policies.
	CompositeAnalyzerVersion = "argus-component-roots/v1+argus-openapi/v1+libopenapi/v0.38.7"
	// MaxImpactDocuments bounds contract discovery for one change set.
	MaxImpactDocuments = 50
	// MaxImpactWarnings bounds partial-analysis explanations.
	MaxImpactWarnings = 100
	// MaxCompositeImpactWarnings retains both bounded producers' explanations.
	MaxCompositeImpactWarnings = 200
	// MaxImpactOperations bounds semantic evidence retained for one document.
	MaxImpactOperations = 5000
	// MaxOperationCapabilities bounds explicit mappings for one operation.
	MaxOperationCapabilities = 50
	// MaxFileMappings bounds one file's explicit root and capability evidence.
	MaxFileMappings = 100
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
	APIVersion       string
	AnalyzerVersion  string
	Change           Reference
	Status           ImpactStatus
	SemanticStatus   ImpactStatus
	FileStatus       ImpactStatus
	Documents        []DocumentImpact
	Files            []FileImpact
	Warnings         []string
	SemanticWarnings []string
	FileWarnings     []string
}

// FileImpact records how one changed file was covered. UnmappedPaths retains
// either side of a rename that lacked an explicit component root.
type FileImpact struct {
	Path           string
	PreviousPath   *string
	Kind           Kind
	Coverage       string
	Capabilities   []string
	ComponentRoots []string
	UnmappedPaths  []string
}

// File coverage identifies the explicit evidence source or an unresolved path.
const (
	FileCoverageOpenAPI   = "openapi"
	FileCoverageComponent = "component-root"
	FileCoverageUnmapped  = "unmapped"
)

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
	canonical.SemanticWarnings = sortedUnique(impact.SemanticWarnings)
	canonical.FileWarnings = sortedUnique(impact.FileWarnings)
	canonical.Files = make([]FileImpact, len(impact.Files))
	for index, file := range impact.Files {
		canonical.Files[index] = file
		canonical.Files[index].PreviousPath = cloneOptionalString(file.PreviousPath)
		canonical.Files[index].Capabilities = sortedUnique(file.Capabilities)
		canonical.Files[index].ComponentRoots = sortedUnique(file.ComponentRoots)
		canonical.Files[index].UnmappedPaths = sortedUnique(file.UnmappedPaths)
	}
	slices.SortFunc(canonical.Files, func(left, right FileImpact) int {
		return cmp.Compare(left.Path, right.Path)
	})
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
	if err := validateImpactVersion(impact); err != nil {
		return err
	}
	if err := validateReference(impact.Change); err != nil {
		return err
	}
	if impact.Status != ImpactComplete && impact.Status != ImpactPartial {
		return fmt.Errorf("%w: impact status", ErrInvalid)
	}
	warningLimit := MaxImpactWarnings
	if impact.APIVersion == CompositeImpactAPIVersion {
		warningLimit = MaxCompositeImpactWarnings
	}
	if len(impact.Documents) > MaxImpactDocuments || len(impact.Warnings) > warningLimit {
		return fmt.Errorf("%w: capability-impact bounds", ErrInvalid)
	}
	if err := validateImpactWarnings(impact); err != nil {
		return err
	}

	if err := validateDocumentImpacts(impact.Documents); err != nil {
		return err
	}
	if impact.APIVersion == CompositeImpactAPIVersion {
		return validateFileImpacts(impact)
	}

	return nil
}

func validateImpactVersion(impact CapabilityImpact) error {
	switch {
	case impact.APIVersion == ImpactAPIVersion && impact.AnalyzerVersion == OpenAPIAnalyzerVersion:
		if len(impact.Files) != 0 || impact.SemanticStatus != "" || impact.FileStatus != "" ||
			len(impact.SemanticWarnings) != 0 || len(impact.FileWarnings) != 0 {
			return fmt.Errorf("%w: v1 impact cannot contain file evidence", ErrInvalid)
		}
	case impact.APIVersion == CompositeImpactAPIVersion && impact.AnalyzerVersion == CompositeAnalyzerVersion:
		if err := validateProducerCompleteness(impact); err != nil {
			return err
		}
	default:
		return fmt.Errorf("%w: unsupported capability-impact version", ErrInvalid)
	}

	return nil
}

func validateProducerCompleteness(impact CapabilityImpact) error {
	if (impact.SemanticStatus != ImpactComplete && impact.SemanticStatus != ImpactPartial) ||
		(impact.FileStatus != ImpactComplete && impact.FileStatus != ImpactPartial) {
		return fmt.Errorf("%w: producer completeness", ErrInvalid)
	}
	combined := ImpactComplete
	if impact.SemanticStatus == ImpactPartial || impact.FileStatus == ImpactPartial {
		combined = ImpactPartial
	}
	if impact.Status != combined || !slices.Equal(sortedUnique(impact.Warnings),
		sortedUnique(append(append([]string(nil), impact.SemanticWarnings...), impact.FileWarnings...))) {
		return fmt.Errorf("%w: inconsistent producer completeness", ErrInvalid)
	}

	return nil
}

func validateImpactWarnings(impact CapabilityImpact) error {
	if impact.Status == ImpactComplete && len(impact.Warnings) != 0 {
		return fmt.Errorf("%w: complete impact contains warnings", ErrInvalid)
	}
	for _, warning := range impact.Warnings {
		if strings.TrimSpace(warning) == "" || len(warning) > 512 {
			return fmt.Errorf("%w: impact warning", ErrInvalid)
		}
	}

	return nil
}

func sortedUnique(values []string) []string {
	result := append([]string(nil), values...)
	slices.Sort(result)
	return slices.Compact(result)
}

func validateFileImpacts(impact CapabilityImpact) error {
	files := impact.Files
	if len(files) > MaxFiles || (len(files) == 0 && impact.FileStatus == ImpactComplete) {
		return fmt.Errorf("%w: file impact bound", ErrInvalid)
	}
	documents := make(map[string]bool, len(impact.Documents))
	for _, document := range impact.Documents {
		documents[document.Path] = true
	}
	seen := make(map[string]bool, len(files))
	for _, file := range files {
		if seen[file.Path] || !validRepositoryPath(file.Path) {
			return fmt.Errorf("%w: duplicate or invalid file impact path", ErrInvalid)
		}
		seen[file.Path] = true
		if err := validateFileImpact(file, documents, impact.FileStatus); err != nil {
			return err
		}
	}

	return nil
}

func validateFileImpact(file FileImpact, documents map[string]bool, status ImpactStatus) error {
	if err := validateFile(File{Path: file.Path, PreviousPath: file.PreviousPath, Kind: file.Kind, PatchStatus: PatchUnavailable}); err != nil {
		return err
	}
	if len(file.Capabilities) > MaxFileMappings || len(file.ComponentRoots) > MaxFileMappings || len(file.UnmappedPaths) > 2 {
		return fmt.Errorf("%w: file mapping bound", ErrInvalid)
	}
	for _, key := range file.Capabilities {
		if !catalog.IsLocalKey(key) {
			return fmt.Errorf("%w: invalid file capability", ErrInvalid)
		}
	}
	for _, root := range file.ComponentRoots {
		if !validRepositoryPath(root) {
			return fmt.Errorf("%w: invalid component root", ErrInvalid)
		}
	}
	for _, path := range file.UnmappedPaths {
		if !validRepositoryPath(path) || (path != file.Path && (file.PreviousPath == nil || path != *file.PreviousPath)) {
			return fmt.Errorf("%w: invalid unmapped path", ErrInvalid)
		}
	}

	return validateFileCoverage(file, documents, status)
}

func validateFileCoverage(file FileImpact, documents map[string]bool, status ImpactStatus) error {
	switch file.Coverage {
	case FileCoverageOpenAPI:
		if !documents[file.Path] || len(file.Capabilities)+len(file.ComponentRoots)+len(file.UnmappedPaths) != 0 {
			return fmt.Errorf("%w: OpenAPI file mapping", ErrInvalid)
		}
	case FileCoverageComponent:
		if len(file.Capabilities) == 0 || len(file.ComponentRoots) == 0 || len(file.UnmappedPaths) != 0 {
			return fmt.Errorf("%w: component file mapping", ErrInvalid)
		}
	case FileCoverageUnmapped:
		if len(file.UnmappedPaths) == 0 || status != ImpactPartial {
			return fmt.Errorf("%w: unmapped file lacks reason", ErrInvalid)
		}
	default:
		return fmt.Errorf("%w: file coverage", ErrInvalid)
	}

	return nil
}

// ValidateImpactFileCoverage ensures a v2 producer retained every normalized
// changed file, including both sides of rename and copy observations.
func ValidateImpactFileCoverage(impact CapabilityImpact, set Set) error {
	if impact.APIVersion != CompositeImpactAPIVersion {
		return nil
	}
	impact = CanonicalCapabilityImpact(impact)
	set = CanonicalSet(set)
	if len(impact.Files) != len(set.Files) {
		return fmt.Errorf("%w: incomplete file impact coverage", ErrInvalid)
	}
	if set.FilesTruncated && impact.FileStatus != ImpactPartial {
		return fmt.Errorf("%w: truncated files require partial impact", ErrInvalid)
	}
	for index, file := range set.Files {
		observed := impact.Files[index]
		if observed.Path != file.Path || observed.Kind != file.Kind ||
			optionalString(observed.PreviousPath) != optionalString(file.PreviousPath) {
			return fmt.Errorf("%w: file impact differs from immutable change", ErrInvalid)
		}
	}

	return nil
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
