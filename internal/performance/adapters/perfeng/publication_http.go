package perfeng

import (
	"context"
	"errors"
	"io"
	"mime"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/stanimirivanov/argus/internal/performance"
	"github.com/stanimirivanov/argus/internal/performance/requestintent"
)

// LookupPublication performs one authenticated scoped analysis GET, preserving
// original publication bytes in an admission binding. Frozen request/settlement
// provenance must come from authoritative scoped storage. Configuration is trusted
// host input; calls clone the client, disable redirects/cookies and bound each
// exchange to ten seconds and 64 KiB. No retry, write or artifact URI access occurs.
// A 404 is scoped absence, never a terminal outcome or permission to resubmit.
func (source IntegrationHTTP) LookupPublication(ctx context.Context, frozen []byte, retained requestintent.Outcome) (PublicationAdmission, error) {
	if err := ctx.Err(); err != nil {
		return PublicationAdmission{}, err
	}
	endpoint, client, err := source.configure()
	if err != nil {
		return PublicationAdmission{}, ErrPublication
	}
	original, err := DecodeIntegration(frozen)
	if err != nil || original.Type != "dev.perfeng.integration.requested.v1" || original.Data.Context.Principal != source.Principal {
		return PublicationAdmission{}, ErrPublication
	}
	if _, err := BindLifecycleAdmission(frozen, retained, source.Provider); err != nil {
		return PublicationAdmission{}, ErrPublication
	}
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	identity := original.Data.Context.Request
	id := url.PathEscape(identity.ID)
	if id == "." || id == ".." {
		id = strings.ReplaceAll(id, ".", "%2E")
	}
	target := endpoint.String() + "/" + id + "/analysis?" + url.Values{"issuer": {identity.Issuer}}.Encode()
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, target, nil)
	if err != nil {
		return PublicationAdmission{}, ErrPublication
	}
	request.Header.Set("Authorization", "Bearer "+source.Token)
	request.Header.Set("Accept", "application/json")
	response, err := client.Do(request)
	if err != nil {
		if ctx.Err() != nil {
			return PublicationAdmission{}, ctx.Err()
		}
		return PublicationAdmission{}, ErrPublication
	}
	raw, err := publicationResponse(response)
	if ctx.Err() != nil {
		return PublicationAdmission{}, ctx.Err()
	}
	if err != nil {
		return PublicationAdmission{}, err
	}

	return BindPublicationAdmission(frozen, retained, raw, source.Provider, performance.Identity(source.Principal))
}

func publicationResponse(response *http.Response) ([]byte, error) {
	if response.StatusCode != http.StatusOK {
		status := &IntegrationHTTPStatusError{Code: response.StatusCode, RetryAfter: cancellationRetryAfter(response)}
		if err := response.Body.Close(); err != nil {
			return nil, errors.Join(status, ErrPublication)
		}

		return nil, status
	}
	raw, readErr := io.ReadAll(io.LimitReader(response.Body, PublicationMaximumBytes+1))
	closeErr := response.Body.Close()
	media, _, mediaErr := mime.ParseMediaType(response.Header.Get("Content-Type"))
	if readErr != nil || closeErr != nil || mediaErr != nil || media != "application/json" || len(raw) > PublicationMaximumBytes {
		return nil, ErrPublication
	}

	return raw, nil
}
