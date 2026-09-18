package github

import (
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/stanimirivanov/argus/internal/change"
)

func TestNormalizeFileBoundsPatchEvidence(t *testing.T) {
	t.Parallel()

	patch := strings.Repeat("a", change.MaxPatchBytes-1) + "é"
	file, consumed, err := normalizeFile(fileResponse{
		Filename: "api/openapi.yaml",
		Status:   "modified",
		Patch:    &patch,
	}, change.MaxPatchBytes)
	if err != nil {
		t.Fatalf("normalize file: %v", err)
	}
	if file.Patch == nil || file.PatchStatus != change.PatchTruncated ||
		len(*file.Patch) > change.MaxPatchBytes || !utf8.ValidString(*file.Patch) ||
		consumed != len(*file.Patch) {
		t.Fatalf("unexpected bounded patch: %#v consumed=%d", file, consumed)
	}

	file, consumed, err = normalizeFile(fileResponse{
		Filename: "api/openapi.yaml",
		Status:   "modified",
		Patch:    &patch,
	}, 0)
	if err != nil || file.Patch != nil || file.PatchStatus != change.PatchBudgetExhausted || consumed != 0 {
		t.Fatalf("unexpected exhausted patch: %#v consumed=%d err=%v", file, consumed, err)
	}
}
