// Package httpapi exposes trusted change ingestion through a deliberately
// narrow webhook boundary.
package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"

	"github.com/stanimirivanov/argus/internal/change"
	contractadapter "github.com/stanimirivanov/argus/internal/change/adapters/contract"
	"github.com/stanimirivanov/argus/internal/change/ingest"
)

const maxWebhookBodyBytes = 25 * 1024 * 1024

// DeliveryDecoder authenticates raw provider bytes and returns normalized
// metadata. The handler never parses the body before this boundary.
type DeliveryDecoder interface {
	Decode(event, deliveryID, signature string, body []byte) (ingest.Delivery, error)
}

// IngestService is the application use case consumed by the HTTP adapter.
type IngestService interface {
	Ingest(context.Context, ingest.Delivery) (ingest.Result, error)
}

type handler struct {
	decoder DeliveryDecoder
	service IngestService
}

// NewHandler returns a webhook-only HTTP handler. GitHub delivery signatures
// provide endpoint authentication; no general Argus API is exposed here.
func NewHandler(decoder DeliveryDecoder, service IngestService) http.Handler {
	h := &handler{decoder: decoder, service: service}
	mux := http.NewServeMux()
	mux.Handle("POST /webhooks/github", h)
	return mux
}

func (handler *handler) ServeHTTP(response http.ResponseWriter, request *http.Request) {
	if !strings.HasPrefix(request.Header.Get("Content-Type"), "application/json") {
		writeError(response, http.StatusUnsupportedMediaType, "content type must be application/json")
		return
	}
	body, err := io.ReadAll(http.MaxBytesReader(response, request.Body, maxWebhookBodyBytes))
	if err != nil {
		var tooLarge *http.MaxBytesError
		if errors.As(err, &tooLarge) {
			writeError(response, http.StatusRequestEntityTooLarge, "webhook body exceeds the configured limit")

			return
		}
		writeError(response, http.StatusBadRequest, "webhook body could not be read")

		return
	}

	delivery, err := handler.decoder.Decode(
		request.Header.Get("X-GitHub-Event"),
		request.Header.Get("X-GitHub-Delivery"),
		request.Header.Get("X-Hub-Signature-256"),
		body,
	)
	if err != nil {
		writeMappedError(response, err)
		return
	}
	result, err := handler.service.Ingest(request.Context(), delivery)
	if err != nil {
		writeMappedError(response, err)
		return
	}
	document, err := contractadapter.ExportV1(result.ChangeSet)
	if err != nil {
		writeMappedError(response, change.ErrUnavailable)
		return
	}

	status := http.StatusOK
	if result.Created {
		status = http.StatusCreated
	}
	response.Header().Set("Content-Type", "application/json")
	response.WriteHeader(status)
	if err := json.NewEncoder(response).Encode(document); err != nil {
		return
	}
}

func writeMappedError(response http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, ingest.ErrUnauthenticated):
		writeError(response, http.StatusUnauthorized, "webhook authentication failed")
	case errors.Is(err, ingest.ErrInvalidDelivery), errors.Is(err, change.ErrInvalid):
		writeError(response, http.StatusBadRequest, "webhook delivery is invalid")
	case errors.Is(err, change.ErrConflict), errors.Is(err, change.ErrStale):
		writeError(response, http.StatusConflict, "webhook delivery conflicts with immutable evidence")
	case errors.Is(err, change.ErrNotFound):
		writeError(response, http.StatusNotFound, "pull request was not found")
	default:
		writeError(response, http.StatusBadGateway, "change evidence is temporarily unavailable")
	}
}

func writeError(response http.ResponseWriter, status int, message string) {
	response.Header().Set("Content-Type", "application/json")
	response.WriteHeader(status)
	if err := json.NewEncoder(response).Encode(struct {
		Error string `json:"error"`
	}{Error: message}); err != nil {
		return
	}
}
