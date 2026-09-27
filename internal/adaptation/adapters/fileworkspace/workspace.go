// Package fileworkspace provides guarded access to a disposable test checkout.
package fileworkspace

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/stanimirivanov/argus/internal/adaptation"
)

// Workspace reads and temporarily writes regular files below one resolved root.
type Workspace struct {
	root string
}

// Open resolves an explicitly supplied disposable checkout root.
func Open(root string) (*Workspace, error) {
	if strings.TrimSpace(root) == "" {
		return nil, fmt.Errorf("%w: workspace root", adaptation.ErrInvalid)
	}
	absolute, err := filepath.Abs(root)
	if err != nil {
		return nil, fmt.Errorf("resolve workspace root: %w", err)
	}
	resolved, err := filepath.EvalSymlinks(absolute)
	if err != nil {
		return nil, fmt.Errorf("resolve workspace links: %w", err)
	}
	info, err := os.Stat(resolved)
	if err != nil {
		return nil, fmt.Errorf("inspect workspace root: %w", err)
	}
	if !info.IsDir() {
		return nil, fmt.Errorf("%w: workspace root is not a directory", adaptation.ErrInvalid)
	}

	return &Workspace{root: resolved}, nil
}

// Root returns the resolved working directory for the validation process.
func (workspace *Workspace) Root() string {
	if workspace == nil {
		return ""
	}

	return workspace.root
}

// Read returns one regular source file after containment and symlink checks.
func (workspace *Workspace) Read(ctx context.Context, path string) ([]byte, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	target, _, err := workspace.resolve(path)
	if err != nil {
		return nil, err
	}
	data, err := readBounded(target)
	if err != nil {
		return nil, fmt.Errorf("read workspace source: %w", err)
	}

	return data, nil
}

// WriteIfUnchanged replaces source bytes only when the current preimage digest
// matches the application service's expectation.
func (workspace *Workspace) WriteIfUnchanged(
	ctx context.Context,
	path string,
	expectedSHA256 string,
	data []byte,
) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	target, mode, err := workspace.resolve(path)
	if err != nil {
		return err
	}
	if len(data) > adaptation.MaxSourceBytes {
		return fmt.Errorf("%w: workspace source exceeds validation bound", adaptation.ErrInvalid)
	}
	current, err := readBounded(target)
	if err != nil {
		return fmt.Errorf("read workspace preimage: %w", err)
	}
	if digest(current) != expectedSHA256 {
		return fmt.Errorf("%w: workspace source changed", adaptation.ErrInvalid)
	}
	if err := os.WriteFile(target, data, mode); err != nil {
		return fmt.Errorf("write workspace source: %w", err)
	}

	return nil
}

// Restore writes trusted original bytes after rechecking path containment.
func (workspace *Workspace) Restore(ctx context.Context, path string, data []byte) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if len(data) > adaptation.MaxSourceBytes {
		return fmt.Errorf("%w: restoration source exceeds validation bound", adaptation.ErrInvalid)
	}
	target, mode, err := workspace.resolve(path)
	if err != nil {
		return err
	}
	if err := os.WriteFile(target, data, mode); err != nil {
		return fmt.Errorf("restore workspace source: %w", err)
	}

	return nil
}

func (workspace *Workspace) resolve(path string) (string, os.FileMode, error) {
	if workspace == nil || workspace.root == "" {
		return "", 0, adaptation.ErrUnavailable
	}
	if err := adaptation.ValidateSourcePath(path); err != nil {
		return "", 0, err
	}
	target := filepath.Join(workspace.root, filepath.FromSlash(path))
	resolved, err := filepath.EvalSymlinks(target)
	if err != nil {
		return "", 0, fmt.Errorf("resolve workspace source: %w", err)
	}
	relative, err := filepath.Rel(workspace.root, resolved)
	if err != nil || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
		return "", 0, fmt.Errorf("%w: source escapes workspace", adaptation.ErrInvalid)
	}
	info, err := os.Stat(resolved)
	if err != nil {
		return "", 0, fmt.Errorf("inspect workspace source: %w", err)
	}
	if !info.Mode().IsRegular() {
		return "", 0, fmt.Errorf("%w: source is not a regular file", adaptation.ErrInvalid)
	}

	return resolved, info.Mode().Perm(), nil
}

func digest(data []byte) string {
	sum := sha256.Sum256(data)

	return hex.EncodeToString(sum[:])
}

func readBounded(path string) ([]byte, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	data, readErr := io.ReadAll(io.LimitReader(file, adaptation.MaxSourceBytes+1))
	closeErr := file.Close()
	if readErr != nil {
		return nil, readErr
	}
	if closeErr != nil {
		return nil, closeErr
	}
	if len(data) > adaptation.MaxSourceBytes {
		return nil, fmt.Errorf("%w: workspace source exceeds validation bound", adaptation.ErrInvalid)
	}

	return data, nil
}
