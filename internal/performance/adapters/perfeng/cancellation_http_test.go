package perfeng

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestCancellationHTTPPreservesAuthenticatedIdentityAndExactLateIntent(t *testing.T) {
	for _, id := range []string{"opaque/with?query#fragment", ".", ".."} {
		source, _ := integrationHTTPFixture(t)
		messages := integrationExchange(t, "cancelled")
		original, intent, reply := lifecycleReply(t, messages[0]), lifecycleReply(t, messages[2]), lifecycleReply(t, messages[3])
		original.Data.Context.Request.ID = id
		intent.Data.Context, reply.Data.Context = original.Data.Context, original.Data.Context
		intent.Time = intent.Data.Context.Deadline // Expiry does not prohibit cancellation.
		frozen, cancellation := cancellationBytes(t, original), cancellationBytes(t, intent)
		response := cancellationBytes(t, reply)
		var calls atomic.Int32
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			calls.Add(1)
			body, err := io.ReadAll(r.Body)
			if err != nil || !bytes.Equal(body, cancellation) || r.Method != http.MethodPost || r.Header.Get("Authorization") != "Bearer secret" || r.Header.Get("Content-Type") != "application/json" || r.Header.Get("Accept") != "application/json" {
				t.Error("changed cancellation bytes or authentication")
			}
			pathID := strings.TrimSuffix(strings.TrimPrefix(r.URL.Path, "/integration/v1/requests/"), "/cancellation")
			if pathID != id || r.URL.Query().Get("issuer") != original.Source {
				t.Error("changed opaque request identity")
			}
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusAccepted)
			if _, err := w.Write(response); err != nil {
				t.Error(err)
			}
		}))
		defer server.Close()
		source.Endpoint = server.URL + "/integration/v1/requests"
		for range 2 {
			got, err := source.Cancel(t.Context(), frozen, cancellation)
			if err != nil || got.Data.State != "CANCELLING" {
				t.Fatal("late cancellation/replay failed", err)
			}
		}
		if calls.Load() != 2 {
			t.Fatal("transport retried or skipped a call")
		}
	}
}

func cancellationBytes(t *testing.T, message IntegrationMessage) []byte {
	t.Helper()
	raw, err := EncodeIntegration(message)
	if err != nil {
		t.Fatal(err)
	}

	return raw
}

func TestCancellationHTTPRejectsInvalidIntentBeforeIO(t *testing.T) {
	for _, name := range []string{"context", "kind", "event", "principal", "endpoint", "token"} {
		source, _ := integrationHTTPFixture(t)
		messages := integrationExchange(t, "cancelled")
		intent := lifecycleReply(t, messages[2])
		switch name {
		case "context":
			intent.Data.Context.Workload.ID = "another-workload"
		case "kind":
			intent = lifecycleReply(t, messages[0])
		case "event":
			intent.ID = lifecycleReply(t, messages[0]).ID
		case "principal":
			source.Principal.ID = "another-principal"
		case "endpoint":
			source.Endpoint = "http://untrusted.example/integration/v1/requests"
		case "token":
			source.Token = ""
		}
		source.Client = &http.Client{Transport: integrationRoundTrip(func(*http.Request) (*http.Response, error) {
			t.Fatal("invalid cancellation contacted provider")
			return nil, ErrIntegration
		})}
		if got, err := source.Cancel(t.Context(), messages[0], cancellationBytes(t, intent)); err == nil || !reflect.DeepEqual(got, IntegrationMessage{}) {
			t.Fatal("invalid cancellation accepted", name, err)
		}
	}
}

func TestCancellationHTTPEnforcesStatusStateAndProviderCorrelation(t *testing.T) {
	messages := integrationExchange(t, "cancelled")
	for _, test := range []struct {
		status        int
		state, change string
		valid         bool
	}{
		{202, "CANCELLING", "", true}, {200, "CANCELLED", "", true}, {200, "COMPLETED", "", true}, {200, "FAILED", "", true},
		{202, "CANCELLED", "", false}, {200, "CANCELLING", "", false}, {200, "RUNNING", "", false},
		{202, "CANCELLING", "provider", false}, {202, "CANCELLING", "run", false}, {202, "CANCELLING", "context", false},
		{202, "CANCELLING", "kind", false}, {202, "CANCELLING", "media", false}, {202, "CANCELLING", "oversize", false},
	} {
		source, _ := integrationHTTPFixture(t)
		reply := lifecycleReply(t, messages[3])
		reply.Data.State = test.state
		switch test.change {
		case "provider":
			reply.Source = "https://other.example"
		case "run":
			reply.Data.Run.Issuer = "https://other.example"
		case "context":
			reply.Data.Context.Attempt.ID = "other-attempt"
		case "kind":
			reply = lifecycleReply(t, messages[1])
		}
		body, media := string(cancellationBytes(t, reply)), "application/json"
		if test.change == "media" {
			media = "text/plain"
		}
		if test.change == "oversize" {
			body = strings.Repeat(" ", IntegrationMaximumBytes+1)
		}
		source.Client = &http.Client{Transport: integrationRoundTrip(func(*http.Request) (*http.Response, error) {
			return &http.Response{StatusCode: test.status, Header: http.Header{"Content-Type": {media}}, Body: io.NopCloser(strings.NewReader(body))}, nil
		})}
		got, err := source.Cancel(t.Context(), messages[0], messages[2])
		if test.valid && (err != nil || got.Data.State != test.state) {
			t.Fatal("valid race outcome rejected", test, err)
		}
		if !test.valid && (err == nil || !reflect.DeepEqual(got, IntegrationMessage{})) {
			t.Fatal("uncorrelated snapshot accepted", test, err)
		}
	}
}

func TestCancellationHTTPStatusErrorsPreserveSafeBackoffAndCloseBodies(t *testing.T) {
	messages := integrationExchange(t, "cancelled")
	for _, status := range []int{401, 403, 404, 409, 410, 422, 429, 503} {
		for _, hint := range []string{"30", "0", "-1", "invalid-secret", "9223372036854775807"} {
			source, _ := integrationHTTPFixture(t)
			body := &cancellationBody{Reader: strings.NewReader("secret response body")}
			source.Client = &http.Client{Transport: integrationRoundTrip(func(*http.Request) (*http.Response, error) {
				return &http.Response{StatusCode: status, Header: http.Header{"Retry-After": {hint}}, Body: body}, nil
			})}
			_, err := source.Cancel(t.Context(), messages[0], messages[2])
			var got *IntegrationHTTPStatusError
			if !errors.As(err, &got) || got.Code != status || !body.closed || strings.Contains(err.Error(), "secret") {
				t.Fatal("lost status or leaked diagnostic", err)
			}
			want := time.Duration(0)
			if (status == 429 || status == 503) && hint == "30" {
				want = 30 * time.Second
			}
			if got.RetryAfter != want {
				t.Fatal("unsafe backoff", status, hint, got.RetryAfter)
			}
		}
	}
}

type cancellationBody struct {
	io.Reader
	closed bool
}

func (body *cancellationBody) Close() error { body.closed = true; return nil }

func TestCancellationHTTPRejectsRedirectsAndCancelsActiveIO(t *testing.T) {
	source, _ := integrationHTTPFixture(t)
	messages := integrationExchange(t, "cancelled")
	var leaked atomic.Int32
	target := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { leaked.Add(1) }))
	defer target.Close()
	redirect := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, target.URL, http.StatusTemporaryRedirect)
	}))
	defer redirect.Close()
	source.Endpoint = redirect.URL + "/integration/v1/requests"
	if _, err := source.Cancel(t.Context(), messages[0], messages[2]); err == nil || leaked.Load() != 0 {
		t.Fatal("cancellation redirect followed")
	}
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	source.Client = &http.Client{Timeout: time.Hour, Transport: integrationRoundTrip(func(r *http.Request) (*http.Response, error) {
		deadline, ok := r.Context().Deadline()
		if !ok || time.Until(deadline) > 10*time.Second {
			t.Error("unbounded cancellation call")
		}
		cancel()
		<-r.Context().Done()

		return nil, errors.New("secret transport failure")
	})}
	if _, err := source.Cancel(ctx, messages[0], messages[2]); !errors.Is(err, context.Canceled) {
		t.Fatal("caller cancellation lost", err)
	}
}
