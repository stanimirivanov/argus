//go:build dbose2e

package main

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	dbosgo "github.com/dbos-inc/dbos-transact-golang/dbos"
	"github.com/jackc/pgx/v5"
	"github.com/stanimirivanov/argus/internal/catalog"
	"github.com/stanimirivanov/argus/internal/change"
	githubadapter "github.com/stanimirivanov/argus/internal/change/adapters/github"
	openapiadapter "github.com/stanimirivanov/argus/internal/change/adapters/openapi"
	"github.com/stanimirivanov/argus/internal/postgres"
	"github.com/testcontainers/testcontainers-go"
	postgrescontainer "github.com/testcontainers/testcontainers-go/modules/postgres"
)

const (
	baseSHA = "0123456789abcdef0123456789abcdef01234567"
	headSHA = "89abcdef0123456789abcdef0123456789abcdef"
)

// This test runs without a container. It guards the provider fixture used by
// the DBOS test against syntactically valid but semantically empty documents.
func TestDBOSProviderFixtureProducesMappedImpact(t *testing.T) {
	provider := newGitHubProviderStub()
	provider.token = randomHex(t, 24)
	provider.releaseFirstDocument()
	providerServer := httptest.NewServer(http.HandlerFunc(provider.serveHTTP))
	t.Cleanup(providerServer.Close)
	client, err := githubadapter.NewClient(githubadapter.ClientOptions{
		BaseURL: providerServer.URL,
		Token:   provider.token,
	})
	if err != nil {
		t.Fatalf("create GitHub fixture client: %v", err)
	}
	set := change.Set{
		APIVersion: change.SetAPIVersion,
		SourceRepository: catalog.Repository{
			Identity: catalog.RepositoryIdentity{
				Provider: catalog.ProviderGitHub, Host: "github.com", ProviderRepositoryID: "1296269",
			},
			Owner: "octocat", Name: "hello-world",
		},
		PullRequestNumber: 42,
		BaseRevision:      catalog.Revision{Algorithm: catalog.RevisionGitSHA1, Digest: baseSHA},
		HeadRevision:      catalog.Revision{Algorithm: catalog.RevisionGitSHA1, Digest: headSHA},
		ObservedAt:        time.Date(2026, 9, 18, 9, 30, 0, 0, time.UTC),
		Trigger: change.Trigger{
			Provider: catalog.ProviderGitHub, DeliveryID: "fixture-impact",
			Event: "pull_request", Action: "synchronize",
		},
		Files: []change.File{{
			Path: "api/openapi.yaml", Kind: change.KindModified,
			Additions: 1, Deletions: 1, PatchStatus: change.PatchUnavailable,
		}},
	}
	impact, err := openapiadapter.NewAnalyzer(client).Analyze(t.Context(), set)
	if err != nil {
		t.Fatalf("analyze mocked immutable OpenAPI documents: %v", err)
	}
	assertMappedImpact(t, impact)
	if got := provider.documents.Load(); got != 2 {
		t.Fatalf("mocked document reads = %d, want both immutable revisions", got)
	}
}

// TestDBOSWebhookIngestion exercises the real control-plane composition root.
// A Docker-compatible runtime is the only external prerequisite: the test owns
// its database, schemas, credentials, provider stub, listener, and cleanup.
func TestDBOSWebhookIngestion(t *testing.T) {
	harness := newDBOSWebhookHarness(t)
	successBody := webhookPayload(t, "synchronize")
	const successDelivery = "dbos-e2e-success"

	t.Run("acknowledgment waits for durable assessment", func(t *testing.T) {
		defer harness.provider.releaseFirstDocument()
		result := make(chan webhookResponse, 1)
		go func() {
			result <- harness.postWebhook(successDelivery, successBody)
		}()

		select {
		case <-harness.provider.firstDocumentReached:
		case <-time.After(20 * time.Second):
			t.Fatal("assessment did not reach the gated provider document request")
		}
		// The provider request is deliberately blocked after change ingestion.
		// A response at this point would acknowledge incomplete durable evidence.
		harness.assertRows(t, successDelivery, 1, 0)
		select {
		case response := <-result:
			t.Fatalf("webhook acknowledged before assessment persisted: %+v", response)
		case <-time.After(200 * time.Millisecond):
		}

		harness.provider.releaseFirstDocument()
		response := awaitWebhook(t, result)
		assertWebhookStatus(t, response, http.StatusCreated)
		harness.assertRows(t, successDelivery, 1, 1)
		harness.assertCompleteImpact(t, successDelivery)
		if got := harness.provider.pullRequests.Load(); got != 2 {
			t.Fatalf("provider pull-request reads = %d, want 2", got)
		}
	})

	t.Run("exact redelivery is immutable and does not reprocess", func(t *testing.T) {
		beforePulls := harness.provider.pullRequests.Load()
		beforeDocuments := harness.provider.documents.Load()
		response := harness.postWebhook(successDelivery, successBody)
		assertWebhookStatus(t, response, http.StatusOK)
		harness.assertRows(t, successDelivery, 1, 1)
		if got := harness.provider.pullRequests.Load(); got != beforePulls {
			t.Fatalf("exact retry repeated provider resolution: reads %d -> %d", beforePulls, got)
		}
		if got := harness.provider.documents.Load(); got != beforeDocuments {
			t.Fatalf("exact retry repeated assessment: document reads %d -> %d", beforeDocuments, got)
		}
	})

	t.Run("validly signed conflicting content is rejected", func(t *testing.T) {
		beforePulls := harness.provider.pullRequests.Load()
		conflictingBody := webhookPayload(t, "reopened")
		response := harness.postWebhook(successDelivery, conflictingBody)
		assertWebhookStatus(t, response, http.StatusConflict)
		harness.assertRows(t, successDelivery, 1, 1)
		if got := harness.provider.pullRequests.Load(); got != beforePulls {
			t.Fatalf("conflict triggered provider resolution: reads %d -> %d", beforePulls, got)
		}
	})

	t.Run("missing provider evidence keeps not-found status", func(t *testing.T) {
		const deliveryID = "dbos-e2e-not-found"
		harness.provider.missingNextPullRequest.Store(true)
		response := harness.postWebhook(deliveryID, successBody)
		assertWebhookStatus(t, response, http.StatusNotFound)
		harness.assertRows(t, deliveryID, 0, 0)
		response = harness.postWebhook(deliveryID, successBody)
		assertWebhookStatus(t, response, http.StatusCreated)
		harness.assertRows(t, deliveryID, 1, 1)
	})

	t.Run("moving provider revision keeps conflict status", func(t *testing.T) {
		const deliveryID = "dbos-e2e-stale"
		harness.provider.staleNextPullRequest.Store(true)
		response := harness.postWebhook(deliveryID, successBody)
		assertWebhookStatus(t, response, http.StatusConflict)
		harness.assertRows(t, deliveryID, 0, 0)
		response = harness.postWebhook(deliveryID, successBody)
		assertWebhookStatus(t, response, http.StatusCreated)
		harness.assertRows(t, deliveryID, 1, 1)
	})

	t.Run("failed assessment resumes without resolving change again", func(t *testing.T) {
		const recoveryDelivery = "dbos-e2e-recovery"
		beforePulls := harness.provider.pullRequests.Load()
		harness.provider.failNextDocument.Store(true)
		response := harness.postWebhook(recoveryDelivery, successBody)
		assertWebhookStatus(t, response, http.StatusBadGateway)
		harness.assertRows(t, recoveryDelivery, 1, 0)
		if got := harness.provider.pullRequests.Load(); got != beforePulls+2 {
			t.Fatalf("first recovery attempt provider reads = %d, want %d", got, beforePulls+2)
		}

		// Reconstruct the application, including its DBOS runtime, from the
		// same database before retrying the incomplete delivery.
		harness.restart(t)
		beforeRetry := harness.provider.pullRequests.Load()
		response = harness.postWebhook(recoveryDelivery, successBody)
		assertWebhookStatus(t, response, http.StatusOK)
		harness.assertRows(t, recoveryDelivery, 1, 1)
		harness.assertCompleteImpact(t, recoveryDelivery)
		if got := harness.provider.pullRequests.Load(); got != beforeRetry {
			t.Fatalf("assessment retry repeated provider resolution: reads %d -> %d", beforeRetry, got)
		}
	})

	t.Run("client disconnect does not cancel accepted assessment", func(t *testing.T) {
		testDBOSClientDisconnect(t, harness, successBody)
	})
}

// TestDBOSSchemaRolloutFailsClosed proves runtime never repairs an absent
// checkpoint schema and that disabling the evaluation preserves the default
// webhook path. The schema is dropped only in this disposable test database.
func TestDBOSSchemaRolloutFailsClosed(t *testing.T) {
	harness := newDBOSWebhookHarness(t)
	if err := harness.stop(); err != nil {
		t.Fatalf("stop evaluation before removing its test schema: %v", err)
	}
	harness.provider.releaseFirstDocument()
	if _, err := harness.reader.Exec(t.Context(), `DROP SCHEMA argus_dbos_eval CASCADE`); err != nil {
		t.Fatalf("remove disposable DBOS schema: %v", err)
	}
	application, err := newApplication(t.Context(), func(name string) string {
		return harness.configuration[name]
	}, false)
	if err == nil || !strings.Contains(err.Error(), "schema is missing or unreadable") {
		closeUnexpectedApplication(t, application)
		t.Fatalf("DBOS evaluation missing-schema startup error = %v, want fail-closed schema error", err)
	}
	assertDBOSSchemaPresent(t, harness, false)

	harness.configuration["ARGUS_DBOS_EVALUATION"] = "false"
	defaultApplication, err := newApplication(t.Context(), func(name string) string {
		return harness.configuration[name]
	}, false)
	if err != nil {
		t.Fatalf("start established path after disabling DBOS: %v", err)
	}
	defaultListener := httptest.NewServer(defaultApplication.server.Handler)
	response := harness.postWebhookAt(t.Context(), defaultListener.URL, "dbos-schema-default", webhookPayload(t, "synchronize"))
	defaultListener.Close()
	if err := defaultApplication.Close(); err != nil {
		t.Fatalf("close established path: %v", err)
	}
	assertWebhookStatus(t, response, http.StatusCreated)
	harness.assertRows(t, "dbos-schema-default", 1, 1)
	assertDBOSSchemaPresent(t, harness, false)

	prepareDBOSEvaluationSchemas(t, harness.configuration["ARGUS_DATABASE_URL"])
	assertDBOSSchemaPresent(t, harness, true)
	if _, err := harness.reader.Exec(t.Context(), `UPDATE argus_dbos_eval.dbos_migrations SET version = version + 1`); err != nil {
		t.Fatalf("simulate a newer DBOS schema in disposable database: %v", err)
	}
	harness.configuration["ARGUS_DBOS_EVALUATION"] = "true"
	application, err = newApplication(t.Context(), func(name string) string {
		return harness.configuration[name]
	}, false)
	if err == nil || !strings.Contains(err.Error(), "schema version is unsupported") {
		closeUnexpectedApplication(t, application)
		t.Fatalf("DBOS evaluation newer-schema startup error = %v, want unsupported-schema error", err)
	}
	if _, err := harness.reader.Exec(t.Context(), `UPDATE argus_dbos_eval.dbos_migrations SET version = version - 1`); err != nil {
		t.Fatalf("restore pinned DBOS schema version in disposable database: %v", err)
	}
	harness.configuration["ARGUS_DBOS_EVALUATION"] = "true"
	harness.start(t)
	response = harness.postWebhook("dbos-schema-restored", webhookPayload(t, "synchronize"))
	assertWebhookStatus(t, response, http.StatusCreated)
	harness.assertRows(t, "dbos-schema-restored", 1, 1)
}

func closeUnexpectedApplication(t *testing.T, application *application) {
	t.Helper()
	if application == nil {
		return
	}
	if err := application.Close(); err != nil {
		t.Errorf("close application after unexpected startup result: %v", err)
	}
}

func assertDBOSSchemaPresent(t *testing.T, harness *dbosWebhookHarness, want bool) {
	t.Helper()
	var present bool
	if err := harness.reader.QueryRow(t.Context(), `
		SELECT to_regclass('argus_dbos_eval.dbos_migrations') IS NOT NULL
	`).Scan(&present); err != nil {
		t.Fatalf("inspect DBOS migration ledger: %v", err)
	}
	if present != want {
		t.Fatalf("DBOS migration ledger present = %t, want %t", present, want)
	}
}

func testDBOSClientDisconnect(t *testing.T, harness *dbosWebhookHarness, body []byte) {
	t.Helper()
	const disconnectedDelivery = "dbos-e2e-disconnected"
	gate := harness.provider.gateNextDocument(t)
	defer gate.release()
	requestContext, cancel := context.WithCancel(t.Context())
	defer cancel()
	result := make(chan webhookResponse, 1)
	go func() {
		result <- harness.postWebhookContext(requestContext, disconnectedDelivery, body)
	}()

	select {
	case <-gate.reached:
	case <-time.After(20 * time.Second):
		t.Fatal("accepted assessment did not reach the gated provider request")
	}
	harness.assertRows(t, disconnectedDelivery, 1, 0)
	cancel()
	if response := awaitWebhook(t, result); !errors.Is(response.err, context.Canceled) {
		t.Fatalf("disconnected webhook client: status=%d err=%v; want canceled request", response.status, response.err)
	}

	gate.release()
	harness.awaitRows(t, disconnectedDelivery, 1, 1)
	harness.assertCompleteImpact(t, disconnectedDelivery)
	beforePulls := harness.provider.pullRequests.Load()
	beforeDocuments := harness.provider.documents.Load()
	response := harness.postWebhook(disconnectedDelivery, body)
	assertWebhookStatus(t, response, http.StatusOK)
	if got := harness.provider.pullRequests.Load(); got != beforePulls {
		t.Fatalf("redelivery after disconnect repeated provider resolution: reads %d -> %d", beforePulls, got)
	}
	if got := harness.provider.documents.Load(); got != beforeDocuments {
		t.Fatalf("redelivery after disconnect repeated assessment: document reads %d -> %d", beforeDocuments, got)
	}
}

type dbosWebhookHarness struct {
	ctx           context.Context
	listener      *httptest.Server
	application   *application
	configuration map[string]string
	provider      *githubProviderStub
	secret        string
	reader        *pgx.Conn
	store         *postgres.ChangeStore
	imageID       string
}

func newDBOSWebhookHarness(t *testing.T) *dbosWebhookHarness {
	t.Helper()
	return newWebhookEvaluationHarness(t, true)
}

func newWebhookEvaluationHarness(t *testing.T, durable bool) *dbosWebhookHarness {
	t.Helper()
	// The SDK reads these independently of Argus's injected getter. Host cloud
	// credentials and application labels must not redirect this experiment.
	for _, name := range []string{"DBOS__APPVERSION", "DBOS__VMID", "DBOS__APPID", "DBOS__CLOUD"} {
		t.Setenv(name, "")
	}
	// The Linux Ryuk container needs the daemon's Unix socket, not the
	// Windows named pipe used by the host client. Respect explicit overrides
	// and remote daemon configuration; never disable the cleanup reaper.
	if runtime.GOOS == "windows" && os.Getenv("TESTCONTAINERS_DOCKER_SOCKET_OVERRIDE") == "" &&
		(os.Getenv("DOCKER_HOST") == "" || strings.HasPrefix(os.Getenv("DOCKER_HOST"), "npipe:")) &&
		(os.Getenv("DOCKER_CONTEXT") == "" || os.Getenv("DOCKER_CONTEXT") == "desktop-linux") {
		t.Setenv("TESTCONTAINERS_DOCKER_SOCKET_OVERRIDE", "/var/run/docker.sock")
	}
	password := randomHex(t, 24)
	container, err := postgrescontainer.Run(t.Context(), "postgres:17.11",
		postgrescontainer.WithDatabase("argus_dbos_e2e"),
		postgrescontainer.WithUsername("argus_test"),
		postgrescontainer.WithPassword(password),
		postgrescontainer.BasicWaitStrategies(),
	)
	if err != nil {
		t.Fatalf("start isolated PostgreSQL container: %v", err)
	}
	testcontainers.CleanupContainer(t, container)
	databaseURL, err := container.ConnectionString(t.Context(), "sslmode=disable")
	if err != nil {
		t.Fatalf("get isolated PostgreSQL connection: %v", err)
	}
	if durable {
		prepareDBOSEvaluationSchemas(t, databaseURL)
	} else {
		prepareArgusEvaluationSchema(t, databaseURL)
	}
	inspection, err := container.Inspect(t.Context())
	if err != nil {
		t.Fatalf("inspect isolated PostgreSQL image provenance: %v", err)
	}

	provider := newGitHubProviderStub()
	t.Cleanup(provider.releaseFirstDocument)
	providerServer := httptest.NewServer(http.HandlerFunc(provider.serveHTTP))
	t.Cleanup(providerServer.Close)
	secret := randomHex(t, 32)
	configuration := map[string]string{
		"ARGUS_DATABASE_URL":          databaseURL,
		"ARGUS_GITHUB_API_URL":        providerServer.URL,
		"ARGUS_GITHUB_TOKEN":          randomHex(t, 24),
		"ARGUS_GITHUB_WEBHOOK_SECRET": secret,
		"ARGUS_DBOS_EVALUATION":       "true",
	}
	provider.token = configuration["ARGUS_GITHUB_TOKEN"]
	harness := &dbosWebhookHarness{
		ctx: t.Context(), configuration: configuration, provider: provider, secret: secret,
		imageID: inspection.Image,
	}
	// Evidence assertions outlive the application under test, including when
	// it is replaced by a child process. Own this read pool at harness scope
	// so stopping or restarting the application cannot invalidate the store.
	evidence, err := postgres.OpenRuntime(t.Context(), databaseURL)
	if err != nil {
		t.Fatalf("open independent capability-impact reader: %v", err)
	}
	t.Cleanup(evidence.Close)
	harness.store = evidence.Change()
	if durable {
		harness.start(t)
	} else {
		startSynchronousEvaluation(t, harness)
	}
	t.Cleanup(func() {
		if err := harness.stop(); err != nil {
			t.Errorf("close DBOS control plane: %v", err)
		}
	})

	reader, err := pgx.Connect(t.Context(), databaseURL)
	if err != nil {
		t.Fatalf("open independent durable-evidence reader: %v", err)
	}
	t.Cleanup(func() {
		if err := reader.Close(context.Background()); err != nil {
			t.Errorf("close durable-evidence reader: %v", err)
		}
	})

	harness.reader = reader

	return harness
}

func (harness *dbosWebhookHarness) start(t *testing.T) {
	t.Helper()
	application, err := newApplication(harness.ctx, func(name string) string {
		return harness.configuration[name]
	}, false)
	if err != nil {
		t.Fatalf("start DBOS control plane in process: %v", err)
	}
	if application.durable == nil {
		if err := application.Close(); err != nil {
			t.Errorf("close unexpectedly non-durable application: %v", err)
		}
		t.Fatal("DBOS evaluation was not enabled by injected configuration")
	}
	harness.application = application
	harness.listener = httptest.NewServer(application.server.Handler)
}

func (harness *dbosWebhookHarness) stop() error {
	if harness.listener != nil {
		harness.listener.Close()
		harness.listener = nil
	}
	if harness.application == nil {
		return nil
	}
	err := harness.application.Close()
	harness.application = nil

	return err
}

func (harness *dbosWebhookHarness) restart(t *testing.T) {
	t.Helper()
	if err := harness.stop(); err != nil {
		t.Fatalf("stop DBOS control plane before restart: %v", err)
	}
	harness.start(t)
}

func prepareDBOSEvaluationSchemas(t *testing.T, databaseURL string) {
	t.Helper()
	prepareArgusEvaluationSchema(t, databaseURL)
	prepareDBOSEvaluationSchema(t, databaseURL)
}

func prepareArgusEvaluationSchema(t *testing.T, databaseURL string) {
	t.Helper()
	migrator, err := postgres.OpenMigrator(t.Context(), databaseURL)
	if err != nil {
		t.Fatalf("open catalog migrator: %v", err)
	}
	defer migrator.Close()
	if err := migrator.Migrate(t.Context()); err != nil {
		t.Fatalf("apply catalog migrations: %v", err)
	}
}

func prepareDBOSEvaluationSchema(t *testing.T, databaseURL string) {
	t.Helper()
	// This is the in-process equivalent of migrate --dbos-evaluation. The
	// ordinary server below starts with SkipMigrations and cannot mutate schema.
	runtime, err := dbosgo.NewContext(t.Context(), dbosgo.Config{
		AppName:        "argus-change-evaluation-migration",
		DatabaseURL:    databaseURL,
		DatabaseSchema: "argus_dbos_eval",
	})
	if err != nil {
		t.Fatalf("prepare DBOS evaluation schema: %v", err)
	}
	if err := dbosgo.Shutdown(runtime, 10*time.Second); err != nil {
		t.Fatalf("close DBOS evaluation migrator: %v", err)
	}
}

type webhookResponse struct {
	status int
	body   string
	err    error
}

func (harness *dbosWebhookHarness) postWebhook(deliveryID string, body []byte) webhookResponse {
	requestContext, cancel := context.WithTimeout(context.Background(), 35*time.Second)
	defer cancel()
	return harness.postWebhookContext(requestContext, deliveryID, body)
}

func (harness *dbosWebhookHarness) postWebhookContext(requestContext context.Context, deliveryID string, body []byte) webhookResponse {
	return harness.postWebhookAt(requestContext, harness.listener.URL, deliveryID, body)
}

func (harness *dbosWebhookHarness) postWebhookAt(requestContext context.Context, baseURL, deliveryID string, body []byte) webhookResponse {
	request, err := http.NewRequestWithContext(
		requestContext, http.MethodPost, baseURL+"/webhooks/github", bytes.NewReader(body),
	)
	if err != nil {
		return webhookResponse{err: err}
	}
	mac := hmac.New(sha256.New, []byte(harness.secret))
	_, _ = mac.Write(body)
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("X-GitHub-Event", "pull_request")
	request.Header.Set("X-GitHub-Delivery", deliveryID)
	request.Header.Set("X-Hub-Signature-256", "sha256="+hex.EncodeToString(mac.Sum(nil)))
	client := &http.Client{Timeout: 35 * time.Second}
	response, err := client.Do(request)
	if err != nil {
		return webhookResponse{err: err}
	}
	responseBody, readErr := io.ReadAll(io.LimitReader(response.Body, 4096))
	closeErr := response.Body.Close()

	return webhookResponse{status: response.StatusCode, body: string(responseBody), err: errors.Join(readErr, closeErr)}
}

func awaitWebhook(t *testing.T, result <-chan webhookResponse) webhookResponse {
	t.Helper()
	select {
	case response := <-result:
		return response
	case <-time.After(35 * time.Second):
		t.Fatal("webhook did not complete before the test deadline")
		return webhookResponse{}
	}
}

func assertWebhookStatus(t *testing.T, response webhookResponse, want int) {
	t.Helper()
	if response.err != nil || response.status != want {
		t.Fatalf("webhook response: status=%d body=%q err=%v; want %d", response.status, response.body, response.err, want)
	}
}

func (harness *dbosWebhookHarness) assertRows(t *testing.T, deliveryID string, wantChanges, wantImpacts int) {
	t.Helper()
	changes, impacts, err := harness.rows(t.Context(), deliveryID)
	if err != nil {
		t.Fatalf("read durable delivery and assessment rows: %v", err)
	}
	if changes != wantChanges || impacts != wantImpacts {
		t.Fatalf("durable rows for %s = change:%d impact:%d; want change:%d impact:%d",
			deliveryID, changes, impacts, wantChanges, wantImpacts)
	}
}

// awaitRows observes the asynchronous workflow after its HTTP client leaves.
// The bounded poll replaces timing assumptions about DBOS scheduling.
func (harness *dbosWebhookHarness) awaitRows(t *testing.T, deliveryID string, wantChanges, wantImpacts int) {
	t.Helper()
	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	defer cancel()
	ticker := time.NewTicker(20 * time.Millisecond)
	defer ticker.Stop()
	for {
		changes, impacts, err := harness.rows(ctx, deliveryID)
		if err != nil {
			t.Fatalf("read durable delivery and assessment rows: %v", err)
		}
		if changes == wantChanges && impacts == wantImpacts {
			return
		}
		select {
		case <-ctx.Done():
			t.Fatalf("durable rows for %s = change:%d impact:%d; want change:%d impact:%d: %v",
				deliveryID, changes, impacts, wantChanges, wantImpacts, ctx.Err())
		case <-ticker.C:
		}
	}
}

func (harness *dbosWebhookHarness) rows(ctx context.Context, deliveryID string) (int, int, error) {
	var changes, impacts int
	err := harness.reader.QueryRow(ctx, `
		SELECT count(c.change_set_id), count(a.assessment_id)
		FROM argus_catalog.change_sets AS c
		LEFT JOIN argus_catalog.openapi_impact_assessments AS a USING (change_set_id)
		WHERE c.delivery_provider = 'github' AND c.delivery_id = $1
	`, deliveryID).Scan(&changes, &impacts)

	return changes, impacts, err
}

func (harness *dbosWebhookHarness) assertCompleteImpact(t *testing.T, deliveryID string) {
	t.Helper()
	impact, err := harness.store.FindCapabilityImpact(t.Context(), catalog.ProviderGitHub, deliveryID)
	if err != nil {
		t.Fatalf("reconstruct durable capability impact: %v", err)
	}
	assertMappedImpact(t, impact)
}

func assertMappedImpact(t *testing.T, impact change.CapabilityImpact) {
	t.Helper()
	if impact.Status != change.ImpactComplete || len(impact.Documents) != 1 ||
		len(impact.Documents[0].Operations) != 1 ||
		len(impact.Documents[0].Operations[0].Capabilities) != 1 ||
		impact.Documents[0].Operations[0].Capabilities[0] != "create-order" {
		t.Fatalf("unexpected persisted capability impact: %+v", impact)
	}
}

func webhookPayload(t *testing.T, action string) []byte {
	t.Helper()
	body, err := json.Marshal(map[string]any{
		"action": action,
		"number": 42,
		"repository": map[string]any{
			"id": 1296269, "full_name": "octocat/hello-world",
		},
		"pull_request": map[string]any{
			"updated_at": "2026-09-18T09:30:00Z",
			"base":       map[string]string{"sha": baseSHA},
			"head":       map[string]string{"sha": headSHA},
		},
	})
	if err != nil {
		t.Fatalf("encode synthetic webhook: %v", err)
	}

	return body
}

func randomHex(t *testing.T, byteCount int) string {
	t.Helper()
	value := make([]byte, byteCount)
	if _, err := rand.Read(value); err != nil {
		t.Fatalf("generate synthetic test credential: %v", err)
	}

	return hex.EncodeToString(value)
}

type githubProviderStub struct {
	token                  string
	pullRequests           atomic.Int32
	documents              atomic.Int32
	failNextDocument       atomic.Bool
	missingNextPullRequest atomic.Bool
	staleNextPullRequest   atomic.Bool
	firstDocumentOnce      sync.Once
	firstDocumentReached   chan struct{}
	firstDocumentRelease   chan struct{}
	releaseOnce            sync.Once
	gateMu                 sync.Mutex
	nextPullRequestGate    *providerRequestGate
	nextDocumentGate       *providerRequestGate
}

type providerRequestGate struct {
	reached chan struct{}
	opened  chan struct{}
	once    sync.Once
}

func (gate *providerRequestGate) release() {
	gate.once.Do(func() { close(gate.opened) })
}

func newGitHubProviderStub() *githubProviderStub {
	return &githubProviderStub{
		firstDocumentReached: make(chan struct{}),
		firstDocumentRelease: make(chan struct{}),
	}
}

func (provider *githubProviderStub) releaseFirstDocument() {
	provider.releaseOnce.Do(func() { close(provider.firstDocumentRelease) })
}

func (provider *githubProviderStub) gateNextDocument(t *testing.T) *providerRequestGate {
	t.Helper()
	gate := &providerRequestGate{reached: make(chan struct{}), opened: make(chan struct{})}
	provider.gateMu.Lock()
	defer provider.gateMu.Unlock()
	if provider.nextDocumentGate != nil {
		t.Fatal("a provider document request is already gated")
	}
	provider.nextDocumentGate = gate

	return gate
}

func (provider *githubProviderStub) gateNextPullRequest(t *testing.T) *providerRequestGate {
	t.Helper()
	gate := &providerRequestGate{reached: make(chan struct{}), opened: make(chan struct{})}
	provider.gateMu.Lock()
	defer provider.gateMu.Unlock()
	if provider.nextPullRequestGate != nil {
		t.Fatal("a provider pull request is already gated")
	}
	provider.nextPullRequestGate = gate

	return gate
}

func (provider *githubProviderStub) serveHTTP(response http.ResponseWriter, request *http.Request) {
	if request.Header.Get("Authorization") != "Bearer "+provider.token {
		http.Error(response, "synthetic token required", http.StatusUnauthorized)
		return
	}
	if request.Method != http.MethodGet {
		http.Error(response, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	response.Header().Set("Content-Type", "application/json")
	switch {
	case request.URL.Path == "/repos/octocat/hello-world/pulls/42" && request.URL.RawQuery == "":
		provider.pullRequests.Add(1)
		provider.gateMu.Lock()
		gate := provider.nextPullRequestGate
		provider.nextPullRequestGate = nil
		provider.gateMu.Unlock()
		if gate != nil {
			close(gate.reached)
			<-gate.opened
		}
		if provider.missingNextPullRequest.Swap(false) {
			http.Error(response, "synthetic missing pull request", http.StatusNotFound)
			return
		}
		head := headSHA
		if provider.staleNextPullRequest.Swap(false) {
			head = baseSHA
		}
		if _, err := io.WriteString(response, fmt.Sprintf(`{"number":42,"changed_files":1,"base":{"sha":%q,"repo":{"id":1296269}},"head":{"sha":%q}}`, baseSHA, head)); err != nil {
			return
		}
	case request.URL.Path == "/repos/octocat/hello-world/pulls/42/files" &&
		request.URL.Query().Get("page") == "1" && request.URL.Query().Get("per_page") == "100":
		if _, err := io.WriteString(response, `[{"filename":"api/openapi.yaml","status":"modified","additions":1,"deletions":1,"patch":"@@ patch"}]`); err != nil {
			return
		}
	case request.URL.Path == "/repos/octocat/hello-world/contents/api/openapi.yaml":
		provider.serveDocument(response, request)
	default:
		http.Error(response, "unexpected GitHub API request", http.StatusNotFound)
	}
}

func (provider *githubProviderStub) serveDocument(response http.ResponseWriter, request *http.Request) {
	provider.documents.Add(1)
	provider.firstDocumentOnce.Do(func() {
		close(provider.firstDocumentReached)
		<-provider.firstDocumentRelease
	})
	provider.gateMu.Lock()
	gate := provider.nextDocumentGate
	provider.nextDocumentGate = nil
	provider.gateMu.Unlock()
	if gate != nil {
		close(gate.reached)
		<-gate.opened
	}
	if provider.failNextDocument.Swap(false) {
		http.Error(response, "synthetic provider outage", http.StatusServiceUnavailable)
		return
	}
	switch request.URL.Query().Get("ref") {
	case baseSHA:
		if _, err := io.WriteString(response, openAPISpec("string")); err != nil {
			return
		}
	case headSHA:
		if _, err := io.WriteString(response, openAPISpec("integer")); err != nil {
			return
		}
	default:
		http.Error(response, "unknown immutable revision", http.StatusNotFound)
	}
}

func openAPISpec(propertyType string) string {
	return `openapi: 3.1.0
info:
  title: Orders
  version: "1"
paths:
  /orders:
    post:
      operationId: createOrder
      x-argus-capabilities:
        - create-order
      requestBody:
        content:
          application/json:
            schema:
              $ref: "#/components/schemas/CreateOrder"
      responses:
        "201":
          description: Created
components:
  schemas:
    CreateOrder:
      type: object
      properties:
        item:
          type: ` + propertyType + "\n"
}
