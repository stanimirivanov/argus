package contract

import (
	"fmt"
	"time"

	"github.com/stanimirivanov/argus/internal/catalog"
	"github.com/stanimirivanov/argus/internal/change"
	"github.com/stanimirivanov/argus/internal/contracts"
)

// ImportCapabilityImpactV1 converts validated external evidence into the
// canonical domain model used by selection and adaptation.
func ImportCapabilityImpactV1(document contracts.CapabilityImpactV1) (change.CapabilityImpact, error) {
	if err := contracts.ValidateCapabilityImpactV1(document); err != nil {
		return change.CapabilityImpact{}, err
	}
	observedAt, err := time.Parse(time.RFC3339Nano, document.ObservedAt)
	if err != nil {
		return change.CapabilityImpact{}, fmt.Errorf("parse impact observation time: %w", err)
	}
	impact := change.CapabilityImpact{
		APIVersion: document.APIVersion, AnalyzerVersion: document.AnalyzerVersion,
		Change: change.Reference{
			SourceRepository: catalog.Repository{
				Identity: catalog.RepositoryIdentity{
					Provider:             catalog.Provider(document.SourceRepository.Provider),
					Host:                 document.SourceRepository.Host,
					ProviderRepositoryID: document.SourceRepository.ProviderRepositoryID,
				},
				Owner: document.SourceRepository.Owner, Name: document.SourceRepository.Name,
			},
			PullRequestNumber: document.PullRequest.Number,
			BaseRevision: catalog.Revision{
				Algorithm: catalog.RevisionAlgorithm(document.BaseRevision.Algorithm), Digest: document.BaseRevision.Digest,
			},
			HeadRevision: catalog.Revision{
				Algorithm: catalog.RevisionAlgorithm(document.HeadRevision.Algorithm), Digest: document.HeadRevision.Digest,
			},
			ObservedAt: observedAt,
			Trigger: change.Trigger{
				Provider: catalog.Provider(document.Trigger.Provider), DeliveryID: document.Trigger.DeliveryID,
				Event: document.Trigger.Event, Action: document.Trigger.Action,
			},
		},
		Status: change.ImpactStatus(document.Status), Warnings: append([]string{}, document.Warnings...),
	}
	for _, sourceDocument := range document.Documents {
		domainDocument := change.DocumentImpact{
			Path: sourceDocument.Path, PreviousPath: cloneString(sourceDocument.PreviousPath),
			Kind: change.SemanticChangeKind(sourceDocument.Kind), TotalChanges: sourceDocument.TotalChanges,
			BreakingChanges: sourceDocument.BreakingChanges,
		}
		for _, operation := range sourceDocument.Operations {
			domainDocument.Operations = append(domainDocument.Operations, change.OperationImpact{
				Method: operation.Method, Path: operation.Path, OperationID: cloneString(operation.OperationID),
				Kind:           change.SemanticChangeKind(operation.Kind),
				Capabilities:   append([]string{}, operation.Capabilities...),
				PotentialBreak: operation.PotentiallyBreaking,
			})
		}
		impact.Documents = append(impact.Documents, domainDocument)
	}
	impact = change.CanonicalCapabilityImpact(impact)
	if err := change.ValidateCapabilityImpact(impact); err != nil {
		return change.CapabilityImpact{}, fmt.Errorf("validate imported capability impact: %w", err)
	}

	return impact, nil
}

// ImportCapabilityImpactV2 restores composite semantic and file provenance.
func ImportCapabilityImpactV2(document contracts.CapabilityImpactV2) (change.CapabilityImpact, error) {
	if err := contracts.ValidateCapabilityImpactV2(document); err != nil {
		return change.CapabilityImpact{}, err
	}
	semantic := document.CapabilityImpactV1
	semantic.APIVersion = change.ImpactAPIVersion
	semantic.AnalyzerVersion = change.OpenAPIAnalyzerVersion
	semantic.Status = document.SemanticStatus
	semantic.Warnings = append([]string{}, document.SemanticWarnings...)
	impact, err := ImportCapabilityImpactV1(semantic)
	if err != nil {
		return change.CapabilityImpact{}, err
	}
	impact.APIVersion = change.CompositeImpactAPIVersion
	impact.AnalyzerVersion = change.CompositeAnalyzerVersion
	impact.Status = change.ImpactStatus(document.Status)
	impact.SemanticStatus = change.ImpactStatus(document.SemanticStatus)
	impact.FileStatus = change.ImpactStatus(document.FileStatus)
	impact.Warnings = append([]string{}, document.Warnings...)
	impact.SemanticWarnings = append([]string{}, document.SemanticWarnings...)
	impact.FileWarnings = append([]string{}, document.FileWarnings...)
	for _, file := range document.Files {
		impact.Files = append(impact.Files, change.FileImpact{
			Path: file.Path, PreviousPath: cloneString(file.PreviousPath), Kind: change.Kind(file.Kind),
			Coverage: file.Coverage, Capabilities: append([]string{}, file.Capabilities...),
			ComponentRoots: append([]string{}, file.ComponentRoots...), UnmappedPaths: append([]string{}, file.UnmappedPaths...),
		})
	}
	impact = change.CanonicalCapabilityImpact(impact)
	if err := change.ValidateCapabilityImpact(impact); err != nil {
		return change.CapabilityImpact{}, err
	}

	return impact, nil
}

// ExportCapabilityImpactV1 converts validated domain evidence to its public contract.
func ExportCapabilityImpactV1(impact change.CapabilityImpact) (contracts.CapabilityImpactV1, error) {
	impact = change.CanonicalCapabilityImpact(impact)
	if err := change.ValidateCapabilityImpact(impact); err != nil {
		return contracts.CapabilityImpactV1{}, err
	}
	documents := make([]contracts.OpenAPIDocumentImpact, 0, len(impact.Documents))
	for _, document := range impact.Documents {
		operations := make([]contracts.OpenAPIOperationImpact, 0, len(document.Operations))
		for _, operation := range document.Operations {
			operations = append(operations, contracts.OpenAPIOperationImpact{
				Method: operation.Method, Path: operation.Path, OperationID: cloneString(operation.OperationID),
				Kind: string(operation.Kind), Capabilities: append([]string{}, operation.Capabilities...),
				PotentiallyBreaking: operation.PotentialBreak,
			})
		}
		documents = append(documents, contracts.OpenAPIDocumentImpact{
			Path: document.Path, PreviousPath: cloneString(document.PreviousPath), Kind: string(document.Kind),
			TotalChanges: document.TotalChanges, BreakingChanges: document.BreakingChanges, Operations: operations,
		})
	}
	reference := impact.Change
	result := contracts.CapabilityImpactV1{
		APIVersion: impact.APIVersion, AnalyzerVersion: impact.AnalyzerVersion,
		SourceRepository: contracts.RepositoryReference{
			Provider:             string(reference.SourceRepository.Identity.Provider),
			Host:                 reference.SourceRepository.Identity.Host,
			ProviderRepositoryID: reference.SourceRepository.Identity.ProviderRepositoryID,
			Owner:                reference.SourceRepository.Owner, Name: reference.SourceRepository.Name,
		},
		PullRequest: contracts.PullRequestReference{Number: reference.PullRequestNumber},
		BaseRevision: contracts.RevisionReference{
			Algorithm: string(reference.BaseRevision.Algorithm), Digest: reference.BaseRevision.Digest,
		},
		HeadRevision: contracts.RevisionReference{
			Algorithm: string(reference.HeadRevision.Algorithm), Digest: reference.HeadRevision.Digest,
		},
		ObservedAt: reference.ObservedAt.Format(time.RFC3339Nano),
		Trigger: contracts.ChangeTrigger{
			Provider: string(reference.Trigger.Provider), DeliveryID: reference.Trigger.DeliveryID,
			Event: reference.Trigger.Event, Action: reference.Trigger.Action,
		},
		Status: string(impact.Status), Documents: documents, Warnings: append([]string{}, impact.Warnings...),
	}
	if err := contracts.ValidateCapabilityImpactV1(result); err != nil {
		return contracts.CapabilityImpactV1{}, fmt.Errorf("export capability impact v1: %w", err)
	}

	return result, nil
}

// ExportCapabilityImpactV2 exposes changed-file and semantic evidence together
// while keeping the v1 wire meaning unchanged for historical assessments.
func ExportCapabilityImpactV2(impact change.CapabilityImpact) (contracts.CapabilityImpactV2, error) {
	impact = change.CanonicalCapabilityImpact(impact)
	if impact.APIVersion != change.CompositeImpactAPIVersion {
		return contracts.CapabilityImpactV2{}, fmt.Errorf("%w: not a v2 assessment", change.ErrInvalid)
	}
	if err := change.ValidateCapabilityImpact(impact); err != nil {
		return contracts.CapabilityImpactV2{}, err
	}
	semantic := impact
	semantic.APIVersion = change.ImpactAPIVersion
	semantic.AnalyzerVersion = change.OpenAPIAnalyzerVersion
	semantic.Status = impact.SemanticStatus
	semantic.Warnings = append([]string(nil), impact.SemanticWarnings...)
	semantic.Files = nil
	semantic.SemanticStatus, semantic.FileStatus = "", ""
	semantic.SemanticWarnings, semantic.FileWarnings = nil, nil
	base, err := ExportCapabilityImpactV1(semantic)
	if err != nil {
		return contracts.CapabilityImpactV2{}, err
	}
	base.APIVersion = impact.APIVersion
	base.AnalyzerVersion = impact.AnalyzerVersion
	base.Status = string(impact.Status)
	base.Warnings = append([]string{}, impact.Warnings...)
	result := contracts.CapabilityImpactV2{
		CapabilityImpactV1: base, SemanticStatus: string(impact.SemanticStatus), FileStatus: string(impact.FileStatus),
		SemanticWarnings: append([]string{}, impact.SemanticWarnings...), FileWarnings: append([]string{}, impact.FileWarnings...),
		Files: make([]contracts.ChangedFileImpact, 0, len(impact.Files)),
	}
	for _, file := range impact.Files {
		result.Files = append(result.Files, contracts.ChangedFileImpact{
			Path: file.Path, PreviousPath: cloneString(file.PreviousPath), Kind: string(file.Kind),
			Coverage: file.Coverage, Capabilities: append([]string{}, file.Capabilities...),
			ComponentRoots: append([]string{}, file.ComponentRoots...), UnmappedPaths: append([]string{}, file.UnmappedPaths...),
		})
	}
	if err := contracts.ValidateCapabilityImpactV2(result); err != nil {
		return contracts.CapabilityImpactV2{}, fmt.Errorf("export capability impact v2: %w", err)
	}

	return result, nil
}

// ProjectCapabilityImpactV1 keeps the original read contract available for
// v2 assessments. Any incomplete file evidence remains a conservative partial
// result; source-only changes never masquerade as complete semantic API impact.
func ProjectCapabilityImpactV1(impact change.CapabilityImpact) (contracts.CapabilityImpactV1, error) {
	if impact.APIVersion == change.ImpactAPIVersion {
		return ExportCapabilityImpactV1(impact)
	}
	if impact.APIVersion != change.CompositeImpactAPIVersion {
		return contracts.CapabilityImpactV1{}, fmt.Errorf("%w: unsupported impact projection", change.ErrInvalid)
	}
	if err := change.ValidateCapabilityImpact(impact); err != nil {
		return contracts.CapabilityImpactV1{}, err
	}
	semantic := impact
	semantic.APIVersion = change.ImpactAPIVersion
	semantic.AnalyzerVersion = change.OpenAPIAnalyzerVersion
	semantic.Status = impact.SemanticStatus
	semantic.Warnings = append([]string(nil), impact.SemanticWarnings...)
	semantic.Files = nil
	semantic.SemanticStatus, semantic.FileStatus = "", ""
	semantic.SemanticWarnings, semantic.FileWarnings = nil, nil
	if impact.FileStatus == change.ImpactPartial {
		semantic.Status = change.ImpactPartial
		semantic.Warnings = appendLegacyFallbackWarning(semantic.Warnings,
			"incomplete changed-file evidence requires broader execution under v1 consumers")
	}
	for _, file := range impact.Files {
		if file.Coverage != change.FileCoverageOpenAPI {
			semantic.Status = change.ImpactPartial
			semantic.Warnings = appendLegacyFallbackWarning(semantic.Warnings,
				"non-OpenAPI file impact requires broader execution under v1 consumers")
			break
		}
	}
	if len(semantic.Documents) == 0 {
		semantic.Status = change.ImpactPartial
		semantic.Warnings = appendLegacyFallbackWarning(semantic.Warnings,
			"no semantic OpenAPI evidence was retained for this change")
	}

	return ExportCapabilityImpactV1(semantic)
}

func appendLegacyFallbackWarning(warnings []string, warning string) []string {
	warnings = append([]string(nil), warnings...)
	if len(warnings) == change.MaxImpactWarnings {
		warnings = warnings[:len(warnings)-1]
	}

	return append(warnings, warning)
}
