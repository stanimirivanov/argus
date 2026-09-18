package change_test

import (
	"errors"
	"reflect"
	"testing"
	"time"

	"github.com/stanimirivanov/argus/internal/catalog"
	"github.com/stanimirivanov/argus/internal/change"
)

func TestCanonicalSetOrdersFilesWithoutMutatingInput(t *testing.T) {
	t.Parallel()

	set := validSet()
	set.Files = []change.File{
		validFile("z/file.go"),
		validFile("a/file.go"),
	}
	original := append([]change.File(nil), set.Files...)

	canonical := change.CanonicalSet(set)
	if canonical.Files[0].Path != "a/file.go" || !reflect.DeepEqual(set.Files, original) {
		t.Fatalf("canonicalization mutated or failed to order files: %#v", canonical.Files)
	}
}

func TestValidateSetRejectsAmbiguousPartialEvidence(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		mutate func(*change.Set)
	}{
		{name: "equal revisions", mutate: func(set *change.Set) { set.HeadRevision = set.BaseRevision }},
		{name: "duplicate path", mutate: func(set *change.Set) { set.Files = append(set.Files, set.Files[0]) }},
		{name: "relative escape", mutate: func(set *change.Set) { set.Files[0].Path = "../secret" }},
		{name: "Windows drive path", mutate: func(set *change.Set) { set.Files[0].Path = "C:/secret" }},
		{name: "missing rename source", mutate: func(set *change.Set) { set.Files[0].Kind = change.KindRenamed }},
		{name: "patch status mismatch", mutate: func(set *change.Set) { set.Files[0].PatchStatus = change.PatchUnavailable }},
	}

	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			set := validSet()
			test.mutate(&set)
			if err := change.ValidateSet(set); !errors.Is(err, change.ErrInvalid) {
				t.Fatalf("validate = %v, want ErrInvalid", err)
			}
		})
	}
}

func validSet() change.Set {
	patch := "@@ -1 +1 @@\n-old\n+new"

	return change.Set{
		APIVersion: change.SetAPIVersion,
		SourceRepository: catalog.Repository{
			Identity: catalog.RepositoryIdentity{
				Provider:             catalog.ProviderGitHub,
				Host:                 "github.com",
				ProviderRepositoryID: "1296269",
			},
			Owner: "octocat",
			Name:  "hello-world",
		},
		PullRequestNumber: 42,
		BaseRevision: catalog.Revision{
			Algorithm: catalog.RevisionGitSHA1,
			Digest:    "0123456789abcdef0123456789abcdef01234567",
		},
		HeadRevision: catalog.Revision{
			Algorithm: catalog.RevisionGitSHA1,
			Digest:    "89abcdef0123456789abcdef0123456789abcdef",
		},
		ObservedAt: time.Date(2026, 9, 18, 9, 30, 0, 0, time.UTC),
		Trigger: change.Trigger{
			Provider:   catalog.ProviderGitHub,
			DeliveryID: "delivery-1",
			Event:      "pull_request",
			Action:     "synchronize",
		},
		Files: []change.File{{
			Path:        "api/openapi.yaml",
			Kind:        change.KindModified,
			Additions:   2,
			Deletions:   1,
			Patch:       &patch,
			PatchStatus: change.PatchComplete,
		}},
	}
}

func validFile(path string) change.File {
	patch := "@@ -1 +1 @@\n-old\n+new"
	return change.File{Path: path, Kind: change.KindModified, Patch: &patch, PatchStatus: change.PatchComplete}
}
