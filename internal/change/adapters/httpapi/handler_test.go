package httpapi_test

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stanimirivanov/argus/internal/change"
	githubadapter "github.com/stanimirivanov/argus/internal/change/adapters/github"
	"github.com/stanimirivanov/argus/internal/change/adapters/httpapi"
	"github.com/stanimirivanov/argus/internal/change/ingest"
)

func TestHandlerAuthenticatesAndReturnsCreatedChangeSet(t *testing.T) {
	t.Parallel()

	secret := []byte("webhook secret")
	body := webhookBody()
	decoder, err := githubadapter.NewWebhookDecoder(secret, "github.com")
	if err != nil {
		t.Fatalf("create decoder: %v", err)
	}
	service := &recordingService{created: true}
	handler := httpapi.NewHandler(decoder, service)
	request := signedRequest(secret, body)
	response := httptest.NewRecorder()

	handler.ServeHTTP(response, request)

	if response.Code != http.StatusCreated || service.calls != 1 {
		t.Fatalf("status=%d calls=%d body=%s", response.Code, service.calls, response.Body.String())
	}
	if !bytes.Contains(response.Body.Bytes(), []byte(`"apiVersion":"argus.dev/change-set/v1"`)) {
		t.Fatalf("response is not a change-set v1 document: %s", response.Body.String())
	}
}

func TestHandlerRejectsInvalidSignatureWithoutCallingService(t *testing.T) {
	t.Parallel()

	decoder, err := githubadapter.NewWebhookDecoder([]byte("webhook secret"), "github.com")
	if err != nil {
		t.Fatalf("create decoder: %v", err)
	}
	service := &recordingService{}
	handler := httpapi.NewHandler(decoder, service)
	request := signedRequest([]byte("wrong secret"), webhookBody())
	response := httptest.NewRecorder()

	handler.ServeHTTP(response, request)

	if response.Code != http.StatusUnauthorized || service.calls != 0 {
		t.Fatalf("status=%d calls=%d", response.Code, service.calls)
	}
}

func TestHandlerReturnsOKForStoredDelivery(t *testing.T) {
	t.Parallel()

	secret := []byte("webhook secret")
	body := webhookBody()
	decoder, err := githubadapter.NewWebhookDecoder(secret, "github.com")
	if err != nil {
		t.Fatalf("create decoder: %v", err)
	}
	service := &recordingService{created: false}
	request := signedRequest(secret, body)
	response := httptest.NewRecorder()

	httpapi.NewHandler(decoder, service).ServeHTTP(response, request)

	if response.Code != http.StatusOK || service.calls != 1 {
		t.Fatalf("status=%d calls=%d body=%s", response.Code, service.calls, response.Body.String())
	}
}

type recordingService struct {
	calls   int
	created bool
}

func (service *recordingService) Ingest(
	_ context.Context,
	delivery ingest.Delivery,
) (ingest.Result, error) {
	service.calls++

	return ingest.Result{Created: service.created, ChangeSet: change.Set{
		APIVersion:        change.SetAPIVersion,
		SourceRepository:  delivery.Repository,
		PullRequestNumber: delivery.PullRequestNumber,
		BaseRevision:      delivery.BaseRevision,
		HeadRevision:      delivery.HeadRevision,
		ObservedAt:        delivery.ObservedAt,
		Trigger: change.Trigger{
			Provider:   delivery.Provider,
			DeliveryID: delivery.ID,
			Event:      delivery.Event,
			Action:     delivery.Action,
		},
	}}, nil
}

func signedRequest(secret, body []byte) *http.Request {
	request := httptest.NewRequest(http.MethodPost, "/webhooks/github", bytes.NewReader(body))
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("X-GitHub-Event", "pull_request")
	request.Header.Set("X-GitHub-Delivery", "delivery-42")
	mac := hmac.New(sha256.New, secret)
	_, _ = mac.Write(body)
	request.Header.Set("X-Hub-Signature-256", "sha256="+hex.EncodeToString(mac.Sum(nil)))

	return request
}

func webhookBody() []byte {
	return []byte(`{
  "action":"synchronize",
  "number":42,
  "repository":{"id":1296269,"full_name":"octocat/hello-world"},
  "pull_request":{
    "updated_at":"2026-09-18T09:30:00Z",
    "base":{"sha":"0123456789abcdef0123456789abcdef01234567"},
    "head":{"sha":"89abcdef0123456789abcdef0123456789abcdef"}
  }
}`)
}
