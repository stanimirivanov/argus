package proposalcli

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/stanimirivanov/argus/internal/adaptation"
	"github.com/stanimirivanov/argus/internal/adaptation/fieldrepair"
	"github.com/stanimirivanov/argus/internal/catalog"
	"github.com/stanimirivanov/argus/internal/change"
)

type fieldSourceFake struct {
	head  string
	loads int
}

func (source *fieldSourceFake) Head(context.Context) (string, error) {
	return source.head, nil
}

func (source *fieldSourceFake) Load(_ context.Context, _, _ string) ([]byte, error) {
	source.loads++
	return []byte("immutable OpenAPI bytes"), nil
}

func TestFindFieldProofRequiresMatchingHeadAndOneRelevantOperation(t *testing.T) {
	t.Parallel()
	operationID := "createOrder"
	impact := change.CapabilityImpact{
		Status: change.ImpactComplete,
		Change: change.Reference{
			BaseRevision: catalog.Revision{Digest: strings.Repeat("1", 40)},
			HeadRevision: catalog.Revision{Digest: strings.Repeat("2", 40)},
		},
		Documents: []change.DocumentImpact{{
			Path: "api/orders.yaml", Kind: change.SemanticModified,
			Operations: []change.OperationImpact{{
				Method: "POST", Path: "/orders", OperationID: &operationID,
				Kind: change.SemanticModified, Capabilities: []string{"create-order"},
			}},
		}},
	}
	test := adaptation.TestReference{Capabilities: []string{"create-order"}}
	source := &fieldSourceFake{head: impact.Change.HeadRevision.Digest}
	proof := change.FieldRename{DocumentPath: "api/orders.yaml", Method: "POST", Path: "/orders"}
	analyze := func(base, head []byte, path, method, route string) (change.FieldRename, bool) {
		if string(base) != "immutable OpenAPI bytes" || string(head) != "immutable OpenAPI bytes" ||
			path != proof.DocumentPath || method != proof.Method || route != proof.Path {
			t.Fatal("analyzer did not receive exact relevant bytes and operation")
		}

		return proof, true
	}
	found, err := findFieldProof(context.Background(), impact, test, source, analyze)
	if err != nil || found.Path != proof.Path || source.loads != 2 {
		t.Fatalf("field proof: %+v, loads=%d, err=%v", found, source.loads, err)
	}
	source.head = strings.Repeat("3", 40)
	if _, err := findFieldProof(context.Background(), impact, test, source, analyze); !errors.Is(err, adaptation.ErrInvalid) {
		t.Fatalf("expected checkout mismatch, got %v", err)
	}
	source.head = impact.Change.HeadRevision.Digest
	impact.Documents[0].Operations = append(impact.Documents[0].Operations, impact.Documents[0].Operations[0])
	if _, err := findFieldProof(context.Background(), impact, test, source, analyze); !errors.Is(err, fieldrepair.ErrNoAuthoritativeRename) {
		t.Fatalf("expected ambiguous proof abstention, got %v", err)
	}
}
