// Command performance-evidence exports independently authorized retained metadata.
package main

import (
	"context"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/stanimirivanov/argus/internal/commandline"
	"github.com/stanimirivanov/argus/internal/performance/adapters/cli/evidencecli"
	"github.com/stanimirivanov/argus/internal/performance/adapters/perfeng"
	"github.com/stanimirivanov/argus/internal/performance/adapters/selectionio"
	"github.com/stanimirivanov/argus/internal/performance/requestintent"
	"github.com/stanimirivanov/argus/internal/postgres"
)

var commandSpec = commandline.Spec{Name: "performance-evidence", Role: "read-only scoped evidence export", Synopsis: "performance-evidence -issuer URI -principal-issuer URI -principal-id ID -position N"}

func main() {
	if code, handled := commandline.HandleMeta(os.Args[1:], os.Stdout, os.Stderr, commandSpec); handled {
		os.Exit(code)
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	if err := evidencecli.Run(ctx, os.Args[1:], os.Stdout, export); err != nil {
		os.Exit(commandline.Report(os.Stderr, err))
	}
}

func export(ctx context.Context, query evidencecli.Query) ([]byte, error) {
	raw, err := protectedApproval(os.Getenv("ARGUS_PERFORMANCE_EVIDENCE_APPROVAL_FILE"))
	if err != nil {
		return nil, evidencecli.ErrDenied
	}

	return authorizedExport(ctx, query, raw, postgres.OpenEvidenceRuntime)
}

func authorizedExport(ctx context.Context, query evidencecli.Query, raw []byte, open func(context.Context, string) (*postgres.Runtime, error)) ([]byte, error) {
	approval, err := evidencecli.Authorize(raw, query)
	if err != nil {
		return nil, err
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	runtime, err := open(ctx, os.Getenv("ARGUS_PERFORMANCE_EVIDENCE_DATABASE_URL"))
	if err != nil {
		return nil, err
	}
	defer runtime.Close()
	admit := func(child requestintent.Child, outcome requestintent.Outcome) (requestintent.LifecycleAdmission, error) {
		return perfeng.BindLifecycleAdmission(child.Payload, outcome, query.Scope.Issuer)
	}
	verify := func(ctx context.Context, child requestintent.Child, data requestintent.AnalysisData) (requestintent.AnalysisEvidence, error) {
		return perfeng.RevalidateAnalysis(ctx, child, data, approval.ExpectedReviewer)
	}
	snapshot, err := runtime.Performance().DecisionSnapshot(ctx, query.Scope, query.Position, admit, verify, perfeng.ProjectAnalysisDecision)
	if err != nil {
		return nil, err
	}

	return selectionio.ProjectDecision(ctx, snapshot, query.Scope, approval.IntentFingerprint, approval.RequestSHA256)
}
