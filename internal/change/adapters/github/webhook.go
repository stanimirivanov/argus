// Package github authenticates GitHub change events and resolves their
// immutable pull-request evidence into provider-neutral change sets.
package github

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/stanimirivanov/argus/internal/catalog"
	"github.com/stanimirivanov/argus/internal/change/ingest"
)

const signaturePrefix = "sha256="

// WebhookDecoder verifies raw request bytes before interpreting any payload
// fields. Host identifies the GitHub or GitHub Enterprise installation.
type WebhookDecoder struct {
	secret []byte
	host   string
}

// NewWebhookDecoder copies the webhook secret so callers may clear their
// configuration buffer after constructing the adapter.
func NewWebhookDecoder(secret []byte, host string) (*WebhookDecoder, error) {
	if len(secret) == 0 {
		return nil, fmt.Errorf("configure GitHub webhook: secret is required")
	}
	host = strings.ToLower(strings.TrimSpace(host))
	if host == "" {
		return nil, fmt.Errorf("configure GitHub webhook: host is required")
	}

	return &WebhookDecoder{secret: append([]byte(nil), secret...), host: host}, nil
}

// Decode authenticates and normalizes a supported pull_request delivery.
func (decoder *WebhookDecoder) Decode(event, deliveryID, signature string, body []byte) (ingest.Delivery, error) {
	if decoder == nil || !validSignature(decoder.secret, signature, body) {
		return ingest.Delivery{}, ingest.ErrUnauthenticated
	}
	if event != "pull_request" || strings.TrimSpace(deliveryID) == "" {
		return ingest.Delivery{}, fmt.Errorf("%w: unsupported GitHub event", ingest.ErrInvalidDelivery)
	}

	var payload webhookPayload
	if err := json.Unmarshal(body, &payload); err != nil {
		return ingest.Delivery{}, fmt.Errorf("%w: decode GitHub payload", ingest.ErrInvalidDelivery)
	}
	owner, name, err := repositoryCoordinates(payload.Repository.FullName)
	if err != nil {
		return ingest.Delivery{}, fmt.Errorf("%w: %w", ingest.ErrInvalidDelivery, err)
	}
	base, err := catalog.NewRevision(catalog.RevisionGitSHA1, payload.PullRequest.Base.SHA)
	if err != nil {
		return ingest.Delivery{}, fmt.Errorf("%w: base revision", ingest.ErrInvalidDelivery)
	}
	head, err := catalog.NewRevision(catalog.RevisionGitSHA1, payload.PullRequest.Head.SHA)
	if err != nil {
		return ingest.Delivery{}, fmt.Errorf("%w: head revision", ingest.ErrInvalidDelivery)
	}
	observedAt, err := time.Parse(time.RFC3339, payload.PullRequest.UpdatedAt)
	if err != nil {
		return ingest.Delivery{}, fmt.Errorf("%w: pull request update time", ingest.ErrInvalidDelivery)
	}

	payloadDigest := sha256.Sum256(body)
	delivery := ingest.Delivery{
		Provider:      catalog.ProviderGitHub,
		ID:            deliveryID,
		Event:         event,
		Action:        payload.Action,
		PayloadSHA256: hex.EncodeToString(payloadDigest[:]),
		Repository: catalog.Repository{
			Identity: catalog.RepositoryIdentity{
				Provider:             catalog.ProviderGitHub,
				Host:                 decoder.host,
				ProviderRepositoryID: strconv.FormatInt(payload.Repository.ID, 10),
			},
			Owner: owner,
			Name:  name,
		},
		PullRequestNumber: payload.Number,
		BaseRevision:      base,
		HeadRevision:      head,
		ObservedAt:        observedAt.UTC(),
	}
	if err := ingest.ValidateDelivery(delivery); err != nil {
		return ingest.Delivery{}, err
	}

	return delivery, nil
}

func validSignature(secret []byte, signature string, body []byte) bool {
	if len(secret) == 0 || !strings.HasPrefix(signature, signaturePrefix) {
		return false
	}
	provided, err := hex.DecodeString(strings.TrimPrefix(signature, signaturePrefix))
	if err != nil || len(provided) != sha256.Size {
		return false
	}
	mac := hmac.New(sha256.New, secret)
	_, _ = mac.Write(body)

	return hmac.Equal(provided, mac.Sum(nil))
}

func repositoryCoordinates(fullName string) (string, string, error) {
	parts := strings.Split(fullName, "/")
	if len(parts) != 2 || strings.TrimSpace(parts[0]) == "" || strings.TrimSpace(parts[1]) == "" {
		return "", "", fmt.Errorf("invalid repository coordinates")
	}

	return parts[0], parts[1], nil
}

type webhookPayload struct {
	Action     string `json:"action"`
	Number     int    `json:"number"`
	Repository struct {
		ID       int64  `json:"id"`
		FullName string `json:"full_name"`
	} `json:"repository"`
	PullRequest struct {
		UpdatedAt string `json:"updated_at"`
		Base      struct {
			SHA string `json:"sha"`
		} `json:"base"`
		Head struct {
			SHA string `json:"sha"`
		} `json:"head"`
	} `json:"pull_request"`
}
