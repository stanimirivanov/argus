package main

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

const (
	documentationPolicy = "CONTRIBUTING.md#documentation"
	decisionPolicy      = "docs/decisions/README.md#naming-and-lifecycle"
	issuePolicy         = "CONTRIBUTING.md#required-issue-structure"
	pullRequestPolicy   = "CONTRIBUTING.md#pull-request-description"
)

type violation struct {
	path    string
	line    int
	rule    string
	message string
	fix     string
	policy  string
}

func (v violation) String() string {
	return fmt.Sprintf(
		"%s:%d: [%s] %s; fix: %s; policy: %s",
		v.path,
		v.line,
		v.rule,
		v.message,
		v.fix,
		v.policy,
	)
}

type repository struct {
	root      string
	documents map[string]*markdownDocument
}

func checkRepository(root string) ([]violation, error) {
	absRoot, err := filepath.Abs(root)
	if err != nil {
		return nil, fmt.Errorf("resolve repository root: %w", err)
	}

	repo := repository{
		root:      absRoot,
		documents: make(map[string]*markdownDocument),
	}
	if err := repo.loadDocuments(); err != nil {
		return nil, err
	}

	var violations []violation
	violations = append(violations, repo.checkMarkdownLinks()...)
	violations = append(violations, repo.checkDocumentSummaries()...)
	violations = append(violations, repo.checkDecisions()...)
	violations = append(violations, repo.checkTemplates()...)
	sortViolations(violations)

	return violations, nil
}

func (r *repository) loadDocuments() error {
	err := filepath.WalkDir(r.root, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() && shouldSkipDirectory(entry.Name()) && path != r.root {
			return filepath.SkipDir
		}
		if entry.IsDir() || !strings.EqualFold(filepath.Ext(entry.Name()), ".md") {
			return nil
		}

		relativePath, err := filepath.Rel(r.root, path)
		if err != nil {
			return fmt.Errorf("make %q relative to repository root: %w", path, err)
		}
		relativePath = filepath.ToSlash(relativePath)

		content, err := os.ReadFile(path)
		if err != nil {
			return fmt.Errorf("read %s: %w", relativePath, err)
		}
		r.documents[relativePath] = parseMarkdown(relativePath, string(content))

		return nil
	})
	if err != nil {
		return fmt.Errorf("discover Markdown documents: %w", err)
	}

	return nil
}

func shouldSkipDirectory(name string) bool {
	switch name {
	case ".git", ".idea", ".local", ".ruff_cache", ".venv", "bin", "node_modules", "testdata", "vendor":
		return true
	default:
		return false
	}
}

func sortViolations(violations []violation) {
	sort.Slice(violations, func(left, right int) bool {
		if violations[left].path != violations[right].path {
			return violations[left].path < violations[right].path
		}
		if violations[left].line != violations[right].line {
			return violations[left].line < violations[right].line
		}
		if violations[left].rule != violations[right].rule {
			return violations[left].rule < violations[right].rule
		}

		return violations[left].message < violations[right].message
	})
}

func (r *repository) document(path string) (*markdownDocument, bool) {
	document, ok := r.documents[filepath.ToSlash(path)]

	return document, ok
}
