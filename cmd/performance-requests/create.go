package main

import (
	"context"
	"os"

	"github.com/stanimirivanov/argus/internal/catalog"
	"github.com/stanimirivanov/argus/internal/change"
	"github.com/stanimirivanov/argus/internal/performance"
	"github.com/stanimirivanov/argus/internal/performance/adapters/cli/requestcli"
	"github.com/stanimirivanov/argus/internal/performance/adapters/selectionio"
	"github.com/stanimirivanov/argus/internal/performance/catalogsync"
	"github.com/stanimirivanov/argus/internal/performance/requestintent"
	"github.com/stanimirivanov/argus/internal/postgres"
)

// Host-controlled configuration must originate in an authenticated owner workflow;
// no submitted flag, policy or review can supply/derive the approved authority.
func createOperation(ctx context.Context, request requestcli.Creation) (requestintent.Intent, bool, error) {
	raw, err := requestcli.ReadCreationFile(os.Getenv("ARGUS_PERFORMANCE_REQUEST_APPROVAL_FILE"))
	if err != nil {
		return requestintent.Intent{}, false, err
	}
	approval, err := selectionio.DecodeRequestApproval(raw)
	if err != nil {
		return requestintent.Intent{}, false, err
	}
	review, err := selectionio.DecodeRequestReview(request.Review)
	if err != nil {
		return requestintent.Intent{}, false, err
	}
	runtime, err := postgres.OpenRuntime(ctx, os.Getenv("ARGUS_DATABASE_URL"))
	if err != nil {
		return requestintent.Intent{}, false, err
	}
	defer runtime.Close()

	return createStored(ctx, request, review, approval, creationStorage{
		find: runtime.Change().FindCapabilityImpact, current: runtime.Performance().Current, create: runtime.Performance().CreateRequest,
	})
}

type creationStorage struct {
	find    func(context.Context, catalog.Provider, string) (change.CapabilityImpact, error)
	current func(context.Context, catalogsync.Scope) (catalogsync.Snapshot, error)
	create  func(context.Context, requestintent.Intent) (bool, error)
}

func createStored(ctx context.Context, request requestcli.Creation, review selectionio.RequestReview, approval selectionio.RequestApproval, store creationStorage) (requestintent.Intent, bool, error) {
	if err := ctx.Err(); err != nil {
		return requestintent.Intent{}, false, err
	}
	impact, err := store.find(ctx, catalog.ProviderGitHub, request.DeliveryID)
	if err != nil {
		return requestintent.Intent{}, false, err
	}
	if impact.Change.Trigger.Provider != catalog.ProviderGitHub || impact.Change.Trigger.DeliveryID != request.DeliveryID {
		return requestintent.Intent{}, false, performance.ErrInvalid
	}
	expected := approval.Expected
	snapshot, err := store.current(ctx, catalogsync.Scope{Issuer: expected.Issuer, PrincipalIssuer: expected.PrincipalIssuer, PrincipalID: expected.PrincipalID})
	if err != nil {
		return requestintent.Intent{}, false, err
	}
	if err := ctx.Err(); err != nil {
		return requestintent.Intent{}, false, err
	}
	evidence, err := selectionio.Inspect(impact, snapshot, expected)
	if err != nil {
		return requestintent.Intent{}, false, err
	}
	compiled, err := selectionio.Compile(evidence, request.Policy, review, approval.Authority, request.Now())
	if err != nil {
		return requestintent.Intent{}, false, err
	}
	intent, err := compiled.Intent()
	if err != nil {
		return requestintent.Intent{}, false, err
	}
	if err := ctx.Err(); err != nil {
		return requestintent.Intent{}, false, err
	}
	created, err := store.create(ctx, intent)
	if err != nil {
		return requestintent.Intent{}, false, err
	}

	return intent, created, nil
}
