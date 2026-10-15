// Package edgereader adapts evaluated catalog mapping evidence to selection.
package edgereader

import (
	"context"
	"slices"
	"time"

	"github.com/stanimirivanov/argus/internal/catalog"
	"github.com/stanimirivanov/argus/internal/catalog/impact"
	"github.com/stanimirivanov/argus/internal/selection"
	"github.com/stanimirivanov/argus/internal/selection/capabilitymapped"
)

const maxMappings = 50_000

// Reader evaluates bounded immutable evidence at the change observation time.
type Reader struct{ source impact.EdgeReader }

// New constructs the selection mapping adapter.
func New(source impact.EdgeReader) *Reader { return &Reader{source: source} }

// ReadMappings returns every edge for the affected capabilities or fails
// closed when a query, provenance, cursor, or bound is untrustworthy.
func (reader *Reader) ReadMappings(
	ctx context.Context, key catalog.SnapshotKey, evaluatedAt time.Time, capabilities []string,
) ([]capabilitymapped.Mapping, error) {
	if reader == nil || reader.source == nil || !key.Valid() || evaluatedAt.IsZero() {
		return nil, selection.ErrInvalid
	}
	service := impact.NewEdgeService(reader.source)
	result := make([]capabilitymapped.Mapping, 0)
	for _, capability := range capabilities {
		mapped, err := readCapability(ctx, service, key, evaluatedAt, capability, maxMappings-len(result))
		if err != nil {
			return nil, err
		}
		result = append(result, mapped...)
	}

	return result, nil
}

func readCapability(ctx context.Context, service *impact.EdgeService, key catalog.SnapshotKey,
	evaluatedAt time.Time, capability string, remaining int,
) ([]capabilitymapped.Mapping, error) {
	result := make([]capabilitymapped.Mapping, 0)
	cursor := ""
	for {
		page, err := service.List(ctx, impact.EdgeQuery{
			Snapshot: key, EvaluatedAt: evaluatedAt, CapabilityKey: capability,
			PageSize: impact.MaxEdgePageSize, Cursor: cursor,
		})
		if err != nil {
			return nil, err
		}
		if page.Snapshot.Key() != key || !page.EvaluatedAt.Equal(evaluatedAt) {
			return nil, selection.ErrUnavailable
		}
		for _, edge := range page.Items {
			if edge.Capability.Key != capability || len(result) == remaining {
				return nil, selection.ErrUnavailable
			}
			result = append(result, projectMapping(edge, capability))
		}
		if page.NextCursor == "" {
			break
		}
		cursor = page.NextCursor
	}

	return result, nil
}

func projectMapping(edge impact.Edge, capability string) capabilitymapped.Mapping {
	mapping := capabilitymapped.Mapping{
		Capability: capability, Test: edge.Identity().Test, Status: string(edge.Status),
	}
	for _, evidence := range edge.Evidence {
		if evidence.State == impact.EvidenceActive && evidence.Assertion == impact.AssertionSupports {
			mapping.EvidenceKeys = append(mapping.EvidenceKeys, evidence.ObservationKey)
		}
	}
	slices.Sort(mapping.EvidenceKeys)

	return mapping
}

var _ capabilitymapped.MappingReader = (*Reader)(nil)
