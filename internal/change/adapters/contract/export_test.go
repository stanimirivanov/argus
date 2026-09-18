package contract_test

import (
	"testing"
	"time"

	"github.com/stanimirivanov/argus/internal/catalog"
	"github.com/stanimirivanov/argus/internal/change"
	contractadapter "github.com/stanimirivanov/argus/internal/change/adapters/contract"
)

func TestExportV1ProducesValidatedCanonicalDocument(t *testing.T) {
	t.Parallel()

	patch := "@@ patch"
	set := change.Set{
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
			DeliveryID: "delivery-42",
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

	document, err := contractadapter.ExportV1(set)
	if err != nil {
		t.Fatalf("export v1: %v", err)
	}
	if document.APIVersion != "argus.dev/change-set/v1" ||
		document.Files[0].Path != "api/openapi.yaml" || document.Trigger.DeliveryID != "delivery-42" {
		t.Fatalf("unexpected document: %#v", document)
	}
}
