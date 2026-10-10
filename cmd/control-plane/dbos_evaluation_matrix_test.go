//go:build dbose2e

package main

import (
	"context"
	"net/http"
	"os/exec"
	"strings"
	"testing"
	"time"
)

type compatibilityReport struct {
	Host                       evaluationHost    `json:"host"`
	Builds                     []evaluationBuild `json:"builds"`
	Observations               []string          `json:"observations"`
	EvidenceUnchanged          bool              `json:"old_evidence_unchanged"`
	MixedVersionRollbackSafe   bool              `json:"mixed_version_rollback_safe"`
	ProductionAdoptionVerdict  string            `json:"production_adoption_verdict"`
	CandidateVulnerabilityScan string            `json:"candidate_vulnerability_scan"`
}

// TestDBOSEvaluationCompatibility uses actual executable-derived application
// versions and SDK-owned migrations, not labels or simulated ledger edits.
// The unsupported rollback is an asserted hazard, never a compatibility pass.
func TestDBOSEvaluationCompatibility(t *testing.T) {
	oldBuild := buildDBOSEvaluation(t, false)
	newBuild := buildDBOSEvaluation(t, true)
	assertEvaluationDependencyGraphs(t, oldBuild, newBuild)
	scanEvaluationCandidate(t, newBuild)
	if oldBuild.BinarySHA256 == newBuild.BinarySHA256 {
		t.Fatal("SDK matrix did not produce distinct executables")
	}
	harness := newDBOSWebhookHarness(t)
	if err := harness.stop(); err != nil {
		t.Fatalf("stop in-process setup runtime: %v", err)
	}
	harness.provider.releaseFirstDocument()
	body := webhookPayload(t, "synchronize")
	const deliveryID = "dbos-sdk-old-interrupted"
	gate := harness.provider.gateNextDocument(t)
	defer gate.release()
	oldWorker := startDBOSWorkerExecutable(t, oldBuild.executable, harness.configuration, false)
	requestContext, cancel := context.WithTimeout(t.Context(), 35*time.Second)
	defer cancel()
	result := make(chan webhookResponse, 1)
	go func() {
		result <- harness.postWebhookAt(requestContext, oldWorker.baseURL, deliveryID, body)
	}()
	select {
	case <-gate.reached:
	case <-time.After(20 * time.Second):
		t.Fatal("baseline binary did not reach the assessment crash boundary")
	}
	harness.assertRows(t, deliveryID, 1, 0)
	oldBuild.ApplicationVersion = evaluationApplicationVersion(t, harness)
	oldWorker.kill(t)
	gate.release()
	if response := awaitWebhook(t, result); response.err == nil {
		t.Fatal("crashed baseline binary acknowledged incomplete evidence")
	}
	// Before touching schema, restore the exact old binary and drain it.
	pulls := harness.provider.pullRequests.Load()
	restored := startDBOSWorkerExecutable(t, oldBuild.executable, harness.configuration, false)
	harness.awaitRows(t, deliveryID, 1, 1)
	awaitDBOSVersionStatus(t, harness, oldBuild.ApplicationVersion, "SUCCESS", 1)
	if harness.provider.pullRequests.Load() != pulls {
		t.Fatal("restoring the old binary repeated provider resolution")
	}
	harness.assertCompleteImpact(t, deliveryID)
	restored.kill(t)
	before := evaluationEvidenceDigest(t, harness)
	// A candidate with the reviewed 123 guard refuses 121 before migration.
	assertEvaluationWorkerRefused(t, newBuild, harness.configuration, "version=121 expected=123")
	configuration := versionedWorkerConfig(harness.configuration, "")
	delete(configuration, "DBOS__APPVERSION")
	configuration["ARGUS_DBOS_MATRIX_MIGRATE"] = "true"
	command := exec.CommandContext(t.Context(), newBuild.executable, "-test.run=^TestDBOSCrashWorker$")
	command.Env = dbosWorkerEnvironment(configuration)
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("candidate SDK migration failed: %v\n%s", err, output)
	}
	assertEvaluationLedger(t, harness, 123)
	if evaluationEvidenceDigest(t, harness) != before {
		t.Fatal("candidate migration modified immutable Argus evidence")
	}
	var oldWorkflowID string
	if err := harness.reader.QueryRow(t.Context(), "SELECT workflow_uuid FROM argus_dbos_eval.workflow_status WHERE application_version = $1", oldBuild.ApplicationVersion).Scan(&oldWorkflowID); err != nil {
		t.Fatalf("read completed old workflow identity: %v", err)
	}
	delete(configuration, "ARGUS_DBOS_MATRIX_MIGRATE")
	configuration["ARGUS_DBOS_MATRIX_VERIFY_WORKFLOW"] = oldWorkflowID
	verify := exec.CommandContext(t.Context(), newBuild.executable, "-test.run=^TestDBOSCrashWorker$")
	verify.Env = dbosWorkerEnvironment(configuration)
	if output, err := verify.CombinedOutput(); err != nil {
		t.Fatalf("candidate could not read completed old SDK workflow result: %v\n%s", err, output)
	}
	candidate := startDBOSWorkerExecutable(t, newBuild.executable, harness.configuration, false)
	response := harness.postWebhookAt(t.Context(), candidate.baseURL, deliveryID, body)
	assertWebhookStatus(t, response, http.StatusOK)
	if evaluationEvidenceDigest(t, harness) != before || harness.provider.pullRequests.Load() != pulls {
		t.Fatal("candidate redelivery changed old evidence or repeated resolution")
	}
	response = harness.postWebhookAt(t.Context(), candidate.baseURL, "dbos-sdk-new-delivery", body)
	assertWebhookStatus(t, response, http.StatusCreated)
	harness.assertCompleteImpact(t, "dbos-sdk-new-delivery")
	newBuild.ApplicationVersion = evaluationNewApplicationVersion(t, harness, oldBuild.ApplicationVersion)
	awaitDBOSVersionStatus(t, harness, newBuild.ApplicationVersion, "SUCCESS", 2)
	candidate.kill(t)
	assertEvaluationWorkerRefused(t, oldBuild, harness.configuration, "version=123 expected=121")
	// Do not falsify rollback by decrementing the ledger or dropping schema.
	assertEvaluationLedger(t, harness, 123)
	report := compatibilityReport{
		Host: evaluationHostInfo(t, harness), Builds: []evaluationBuild{oldBuild, newBuild},
		EvidenceUnchanged: true, MixedVersionRollbackSafe: false, ProductionAdoptionVerdict: "defer",
		CandidateVulnerabilityScan: "passed; pinned govulncheck binary scan",
		Observations: []string{
			"exact old binary recovered interrupted assessment without resolving change again",
			"candidate refused old schema before explicit SDK migration",
			"SDK 1.6.0 migrated 121 to 123 without altering Argus evidence",
			"candidate SDK retrieved the completed old SDK workflow result",
			"candidate read and deduplicated old evidence and admitted new work",
			"old binary refused migrated schema; in-place binary rollback is unsupported",
		},
	}
	retainDBOSEvaluationReport(t, "compatibility", report)
}

func assertEvaluationWorkerRefused(t *testing.T, build evaluationBuild, configuration map[string]string, expected string) {
	t.Helper()
	command := exec.CommandContext(t.Context(), build.executable, "-test.run=^TestDBOSCrashWorker$")
	command.Env = dbosWorkerEnvironment(configuration)
	output, err := command.CombinedOutput()
	if err == nil || !strings.Contains(string(output), expected) {
		t.Fatalf("SDK %s startup did not fail closed with %q: err=%v output=%s", build.SDK, expected, err, output)
	}
}

func assertEvaluationLedger(t *testing.T, harness *dbosWebhookHarness, want int) {
	t.Helper()
	var rows, version int
	if err := harness.reader.QueryRow(t.Context(), "SELECT count(*), max(version) FROM argus_dbos_eval.dbos_migrations").Scan(&rows, &version); err != nil {
		t.Fatalf("read evaluation migration ledger: %v", err)
	}
	if rows != 1 || version != want {
		t.Fatalf("evaluation ledger: rows=%d version=%d, want one row at %d", rows, version, want)
	}
}

func evaluationApplicationVersion(t *testing.T, harness *dbosWebhookHarness) string {
	t.Helper()
	var version string
	if err := harness.reader.QueryRow(t.Context(), "SELECT DISTINCT application_version FROM argus_dbos_eval.workflow_status").Scan(&version); err != nil {
		t.Fatalf("read baseline application version: %v", err)
	}
	if version == "" {
		t.Fatal("baseline application version is empty")
	}

	return version
}

func evaluationNewApplicationVersion(t *testing.T, harness *dbosWebhookHarness, old string) string {
	t.Helper()
	var version string
	if err := harness.reader.QueryRow(t.Context(), "SELECT DISTINCT application_version FROM argus_dbos_eval.workflow_status WHERE application_version <> $1", old).Scan(&version); err != nil {
		t.Fatalf("read distinct candidate application version: %v", err)
	}
	if version == "" {
		t.Fatal("candidate application version is empty")
	}

	return version
}
