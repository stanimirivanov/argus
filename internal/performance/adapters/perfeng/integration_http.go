package perfeng

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"mime"
	"net/http"
	"net/netip"
	"net/url"
	"reflect"
	"regexp"
	"strings"
	"time"
)

// IntegrationHTTP submits frozen requests and reads scoped provider snapshots.
// Configuration must be independently authorized, never derived from wire claims.
// Client is an optional trusted dependency. Calls clone it, disable cookies and
// redirects, and bound each exchange to ten seconds. No call retries or grants
// approval; callers own eligibility, durable claims and uncertain-outcome recovery.
// Callers must not mutate configuration or input bytes during a call.
type IntegrationHTTP struct {
	Endpoint, Token, Provider string
	Principal                 IntegrationIdentity
	Client                    *http.Client
}

// IntegrationHTTPStatusError exposes only status, not remote bodies or credentials.
// A failed POST can still have committed; status alone never authorizes a new ID.
type IntegrationHTTPStatusError struct{ Code int }

func (err *IntegrationHTTPStatusError) Error() string {
	return fmt.Sprintf("performance integration HTTP status %d", err.Code)
}

var integrationBearer = regexp.MustCompile(`^[A-Za-z0-9._~+/-]+=*$`)

// Submit sends the exact validated requested.v1 bytes once. Only correlated
// accepted (202) or rejected (422) evidence is successful transport output.
// Rejection is a disposition, not permission to execute or automatically retry.
func (source IntegrationHTTP) Submit(ctx context.Context, raw []byte) (IntegrationMessage, error) {
	return source.exchange(ctx, http.MethodPost, raw)
}

// Lookup recovers evidence using the original request issuer/ID and principal.
// A 404 is not proof that an earlier uncertain submission cannot still commit.
// Lifecycle output is a snapshot; this adapter does not enforce revision ordering.
func (source IntegrationHTTP) Lookup(ctx context.Context, raw []byte) (IntegrationMessage, error) {
	return source.exchange(ctx, http.MethodGet, raw)
}

func (source IntegrationHTTP) configure() (*url.URL, *http.Client, error) {
	endpoint, err := url.Parse(source.Endpoint)
	if err != nil || endpoint.Host == "" || endpoint.User != nil || endpoint.Opaque != "" ||
		strings.ContainsAny(source.Endpoint, "?#") || endpoint.EscapedPath() != "/integration/v1/requests" {
		return nil, nil, ErrIntegration
	}
	address, addressErr := netip.ParseAddr(endpoint.Hostname())
	if endpoint.Scheme != "https" && !(endpoint.Scheme == "http" && addressErr == nil && address.IsLoopback()) {
		return nil, nil, ErrIntegration
	}
	for _, issuer := range []string{source.Provider, source.Principal.Issuer} {
		parsed, parseErr := url.Parse(issuer)
		if parseErr != nil || !parsed.IsAbs() || parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" || len(issuer) > 512 {
			return nil, nil, ErrIntegration
		}
	}
	if !integrationBearer.MatchString(source.Token) || strings.TrimSpace(source.Principal.ID) == "" {
		return nil, nil, ErrIntegration
	}
	client := http.Client{Timeout: 10 * time.Second}
	if source.Client != nil {
		client = *source.Client
		if client.Timeout <= 0 || client.Timeout > 10*time.Second {
			client.Timeout = 10 * time.Second
		}
	}
	client.Jar = nil
	client.CheckRedirect = func(*http.Request, []*http.Request) error { return errors.New("integration redirect rejected") }

	return endpoint, &client, nil
}

func (source IntegrationHTTP) exchange(ctx context.Context, method string, raw []byte) (IntegrationMessage, error) {
	endpoint, client, err := source.configure()
	if err != nil {
		return IntegrationMessage{}, err
	}
	original, err := DecodeIntegration(raw)
	if err != nil || original.Type != "dev.perfeng.integration.requested.v1" || original.Data.Context.Principal != source.Principal {
		return IntegrationMessage{}, ErrIntegration
	}
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	if err := ctx.Err(); err != nil {
		return IntegrationMessage{}, err
	}
	target := endpoint.String()
	var body io.Reader
	if method == http.MethodPost {
		body = bytes.NewReader(raw)
	} else {
		identity := original.Data.Context.Request
		id := url.PathEscape(identity.ID)
		if id == "." || id == ".." {
			id = strings.ReplaceAll(id, ".", "%2E")
		}
		target += "/" + id + "?" + url.Values{"issuer": {identity.Issuer}}.Encode()
	}
	request, err := http.NewRequestWithContext(ctx, method, target, body)
	if err != nil {
		return IntegrationMessage{}, ErrIntegration
	}
	request.Header.Set("Authorization", "Bearer "+source.Token)
	request.Header.Set("Accept", "application/json")
	if method == http.MethodPost {
		request.Header.Set("Content-Type", "application/json")
	}
	response, err := client.Do(request)
	if err != nil {
		return IntegrationMessage{}, integrationIOError(ctx)
	}
	reply, err := integrationResponse(response, method, original, source.Provider)
	if ctx.Err() != nil {
		return IntegrationMessage{}, ctx.Err()
	}

	return reply, err
}

func integrationIOError(ctx context.Context) error {
	if ctx.Err() != nil {
		return ctx.Err()
	}

	return errors.New("performance integration exchange failed or timed out")
}

func integrationResponse(response *http.Response, method string, original IntegrationMessage, provider string) (IntegrationMessage, error) {
	status := response.StatusCode
	expected := "dev.perfeng.integration.rejected.v1"
	if method == http.MethodPost && status == http.StatusAccepted {
		expected = "dev.perfeng.integration.accepted.v1"
	} else if method == http.MethodGet && status == http.StatusOK {
		expected = "dev.perfeng.integration.lifecycle.v1"
	} else if status != http.StatusUnprocessableEntity {
		_ = response.Body.Close() //nolint:errcheck // Status takes precedence; no remote diagnostic is exposed.
		return IntegrationMessage{}, &IntegrationHTTPStatusError{Code: status}
	}
	raw, readErr := io.ReadAll(io.LimitReader(response.Body, IntegrationMaximumBytes+1))
	closeErr := response.Body.Close()
	media, _, mediaErr := mime.ParseMediaType(response.Header.Get("Content-Type"))
	if readErr != nil || closeErr != nil || mediaErr != nil || media != "application/json" || len(raw) > IntegrationMaximumBytes {
		return IntegrationMessage{}, ErrIntegration
	}
	reply, err := DecodeIntegration(raw)
	if err != nil || reply.Type != expected || reply.Source != provider || !reflect.DeepEqual(reply.Data.Context, original.Data.Context) ||
		(reply.Data.Run != nil && reply.Data.Run.Issuer != provider) {
		return IntegrationMessage{}, ErrIntegration
	}

	return reply, nil
}
