package selection

import (
	"fmt"
	"path"
	"strings"

	"github.com/stanimirivanov/argus/internal/change"
)

// BrowserPathProjection is the selection-owned view of a verified provider
// change set. Adapters alone interpret changed-file transport evidence.
type BrowserPathProjection struct {
	Change   change.Reference
	Paths    []string
	Complete bool
}

// ValidateBrowserPathProjection rejects malformed or noncanonical bridge output.
func ValidateBrowserPathProjection(projection BrowserPathProjection) error {
	if err := validateChangeReference(projection.Change); err != nil {
		return err
	}
	if len(projection.Paths) > MaxManifestDecisions || (projection.Complete && len(projection.Paths) == 0) {
		return fmt.Errorf("%w: browser path coverage", ErrInvalid)
	}
	for index, value := range projection.Paths {
		if strings.TrimSpace(value) == "" || len(value) > 1024 ||
			path.Clean(value) != value || value == "." || value == ".." ||
			strings.HasPrefix(value, "/") || strings.HasPrefix(value, "../") ||
			strings.Contains(value, "\\") || (len(value) >= 2 && value[1] == ':') ||
			(index > 0 && projection.Paths[index-1] >= value) {
			return fmt.Errorf("%w: browser path ordering", ErrInvalid)
		}
	}

	return nil
}
