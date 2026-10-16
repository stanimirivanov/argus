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

	impactDocument, err := contractadapter.ExportCapabilityImpactV1(change.CapabilityImpact{
		APIVersion:      change.ImpactAPIVersion,
		AnalyzerVersion: change.OpenAPIAnalyzerVersion,
		Change:          set.Reference(),
		Status:          change.ImpactComplete,
	})
	if err != nil {
		t.Fatalf("export empty capability impact: %v", err)
	}
	if impactDocument.Documents == nil || impactDocument.Warnings == nil {
		t.Fatalf("empty arrays encoded as null: %#v", impactDocument)
	}
	composite := change.CapabilityImpact{
		APIVersion: change.CompositeImpactAPIVersion, AnalyzerVersion: change.CompositeAnalyzerVersion,
		Change: set.Reference(), Status: change.ImpactComplete,
		SemanticStatus: change.ImpactComplete, FileStatus: change.ImpactComplete,
		Files: []change.FileImpact{{Path: "api/openapi.yaml", Kind: change.KindModified,
			Coverage: change.FileCoverageComponent, Capabilities: []string{"create-order"},
			ComponentRoots: []string{"api"}}},
	}
	v2, err := contractadapter.ExportCapabilityImpactV2(composite)
	if err != nil || v2.APIVersion != change.CompositeImpactAPIVersion || len(v2.Files) != 1 {
		t.Fatalf("export v2: document=%+v err=%v", v2, err)
	}
	restored, err := contractadapter.ImportCapabilityImpactV2(v2)
	if err != nil || restored.APIVersion != composite.APIVersion || restored.Change != composite.Change ||
		len(restored.Files) != 1 || restored.Files[0].Coverage != change.FileCoverageComponent {
		t.Fatalf("import v2: impact=%+v err=%v", restored, err)
	}
	legacy, err := contractadapter.ProjectCapabilityImpactV1(composite)
	if err != nil || legacy.APIVersion != change.ImpactAPIVersion || legacy.Status != string(change.ImpactPartial) {
		t.Fatalf("conservative v1 projection: document=%+v err=%v", legacy, err)
	}
}
