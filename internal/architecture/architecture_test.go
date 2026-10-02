// Package architecture_test verifies Argus's product-level package boundaries.
package architecture_test

import (
	"fmt"
	"go/parser"
	"go/token"
	"io/fs"
	"path/filepath"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"testing"
)

const (
	modulePath   = "github.com/stanimirivanov/argus"
	adrReference = "ADR-0006 (docs/decisions/0006-enforce-capability-oriented-hexagonal-boundaries.md)"
)

type architectureLayer string

const (
	domainLayer          architectureLayer = "domain"
	applicationLayer     architectureLayer = "application"
	adapterLayer         architectureLayer = "adapter"
	compositionRootLayer architectureLayer = "composition-root"
)

type capability string

const (
	adaptationCapability capability = "adaptation"
	catalogCapability    capability = "catalog"
	changeCapability     capability = "change"
	executionCapability  capability = "execution"
	selectionCapability  capability = "selection"
	productCapability    capability = "product-composition"
)

type packagePolicy struct {
	layer      architectureLayer
	capability capability
}

type adapterKind string

const (
	drivingAdapter  adapterKind = "driving"
	drivenAdapter   adapterKind = "driven"
	contractAdapter adapterKind = "contract-conversion"
	bridgeAdapter   adapterKind = "application-bridge"
	platformAdapter adapterKind = "platform-utility"
)

// productionPackagePolicy is intentionally explicit. A new production package
// must declare its architectural ownership here before it can enter the graph.
// That makes an otherwise invisible new boundary a reviewable policy change.
var productionPackagePolicy = map[string]packagePolicy{
	modulePath + "/internal/adaptation":                                   {domainLayer, adaptationCapability},
	modulePath + "/internal/adaptation/functionalapi":                     {applicationLayer, adaptationCapability},
	modulePath + "/internal/adaptation/outcome":                           {applicationLayer, adaptationCapability},
	modulePath + "/internal/adaptation/review":                            {applicationLayer, adaptationCapability},
	modulePath + "/internal/adaptation/validation":                        {applicationLayer, adaptationCapability},
	modulePath + "/internal/adaptation/adapters/cli/evidencecli":          {adapterLayer, adaptationCapability},
	modulePath + "/internal/adaptation/adapters/cli/outcomecli":           {adapterLayer, adaptationCapability},
	modulePath + "/internal/adaptation/adapters/cli/proposalcli":          {adapterLayer, adaptationCapability},
	modulePath + "/internal/adaptation/adapters/cli/reviewcli":            {adapterLayer, adaptationCapability},
	modulePath + "/internal/adaptation/adapters/cli/validationcli":        {adapterLayer, adaptationCapability},
	modulePath + "/internal/adaptation/adapters/contract":                 {adapterLayer, adaptationCapability},
	modulePath + "/internal/adaptation/adapters/fileworkspace":            {adapterLayer, adaptationCapability},
	modulePath + "/internal/adaptation/adapters/githubreview":             {adapterLayer, adaptationCapability},
	modulePath + "/internal/adaptation/adapters/processadapter":           {adapterLayer, adaptationCapability},
	modulePath + "/internal/adaptation/adapters/validationprocessadapter": {adapterLayer, adaptationCapability},
	modulePath + "/internal/catalog":                                      {domainLayer, catalogCapability},
	modulePath + "/internal/catalog/impact":                               {applicationLayer, catalogCapability},
	modulePath + "/internal/catalog/snapshot":                             {applicationLayer, catalogCapability},
	modulePath + "/internal/catalog/testquery":                            {applicationLayer, catalogCapability},
	modulePath + "/internal/catalog/adapters/cli/catalogcli":              {adapterLayer, productCapability},
	modulePath + "/internal/catalog/adapters/cli/descriptorcli":           {adapterLayer, catalogCapability},
	modulePath + "/internal/catalog/adapters/contract/descriptor":         {adapterLayer, catalogCapability},
	modulePath + "/internal/catalog/adapters/contract/evidence":           {adapterLayer, catalogCapability},
	modulePath + "/internal/contracts":                                    {adapterLayer, productCapability},
	modulePath + "/internal/postgres":                                     {adapterLayer, productCapability},
	modulePath + "/internal/change":                                       {domainLayer, changeCapability},
	modulePath + "/internal/change/impact":                                {applicationLayer, changeCapability},
	modulePath + "/internal/change/ingest":                                {applicationLayer, changeCapability},
	modulePath + "/internal/change/workflow":                              {applicationLayer, changeCapability},
	modulePath + "/internal/change/adapters/contract":                     {adapterLayer, changeCapability},
	modulePath + "/internal/change/adapters/github":                       {adapterLayer, changeCapability},
	modulePath + "/internal/change/adapters/httpapi":                      {adapterLayer, changeCapability},
	modulePath + "/internal/change/adapters/openapi":                      {adapterLayer, changeCapability},
	modulePath + "/internal/commandline":                                  {adapterLayer, productCapability},
	modulePath + "/internal/execution":                                    {domainLayer, executionCapability},
	modulePath + "/internal/execution/attempts":                           {applicationLayer, executionCapability},
	modulePath + "/internal/execution/functionalapi":                      {applicationLayer, executionCapability},
	modulePath + "/internal/execution/planning":                           {applicationLayer, executionCapability},
	modulePath + "/internal/execution/shadow":                             {applicationLayer, executionCapability},
	modulePath + "/internal/execution/adapters/cli/evidencecli":           {adapterLayer, executionCapability},
	modulePath + "/internal/execution/adapters/cli/executioncli":          {adapterLayer, executionCapability},
	modulePath + "/internal/execution/adapters/cli/planningcli":           {adapterLayer, executionCapability},
	modulePath + "/internal/execution/adapters/contract":                  {adapterLayer, executionCapability},
	modulePath + "/internal/execution/adapters/processadapter":            {adapterLayer, executionCapability},
	modulePath + "/internal/githubtransport":                              {adapterLayer, productCapability},
	modulePath + "/internal/processprotocol":                              {adapterLayer, productCapability},
	modulePath + "/internal/selection":                                    {domainLayer, selectionCapability},
	modulePath + "/internal/selection/functionalapi":                      {applicationLayer, selectionCapability},
	modulePath + "/internal/selection/adapters/catalogreader":             {adapterLayer, selectionCapability},
	modulePath + "/internal/selection/adapters/cli/selectioncli":          {adapterLayer, selectionCapability},
	modulePath + "/internal/selection/adapters/contract":                  {adapterLayer, selectionCapability},
	modulePath + "/internal/selection/adapters/impactreader":              {adapterLayer, selectionCapability},
	modulePath + "/cmd/adaptation-evidence":                               {compositionRootLayer, productCapability},
	modulePath + "/cmd/capture-functional-api-review-outcome":             {compositionRootLayer, productCapability},
	modulePath + "/cmd/catalog":                                           {compositionRootLayer, productCapability},
	modulePath + "/cmd/control-plane":                                     {compositionRootLayer, productCapability},
	modulePath + "/cmd/descriptor":                                        {compositionRootLayer, productCapability},
	modulePath + "/cmd/execution-evidence":                                {compositionRootLayer, productCapability},
	modulePath + "/cmd/migrate":                                           {compositionRootLayer, productCapability},
	modulePath + "/cmd/open-functional-api-repair-pr":                     {compositionRootLayer, productCapability},
	modulePath + "/cmd/plan-functional-api":                               {compositionRootLayer, productCapability},
	modulePath + "/cmd/propose-functional-api-repair":                     {compositionRootLayer, productCapability},
	modulePath + "/cmd/run-functional-api":                                {compositionRootLayer, productCapability},
	modulePath + "/cmd/select":                                            {compositionRootLayer, productCapability},
	modulePath + "/cmd/validate-functional-api-repair":                    {compositionRootLayer, productCapability},
}

// adapterPolicy distinguishes driving adapters from infrastructure selected by
// composition roots. Contract converters and narrow application bridges are
// the only adapters a driving adapter may compose directly.
var adapterPolicy = map[string]adapterKind{
	modulePath + "/internal/adaptation/adapters/cli/evidencecli":          drivingAdapter,
	modulePath + "/internal/adaptation/adapters/cli/outcomecli":           drivingAdapter,
	modulePath + "/internal/adaptation/adapters/cli/proposalcli":          drivingAdapter,
	modulePath + "/internal/adaptation/adapters/cli/reviewcli":            drivingAdapter,
	modulePath + "/internal/adaptation/adapters/cli/validationcli":        drivingAdapter,
	modulePath + "/internal/adaptation/adapters/contract":                 contractAdapter,
	modulePath + "/internal/adaptation/adapters/fileworkspace":            drivenAdapter,
	modulePath + "/internal/adaptation/adapters/githubreview":             drivenAdapter,
	modulePath + "/internal/adaptation/adapters/processadapter":           drivenAdapter,
	modulePath + "/internal/adaptation/adapters/validationprocessadapter": drivenAdapter,
	modulePath + "/internal/catalog/adapters/cli/catalogcli":              drivingAdapter,
	modulePath + "/internal/catalog/adapters/cli/descriptorcli":           drivingAdapter,
	modulePath + "/internal/catalog/adapters/contract/descriptor":         contractAdapter,
	modulePath + "/internal/catalog/adapters/contract/evidence":           contractAdapter,
	modulePath + "/internal/contracts":                                    contractAdapter,
	modulePath + "/internal/postgres":                                     drivenAdapter,
	modulePath + "/internal/change/adapters/contract":                     contractAdapter,
	modulePath + "/internal/change/adapters/github":                       drivenAdapter,
	modulePath + "/internal/change/adapters/httpapi":                      drivingAdapter,
	modulePath + "/internal/change/adapters/openapi":                      drivenAdapter,
	modulePath + "/internal/commandline":                                  contractAdapter,
	modulePath + "/internal/execution/adapters/cli/evidencecli":           drivingAdapter,
	modulePath + "/internal/execution/adapters/cli/executioncli":          drivingAdapter,
	modulePath + "/internal/execution/adapters/cli/planningcli":           drivingAdapter,
	modulePath + "/internal/execution/adapters/contract":                  contractAdapter,
	modulePath + "/internal/execution/adapters/processadapter":            drivenAdapter,
	modulePath + "/internal/githubtransport":                              platformAdapter,
	modulePath + "/internal/processprotocol":                              platformAdapter,
	modulePath + "/internal/selection/adapters/catalogreader":             bridgeAdapter,
	modulePath + "/internal/selection/adapters/cli/selectioncli":          drivingAdapter,
	modulePath + "/internal/selection/adapters/contract":                  contractAdapter,
	modulePath + "/internal/selection/adapters/impactreader":              bridgeAdapter,
}

// generatedContractImporters is deliberately narrower than the adapter layer.
// Only protocol conversion and the current CLI/process protocol boundaries may
// see generated wire DTOs; provider and persistence adapters must use domain
// values and application-owned ports instead.
var generatedContractImporters = map[string]bool{
	modulePath + "/internal/adaptation/adapters/cli/evidencecli":          true,
	modulePath + "/internal/adaptation/adapters/cli/outcomecli":           true,
	modulePath + "/internal/adaptation/adapters/cli/proposalcli":          true,
	modulePath + "/internal/adaptation/adapters/cli/reviewcli":            true,
	modulePath + "/internal/adaptation/adapters/cli/validationcli":        true,
	modulePath + "/internal/adaptation/adapters/contract":                 true,
	modulePath + "/internal/adaptation/adapters/processadapter":           true,
	modulePath + "/internal/adaptation/adapters/validationprocessadapter": true,
	modulePath + "/internal/catalog/adapters/cli/catalogcli":              true,
	modulePath + "/internal/catalog/adapters/cli/descriptorcli":           true,
	modulePath + "/internal/catalog/adapters/contract/descriptor":         true,
	modulePath + "/internal/catalog/adapters/contract/evidence":           true,
	modulePath + "/internal/change/adapters/contract":                     true,
	modulePath + "/internal/execution/adapters/cli/evidencecli":           true,
	modulePath + "/internal/execution/adapters/cli/executioncli":          true,
	modulePath + "/internal/execution/adapters/cli/planningcli":           true,
	modulePath + "/internal/execution/adapters/contract":                  true,
	modulePath + "/internal/execution/adapters/processadapter":            true,
	modulePath + "/internal/selection/adapters/cli/selectioncli":          true,
	modulePath + "/internal/selection/adapters/contract":                  true,
}

var allowedAdapterDependencies = map[adapterKind]map[adapterKind]bool{
	drivingAdapter: {
		contractAdapter: true,
		bridgeAdapter:   true,
	},
	drivenAdapter: {
		contractAdapter: true,
		platformAdapter: true,
	},
	contractAdapter: {},
	bridgeAdapter:   {},
	platformAdapter: {},
}

// The process runner shares infrastructure mechanics, not domain concepts.
// Only the three protocol adapters may import this cross-capability utility.
var processProtocolUsers = map[string]bool{
	modulePath + "/internal/execution/adapters/processadapter":            true,
	modulePath + "/internal/adaptation/adapters/processadapter":           true,
	modulePath + "/internal/adaptation/adapters/validationprocessadapter": true,
}

// Only command composition roots and argument-parsing CLI adapters may use
// command protocol classification. Domain and persistence code cannot acquire
// a dependency on executable behavior through this shared adapter utility.
var commandlineUsers = map[string]bool{
	modulePath + "/internal/adaptation/adapters/cli/evidencecli":   true,
	modulePath + "/internal/adaptation/adapters/cli/outcomecli":    true,
	modulePath + "/internal/adaptation/adapters/cli/proposalcli":   true,
	modulePath + "/internal/adaptation/adapters/cli/reviewcli":     true,
	modulePath + "/internal/adaptation/adapters/cli/validationcli": true,
	modulePath + "/internal/catalog/adapters/cli/catalogcli":       true,
	modulePath + "/internal/catalog/adapters/cli/descriptorcli":    true,
	modulePath + "/internal/execution/adapters/cli/evidencecli":    true,
	modulePath + "/internal/execution/adapters/cli/executioncli":   true,
	modulePath + "/internal/execution/adapters/cli/planningcli":    true,
	modulePath + "/internal/selection/adapters/cli/selectioncli":   true,
}

var githubTransportUsers = map[string]bool{
	modulePath + "/internal/change/adapters/github":           true,
	modulePath + "/internal/adaptation/adapters/githubreview": true,
}

// Cross-application orchestration is exceptional and explicit. Most use-case
// packages own one port and must not reach sideways into another use case.
var allowedApplicationDependencies = map[string]map[string]bool{
	modulePath + "/internal/change/workflow": {
		modulePath + "/internal/change/impact": true,
		modulePath + "/internal/change/ingest": true,
	},
	modulePath + "/internal/execution/shadow": {
		modulePath + "/internal/execution/planning": true,
	},
}

// allowedLayerDependencies is the inward-pointing dependency matrix. Rows are
// importers and columns are the layers they may import.
var allowedLayerDependencies = map[architectureLayer]map[architectureLayer]bool{
	domainLayer: {
		domainLayer: true,
	},
	applicationLayer: {
		domainLayer:      true,
		applicationLayer: true,
	},
	adapterLayer: {
		domainLayer:      true,
		applicationLayer: true,
		adapterLayer:     true,
	},
	compositionRootLayer: {
		domainLayer:      true,
		applicationLayer: true,
		adapterLayer:     true,
	},
}

// allowedCapabilityDependencies records the deliberate product dependency
// direction. product-composition is used only by cross-capability adapters and
// command composition roots; it is not an inward domain capability.
var allowedCapabilityDependencies = map[capability]map[capability]bool{
	catalogCapability: {
		catalogCapability: true,
	},
	changeCapability: {
		catalogCapability: true,
		changeCapability:  true,
	},
	selectionCapability: {
		catalogCapability:   true,
		changeCapability:    true,
		selectionCapability: true,
	},
	executionCapability: {
		catalogCapability:   true,
		executionCapability: true,
		selectionCapability: true,
	},
	adaptationCapability: {
		adaptationCapability: true,
		catalogCapability:    true,
		changeCapability:     true,
		selectionCapability:  true,
	},
	productCapability: {
		adaptationCapability: true,
		catalogCapability:    true,
		changeCapability:     true,
		executionCapability:  true,
		selectionCapability:  true,
		productCapability:    true,
	},
}

// TestProductionPackagesAreClassified makes package creation an explicit
// architecture decision instead of silently assigning a layer by directory
// naming convention.
func TestProductionPackagesAreClassified(t *testing.T) {
	t.Parallel()

	packages := discoverProductionPackages(t)
	assertProductionPackagesDeclared(t, packages)
	assertPackagePoliciesCurrent(t, packages)
	assertAdapterPoliciesCurrent(t)
	assertGeneratedContractImportersCurrent(t, packages)
}

func assertProductionPackagesDeclared(t *testing.T, packages map[string]productionPackage) {
	t.Helper()

	for _, importPath := range sortedPackagePaths(packages) {
		if _, ok := productionPackagePolicy[importPath]; !ok {
			t.Errorf("%s package classification violation: %s is production code but has no declared layer and capability; add an explicit entry to productionPackagePolicy, choose the narrowest inward dependency direction, and review the boundary against %s", adrReference, importPath, adrReference)
		}
	}
}

func assertPackagePoliciesCurrent(t *testing.T, packages map[string]productionPackage) {
	t.Helper()

	for _, importPath := range sortedPolicyPaths(productionPackagePolicy) {
		policy := productionPackagePolicy[importPath]
		if _, ok := packages[importPath]; !ok {
			t.Errorf("%s stale package classification: %s is declared but contains no production Go files; remove or correct the policy entry rather than leaving an obsolete boundary", adrReference, importPath)
		}
		if err := validatePackageClassification(importPath, policy); err != nil {
			t.Error(err)
		}
		_, adapterClassified := adapterPolicy[importPath]
		if policy.layer == adapterLayer && !adapterClassified {
			t.Errorf("%s adapter classification violation: %s is an adapter but does not declare whether it is driving, driven, contract-conversion, or an application bridge; classify its authority before adding dependencies", adrReference, importPath)
		}
		if policy.layer != adapterLayer && adapterClassified {
			t.Errorf("%s stale adapter classification: %s is classified as %s rather than adapter; remove or correct its adapterPolicy entry", adrReference, importPath, policy.layer)
		}
	}
}

func assertAdapterPoliciesCurrent(t *testing.T) {
	t.Helper()

	for importPath := range adapterPolicy {
		if _, ok := productionPackagePolicy[importPath]; !ok {
			t.Errorf("%s orphan adapter classification: %s has adapter authority but no production package policy", adrReference, importPath)
		}
	}
}

func assertGeneratedContractImportersCurrent(t *testing.T, packages map[string]productionPackage) {
	t.Helper()

	for importPath := range generatedContractImporters {
		pkg, ok := packages[importPath]
		if !ok {
			t.Errorf("%s stale generated-contract importer: %s is allowlisted but is not a production package", adrReference, importPath)
			continue
		}
		if !importsGeneratedContracts(pkg.imports) {
			t.Errorf("%s stale generated-contract importer: %s is allowlisted but no longer imports generated contracts; remove its authority", adrReference, importPath)
		}
	}
}

// TestHexagonalImportBoundaries verifies every production import against the
// layer and capability matrices and rejects infrastructure in the core.
func TestHexagonalImportBoundaries(t *testing.T) {
	t.Parallel()

	packages := discoverProductionPackages(t)
	for _, sourcePath := range sortedPackagePaths(packages) {
		sourcePolicy, ok := productionPackagePolicy[sourcePath]
		if !ok {
			// The classification test emits the actionable ownership failure.
			continue
		}
		for _, importedPath := range sortedImports(packages[sourcePath].imports) {
			if err := validateImport(sourcePath, sourcePolicy, importedPath); err != nil {
				t.Errorf("%s: %v", packages[sourcePath].files[importedPath], err)
			}
		}
	}
}

type productionPackage struct {
	imports map[string]struct{}
	files   map[string]string
}

func discoverProductionPackages(t *testing.T) map[string]productionPackage {
	t.Helper()

	repositoryRoot := architectureRepositoryRoot(t)
	packages := make(map[string]productionPackage)
	for _, sourceRoot := range []string{"internal", "cmd"} {
		err := walkProductionRoot(repositoryRoot, sourceRoot, packages)
		if err != nil {
			t.Fatalf("discover production packages under %s: %v", sourceRoot, err)
		}
	}

	return packages
}

func walkProductionRoot(repositoryRoot, sourceRoot string, packages map[string]productionPackage) error {
	root := filepath.Join(repositoryRoot, sourceRoot)

	return filepath.WalkDir(root, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if !isProductionGoFile(path, entry) {
			return nil
		}

		return collectProductionFile(repositoryRoot, path, packages)
	})
}

func isProductionGoFile(path string, entry fs.DirEntry) bool {
	return !entry.IsDir() && strings.HasSuffix(path, ".go") && !strings.HasSuffix(path, "_test.go")
}

func collectProductionFile(
	repositoryRoot string,
	path string,
	packages map[string]productionPackage,
) error {
	dependencies, err := parseImports(path)
	if err != nil {
		return err
	}
	importPath, err := packageImportPath(repositoryRoot, path)
	if err != nil {
		return err
	}

	pkg := packages[importPath]
	if pkg.imports == nil {
		pkg.imports = make(map[string]struct{})
		pkg.files = make(map[string]string)
	}
	for _, dependency := range dependencies {
		pkg.imports[dependency] = struct{}{}
		if _, exists := pkg.files[dependency]; !exists {
			pkg.files[dependency] = path
		}
	}
	packages[importPath] = pkg

	return nil
}

func parseImports(path string) ([]string, error) {
	parsed, err := parser.ParseFile(token.NewFileSet(), path, nil, parser.ImportsOnly)
	if err != nil {
		return nil, fmt.Errorf("parse %s: %w", path, err)
	}

	dependencies := make([]string, 0, len(parsed.Imports))
	for _, imported := range parsed.Imports {
		dependency, err := strconv.Unquote(imported.Path.Value)
		if err != nil {
			return nil, fmt.Errorf("decode import in %s: %w", path, err)
		}
		dependencies = append(dependencies, dependency)
	}

	return dependencies, nil
}

func packageImportPath(repositoryRoot, path string) (string, error) {
	relativeDirectory, err := filepath.Rel(repositoryRoot, filepath.Dir(path))
	if err != nil {
		return "", fmt.Errorf("resolve package for %s: %w", path, err)
	}

	return modulePath + "/" + filepath.ToSlash(relativeDirectory), nil
}

func architectureRepositoryRoot(t *testing.T) string {
	t.Helper()
	_, filename, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("locate architecture policy test")
	}

	return filepath.Clean(filepath.Join(filepath.Dir(filename), "..", ".."))
}

func validateImport(sourcePath string, source packagePolicy, importedPath string) error {
	if importedPath == modulePath+"/internal/contracts" {
		if !generatedContractImporters[sourcePath] {
			return fmt.Errorf("%s outward contract dependency violation: %s (%s/%s) imports %s but is not an approved generated-contract importer; keep wire DTO conversion in an explicitly allowlisted contract, CLI, or process-protocol boundary", adrReference, sourcePath, source.capability, source.layer, importedPath)
		}
		return nil
	}
	if target, ok := productionPackagePolicy[importedPath]; ok {
		return validateInternalDependency(sourcePath, source, importedPath, target)
	}
	if strings.HasPrefix(importedPath, modulePath+"/internal/") || strings.HasPrefix(importedPath, modulePath+"/cmd/") {
		return fmt.Errorf("%s imported package classification violation: %s imports unclassified production package %s; classify the target and then follow the allowed dependency matrix required by %s", adrReference, sourcePath, importedPath, adrReference)
	}
	if isGeneratedContractImport(importedPath) {
		if sourcePath != modulePath+"/internal/contracts" {
			return fmt.Errorf("%s outward contract dependency violation: %s (%s/%s) imports %s but only the internal Go binding may read the portable schema assets", adrReference, sourcePath, source.capability, source.layer, importedPath)
		}

		return nil
	}
	if strings.HasPrefix(importedPath, modulePath+"/") {
		return fmt.Errorf("%s unknown repository boundary: %s imports %s, which is neither a classified production package nor the known contracts boundary; classify the package or isolate its representation in an adapter", adrReference, sourcePath, importedPath)
	}
	if source.layer != domainLayer && source.layer != applicationLayer {
		return nil
	}
	if isThirdPartyImport(importedPath) {
		return fmt.Errorf("%s outward infrastructure dependency violation: %s (%s/%s) imports third-party package %s; domain and application packages must express the need through an inward type or consumer-owned port and place the dependency in an adapter", adrReference, sourcePath, source.capability, source.layer, importedPath)
	}
	if isInfrastructureStandardLibrary(importedPath) {
		return fmt.Errorf("%s outward infrastructure dependency violation: %s (%s/%s) imports standard-library infrastructure package %s; move transport, persistence, process, or plugin access to an adapter and expose only a consumer-owned port inward", adrReference, sourcePath, source.capability, source.layer, importedPath)
	}

	return nil
}

func validatePackageClassification(importPath string, policy packagePolicy) error {
	if policy.capability == productCapability && policy.layer != adapterLayer && policy.layer != compositionRootLayer {
		return fmt.Errorf("%s product-composition classification violation: %s assigns product-composition authority to the inward %s layer; only adapters and cmd composition roots may coordinate capabilities, so assign the owning domain capability or move orchestration outward", adrReference, importPath, policy.layer)
	}

	return nil
}

func validateInternalDependency(sourcePath string, source packagePolicy, targetPath string, target packagePolicy) error {
	if !allowedLayerDependencies[source.layer][target.layer] {
		return fmt.Errorf("%s dependency direction violation: %s (%s/%s) imports %s (%s/%s); allowed target layers for %s are %s; move infrastructure outward or depend on a consumer-owned inward port", adrReference, sourcePath, source.capability, source.layer, targetPath, target.capability, target.layer, source.layer, allowedLayerNames(source.layer))
	}
	if source.layer == applicationLayer && target.layer == applicationLayer && !allowedApplicationDependencies[sourcePath][targetPath] {
		return fmt.Errorf("%s application capability isolation violation: %s (%s) imports application package %s (%s); sideways use-case composition must be declared as a deliberate workflow, otherwise depend on inward domain language or compose the capabilities in an adapter", adrReference, sourcePath, source.capability, targetPath, target.capability)
	}
	if source.layer == adapterLayer && target.layer == adapterLayer {
		sourceKind := adapterPolicy[sourcePath]
		targetKind := adapterPolicy[targetPath]
		if !allowedAdapterDependencies[sourceKind][targetKind] {
			return fmt.Errorf("%s adapter authority violation: %s (%s) imports %s (%s); allowed adapter targets for %s adapters are %s; select concrete infrastructure in a cmd composition root and inject it through the application-owned port", adrReference, sourcePath, sourceKind, targetPath, targetKind, sourceKind, allowedAdapterKindNames(sourceKind))
		}
	}
	if targetPath == modulePath+"/internal/processprotocol" {
		if processProtocolUsers[sourcePath] {
			return nil
		}
		return fmt.Errorf("%s adapter utility violation: %s may not import %s; only protocol adapters own process execution", adrReference, sourcePath, targetPath)
	}
	if targetPath == modulePath+"/internal/commandline" {
		if source.layer == compositionRootLayer || commandlineUsers[sourcePath] {
			return nil
		}
		return fmt.Errorf("%s command protocol utility violation: %s may not import %s; only command roots and CLI argument adapters own executable behavior", adrReference, sourcePath, targetPath)
	}
	if targetPath == modulePath+"/internal/githubtransport" {
		if githubTransportUsers[sourcePath] {
			return nil
		}
		return fmt.Errorf("%s adapter utility violation: %s may not import %s; only GitHub adapters own credential transport", adrReference, sourcePath, targetPath)
	}
	if !allowedCapabilityDependencies[source.capability][target.capability] {
		return fmt.Errorf("%s capability dependency violation: %s (%s/%s) imports %s (%s/%s); allowed target capabilities for %s are %s; move orchestration to product composition or extract genuinely shared language inward", adrReference, sourcePath, source.capability, source.layer, targetPath, target.capability, target.layer, source.capability, allowedCapabilityNames(source.capability))
	}

	return nil
}

func isThirdPartyImport(importPath string) bool {
	firstSegment, _, _ := strings.Cut(importPath, "/")
	return strings.Contains(firstSegment, ".")
}

func isGeneratedContractImport(importPath string) bool {
	return importPath == modulePath+"/contracts" || strings.HasPrefix(importPath, modulePath+"/contracts/")
}

func importsGeneratedContracts(imports map[string]struct{}) bool {
	for importPath := range imports {
		if importPath == modulePath+"/internal/contracts" {
			return true
		}
	}

	return false
}

func isInfrastructureStandardLibrary(importPath string) bool {
	// URL and IP address value types are safe core vocabulary; the rest of net
	// grants protocol, resolver, listener, or socket authority.
	if importPath == "net/url" || importPath == "net/netip" {
		return false
	}

	for _, prefix := range []string{
		"crypto/tls",
		"crypto/x509",
		"database/sql",
		"io/fs",
		"net",
		"os",
		"path/filepath",
		"plugin",
		"syscall",
		"unsafe",
	} {
		if importPath == prefix || strings.HasPrefix(importPath, prefix+"/") {
			return true
		}
	}

	return false
}

func allowedLayerNames(source architectureLayer) string {
	var names []string
	for target, allowed := range allowedLayerDependencies[source] {
		if allowed {
			names = append(names, string(target))
		}
	}
	sort.Strings(names)

	return strings.Join(names, ", ")
}

func allowedCapabilityNames(source capability) string {
	var names []string
	for target, allowed := range allowedCapabilityDependencies[source] {
		if allowed {
			names = append(names, string(target))
		}
	}
	sort.Strings(names)

	return strings.Join(names, ", ")
}

func allowedAdapterKindNames(source adapterKind) string {
	var names []string
	for target, allowed := range allowedAdapterDependencies[source] {
		if allowed {
			names = append(names, string(target))
		}
	}
	sort.Strings(names)
	if len(names) == 0 {
		return "none"
	}

	return strings.Join(names, ", ")
}

func sortedPackagePaths(packages map[string]productionPackage) []string {
	paths := make([]string, 0, len(packages))
	for path := range packages {
		paths = append(paths, path)
	}
	sort.Strings(paths)

	return paths
}

func sortedPolicyPaths(policies map[string]packagePolicy) []string {
	paths := make([]string, 0, len(policies))
	for path := range policies {
		paths = append(paths, path)
	}
	sort.Strings(paths)

	return paths
}

func sortedImports(imports map[string]struct{}) []string {
	paths := make([]string, 0, len(imports))
	for path := range imports {
		paths = append(paths, path)
	}
	sort.Strings(paths)

	return paths
}

func TestArchitecturePolicyRejectsOutwardAndUnclassifiedDependencies(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name         string
		source       packagePolicy
		importedPath string
		want         string
	}{
		{
			name:         "domain third-party dependency",
			source:       packagePolicy{domainLayer, catalogCapability},
			importedPath: "example.com/database/client",
			want:         "outward infrastructure dependency violation",
		},
		{
			name:         "application standard-library transport",
			source:       packagePolicy{applicationLayer, selectionCapability},
			importedPath: "net/http",
			want:         "outward infrastructure dependency violation",
		},
		{
			name:         "application generated contract",
			source:       packagePolicy{applicationLayer, executionCapability},
			importedPath: modulePath + "/internal/contracts",
			want:         "outward contract dependency violation",
		},
		{
			name:         "unknown repository package",
			source:       packagePolicy{adapterLayer, catalogCapability},
			importedPath: modulePath + "/new-boundary",
			want:         "unknown repository boundary",
		},
	}

	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			err := validateImport(modulePath+"/internal/example", test.source, test.importedPath)
			if err == nil || !strings.Contains(err.Error(), test.want) || !strings.Contains(err.Error(), "ADR-0006") {
				t.Fatalf("validateImport() error = %v, want an actionable %q error referencing ADR-0006", err, test.want)
			}
		})
	}
}

func TestProductCompositionCapabilityStaysOutsideCore(t *testing.T) {
	t.Parallel()

	for _, layer := range []architectureLayer{domainLayer, applicationLayer} {
		layer := layer
		t.Run(string(layer), func(t *testing.T) {
			t.Parallel()

			err := validatePackageClassification(
				modulePath+"/internal/example",
				packagePolicy{layer: layer, capability: productCapability},
			)
			if err == nil || !strings.Contains(err.Error(), "product-composition classification violation") || !strings.Contains(err.Error(), "move orchestration outward") {
				t.Fatalf("validatePackageClassification() error = %v, want actionable product-composition rejection", err)
			}
		})
	}
}

func TestGeneratedContractsRequireExplicitImporter(t *testing.T) {
	t.Parallel()

	sourcePath := modulePath + "/internal/postgres"
	err := validateImport(
		sourcePath,
		productionPackagePolicy[sourcePath],
		modulePath+"/internal/contracts",
	)
	if err == nil || !strings.Contains(err.Error(), "outward contract dependency violation") || !strings.Contains(err.Error(), "not an approved generated-contract importer") {
		t.Fatalf("validateImport() error = %v, want explicit generated-contract importer rejection", err)
	}
}

func TestPortableSchemaAssetsAreOnlyImportedByInternalBinding(t *testing.T) {
	t.Parallel()

	sourcePath := modulePath + "/internal/adaptation/adapters/contract"
	err := validateImport(sourcePath, productionPackagePolicy[sourcePath], modulePath+"/contracts")
	if err == nil || !strings.Contains(err.Error(), "only the internal Go binding") {
		t.Fatalf("validateImport() error = %v, want portable schema asset boundary rejection", err)
	}
	if err := validateImport(modulePath+"/internal/contracts", productionPackagePolicy[modulePath+"/internal/contracts"], modulePath+"/contracts"); err != nil {
		t.Fatalf("internal Go binding may import schema assets: %v", err)
	}
}

func TestPortableContractGoSurfaceStaysAssetOnly(t *testing.T) {
	t.Parallel()

	files, err := filepath.Glob(filepath.Join(architectureRepositoryRoot(t), "contracts", "*.go"))
	if err != nil {
		t.Fatalf("list root Go contract files: %v", err)
	}
	for _, file := range files {
		name := filepath.Base(file)
		if strings.HasSuffix(name, "_test.go") {
			continue
		}
		if name != "assets.go" {
			t.Errorf("root contracts Go package unexpectedly contains %s; keep DTOs under internal/contracts", name)
		}
	}
}

func TestArchitecturePolicyRejectsInvalidInternalDirections(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		source packagePolicy
		target packagePolicy
		want   string
	}{
		{
			name:   "domain imports adapter",
			source: packagePolicy{domainLayer, catalogCapability},
			target: packagePolicy{adapterLayer, catalogCapability},
			want:   "dependency direction violation",
		},
		{
			name:   "application imports an undeclared peer use case",
			source: packagePolicy{applicationLayer, selectionCapability},
			target: packagePolicy{applicationLayer, catalogCapability},
			want:   "application capability isolation violation",
		},
		{
			name:   "catalog points to change",
			source: packagePolicy{domainLayer, catalogCapability},
			target: packagePolicy{domainLayer, changeCapability},
			want:   "capability dependency violation",
		},
	}

	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			err := validateInternalDependency("source", test.source, "target", test.target)
			if err == nil || !strings.Contains(err.Error(), test.want) || !strings.Contains(err.Error(), "ADR-0006") {
				t.Fatalf("validateInternalDependency() error = %v, want an actionable %q error referencing ADR-0006", err, test.want)
			}
		})
	}
}

func TestArchitecturePolicyRejectsDrivingAdapterInfrastructureSelection(t *testing.T) {
	t.Parallel()

	sourcePath := modulePath + "/internal/selection/adapters/cli/selectioncli"
	targetPath := modulePath + "/internal/postgres"
	err := validateInternalDependency(
		sourcePath,
		productionPackagePolicy[sourcePath],
		targetPath,
		productionPackagePolicy[targetPath],
	)
	if err == nil || !strings.Contains(err.Error(), "adapter authority violation") || !strings.Contains(err.Error(), "composition root") {
		t.Fatalf("validateInternalDependency() error = %v, want actionable adapter authority violation", err)
	}
}

func TestSharedAdapterUtilitiesHaveExactImporters(t *testing.T) {
	t.Parallel()
	for _, test := range []struct {
		name   string
		source string
		target string
		allow  bool
	}{
		{"execution process adapter", modulePath + "/internal/execution/adapters/processadapter", modulePath + "/internal/processprotocol", true},
		{"adaptation process adapter", modulePath + "/internal/adaptation/adapters/processadapter", modulePath + "/internal/processprotocol", true},
		{"validation process adapter", modulePath + "/internal/adaptation/adapters/validationprocessadapter", modulePath + "/internal/processprotocol", true},
		{"change GitHub adapter", modulePath + "/internal/change/adapters/github", modulePath + "/internal/githubtransport", true},
		{"review GitHub adapter", modulePath + "/internal/adaptation/adapters/githubreview", modulePath + "/internal/githubtransport", true},
		{"PostgreSQL cannot invoke process", modulePath + "/internal/postgres", modulePath + "/internal/processprotocol", false},
		{"PostgreSQL cannot use GitHub transport", modulePath + "/internal/postgres", modulePath + "/internal/githubtransport", false},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			err := validateInternalDependency(test.source, productionPackagePolicy[test.source], test.target, productionPackagePolicy[test.target])
			if test.allow && err != nil || !test.allow && (err == nil || !strings.Contains(err.Error(), "adapter utility violation")) {
				t.Fatalf("validateInternalDependency(%s, %s) = %v; allow = %v", test.source, test.target, err, test.allow)
			}
		})
	}
}

func TestSelectionImpactProjectionBoundary(t *testing.T) {
	t.Parallel()

	packages := discoverProductionPackages(t)
	changePath := modulePath + "/internal/change"
	bridgePath := modulePath + "/internal/selection/adapters/impactreader"
	// The domain still uses the shared change reference and the v1 contract
	// converter still translates it. Only the impact bridge may interpret the
	// producer's assessment; application policy has no change dependency.
	approvedChangeImporters := map[string]bool{
		modulePath + "/internal/selection":                   true,
		modulePath + "/internal/selection/adapters/contract": true,
		bridgePath: true,
	}
	for _, path := range sortedPackagePaths(packages) {
		if productionPackagePolicy[path].capability != selectionCapability {
			continue
		}
		_, importsChange := packages[path].imports[changePath]
		if importsChange != approvedChangeImporters[path] {
			t.Errorf("%s change dependency = %t; allowed = %t; keep impact interpretation in the bridge", path, importsChange, approvedChangeImporters[path])
		}
	}
	if adapterPolicy[bridgePath] != bridgeAdapter {
		t.Fatalf("%s must remain a classified application bridge", bridgePath)
	}
}
