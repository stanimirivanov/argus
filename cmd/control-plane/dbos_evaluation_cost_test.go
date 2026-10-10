//go:build dbose2e

package main

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	dbosgo "github.com/dbos-inc/dbos-transact-golang/dbos"
	"github.com/jackc/pgx/v5"
)

const evaluationSampleCount = 20

type evaluationStorage struct {
	Workflows       int64 `json:"workflows"`
	Steps           int64 `json:"steps"`
	WorkflowBytes   int64 `json:"workflow_row_bytes"`
	CheckpointBytes int64 `json:"step_row_bytes"`
	RelationBytes   int64 `json:"schema_relation_bytes_including_indexes"`
}

type evaluationCost struct {
	Mode               string            `json:"mode"`
	Host               evaluationHost    `json:"host"`
	FirstDelivery      evaluationLatency `json:"first_delivery"`
	ExactRedelivery    evaluationLatency `json:"exact_redelivery"`
	FailedAssessmentNS int64             `json:"failed_assessment_ns"`
	RecoveryNS         int64             `json:"recovery_redelivery_ns"`
	ResolutionReads    int32             `json:"resolution_reads_measured_batch"`
	DocumentReads      int32             `json:"document_reads_measured_batch"`
	RecoveryExtraReads int32             `json:"recovery_extra_resolution_reads"`
	BeforeRetention    evaluationStorage `json:"before_retention"`
	AfterRetention     evaluationStorage `json:"after_retention"`
	AfterRedelivery    evaluationStorage `json:"after_cleanup_redelivery"`
	DeletedWorkflows   int               `json:"deleted_terminal_workflows"`
	EvidenceUnchanged  bool              `json:"cleanup_preserved_all_argus_evidence"`
	PendingRetained    bool              `json:"pending_workflow_retained"`
	RetentionTested    bool              `json:"retention_tested"`
}

// TestDBOSEvaluationCost compares the real synchronous and DBOS compositions
// with the same serial workload on separate fresh, isolated databases. Timing
// is the subject, not a pass/fail oracle: invariants are asserted, not budgets.
func TestDBOSEvaluationCost(t *testing.T) {
	var reports []evaluationCost
	for _, durable := range []bool{false, true} {
		name := "synchronous"
		if durable {
			name = "dbos-1.5.0"
		}
		t.Run(name, func(t *testing.T) {
			reports = append(reports, measureDBOSEvaluationCost(t, durable, name))
		})
	}
	if t.Failed() {
		return
	}
	retainDBOSEvaluationReport(t, "cost", struct {
		SampleCount int              `json:"sample_count_per_mode"`
		Percentile  string           `json:"percentile_method"`
		Results     []evaluationCost `json:"results"`
		Verdict     string           `json:"production_adoption_verdict"`
	}{evaluationSampleCount, "nearest rank; raw nanosecond samples retained; warmup excluded", reports, "defer"})
}

func measureDBOSEvaluationCost(t *testing.T, durable bool, name string) evaluationCost {
	t.Helper()
	harness := newWebhookEvaluationHarness(t, durable)
	harness.provider.releaseFirstDocument()
	report := evaluationCost{Mode: name, Host: evaluationHostInfo(t, harness)}
	body := webhookPayload(t, "synchronize")
	// Both paths warm the parser, connections and HTTP transport identically.
	assertWebhookStatus(t, harness.postWebhook("cost-warmup", body), http.StatusCreated)
	assertWebhookStatus(t, harness.postWebhook("cost-warmup", body), http.StatusOK)
	pulls, documents := harness.provider.pullRequests.Load(), harness.provider.documents.Load()
	var first, retry []int64
	for index := range evaluationSampleCount {
		id := fmt.Sprintf("cost-delivery-%02d", index)
		first = append(first, timedEvaluationWebhook(t, harness, id, body, http.StatusCreated))
		retry = append(retry, timedEvaluationWebhook(t, harness, id, body, http.StatusOK))
		harness.assertRows(t, id, 1, 1)
		harness.assertCompleteImpact(t, id)
	}
	report.FirstDelivery, report.ExactRedelivery = summarizeEvaluationLatency(first), summarizeEvaluationLatency(retry)
	report.ResolutionReads = harness.provider.pullRequests.Load() - pulls
	report.DocumentReads = harness.provider.documents.Load() - documents
	if report.ResolutionReads != 2*evaluationSampleCount || report.DocumentReads != 2*evaluationSampleCount {
		t.Fatalf("first/retry workload amplified provider I/O: %+v", report)
	}
	harness.provider.failNextDocument.Store(true)
	report.FailedAssessmentNS = timedEvaluationWebhook(t, harness, "cost-failure", body, http.StatusBadGateway)
	harness.assertRows(t, "cost-failure", 1, 0)
	pulls = harness.provider.pullRequests.Load()
	report.RecoveryNS = timedEvaluationWebhook(t, harness, "cost-failure", body, http.StatusOK)
	report.RecoveryExtraReads = harness.provider.pullRequests.Load() - pulls
	if report.RecoveryExtraReads != 0 {
		t.Fatal("assessment redelivery repeated provider resolution")
	}
	harness.assertCompleteImpact(t, "cost-failure")
	if durable {
		measureEvaluationRetention(t, harness, body, &report)
	} else {
		assertDBOSSchemaPresent(t, harness, false)
	}

	return report
}

func startSynchronousEvaluation(t *testing.T, harness *dbosWebhookHarness) {
	t.Helper()
	if err := harness.stop(); err != nil {
		t.Fatalf("stop setup DBOS runtime: %v", err)
	}
	harness.configuration["ARGUS_DBOS_EVALUATION"] = "false"
	application, err := newApplication(t.Context(), func(name string) string { return harness.configuration[name] }, false)
	if err != nil {
		t.Fatalf("start synchronous comparison: %v", err)
	}
	harness.application = application
	harness.listener = httptest.NewServer(application.server.Handler)
}

func timedEvaluationWebhook(t *testing.T, harness *dbosWebhookHarness, id string, body []byte, want int) int64 {
	t.Helper()
	start := time.Now()
	response := harness.postWebhook(id, body)
	elapsed := time.Since(start).Nanoseconds()
	assertWebhookStatus(t, response, want)

	return elapsed
}

func measureEvaluationRetention(t *testing.T, harness *dbosWebhookHarness, body []byte, report *evaluationCost) {
	t.Helper()
	// Keep one genuine active assessment across deletion. The SDK deletion
	// API itself allows active workflow deletion; eligibility is our explicit
	// test-only terminal snapshot, not a claim that its API protects pending work.
	gate := harness.provider.gateNextDocument(t)
	defer gate.release()
	ctx, cancel := context.WithTimeout(t.Context(), 35*time.Second)
	defer cancel()
	response := make(chan webhookResponse, 1)
	go func() { response <- harness.postWebhookContext(ctx, "cost-retained-pending", body) }()
	select {
	case <-gate.reached:
	case <-time.After(20 * time.Second):
		t.Fatal("retention control did not reach the pending assessment")
	}
	version := evaluationApplicationVersion(t, harness)
	// Warmup(2), measured first/retry(40), and failed/recovered attempts(2).
	awaitDBOSVersionStatus(t, harness, version, "SUCCESS", 2*evaluationSampleCount+3)
	assertDBOSVersionStatus(t, harness, version, "ERROR", 1)
	assertDBOSVersionStatus(t, harness, version, "PENDING", 1)
	report.BeforeRetention = evaluationStorageSnapshot(t, harness)
	if report.BeforeRetention.Workflows != 2*evaluationSampleCount+5 {
		t.Fatalf("unexpected workflow amplification: %+v", report.BeforeRetention)
	}
	before := evaluationEvidenceDigest(t, harness)
	ids := evaluationTerminalWorkflowIDs(t, harness)
	if len(ids) != 2*evaluationSampleCount+4 {
		t.Fatalf("terminal retention selection = %d, want %d", len(ids), 2*evaluationSampleCount+4)
	}
	if err := dbosgo.DeleteWorkflows(harness.application.durable, ids); err != nil {
		t.Fatalf("delete selected terminal checkpoints in isolated database: %v", err)
	}
	report.DeletedWorkflows = len(ids)
	report.AfterRetention = evaluationStorageSnapshot(t, harness)
	if evaluationEvidenceDigest(t, harness) != before {
		t.Fatal("checkpoint deletion changed immutable Argus evidence")
	}
	assertDBOSVersionStatus(t, harness, version, "PENDING", 1)
	if report.AfterRetention.Workflows != 1 || report.AfterRetention.Steps != 1 {
		t.Fatalf("terminal deletion left checkpoint rows or removed pending work: %+v", report.AfterRetention)
	}
	report.EvidenceUnchanged, report.PendingRetained, report.RetentionTested = true, true, true
	gate.release()
	assertWebhookStatus(t, awaitWebhook(t, response), http.StatusCreated)
	harness.assertCompleteImpact(t, "cost-retained-pending")
	pulls, documents := harness.provider.pullRequests.Load(), harness.provider.documents.Load()
	before = evaluationEvidenceDigest(t, harness)
	assertWebhookStatus(t, harness.postWebhook("cost-delivery-00", body), http.StatusOK)
	awaitDBOSVersionStatus(t, harness, version, "SUCCESS", 2)
	if evaluationEvidenceDigest(t, harness) != before || harness.provider.pullRequests.Load() != pulls || harness.provider.documents.Load() != documents {
		t.Fatal("redelivery after checkpoint deletion repeated business effects")
	}
	report.AfterRedelivery = evaluationStorageSnapshot(t, harness)
}

func evaluationTerminalWorkflowIDs(t *testing.T, harness *dbosWebhookHarness) []string {
	t.Helper()
	// Retention uses database time, not the client's clock. No global GC is
	// enabled, and no workflow from another application is in this schema.
	rows, err := harness.reader.Query(t.Context(), `
		SELECT workflow_uuid FROM argus_dbos_eval.workflow_status
		WHERE status IN ('SUCCESS', 'ERROR') AND application_name = 'argus-change-evaluation'
		AND created_at <= (extract(epoch FROM clock_timestamp()) * 1000)::bigint
		ORDER BY workflow_uuid
	`)
	if err != nil {
		t.Fatalf("select terminal retention candidates: %v", err)
	}
	ids, err := pgx.CollectRows(rows, pgx.RowTo[string])
	if err != nil {
		t.Fatalf("read terminal retention candidates: %v", err)
	}

	return ids
}

func evaluationStorageSnapshot(t *testing.T, harness *dbosWebhookHarness) evaluationStorage {
	t.Helper()
	var result evaluationStorage
	err := harness.reader.QueryRow(t.Context(), `
		SELECT
		  (SELECT count(*) FROM argus_dbos_eval.workflow_status),
		  (SELECT count(*) FROM argus_dbos_eval.operation_outputs),
		  (SELECT coalesce(sum(pg_column_size(w)), 0) FROM argus_dbos_eval.workflow_status w) +
		  (SELECT coalesce(sum(pg_column_size(i)), 0) FROM argus_dbos_eval.workflow_input i) +
		  (SELECT coalesce(sum(pg_column_size(o)), 0) FROM argus_dbos_eval.workflow_output o),
		  (SELECT coalesce(sum(pg_column_size(s)), 0) FROM argus_dbos_eval.operation_outputs s),
		  (SELECT coalesce(sum(pg_total_relation_size(c.oid)), 0) FROM pg_class c
		   JOIN pg_namespace n ON n.oid = c.relnamespace
		   WHERE n.nspname = 'argus_dbos_eval' AND c.relkind = 'r')
	`).Scan(&result.Workflows, &result.Steps, &result.WorkflowBytes, &result.CheckpointBytes, &result.RelationBytes)
	if err != nil {
		t.Fatalf("measure DBOS checkpoint rows and relation allocation: %v", err)
	}

	return result
}
