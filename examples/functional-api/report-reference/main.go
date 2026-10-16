// Command report-reference evaluates an immutable file-backed campaign after
// the producing processes have exited. CI can also ingest these same attempt
// documents into PostgreSQL without changing their contents.
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"

	"github.com/stanimirivanov/argus/internal/contracts"
	"github.com/stanimirivanov/argus/internal/execution"
	executioncontract "github.com/stanimirivanov/argus/internal/execution/adapters/contract"
	"github.com/stanimirivanov/argus/internal/execution/shadow"
)

func main() {
	planPath := flag.String("plan", "", "execution plan JSON")
	bindingsPath := flag.String("attempt-bindings", "", "explicit attempt bindings JSON")
	attemptDirectory := flag.String("attempt-directory", "", "immutable attempt documents")
	flag.Parse()
	if *planPath == "" || *bindingsPath == "" || *attemptDirectory == "" || flag.NArg() != 0 {
		fail(fmt.Errorf("usage: report-reference -plan FILE -attempt-bindings FILE -attempt-directory DIR"))
	}
	planData, err := os.ReadFile(*planPath)
	if err != nil {
		fail(err)
	}
	planDocument, err := contracts.DecodeFunctionalAPIExecutionPlanV1(planData)
	if err != nil {
		fail(err)
	}
	plan, err := executioncontract.ImportFunctionalAPIExecutionPlanV1(planDocument)
	if err != nil {
		fail(err)
	}
	planDigest, err := executioncontract.PlanSHA256V1(plan)
	if err != nil {
		fail(err)
	}
	bindingsData, err := os.ReadFile(*bindingsPath)
	if err != nil {
		fail(err)
	}
	bindingsDocument, err := contracts.DecodeExecutionPlanAttemptBindingsV1(bindingsData)
	if err != nil {
		fail(err)
	}
	bindings, err := executioncontract.ImportExecutionPlanAttemptBindingsV1(bindingsDocument)
	if err != nil {
		fail(err)
	}
	report, err := shadow.NewService(fileAttempts{directory: *attemptDirectory}).ComparePlan(
		context.Background(), plan, planDigest, bindings)
	if err != nil {
		fail(err)
	}
	document, err := executioncontract.ExportSelectionPlanShadowReportV1(report)
	if err != nil {
		fail(err)
	}
	encoder := json.NewEncoder(os.Stdout)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(document); err != nil {
		fail(err)
	}
}

func fail(err error) {
	fmt.Fprintln(os.Stderr, err)
	os.Exit(1)
}

type fileAttempts struct{ directory string }

func (store fileAttempts) FindExecutionAttempt(_ context.Context, attemptID string) (execution.Attempt, error) {
	if err := execution.ValidateAttemptID(attemptID); err != nil {
		return execution.Attempt{}, err
	}
	data, err := os.ReadFile(filepath.Join(store.directory, attemptID+".json"))
	if err != nil {
		return execution.Attempt{}, err
	}
	document, err := contracts.DecodeExecutionAttemptV1(data)
	if err != nil {
		return execution.Attempt{}, err
	}

	return executioncontract.ImportAttemptV1(document)
}
