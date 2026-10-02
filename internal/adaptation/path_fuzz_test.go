package adaptation

import (
	"path"
	"path/filepath"
	"strings"
	"testing"
)

// FuzzSourcePath keeps accepted source paths portable and repository-relative.
func FuzzSourcePath(f *testing.F) {
	for _, seed := range []string{"tests/orders.spec.ts", "../escape", "/absolute", `C:/escape`, "file:stream", `nested\file`, "a/../b", "a//b", "a\x00b"} {
		f.Add(seed)
	}
	f.Fuzz(func(t *testing.T, candidate string) {
		if len(candidate) > MaxSourcePathLength+1 {
			t.Skip()
		}
		if err := ValidateSourcePath(candidate); err != nil {
			return
		}
		if candidate == "." || candidate == "" || strings.HasPrefix(candidate, "../") ||
			path.IsAbs(candidate) || filepath.IsAbs(filepath.FromSlash(candidate)) ||
			filepath.VolumeName(filepath.FromSlash(candidate)) != "" || strings.ContainsAny(candidate, "\\:\x00") ||
			candidate != path.Clean(candidate) {
			t.Fatalf("accepted unsafe repository path %q", candidate)
		}
	})
}
