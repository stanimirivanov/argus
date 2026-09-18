package main

import (
	"time"

	"github.com/stanimirivanov/argus/contracts"
	"github.com/stanimirivanov/argus/internal/catalog"
)

func newImpactEdgePageOutput(page catalog.ImpactEdgePage) contracts.ImpactEdgePageV1 {
	items := make([]contracts.ImpactEdge, len(page.Items))
	for index, edge := range page.Items {
		evidence := make([]contracts.ImpactEvidenceReference, len(edge.Evidence))
		for evidenceIndex, item := range edge.Evidence {
			evidence[evidenceIndex] = newImpactEvidenceReference(item)
		}

		var conflict *contracts.MappingConflict
		if edge.Conflict != nil {
			conflict = &contracts.MappingConflict{
				Kind:                    "contradictory-active-assertions",
				SupportingEvidenceCount: edge.Conflict.SupportingEvidenceCount,
				RefutingEvidenceCount:   edge.Conflict.RefutingEvidenceCount,
			}
		}
		items[index] = contracts.ImpactEdge{
			Capability: contracts.CapabilityReference{
				Key:  edge.Capability.Key,
				Name: edge.Capability.Name,
			},
			TestRepository: newContractRepositoryReference(edge.TestRepository),
			Suite: contracts.TestCatalogSuiteReference{
				Key:     edge.SuiteKey,
				Family:  string(edge.Family),
				Adapter: edge.Adapter,
			},
			Test: contracts.TestCatalogTestReference{
				Key:  edge.TestKey,
				Name: edge.TestName,
			},
			Status:   string(edge.Status),
			Evidence: evidence,
			Conflict: conflict,
		}
	}

	var nextCursor *string
	if page.NextCursor != "" {
		nextCursor = &page.NextCursor
	}

	return contracts.ImpactEdgePageV1{
		APIVersion: contracts.ImpactEdgePageV1APIVersion,
		Snapshot: contracts.CatalogSnapshotReference{
			SourceRepository: newContractRepositoryReference(page.Snapshot.SourceRepository),
			Revision: contracts.RevisionReference{
				Algorithm: string(page.Snapshot.Revision.Algorithm),
				Digest:    page.Snapshot.Revision.Digest,
			},
			DescriptorAPIVersion: page.Snapshot.DescriptorAPIVersion,
		},
		EvaluatedAt: page.EvaluatedAt.UTC().Format(time.RFC3339Nano),
		Items:       items,
		NextCursor:  nextCursor,
	}
}

func newImpactEvidenceReference(item catalog.EvaluatedImpactEvidence) contracts.ImpactEvidenceReference {
	var expiresAt *string
	if item.ExpiresAt != nil {
		formatted := item.ExpiresAt.UTC().Format(time.RFC3339Nano)
		expiresAt = &formatted
	}

	return contracts.ImpactEvidenceReference{
		Producer: contracts.ImpactEvidenceProducer{
			Repository: newContractRepositoryReference(item.Producer.Repository),
			Revision: contracts.RevisionReference{
				Algorithm: string(item.Producer.Revision.Algorithm),
				Digest:    item.Producer.Revision.Digest,
			},
			Adapter: item.Producer.Adapter,
		},
		ObservationKey:        item.ObservationKey,
		Assertion:             string(item.Assertion),
		EvidenceType:          string(item.EvidenceType),
		ConfidenceBasisPoints: item.ConfidenceBasisPoints,
		Rationale:             item.Rationale,
		ObservedAt:            item.ObservedAt.UTC().Format(time.RFC3339Nano),
		ExpiresAt:             expiresAt,
		State:                 string(item.State),
	}
}
