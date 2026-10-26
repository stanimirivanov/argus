package perfeng

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/stanimirivanov/argus/internal/performance"
	"github.com/stanimirivanov/argus/internal/performance/requestintent"
)

var _ requestintent.RecoveryProvider = IntegrationHTTP{}

func recoveryHTTPFixture(t *testing.T) (IntegrationHTTP, requestintent.Scope, requestintent.Child) {
	t.Helper()
	source, raw := integrationHTTPFixture(t)
	original, err := DecodeIntegration(raw)
	if err != nil {
		t.Fatal(err)
	}
	c := original.Data.Context
	requested, err := time.Parse(time.RFC3339Nano, c.RequestedAt)
	if err != nil {
		t.Fatal(err)
	}
	deadline, err := time.Parse(time.RFC3339Nano, c.Deadline)
	if err != nil {
		t.Fatal(err)
	}
	digest := sha256.Sum256(raw)
	child := requestintent.Child{Request: performance.Identity(c.Request), Attempt: performance.Identity(c.Attempt), EventID: original.ID, SHA256: hex.EncodeToString(digest[:]), RequestedAt: requested, Deadline: deadline, Payload: raw, Reasons: []requestintent.Reason{{MappingID: "mapping", Capability: "load", Reason: "reviewed"}}}

	return source, requestintent.Scope{Issuer: source.Provider, PrincipalIssuer: source.Principal.Issuer, PrincipalID: source.Principal.ID}, child
}

func TestLookupOutcomeChecksFrozenEvidenceBeforeNetwork(t *testing.T) {
	t.Parallel()
	for _, change := range []func(*requestintent.Scope, *requestintent.Child){
		func(s *requestintent.Scope, _ *requestintent.Child) { s.Issuer += "-other" },
		func(s *requestintent.Scope, _ *requestintent.Child) { s.PrincipalIssuer += "-other" },
		func(s *requestintent.Scope, _ *requestintent.Child) { s.PrincipalID += "-other" },
		func(_ *requestintent.Scope, c *requestintent.Child) { c.Request.ID += "-other" },
		func(_ *requestintent.Scope, c *requestintent.Child) { c.Attempt.ID += "-other" },
		func(_ *requestintent.Scope, c *requestintent.Child) { c.EventID += "-other" },
		func(_ *requestintent.Scope, c *requestintent.Child) {
			c.RequestedAt = c.RequestedAt.Add(time.Microsecond)
		},
		func(_ *requestintent.Scope, c *requestintent.Child) { c.Deadline = c.Deadline.Add(time.Microsecond) },
		func(_ *requestintent.Scope, c *requestintent.Child) { c.SHA256 = strings.Repeat("0", 64) },
	} {
		source, scope, child := recoveryHTTPFixture(t)
		change(&scope, &child)
		source.Client = &http.Client{Transport: integrationRoundTrip(func(*http.Request) (*http.Response, error) {
			t.Fatal("invalid retained evidence contacted provider")
			return nil, ErrIntegration
		})}
		if got, err := source.LookupOutcome(t.Context(), scope, child); !errors.Is(err, ErrIntegration) || got.Fingerprint() != "" {
			t.Fatal("invalid correlation accepted", err)
		}
	}
}

func TestLookupOutcomeNeverSubmitsOrRetries(t *testing.T) {
	t.Parallel()
	for _, status := range []int{http.StatusOK, http.StatusUnprocessableEntity, http.StatusNotFound, http.StatusServiceUnavailable} {
		source, scope, child := recoveryHTTPFixture(t)
		exchange := integrationExchange(t, "completed")
		if status == http.StatusUnprocessableEntity {
			exchange = integrationExchange(t, "rejected")
		}
		calls := 0
		source.Client = &http.Client{Transport: integrationRoundTrip(func(r *http.Request) (*http.Response, error) {
			calls++
			if r.Method != http.MethodGet || r.URL.Path != "/integration/v1/requests/request-1" || r.URL.Query().Get("issuer") != child.Request.Issuer || r.Header.Get("Authorization") != "Bearer secret" || r.Body != nil {
				t.Error("recovery changed identity or submitted")
			}

			return &http.Response{StatusCode: status, Header: http.Header{"Content-Type": {"application/json"}}, Body: io.NopCloser(strings.NewReader(string(exchange[len(exchange)-1])))}, nil
		})}
		got, err := source.LookupOutcome(context.Background(), scope, child)
		if calls != 1 {
			t.Fatal("unexpected retry", calls)
		}
		if status == http.StatusNotFound || status == http.StatusServiceUnavailable {
			var failure *IntegrationHTTPStatusError
			if !errors.As(err, &failure) || failure.Code != status || got.Fingerprint() != "" {
				t.Fatal("unresolved lookup returned evidence", err)
			}
		} else if err != nil || got.Fingerprint() == "" || got.Data().Request != child.Request {
			t.Fatal("lookup lost evidence", err)
		}
	}
}
