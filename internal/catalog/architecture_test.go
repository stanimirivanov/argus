package catalog_test

import (
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"
)

const catalogImportPath = "github.com/stanimirivanov/argus/internal/catalog"
const changeImportPath = "github.com/stanimirivanov/argus/internal/change"

// TestHexagonalImportBoundaries turns the catalog's dependency direction into
// an executable constraint. It intentionally checks production imports rather
// than package names, so contributors remain free to organize files within a
// capability without weakening the boundary.
func TestHexagonalImportBoundaries(t *testing.T) {
	t.Parallel()

	root := catalogSourceRoot(t)
	changeRoot := filepath.Join(filepath.Dir(root), "change")
	rules := []importRule{
		{
			name:      "shared domain remains dependency free",
			directory: root,
			recursive: false,
			forbidden: []string{
				"github.com/jackc/pgx",
				"github.com/stanimirivanov/argus/contracts",
				catalogImportPath + "/adapters",
				catalogImportPath + "/impact",
				catalogImportPath + "/snapshot",
				catalogImportPath + "/testquery",
			},
		},
		{
			name:      "application capabilities depend inward",
			directory: root,
			recursive: true,
			includeDirectories: []string{
				filepath.Join(root, "impact"),
				filepath.Join(root, "snapshot"),
				filepath.Join(root, "testquery"),
			},
			forbidden: []string{
				"github.com/jackc/pgx",
				"github.com/stanimirivanov/argus/contracts",
				catalogImportPath + "/adapters",
				catalogImportPath + "/impact",
				catalogImportPath + "/snapshot",
				catalogImportPath + "/testquery",
			},
		},
		{
			name:      "driving CLI adapter does not select infrastructure",
			directory: filepath.Join(root, "adapters", "cli"),
			recursive: true,
			forbidden: []string{
				"github.com/jackc/pgx",
				catalogImportPath + "/adapters/postgres",
			},
		},
		{
			name:      "change domain remains dependency free",
			directory: changeRoot,
			recursive: false,
			forbidden: []string{
				"github.com/jackc/pgx",
				"github.com/stanimirivanov/argus/contracts",
				changeImportPath + "/adapters",
				changeImportPath + "/ingest",
			},
		},
		{
			name:      "change ingestion application depends inward",
			directory: filepath.Join(changeRoot, "ingest"),
			recursive: true,
			forbidden: []string{
				"github.com/jackc/pgx",
				"github.com/stanimirivanov/argus/contracts",
				changeImportPath + "/adapters",
			},
		},
	}

	for _, rule := range rules {
		rule := rule
		t.Run(rule.name, func(t *testing.T) {
			t.Parallel()
			assertImportsAllowed(t, rule)
		})
	}
}

type importRule struct {
	name               string
	directory          string
	recursive          bool
	includeDirectories []string
	forbidden          []string
}

func catalogSourceRoot(t *testing.T) string {
	t.Helper()
	_, filename, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("locate catalog architecture test")
	}

	return filepath.Dir(filename)
}

func assertImportsAllowed(t *testing.T, rule importRule) {
	t.Helper()

	err := filepath.WalkDir(rule.directory, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() {
			if path != rule.directory && !rule.recursive {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") ||
			!includedByRule(path, rule.includeDirectories) {
			return nil
		}

		file, err := parser.ParseFile(token.NewFileSet(), path, nil, parser.ImportsOnly)
		if err != nil {
			return err
		}
		for _, imported := range file.Imports {
			assertImportAllowed(t, path, imported, rule.forbidden)
		}

		return nil
	})
	if err != nil {
		t.Fatalf("inspect imports: %v", err)
	}
}

func includedByRule(path string, directories []string) bool {
	if len(directories) == 0 {
		return true
	}
	for _, directory := range directories {
		relative, err := filepath.Rel(directory, path)
		if err == nil && relative != ".." && !strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
			return true
		}
	}

	return false
}

func assertImportAllowed(t *testing.T, filename string, imported *ast.ImportSpec, forbidden []string) {
	t.Helper()
	path, err := strconv.Unquote(imported.Path.Value)
	if err != nil {
		t.Errorf("%s: decode import path: %v", filename, err)
		return
	}
	for _, prefix := range forbidden {
		if path == prefix || strings.HasPrefix(path, prefix+"/") {
			t.Errorf("%s imports forbidden dependency %q", filename, path)
		}
	}
}
