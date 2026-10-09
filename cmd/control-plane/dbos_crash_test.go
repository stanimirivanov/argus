//go:build dbose2e

package main

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stanimirivanov/argus/internal/catalog"
	"github.com/stanimirivanov/argus/internal/postgres"
)

const (
	crashWorkerFlag   = "ARGUS_DBOS_CRASH_WORKER"
	crashResponseGate = "ARGUS_DBOS_CRASH_RESPONSE_GATE"
	crashReadyMarker  = "ARGUS_CRASH_READY "
	crashReplyMarker  = "ARGUS_CRASH_RESPONSE_READY"
)

type crashBoundary string

const (
	crashBeforeChange crashBoundary = "before-change"
	crashAfterChange  crashBoundary = "after-change"
	crashAfterImpact  crashBoundary = "after-impact"
)

// TestDBOSHardCrashRecovery kills the process rather than calling Shutdown.
// Every case owns its database so recovered workflows cannot affect another
// boundary's provider gate or row assertions.
func TestDBOSHardCrashRecovery(t *testing.T) {
	for _, boundary := range []crashBoundary{crashBeforeChange, crashAfterChange, crashAfterImpact} {
		t.Run(string(boundary), func(t *testing.T) {
			testDBOSCrashBoundary(t, boundary)
		})
	}
}

// TestDBOSApplicationVersionRolloutAndRollback exercises the deployment hazard
// that a new DBOS application version does not recover a predecessor's pending
// workflows. The old version must remain available (or be rolled back) until
// its work has drained; a new delivery can proceed on the new version meanwhile.
func TestDBOSApplicationVersionRolloutAndRollback(t *testing.T) {
	harness := newDBOSWebhookHarness(t)
	if err := harness.stop(); err != nil {
		t.Fatalf("stop setup control plane before versioned workers: %v", err)
	}
	harness.provider.releaseFirstDocument()
	const oldVersion = "argus-evaluation-release-a"
	const newVersion = "argus-evaluation-release-b"
	const interruptedDelivery = "dbos-rollout-interrupted"
	const newDelivery = "dbos-rollout-new"
	body := webhookPayload(t, "synchronize")
	gate := harness.provider.gateNextDocument(t)
	defer gate.release()

	oldWorker := startDBOSCrashWorker(t, versionedWorkerConfig(harness.configuration, oldVersion), false)
	requestContext, cancel := context.WithTimeout(t.Context(), 35*time.Second)
	defer cancel()
	result := make(chan webhookResponse, 1)
	go func() {
		result <- harness.postWebhookAt(requestContext, oldWorker.baseURL, interruptedDelivery, body)
	}()
	select {
	case <-gate.reached:
	case <-time.After(20 * time.Second):
		t.Fatal("old-version assessment did not reach the provider gate")
	}
	harness.assertRows(t, interruptedDelivery, 1, 0)
	oldWorker.kill(t)
	gate.release()
	if response := awaitWebhook(t, result); response.err == nil {
		t.Fatalf("killed old worker unexpectedly acknowledged delivery: %+v", response)
	}
	assertDBOSVersionStatus(t, harness, oldVersion, "PENDING", 1)

	// The explicit migration operation is repeatable against existing history.
	// Startup of the new version still runs with SkipMigrations=true.
	prepareDBOSEvaluationSchemas(t, harness.configuration["ARGUS_DATABASE_URL"])
	assertDBOSVersionStatus(t, harness, oldVersion, "PENDING", 1)
	newWorker := startDBOSCrashWorker(t, versionedWorkerConfig(harness.configuration, newVersion), false)
	assertDBOSVersionStatus(t, harness, oldVersion, "PENDING", 1)
	response := harness.postWebhookAt(t.Context(), newWorker.baseURL, newDelivery, body)
	assertWebhookStatus(t, response, http.StatusCreated)
	harness.assertRows(t, newDelivery, 1, 1)
	assertDBOSVersionStatus(t, harness, newVersion, "SUCCESS", 1)
	// A new-version process cannot safely replace every old-version process
	// while old work remains pending, even though new deliveries succeed.
	assertDBOSVersionStatus(t, harness, oldVersion, "PENDING", 1)
	newWorker.kill(t)

	beforeRecoveryPulls := harness.provider.pullRequests.Load()
	rollback := startDBOSCrashWorker(t, versionedWorkerConfig(harness.configuration, oldVersion), false)
	harness.awaitRows(t, interruptedDelivery, 1, 1)
	harness.assertCompleteImpact(t, interruptedDelivery)
	response = harness.postWebhookAt(t.Context(), rollback.baseURL, interruptedDelivery, body)
	assertWebhookStatus(t, response, http.StatusOK)
	if got := harness.provider.pullRequests.Load(); got != beforeRecoveryPulls {
		t.Fatalf("old-version recovery repeated provider resolution: reads %d -> %d", beforeRecoveryPulls, got)
	}
	assertDBOSVersionStatus(t, harness, oldVersion, "PENDING", 0)
	assertDBOSVersionStatus(t, harness, oldVersion, "SUCCESS", 2)
	rollback.kill(t)
}

func versionedWorkerConfig(configuration map[string]string, version string) map[string]string {
	result := make(map[string]string, len(configuration)+1)
	for name, value := range configuration {
		result[name] = value
	}
	result["DBOS__APPVERSION"] = version
	return result
}

func assertDBOSVersionStatus(t *testing.T, harness *dbosWebhookHarness, version, status string, want int) {
	t.Helper()
	var count int
	err := harness.reader.QueryRow(t.Context(), `
		SELECT count(*) FROM argus_dbos_eval.workflow_status
		WHERE application_version = $1 AND status = $2
	`, version, status).Scan(&count)
	if err != nil {
		t.Fatalf("read DBOS workflow status for version %q: %v", version, err)
	}
	if count != want {
		t.Fatalf("DBOS workflows at version %q status %q = %d, want %d", version, status, count, want)
	}
}

func testDBOSCrashBoundary(t *testing.T, boundary crashBoundary) {
	t.Helper()
	harness := newDBOSWebhookHarness(t)
	if err := harness.stop(); err != nil {
		t.Fatalf("stop setup control plane before process test: %v", err)
	}
	harness.provider.releaseFirstDocument()
	reader, err := postgres.OpenRuntime(t.Context(), harness.configuration["ARGUS_DATABASE_URL"])
	if err != nil {
		t.Fatalf("open independent impact reader: %v", err)
	}
	t.Cleanup(reader.Close)
	deliveryID := "dbos-crash-" + string(boundary)
	body := webhookPayload(t, "synchronize")
	var gate *providerRequestGate
	switch boundary {
	case crashBeforeChange:
		gate = harness.provider.gateNextPullRequest(t)
	case crashAfterChange:
		gate = harness.provider.gateNextDocument(t)
	case crashAfterImpact:
	default:
		t.Fatalf("unknown crash boundary %q", boundary)
	}
	if gate != nil {
		defer gate.release()
	}

	worker := startDBOSCrashWorker(t, harness.configuration, boundary == crashAfterImpact)
	requestContext, cancel := context.WithTimeout(t.Context(), 35*time.Second)
	defer cancel()
	result := make(chan webhookResponse, 1)
	go func() {
		result <- harness.postWebhookAt(requestContext, worker.baseURL, deliveryID, body)
	}()
	if gate != nil {
		select {
		case <-gate.reached:
		case <-time.After(20 * time.Second):
			t.Fatal("workflow did not reach the requested provider crash boundary")
		}
	} else {
		worker.await(t, crashReplyMarker)
	}
	beforeChanges, beforeImpacts := expectedCrashRows(boundary)
	harness.assertRows(t, deliveryID, beforeChanges, beforeImpacts)
	worker.kill(t)
	if gate != nil {
		gate.release()
	}
	if response := awaitWebhook(t, result); response.err == nil {
		t.Fatalf("killed worker unexpectedly acknowledged delivery: status=%d body=%q", response.status, response.body)
	}
	harness.assertRows(t, deliveryID, beforeChanges, beforeImpacts)

	beforeRetryPulls := harness.provider.pullRequests.Load()
	beforeRetryDocuments := harness.provider.documents.Load()
	restarted := startDBOSCrashWorker(t, harness.configuration, false)
	retryContext, stopRetry := context.WithTimeout(t.Context(), 35*time.Second)
	defer stopRetry()
	retry := harness.postWebhookAt(retryContext, restarted.baseURL, deliveryID, body)
	assertCrashRetryStatus(t, boundary, retry)
	harness.assertRows(t, deliveryID, 1, 1)
	impact, err := reader.Change().FindCapabilityImpact(t.Context(), catalog.ProviderGitHub, deliveryID)
	if err != nil {
		t.Fatalf("reconstruct impact after process restart: %v", err)
	}
	assertMappedImpact(t, impact)
	if boundary != crashBeforeChange && harness.provider.pullRequests.Load() != beforeRetryPulls {
		t.Fatal("recovery repeated provider resolution after change evidence was stored")
	}
	if boundary == crashAfterImpact && harness.provider.documents.Load() != beforeRetryDocuments {
		t.Fatal("recovery repeated assessment after impact evidence was stored")
	}
	restarted.kill(t)
}

func assertCrashRetryStatus(t *testing.T, boundary crashBoundary, retry webhookResponse) {
	t.Helper()
	if boundary != crashBeforeChange {
		assertWebhookStatus(t, retry, http.StatusOK)
		return
	}
	// Startup recovery and explicit redelivery race to create the same
	// immutable change. Either one may win; the row assertions are the oracle.
	if retry.err != nil || (retry.status != http.StatusOK && retry.status != http.StatusCreated) {
		t.Fatalf("redelivery after pre-change crash: status=%d body=%q err=%v; want 200 or 201",
			retry.status, retry.body, retry.err)
	}
}

func expectedCrashRows(boundary crashBoundary) (int, int) {
	switch boundary {
	case crashBeforeChange:
		return 0, 0
	case crashAfterChange:
		return 1, 0
	case crashAfterImpact:
		return 1, 1
	default:
		panic("unexpected crash boundary")
	}
}

// TestDBOSCrashWorker is invoked only by the parent test's own test binary.
// A successful parent case terminates this process with Kill, so no graceful
// DBOS Shutdown or Go test cleanup can hide a missing recovery transition.
func TestDBOSCrashWorker(t *testing.T) {
	if os.Getenv(crashWorkerFlag) != "true" {
		return
	}
	application, err := newApplication(t.Context(), os.Getenv, false)
	if err != nil {
		t.Fatalf("start crash worker: %v", err)
	}
	t.Cleanup(func() {
		if err := application.Close(); err != nil {
			t.Errorf("close crash worker: %v", err)
		}
	})
	if os.Getenv(crashResponseGate) == "true" {
		original := application.server.Handler
		application.server.Handler = http.HandlerFunc(func(_ http.ResponseWriter, request *http.Request) {
			recorder := httptest.NewRecorder()
			original.ServeHTTP(recorder, request)
			if _, err := fmt.Fprintln(os.Stdout, crashReplyMarker); err != nil {
				return
			}
			<-request.Context().Done()
		})
	}
	listener, err := (&net.ListenConfig{}).Listen(t.Context(), "tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen for crash worker: %v", err)
	}
	if _, err := fmt.Fprintln(os.Stdout, crashReadyMarker+"http://"+listener.Addr().String()); err != nil {
		t.Fatalf("announce crash worker listener: %v", err)
	}
	if err := application.server.Serve(listener); err != nil && err != http.ErrServerClosed {
		t.Fatalf("serve crash worker: %v", err)
	}
}

type dbosCrashWorker struct {
	cmd     *exec.Cmd
	baseURL string
	events  chan string
	stdout  bytes.Buffer
	stderr  bytes.Buffer
	once    sync.Once
	scanned chan struct{}
	killErr error
}

func startDBOSCrashWorker(t *testing.T, configuration map[string]string, gateResponse bool) *dbosCrashWorker {
	t.Helper()
	worker := &dbosCrashWorker{events: make(chan string, 2), scanned: make(chan struct{})}
	// Only the explicit Kill models a hard crash. Test context cancellation
	// must not quietly terminate the child before the boundary is observed.
	worker.cmd = exec.CommandContext(context.WithoutCancel(t.Context()), os.Args[0], "-test.run=^TestDBOSCrashWorker$")
	worker.cmd.Env = append(os.Environ(), crashWorkerFlag+"=true")
	for name, value := range configuration {
		worker.cmd.Env = append(worker.cmd.Env, name+"="+value)
	}
	if gateResponse {
		worker.cmd.Env = append(worker.cmd.Env, crashResponseGate+"=true")
	} else {
		worker.cmd.Env = append(worker.cmd.Env, crashResponseGate+"=false")
	}
	stdout, err := worker.cmd.StdoutPipe()
	if err != nil {
		t.Fatalf("pipe crash worker output: %v", err)
	}
	worker.cmd.Stderr = &worker.stderr
	if err := worker.cmd.Start(); err != nil {
		t.Fatalf("start crash worker process: %v", err)
	}
	t.Cleanup(func() { worker.kill(t) })
	go func() {
		defer close(worker.scanned)
		scanner := bufio.NewScanner(stdout)
		for scanner.Scan() {
			line := scanner.Text()
			fmt.Fprintln(&worker.stdout, line)
			if strings.HasPrefix(line, crashReadyMarker) || line == crashReplyMarker {
				worker.events <- line
			}
		}
		close(worker.events)
	}()
	worker.baseURL = strings.TrimPrefix(worker.await(t, crashReadyMarker), crashReadyMarker)

	return worker
}

func (worker *dbosCrashWorker) await(t *testing.T, prefix string) string {
	t.Helper()
	timer := time.NewTimer(30 * time.Second)
	defer timer.Stop()
	for {
		select {
		case line, ok := <-worker.events:
			if !ok {
				worker.kill(t)
				t.Fatalf("crash worker exited before %q: stdout=%q stderr=%q", prefix, worker.stdout.String(), worker.stderr.String())
			}
			if strings.HasPrefix(line, prefix) {
				return line
			}
		case <-timer.C:
			worker.kill(t)
			t.Fatalf("crash worker did not reach %q: stdout=%q stderr=%q", prefix, worker.stdout.String(), worker.stderr.String())
		}
	}
}

func (worker *dbosCrashWorker) kill(t *testing.T) {
	t.Helper()
	worker.once.Do(func() {
		if err := worker.cmd.Process.Kill(); err != nil {
			worker.killErr = fmt.Errorf("kill crash worker: %w", err)
		}
		if err := worker.cmd.Wait(); err == nil {
			worker.killErr = errors.Join(worker.killErr, errors.New("crash worker exited normally"))
		} else {
			var exitError *exec.ExitError
			if !errors.As(err, &exitError) {
				worker.killErr = errors.Join(worker.killErr, fmt.Errorf("wait for crash worker: %w", err))
			}
		}
		<-worker.scanned
	})
	if worker.killErr != nil {
		t.Errorf("terminate crash worker: %v", worker.killErr)
	}
}
