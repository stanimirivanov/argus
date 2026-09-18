// Package openapi discovers changed OpenAPI 3 documents and converts semantic
// operation changes into provider-neutral capability impact.
package openapi

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"slices"
	"strings"

	"github.com/pb33f/libopenapi"
	"github.com/stanimirivanov/argus/internal/catalog"
	"github.com/stanimirivanov/argus/internal/change"
	"go.yaml.in/yaml/v3"
)

var errUnsupportedReference = errors.New("unsupported external OpenAPI reference")

// DocumentSource loads one repository-relative file at an immutable revision.
type DocumentSource interface {
	LoadDocument(context.Context, catalog.Repository, catalog.Revision, string) ([]byte, error)
}

// Analyzer uses libopenapi for standards-aware parsing and semantic change
// counts, then applies Argus's explicit x-argus-capabilities mapping policy.
type Analyzer struct {
	source DocumentSource
}

// NewAnalyzer constructs the OpenAPI impact adapter.
func NewAnalyzer(source DocumentSource) *Analyzer {
	return &Analyzer{source: source}
}

// Analyze inspects bounded YAML and JSON changes. Unsupported or invalid
// candidate documents produce a persisted partial result rather than a false
// complete answer.
func (analyzer *Analyzer) Analyze(ctx context.Context, set change.Set) (change.CapabilityImpact, error) {
	if analyzer == nil || analyzer.source == nil {
		return change.CapabilityImpact{}, change.ErrUnavailable
	}
	set = change.CanonicalSet(set)
	if err := change.ValidateSet(set); err != nil {
		return change.CapabilityImpact{}, err
	}

	impact := change.CapabilityImpact{
		APIVersion:      change.ImpactAPIVersion,
		AnalyzerVersion: change.OpenAPIAnalyzerVersion,
		Change:          set.Reference(),
		Status:          change.ImpactComplete,
	}
	if set.FilesTruncated {
		markPartial(&impact, "change set omitted files beyond the ingestion limit")
	}
	candidates := candidateFiles(set.Files)
	if len(candidates) > change.MaxImpactDocuments {
		markPartial(&impact, fmt.Sprintf(
			"OpenAPI discovery inspected the first %d candidate files",
			change.MaxImpactDocuments,
		))
		candidates = candidates[:change.MaxImpactDocuments]
	}

	for _, file := range candidates {
		documentImpact, found, warning, err := analyzer.analyzeFile(ctx, set, file)
		if err != nil {
			return change.CapabilityImpact{}, err
		}
		if warning != "" {
			markPartial(&impact, warning)
		}
		if found {
			appendDocumentImpact(&impact, documentImpact)
		}
	}

	impact = change.CanonicalCapabilityImpact(impact)
	if err := change.ValidateCapabilityImpact(impact); err != nil {
		return change.CapabilityImpact{}, fmt.Errorf("normalize OpenAPI impact: %w", err)
	}

	return impact, nil
}

func appendDocumentImpact(impact *change.CapabilityImpact, document change.DocumentImpact) {
	if len(document.Operations) > change.MaxImpactOperations {
		slices.SortFunc(document.Operations, func(left, right change.OperationImpact) int {
			if compared := strings.Compare(left.Path, right.Path); compared != 0 {
				return compared
			}

			return strings.Compare(left.Method, right.Method)
		})
		document.Operations = document.Operations[:change.MaxImpactOperations]
		markPartial(impact, fmt.Sprintf(
			"OpenAPI candidate %s retained the first %d changed operations",
			document.Path,
			change.MaxImpactOperations,
		))
	}
	impact.Documents = append(impact.Documents, document)
}

func (analyzer *Analyzer) analyzeFile(
	ctx context.Context,
	set change.Set,
	file change.File,
) (change.DocumentImpact, bool, string, error) {
	basePath, headPath := file.Path, file.Path
	switch file.Kind {
	case change.KindAdded:
		basePath = ""
	case change.KindDeleted:
		headPath = ""
	case change.KindRenamed:
		basePath = *file.PreviousPath
	case change.KindCopied:
		basePath = ""
	}

	base, baseOpenAPI, err := analyzer.load(ctx, set.SourceRepository, set.BaseRevision, basePath)
	if err != nil {
		return change.DocumentImpact{}, false, "", err
	}
	head, headOpenAPI, err := analyzer.load(ctx, set.SourceRepository, set.HeadRevision, headPath)
	if err != nil {
		return change.DocumentImpact{}, false, "", err
	}
	if !baseOpenAPI && !headOpenAPI {
		return change.DocumentImpact{}, false, "", nil
	}
	if (baseOpenAPI && base == nil) || (headOpenAPI && head == nil) {
		return change.DocumentImpact{}, false,
			fmt.Sprintf("OpenAPI candidate %s could not be parsed at both immutable revisions", file.Path), nil
	}

	documentImpact, changed, err := compareDocuments(base, head, file.Path, previousPath(file))
	if err != nil {
		if errors.Is(err, errUnsupportedReference) {
			return change.DocumentImpact{}, false,
				fmt.Sprintf("OpenAPI candidate %s uses an unsupported external reference", file.Path), nil
		}

		return change.DocumentImpact{}, false,
			fmt.Sprintf("OpenAPI candidate %s could not be analyzed semantically", file.Path), nil
	}

	return documentImpact, changed, "", nil
}

func (analyzer *Analyzer) load(
	ctx context.Context,
	repository catalog.Repository,
	revision catalog.Revision,
	path string,
) (*parsedDocument, bool, error) {
	if path == "" {
		return nil, false, nil
	}
	data, err := analyzer.source.LoadDocument(ctx, repository, revision, path)
	if err != nil {
		return nil, false, err
	}
	document, isOpenAPI, err := parseDocument(data)
	if err != nil {
		return nil, true, nil
	}

	return document, isOpenAPI, nil
}

type parsedDocument struct {
	document   libopenapi.Document
	root       map[string]any
	operations map[operationKey]parsedOperation
}

type parsedOperation struct {
	operationID  *string
	capabilities []string
	fingerprint  string
}

type operationKey struct {
	method string
	path   string
}

func parseDocument(data []byte) (*parsedDocument, bool, error) {
	var root map[string]any
	if err := yaml.Unmarshal(data, &root); err != nil {
		return nil, false, err
	}
	version, ok := root["openapi"].(string)
	if !ok || (!strings.HasPrefix(version, "3.0.") && !strings.HasPrefix(version, "3.1.") &&
		!strings.HasPrefix(version, "3.2.")) {
		return nil, false, nil
	}
	document, err := libopenapi.NewDocument(data)
	if err != nil {
		return nil, true, err
	}
	if _, buildErr := document.BuildV3Model(); buildErr != nil {
		return nil, true, buildErr
	}
	operations, err := extractOperations(root)
	if err != nil {
		return nil, true, err
	}

	return &parsedDocument{document: document, root: root, operations: operations}, true, nil
}

func extractOperations(root map[string]any) (map[operationKey]parsedOperation, error) {
	paths, ok := stringMap(root["paths"])
	if !ok {
		return nil, errors.New("OpenAPI paths must be an object")
	}
	operations := make(map[operationKey]parsedOperation)
	for route, rawPathItem := range paths {
		pathItem, ok := stringMap(rawPathItem)
		if !ok || !strings.HasPrefix(route, "/") {
			continue
		}
		for method, rawOperation := range pathItem {
			key, operation, found, err := extractOperation(root, pathItem, route, method, rawOperation)
			if err != nil {
				return nil, err
			}
			if !found {
				continue
			}
			operations[key] = operation
		}
	}

	return operations, nil
}

func extractOperation(
	root map[string]any,
	pathItem map[string]any,
	route string,
	method string,
	rawOperation any,
) (operationKey, parsedOperation, bool, error) {
	upperMethod := strings.ToUpper(method)
	if !isHTTPMethod(upperMethod) {
		return operationKey{}, parsedOperation{}, false, nil
	}
	operation, ok := stringMap(rawOperation)
	if !ok {
		return operationKey{}, parsedOperation{}, false,
			fmt.Errorf("OpenAPI operation %s %s must be an object", upperMethod, route)
	}
	capabilities, err := operationCapabilities(operation)
	if err != nil {
		return operationKey{}, parsedOperation{}, false,
			fmt.Errorf("OpenAPI operation %s %s: %w", upperMethod, route, err)
	}
	material := map[string]any{
		"operation":      operation,
		"pathParameters": pathItem["parameters"],
		"pathServers":    pathItem["servers"],
		"security":       root["security"],
		"servers":        root["servers"],
	}
	fingerprint, err := semanticFingerprint(root, material)
	if err != nil {
		return operationKey{}, parsedOperation{}, false, err
	}
	var operationID *string
	if value, exists := operation["operationId"].(string); exists && value != "" {
		operationID = &value
	}

	return operationKey{method: upperMethod, path: route}, parsedOperation{
		operationID: operationID, capabilities: capabilities, fingerprint: fingerprint,
	}, true, nil
}

func semanticFingerprint(root map[string]any, material map[string]any) (string, error) {
	references := make(map[string]any)
	queue := collectReferences(material)
	seen := make(map[string]struct{})
	for len(queue) != 0 {
		reference := queue[0]
		queue = queue[1:]
		if _, exists := seen[reference]; exists {
			continue
		}
		seen[reference] = struct{}{}
		if !strings.HasPrefix(reference, "#/") {
			return "", errUnsupportedReference
		}
		resolved, ok := resolveJSONPointer(root, reference)
		if !ok {
			return "", fmt.Errorf("unresolved OpenAPI reference %q", reference)
		}
		references[reference] = resolved
		queue = append(queue, collectReferences(resolved)...)
	}
	data, err := json.Marshal(map[string]any{"material": material, "references": references})
	if err != nil {
		return "", fmt.Errorf("canonicalize OpenAPI operation: %w", err)
	}
	sum := sha256.Sum256(data)

	return hex.EncodeToString(sum[:]), nil
}

func compareDocuments(
	base *parsedDocument,
	head *parsedDocument,
	path string,
	previous *string,
) (change.DocumentImpact, bool, error) {
	documentImpact := change.DocumentImpact{Path: path, PreviousPath: previous}
	switch {
	case base == nil:
		documentImpact.Kind = change.SemanticAdded
		documentImpact.TotalChanges = max(1, len(head.operations))
		documentImpact.Operations = addedOperations(head.operations)
	case head == nil:
		documentImpact.Kind = change.SemanticRemoved
		documentImpact.TotalChanges = max(1, len(base.operations))
		documentImpact.BreakingChanges = documentImpact.TotalChanges
		documentImpact.Operations = removedOperations(base.operations)
	default:
		semanticChanges, err := libopenapi.CompareDocuments(base.document, head.document)
		if err != nil {
			return change.DocumentImpact{}, false, err
		}
		if semanticChanges == nil || semanticChanges.TotalChanges() == 0 {
			return change.DocumentImpact{}, false, nil
		}
		documentImpact.Kind = change.SemanticModified
		documentImpact.TotalChanges = semanticChanges.TotalChanges()
		documentImpact.BreakingChanges = semanticChanges.TotalBreakingChanges()
		documentImpact.Operations = changedOperations(
			base.operations,
			head.operations,
			documentImpact.BreakingChanges > 0,
		)
	}

	return documentImpact, true, nil
}

func addedOperations(operations map[operationKey]parsedOperation) []change.OperationImpact {
	result := make([]change.OperationImpact, 0, len(operations))
	for key, operation := range operations {
		result = append(result, newOperationImpact(key, operation, change.SemanticAdded, false))
	}

	return result
}

func removedOperations(operations map[operationKey]parsedOperation) []change.OperationImpact {
	result := make([]change.OperationImpact, 0, len(operations))
	for key, operation := range operations {
		result = append(result, newOperationImpact(key, operation, change.SemanticRemoved, true))
	}

	return result
}

func changedOperations(
	base map[operationKey]parsedOperation,
	head map[operationKey]parsedOperation,
	potentialBreak bool,
) []change.OperationImpact {
	keys := make(map[operationKey]struct{}, len(base)+len(head))
	for key := range base {
		keys[key] = struct{}{}
	}
	for key := range head {
		keys[key] = struct{}{}
	}
	result := make([]change.OperationImpact, 0)
	for key := range keys {
		before, beforeExists := base[key]
		after, afterExists := head[key]
		switch {
		case !beforeExists:
			result = append(result, newOperationImpact(key, after, change.SemanticAdded, false))
		case !afterExists:
			result = append(result, newOperationImpact(key, before, change.SemanticRemoved, true))
		case before.fingerprint != after.fingerprint:
			after.capabilities = unionCapabilities(before.capabilities, after.capabilities)
			result = append(result, newOperationImpact(key, after, change.SemanticModified, potentialBreak))
		}
	}

	return result
}

func newOperationImpact(
	key operationKey,
	operation parsedOperation,
	kind change.SemanticChangeKind,
	potentialBreak bool,
) change.OperationImpact {
	return change.OperationImpact{
		Method:         key.method,
		Path:           key.path,
		OperationID:    operation.operationID,
		Kind:           kind,
		Capabilities:   append([]string(nil), operation.capabilities...),
		PotentialBreak: potentialBreak,
	}
}

func candidateFiles(files []change.File) []change.File {
	candidates := make([]change.File, 0)
	for _, file := range files {
		extension := strings.ToLower(filepath.Ext(file.Path))
		if extension == ".yaml" || extension == ".yml" || extension == ".json" {
			candidates = append(candidates, file)
		}
	}

	return candidates
}

func operationCapabilities(operation map[string]any) ([]string, error) {
	raw, exists := operation["x-argus-capabilities"]
	if !exists {
		return nil, nil
	}
	values, ok := raw.([]any)
	if !ok || len(values) == 0 || len(values) > change.MaxOperationCapabilities {
		return nil, errors.New("x-argus-capabilities must be an array of 1 to 50 capability keys")
	}
	capabilities := make([]string, 0, len(values))
	for _, rawValue := range values {
		value, ok := rawValue.(string)
		if !ok || !catalog.IsLocalKey(value) {
			return nil, errors.New("x-argus-capabilities contains an invalid capability key")
		}
		capabilities = append(capabilities, value)
	}
	slices.Sort(capabilities)
	if len(slices.Compact(capabilities)) != len(capabilities) {
		return nil, errors.New("x-argus-capabilities contains a duplicate capability key")
	}

	return capabilities, nil
}

func collectReferences(value any) []string {
	var references []string
	switch typed := value.(type) {
	case map[string]any:
		for key, item := range typed {
			if key == "$ref" {
				if reference, ok := item.(string); ok {
					references = append(references, reference)
				}
				continue
			}
			references = append(references, collectReferences(item)...)
		}
	case []any:
		for _, item := range typed {
			references = append(references, collectReferences(item)...)
		}
	}

	return references
}

func resolveJSONPointer(root map[string]any, reference string) (any, bool) {
	var current any = root
	for _, token := range strings.Split(strings.TrimPrefix(reference, "#/"), "/") {
		token = strings.ReplaceAll(strings.ReplaceAll(token, "~1", "/"), "~0", "~")
		mapping, ok := stringMap(current)
		if !ok {
			return nil, false
		}
		current, ok = mapping[token]
		if !ok {
			return nil, false
		}
	}

	return current, true
}

func stringMap(value any) (map[string]any, bool) {
	mapping, ok := value.(map[string]any)
	return mapping, ok
}

func previousPath(file change.File) *string {
	if file.Kind != change.KindRenamed {
		return nil
	}
	value := *file.PreviousPath

	return &value
}

func unionCapabilities(left, right []string) []string {
	result := append(append([]string(nil), left...), right...)
	slices.Sort(result)
	return slices.Compact(result)
}

func isHTTPMethod(method string) bool {
	switch method {
	case "GET", "PUT", "POST", "DELETE", "OPTIONS", "HEAD", "PATCH", "TRACE", "QUERY":
		return true
	default:
		return false
	}
}

func markPartial(impact *change.CapabilityImpact, warning string) {
	impact.Status = change.ImpactPartial
	if len(impact.Warnings) < change.MaxImpactWarnings {
		impact.Warnings = append(impact.Warnings, warning)
	}
}
