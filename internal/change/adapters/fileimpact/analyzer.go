// Package fileimpact composes semantic OpenAPI evidence with explicit catalog
// component roots for changed files. Path proximity alone has no authority.
package fileimpact

import (
	"context"
	"errors"
	"fmt"
	"path"
	"slices"
	"strings"

	"github.com/stanimirivanov/argus/internal/catalog"
	"github.com/stanimirivanov/argus/internal/change"
)

const descriptorVersion = "argus.dev/repository-descriptor/v1"

// SemanticAnalyzer provides the already versioned OpenAPI evidence.
type SemanticAnalyzer interface {
	Analyze(context.Context, change.Set) (change.CapabilityImpact, error)
}

// SnapshotReader reads an immutable descriptor at the change's base revision.
type SnapshotReader interface {
	GetSnapshot(context.Context, catalog.SnapshotKey) (catalog.Snapshot, error)
}

// Analyzer adds explicit file-root evidence without rewriting OpenAPI v1 rows.
type Analyzer struct {
	semantic  SemanticAnalyzer
	snapshots SnapshotReader
}

// NewAnalyzer composes semantic evidence with immutable descriptor mappings.
func NewAnalyzer(semantic SemanticAnalyzer, snapshots SnapshotReader) *Analyzer {
	return &Analyzer{semantic: semantic, snapshots: snapshots}
}

// Analyze retains one file row for every bounded changed file. A missing base
// descriptor produces incomplete evidence; a failed catalog read is an error.
func (analyzer *Analyzer) Analyze(ctx context.Context, set change.Set) (change.CapabilityImpact, error) {
	if analyzer == nil || analyzer.semantic == nil || analyzer.snapshots == nil {
		return change.CapabilityImpact{}, change.ErrUnavailable
	}
	set = change.CanonicalSet(set)
	if err := change.ValidateSet(set); err != nil {
		return change.CapabilityImpact{}, err
	}
	result, err := analyzer.semantic.Analyze(ctx, set)
	if err != nil {
		return change.CapabilityImpact{}, err
	}
	if err := change.ValidateCapabilityImpact(result); err != nil || result.APIVersion != change.ImpactAPIVersion || result.Change != set.Reference() {
		return change.CapabilityImpact{}, fmt.Errorf("%w: invalid semantic impact provenance", change.ErrUnavailable)
	}
	result.SemanticStatus = result.Status
	result.SemanticWarnings = append([]string(nil), result.Warnings...)
	result.FileStatus = change.ImpactComplete
	result.APIVersion = change.CompositeImpactAPIVersion
	result.AnalyzerVersion = change.CompositeAnalyzerVersion

	semanticPaths := documentPaths(result.Documents)
	snapshot, err := analyzer.baseSnapshot(ctx, set, semanticPaths)
	if err != nil {
		return change.CapabilityImpact{}, err
	}
	result.Files, result.FileWarnings = mapChangedFiles(set, snapshot, semanticPaths)
	if len(result.FileWarnings) != 0 {
		result.FileStatus = change.ImpactPartial
	}
	if result.SemanticStatus == change.ImpactPartial || result.FileStatus == change.ImpactPartial {
		result.Status = change.ImpactPartial
	}
	result.Warnings = append(append([]string(nil), result.SemanticWarnings...), result.FileWarnings...)
	result = change.CanonicalCapabilityImpact(result)
	if err := change.ValidateCapabilityImpact(result); err != nil {
		return change.CapabilityImpact{}, fmt.Errorf("normalize file impact: %w", err)
	}

	return result, nil
}

func documentPaths(documents []change.DocumentImpact) map[string]bool {
	paths := make(map[string]bool, len(documents))
	for _, document := range documents {
		paths[document.Path] = true
	}

	return paths
}

func (analyzer *Analyzer) baseSnapshot(ctx context.Context, set change.Set, semanticPaths map[string]bool) (catalog.Snapshot, error) {
	needSnapshot := false
	for _, file := range set.Files {
		if !semanticPaths[file.Path] {
			needSnapshot = true
			break
		}
	}
	if !needSnapshot {
		return catalog.Snapshot{}, nil
	}
	key := catalog.SnapshotKey{Repository: set.SourceRepository.Identity, Revision: set.BaseRevision, APIVersion: descriptorVersion}
	snapshot, err := analyzer.snapshots.GetSnapshot(ctx, key)
	if errors.Is(err, catalog.ErrNotFound) {
		return catalog.Snapshot{}, nil
	}
	if err != nil {
		return catalog.Snapshot{}, fmt.Errorf("load base descriptor: %w", err)
	}
	if snapshot.Repository.Identity != key.Repository || snapshot.Revision != key.Revision || snapshot.APIVersion != key.APIVersion {
		return catalog.Snapshot{}, fmt.Errorf("%w: descriptor provenance mismatch", change.ErrUnavailable)
	}
	if err := validateSnapshotMappings(snapshot); err != nil {
		return catalog.Snapshot{}, err
	}

	return snapshot, nil
}

func mapChangedFiles(set change.Set, snapshot catalog.Snapshot, semanticPaths map[string]bool) ([]change.FileImpact, []string) {
	files := make([]change.FileImpact, 0, len(set.Files))
	unmapped := false
	for _, file := range set.Files {
		observation := change.FileImpact{Path: file.Path, PreviousPath: file.PreviousPath, Kind: file.Kind}
		if semanticPaths[file.Path] {
			observation.Coverage = change.FileCoverageOpenAPI
		} else {
			observation = mapComponentFile(observation, snapshot.Components)
			unmapped = unmapped || observation.Coverage == change.FileCoverageUnmapped
		}
		files = append(files, observation)
	}
	var warnings []string
	if unmapped {
		warnings = append(warnings, "some changed paths have missing or bounded component mapping evidence")
	}
	if len(set.Files) == 0 {
		warnings = append(warnings, "no changed-file evidence was available")
	}
	if set.FilesTruncated {
		warnings = append(warnings, "changed-file evidence was truncated")
	}

	return files, warnings
}

func mapComponentFile(file change.FileImpact, components []catalog.Component) change.FileImpact {
	file.Coverage = change.FileCoverageComponent
	for _, filePath := range filePaths(change.File{Path: file.Path, PreviousPath: file.PreviousPath}) {
		matched := false
		for _, component := range components {
			if filePath != component.Root && !strings.HasPrefix(filePath, component.Root+"/") {
				continue
			}
			matched = true
			file.ComponentRoots = append(file.ComponentRoots, component.Root)
			file.Capabilities = append(file.Capabilities, component.Capabilities...)
		}
		if !matched {
			file.Coverage = change.FileCoverageUnmapped
			file.UnmappedPaths = append(file.UnmappedPaths, filePath)
		}
	}
	file.ComponentRoots = sortedBounded(file.ComponentRoots)
	file.Capabilities = sortedBounded(file.Capabilities)
	if len(file.ComponentRoots) == change.MaxFileMappings || len(file.Capabilities) == change.MaxFileMappings {
		file.Coverage = change.FileCoverageUnmapped
		file.UnmappedPaths = append(file.UnmappedPaths, file.Path)
	}

	return file
}

func sortedBounded(values []string) []string {
	slices.Sort(values)
	values = slices.Compact(values)
	if len(values) > change.MaxFileMappings {
		return values[:change.MaxFileMappings]
	}

	return values
}

func filePaths(file change.File) []string {
	paths := []string{file.Path}
	if file.PreviousPath != nil {
		paths = append(paths, *file.PreviousPath)
	}

	return paths
}

func validateSnapshotMappings(snapshot catalog.Snapshot) error {
	known := make(map[string]bool, len(snapshot.Capabilities))
	for _, capability := range snapshot.Capabilities {
		known[capability.Key] = true
	}
	for _, component := range snapshot.Components {
		if component.Root == "" || component.Root == "." || len(component.Root) > 1024 || path.Clean(component.Root) != component.Root ||
			strings.HasPrefix(component.Root, "/") || strings.Contains(component.Root, "\\") ||
			component.Root == ".." || strings.HasPrefix(component.Root, "../") || strings.Contains(component.Root, "//") ||
			(len(component.Root) >= 2 && component.Root[1] == ':') ||
			len(component.Capabilities) == 0 {
			return fmt.Errorf("%w: invalid descriptor component root", change.ErrUnavailable)
		}
		for _, capability := range component.Capabilities {
			if !known[capability] || !catalog.IsLocalKey(capability) {
				return fmt.Errorf("%w: invalid descriptor component capability", change.ErrUnavailable)
			}
		}
	}

	return nil
}
