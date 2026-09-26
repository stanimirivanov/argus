package contract

import (
	"fmt"

	"github.com/stanimirivanov/argus/contracts"
	"github.com/stanimirivanov/argus/internal/catalog"
	"github.com/stanimirivanov/argus/internal/execution/planning"
)

// ImportFunctionalAPIExecutionBindingsV1 converts structurally valid reviewed
// configuration into canonical planning input.
func ImportFunctionalAPIExecutionBindingsV1(
	document contracts.FunctionalAPIExecutionBindingsV1,
) (planning.Bindings, error) {
	if err := contracts.ValidateFunctionalAPIExecutionBindingsV1(document); err != nil {
		return planning.Bindings{}, err
	}
	bindings := planning.Bindings{
		APIVersion: document.APIVersion,
		Groups:     make([]planning.Binding, 0, len(document.Groups)),
	}
	for _, group := range document.Groups {
		bindings.Groups = append(bindings.Groups, planning.Binding{
			GroupKey: group.GroupKey, TestRepository: importExecutionRepository(group.TestRepository),
			TestRevision: importExecutionRevision(group.TestRevision), Adapter: group.Adapter,
		})
	}
	bindings = planning.CanonicalBindings(bindings)
	if err := planning.ValidateBindings(bindings); err != nil {
		return planning.Bindings{}, fmt.Errorf("validate execution bindings semantics: %w", err)
	}

	return bindings, nil
}

// ExportFunctionalAPIExecutionPlanV1 converts a validated plan to its public contract.
func ExportFunctionalAPIExecutionPlanV1(
	plan planning.Plan,
) (contracts.FunctionalAPIExecutionPlanV1, error) {
	plan = planning.CanonicalPlan(plan)
	if err := planning.ValidatePlan(plan); err != nil {
		return contracts.FunctionalAPIExecutionPlanV1{}, err
	}
	jobs := make([]contracts.FunctionalAPIExecutionJob, 0, len(plan.Jobs))
	for _, job := range plan.Jobs {
		jobs = append(jobs, contracts.FunctionalAPIExecutionJob{
			GroupKey: job.GroupKey, Stage: string(job.Stage),
			TestRepository: repositoryReference(job.TestRepository),
			TestRevision:   revisionReference(job.TestRevision),
			Adapter:        job.Adapter, TestCount: job.TestCount,
		})
	}
	document := contracts.FunctionalAPIExecutionPlanV1{
		APIVersion: plan.APIVersion,
		Manifest: contracts.ManifestDigestReference{
			APIVersion: plan.Manifest.APIVersion, SHA256: plan.Manifest.SHA256,
		},
		Jobs: jobs,
	}
	if err := contracts.ValidateFunctionalAPIExecutionPlanV1(document); err != nil {
		return contracts.FunctionalAPIExecutionPlanV1{}, fmt.Errorf("export functional API execution plan: %w", err)
	}

	return document, nil
}

func importExecutionRepository(reference contracts.RepositoryReference) catalog.Repository {
	return catalog.Repository{
		Identity: catalog.RepositoryIdentity{
			Provider: catalog.Provider(reference.Provider), Host: reference.Host,
			ProviderRepositoryID: reference.ProviderRepositoryID,
		},
		Owner: reference.Owner, Name: reference.Name,
	}
}

func importExecutionRevision(reference contracts.RevisionReference) catalog.Revision {
	return catalog.Revision{
		Algorithm: catalog.RevisionAlgorithm(reference.Algorithm), Digest: reference.Digest,
	}
}
