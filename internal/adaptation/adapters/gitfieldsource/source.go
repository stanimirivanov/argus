// Package gitfieldsource reads bounded immutable OpenAPI blobs from a reviewed
// local source checkout. It never fetches or mutates provider state.
package gitfieldsource

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"github.com/stanimirivanov/argus/internal/adaptation"
	"github.com/stanimirivanov/argus/internal/processprotocol"
)

// Source binds a caller-supplied source checkout path.
type Source struct{ directory string }

// New validates and retains the checkout path for immutable Git reads.
func New(directory string) (*Source, error) {
	if strings.TrimSpace(directory) == "" {
		return nil, fmt.Errorf("%w: missing source checkout", adaptation.ErrInvalid)
	}
	absolute, err := filepath.Abs(directory)
	if err != nil {
		return nil, err
	}

	return &Source{directory: absolute}, nil
}

// Head reports the checked-out commit so callers can reject a stale checkout.
func (source *Source) Head(ctx context.Context) (string, error) {
	output, err := source.run(ctx, "rev-parse", "HEAD")

	return strings.TrimSpace(string(output)), err
}

// Load reads one file from an exact full Git revision without checking out or
// materializing the tree. Callers validate revisions and repository paths.
func (source *Source) Load(ctx context.Context, revision, path string) ([]byte, error) {
	if source == nil || revision == "" || errInvalidPath(path) {
		return nil, fmt.Errorf("%w: immutable source object", adaptation.ErrInvalid)
	}

	return source.run(ctx, "show", revision+":"+path)
}

func (source *Source) run(ctx context.Context, args ...string) ([]byte, error) {
	if source == nil || source.directory == "" {
		return nil, adaptation.ErrUnavailable
	}
	command := append([]string{"git", "-C", source.directory}, args...)

	return processprotocol.Run(ctx, processprotocol.Options{
		Command: command, Timeout: 30 * time.Second,
	})
}

func errInvalidPath(path string) bool {
	return adaptation.ValidateSourcePath(path) != nil
}
