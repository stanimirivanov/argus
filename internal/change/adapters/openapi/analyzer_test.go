package openapi

import (
	"context"
	"testing"
	"time"

	"github.com/stanimirivanov/argus/internal/catalog"
	"github.com/stanimirivanov/argus/internal/change"
)

func TestAnalyzerMapsReferencedSchemaChangeToCapability(t *testing.T) {
	t.Parallel()
	set := openAPIChangeSet()
	source := &memoryDocumentSource{documents: map[string]string{
		set.BaseRevision.Digest + ":api/openapi.yaml": openAPISpec("string"),
		set.HeadRevision.Digest + ":api/openapi.yaml": openAPISpec("integer"),
	}}

	impact, err := NewAnalyzer(source).Analyze(t.Context(), set)
	if err != nil {
		t.Fatalf("analyze OpenAPI change: %v", err)
	}
	if impact.Status != change.ImpactComplete || len(impact.Documents) != 1 {
		t.Fatalf("impact status/documents = %s/%d, want complete/1", impact.Status, len(impact.Documents))
	}
	document := impact.Documents[0]
	if document.Kind != change.SemanticModified || document.TotalChanges == 0 {
		t.Fatalf("document impact = %+v, want semantic modification", document)
	}
	if len(document.Operations) != 1 {
		t.Fatalf("operation count = %d, want 1", len(document.Operations))
	}
	operation := document.Operations[0]
	if operation.Method != "POST" || operation.Path != "/orders" ||
		len(operation.Capabilities) != 1 || operation.Capabilities[0] != "create-order" {
		t.Fatalf("operation impact = %+v", operation)
	}
	if source.loads != 2 {
		t.Fatalf("document loads = %d, want 2 immutable revisions", source.loads)
	}
}

func TestAnalyzerRetainsUnmappedOperationAsEvidence(t *testing.T) {
	t.Parallel()
	set := openAPIChangeSet()
	source := &memoryDocumentSource{documents: map[string]string{
		set.BaseRevision.Digest + ":api/openapi.yaml": openAPISpecWithoutMapping("string"),
		set.HeadRevision.Digest + ":api/openapi.yaml": openAPISpecWithoutMapping("integer"),
	}}

	impact, err := NewAnalyzer(source).Analyze(t.Context(), set)
	if err != nil {
		t.Fatalf("analyze OpenAPI change: %v", err)
	}
	if len(impact.Documents) != 1 || len(impact.Documents[0].Operations) != 1 {
		t.Fatalf("unexpected impact: %+v", impact)
	}
	if len(impact.Documents[0].Operations[0].Capabilities) != 0 {
		t.Fatalf("unmapped operation was assigned capabilities: %+v", impact.Documents[0].Operations[0])
	}
}

type memoryDocumentSource struct {
	documents map[string]string
	loads     int
}

func (source *memoryDocumentSource) LoadDocument(
	_ context.Context,
	_ catalog.Repository,
	revision catalog.Revision,
	path string,
) ([]byte, error) {
	source.loads++
	return []byte(source.documents[revision.Digest+":"+path]), nil
}

func openAPIChangeSet() change.Set {
	return change.Set{
		APIVersion: change.SetAPIVersion,
		SourceRepository: catalog.Repository{
			Identity: catalog.RepositoryIdentity{
				Provider: catalog.ProviderGitHub, Host: "github.com", ProviderRepositoryID: "42",
			},
			Owner: "acme", Name: "orders",
		},
		PullRequestNumber: 7,
		BaseRevision:      catalog.Revision{Algorithm: catalog.RevisionGitSHA1, Digest: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"},
		HeadRevision:      catalog.Revision{Algorithm: catalog.RevisionGitSHA1, Digest: "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"},
		ObservedAt:        time.Date(2026, 9, 18, 12, 0, 0, 0, time.UTC),
		Trigger: change.Trigger{
			Provider: catalog.ProviderGitHub, DeliveryID: "delivery-1",
			Event: "pull_request", Action: "synchronize",
		},
		Files: []change.File{{
			Path: "api/openapi.yaml", Kind: change.KindModified,
			Additions: 1, Deletions: 1, PatchStatus: change.PatchUnavailable,
		}},
	}
}

func openAPISpec(propertyType string) string {
	return `openapi: 3.1.0
info:
  title: Orders
  version: "1"
paths:
  /orders:
    post:
      operationId: createOrder
      x-argus-capabilities:
        - create-order
      requestBody:
        content:
          application/json:
            schema:
              $ref: "#/components/schemas/CreateOrder"
      responses:
        "201":
          description: Created
components:
  schemas:
    CreateOrder:
      type: object
      properties:
        item:
          type: ` + propertyType + "\n"
}

func openAPISpecWithoutMapping(propertyType string) string {
	return `openapi: 3.1.0
info:
  title: Orders
  version: "1"
paths:
  /orders:
    post:
      operationId: createOrder
      requestBody:
        content:
          application/json:
            schema:
              $ref: "#/components/schemas/CreateOrder"
      responses:
        "201":
          description: Created
components:
  schemas:
    CreateOrder:
      type: object
      properties:
        item:
          type: ` + propertyType + "\n"
}
