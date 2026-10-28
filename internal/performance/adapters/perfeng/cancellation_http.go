package perfeng

import (
	"bytes"
	"context"
	"io"
	"mime"
	"net/http"
	"net/url"
	"reflect"
	"strconv"
	"strings"
	"time"
)

// Cancel sends the exact caller-approved cancel-requested.v1 envelope once, bound
// to the original frozen request and independently configured principal/provider.
// Callers own authorization, durable intent, first-send cutoff, ordering and recovery;
// this transport neither mutates storage nor activates cancellation or reruns.
// Late cancellation is valid. A failed exchange may have committed; reconcile the
// original identities. A 202 confirms intent only, while 200 may preserve a racing
// completion/failure. Configuration and input bytes must remain immutable during calls.
func (source IntegrationHTTP) Cancel(ctx context.Context, frozen, cancellation []byte) (IntegrationMessage, error) {
	endpoint, client, err := source.configure()
	if err != nil {
		return IntegrationMessage{}, err
	}
	original, err := DecodeIntegration(frozen)
	if err != nil || original.Type != "dev.perfeng.integration.requested.v1" || original.Data.Context.Principal != source.Principal {
		return IntegrationMessage{}, ErrIntegration
	}
	intent, err := DecodeIntegration(cancellation)
	if err != nil || intent.Type != "dev.perfeng.integration.cancel-requested.v1" || intent.ID == original.ID || !reflect.DeepEqual(intent.Data.Context, original.Data.Context) {
		return IntegrationMessage{}, ErrIntegration
	}
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	if err := ctx.Err(); err != nil {
		return IntegrationMessage{}, err
	}
	id := url.PathEscape(original.Data.Context.Request.ID)
	if id == "." || id == ".." {
		id = strings.ReplaceAll(id, ".", "%2E")
	}
	target := endpoint.String() + "/" + id + "/cancellation?" + url.Values{"issuer": {original.Data.Context.Request.Issuer}}.Encode()
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, target, bytes.NewReader(cancellation))
	if err != nil {
		return IntegrationMessage{}, ErrIntegration
	}
	request.Header.Set("Authorization", "Bearer "+source.Token)
	request.Header.Set("Accept", "application/json")
	request.Header.Set("Content-Type", "application/json")
	response, err := client.Do(request)
	if err != nil {
		return IntegrationMessage{}, integrationIOError(ctx)
	}
	reply, err := cancellationResponse(response, original, source.Provider)
	if ctx.Err() != nil {
		return IntegrationMessage{}, ctx.Err()
	}

	return reply, err
}

func cancellationResponse(response *http.Response, original IntegrationMessage, provider string) (IntegrationMessage, error) {
	if response.StatusCode != http.StatusOK && response.StatusCode != http.StatusAccepted {
		_ = response.Body.Close() //nolint:errcheck // Status takes precedence; remote diagnostics remain private.
		return IntegrationMessage{}, &IntegrationHTTPStatusError{Code: response.StatusCode, RetryAfter: cancellationRetryAfter(response)}
	}
	raw, readErr := io.ReadAll(io.LimitReader(response.Body, IntegrationMaximumBytes+1))
	closeErr := response.Body.Close()
	media, _, mediaErr := mime.ParseMediaType(response.Header.Get("Content-Type"))
	if readErr != nil || closeErr != nil || mediaErr != nil || media != "application/json" || len(raw) > IntegrationMaximumBytes {
		return IntegrationMessage{}, ErrIntegration
	}
	reply, err := DecodeIntegration(raw)
	if err != nil || reply.Type != "dev.perfeng.integration.lifecycle.v1" || reply.Source != provider || reply.Data.Run == nil || reply.Data.Run.Issuer != provider || !reflect.DeepEqual(reply.Data.Context, original.Data.Context) {
		return IntegrationMessage{}, ErrIntegration
	}
	terminal := reply.Data.State == "COMPLETED" || reply.Data.State == "FAILED" || reply.Data.State == "CANCELLED"
	if (response.StatusCode == http.StatusOK && !terminal) || (response.StatusCode == http.StatusAccepted && reply.Data.State != "CANCELLING") {
		return IntegrationMessage{}, ErrIntegration
	}

	return reply, nil
}

func cancellationRetryAfter(response *http.Response) time.Duration {
	seconds, err := strconv.ParseUint(response.Header.Get("Retry-After"), 10, 63)
	if (response.StatusCode != http.StatusTooManyRequests && response.StatusCode != http.StatusServiceUnavailable) || err != nil || seconds == 0 || seconds > uint64((1<<63-1)/time.Second) {
		return 0
	}

	return time.Duration(seconds) * time.Second
}
